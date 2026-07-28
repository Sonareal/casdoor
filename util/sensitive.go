// Copyright 2026 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Sensitive word filtering for user-supplied text (display name, bio, ...).
//
// Matching is done on a normalized form of the text so that the common evasion
// tricks do not work: full-width characters, inserted spaces/punctuation,
// zero-width characters, emoji, letter case and leetspeak digits are all
// folded away before matching. Both the input and the word list go through the
// exact same normalization, so the word list is written in plain form.
//
// Two things normalization deliberately does NOT do, because they need data
// tables we do not want to vendor:
//   - traditional -> simplified Chinese
//   - Chinese -> pinyin
//
// Add those variants to the word list as their own entries instead.

// confusables are folded before non-alphanumeric runes are dropped, so entries
// such as '@' still take effect. Kept deliberately small: every entry here is a
// potential false positive on legitimate names.
var confusables = map[rune]rune{
	'0': 'o',
	'1': 'i',
	'3': 'e',
	'4': 'a',
	'5': 's',
	'7': 't',
	'@': 'a',
	'$': 's',
}

// NormalizeForMatch folds text into the form used for sensitive word matching.
// It is applied to both the word list and the text being checked.
func NormalizeForMatch(s string) string {
	// NFKC folds full-width forms (ｈｅｒｏｉｎ), circled/superscript digits and
	// most compatibility variants into their canonical counterparts.
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if c, ok := confusables[r]; ok {
			r = c
		}
		// Everything that is not a letter or a digit is dropped. This covers
		// spaces, punctuation, symbols, emoji and — importantly — zero-width
		// characters (Cf), which are invisible and a very common evasion.
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type acNode struct {
	children map[rune]*acNode
	fail     *acNode
	word     string // non-empty on a terminal node: the original word list entry
}

func newACNode() *acNode {
	return &acNode{children: map[rune]*acNode{}}
}

// SensitiveMatcher is an Aho-Corasick automaton: one O(n) pass over the text
// regardless of how many words are in the list.
type SensitiveMatcher struct {
	root  *acNode
	count int
}

// NewSensitiveMatcher builds an automaton from raw word list entries. Entries
// are normalized here, so callers pass them in plain form. Entries that
// normalize to an empty string are skipped.
func NewSensitiveMatcher(words []string) *SensitiveMatcher {
	m := &SensitiveMatcher{root: newACNode()}

	for _, raw := range words {
		normalized := NormalizeForMatch(raw)
		if normalized == "" {
			continue
		}

		node := m.root
		for _, r := range normalized {
			next, ok := node.children[r]
			if !ok {
				next = newACNode()
				node.children[r] = next
			}
			node = next
		}
		if node.word == "" {
			node.word = strings.TrimSpace(raw)
			m.count++
		}
	}

	m.buildFailLinks()
	return m
}

func (m *SensitiveMatcher) buildFailLinks() {
	m.root.fail = m.root
	queue := make([]*acNode, 0, len(m.root.children))
	for _, child := range m.root.children {
		child.fail = m.root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for r, child := range node.children {
			fail := node.fail
			for fail != m.root {
				if _, ok := fail.children[r]; ok {
					break
				}
				fail = fail.fail
			}

			child.fail = m.root
			if next, ok := fail.children[r]; ok && next != child {
				child.fail = next
			}

			queue = append(queue, child)
		}
	}
}

// Size reports how many distinct words the automaton holds.
func (m *SensitiveMatcher) Size() int {
	if m == nil {
		return 0
	}
	return m.count
}

// Match returns the first word list entry found in text, if any. The text is
// normalized internally.
func (m *SensitiveMatcher) Match(text string) (string, bool) {
	hits := m.match(text, true)
	if len(hits) == 0 {
		return "", false
	}
	return hits[0], true
}

// MatchAll returns every distinct word list entry found in text.
func (m *SensitiveMatcher) MatchAll(text string) []string {
	return m.match(text, false)
}

func (m *SensitiveMatcher) match(text string, stopAtFirst bool) []string {
	if m == nil || m.root == nil {
		return nil
	}

	normalized := NormalizeForMatch(text)
	if normalized == "" {
		return nil
	}

	var (
		hits []string
		seen = map[string]bool{}
	)

	node := m.root
	for _, r := range normalized {
		for {
			if next, ok := node.children[r]; ok {
				node = next
				break
			}
			if node == m.root {
				break
			}
			node = node.fail
		}

		// Walk the fail chain so that words which are suffixes of the current
		// path are reported too (e.g. both "abc" and "bc" in the list).
		for t := node; t != m.root; t = t.fail {
			if t.word == "" || seen[t.word] {
				continue
			}
			seen[t.word] = true
			hits = append(hits, t.word)
			if stopAtFirst {
				return hits
			}
		}
	}

	return hits
}

// ---------------------------------------------------------------------------
// Process-wide word list, loaded from a file and hot-reloaded on change.
// ---------------------------------------------------------------------------

const sensitiveStatInterval = 10 * time.Second

var (
	sensitiveMutex    sync.RWMutex
	sensitiveMatcher  *SensitiveMatcher
	sensitivePath     string
	sensitiveModTime  time.Time
	sensitiveNextStat time.Time
)

// InitSensitiveFilter loads the word list at path. A missing file is not an
// error: filtering is simply disabled, which keeps the server bootable on a
// fresh deployment that has not shipped a word list yet.
func InitSensitiveFilter(path string) error {
	sensitiveMutex.Lock()
	defer sensitiveMutex.Unlock()

	sensitivePath = path
	return loadSensitiveWordsLocked()
}

// loadSensitiveWordsLocked must be called with sensitiveMutex held for writing.
func loadSensitiveWordsLocked() error {
	sensitiveNextStat = time.Now().Add(sensitiveStatInterval)

	if sensitivePath == "" {
		sensitiveMatcher = nil
		return nil
	}

	info, err := os.Stat(sensitivePath)
	if err != nil {
		if os.IsNotExist(err) {
			sensitiveMatcher = nil
			return nil
		}
		return err
	}

	file, err := os.Open(sensitivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	words := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// "#" starts a comment; "[Category]" lines label a section.
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		words = append(words, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	sensitiveMatcher = NewSensitiveMatcher(words)
	sensitiveModTime = info.ModTime()
	return nil
}

// reloadSensitiveWordsIfChanged re-reads the word list when the file's mtime
// has moved. The stat itself is throttled so a hot path does not syscall on
// every single call.
func reloadSensitiveWordsIfChanged() {
	sensitiveMutex.RLock()
	path := sensitivePath
	due := time.Now().After(sensitiveNextStat)
	sensitiveMutex.RUnlock()

	if path == "" || !due {
		return
	}

	info, err := os.Stat(path)

	sensitiveMutex.Lock()
	defer sensitiveMutex.Unlock()

	sensitiveNextStat = time.Now().Add(sensitiveStatInterval)
	if err != nil || info.ModTime().Equal(sensitiveModTime) {
		return
	}
	// A reload failure keeps the previously loaded list in place on purpose:
	// a truncated or unreadable file must not silently disable filtering.
	_ = loadSensitiveWordsLocked()
}

// ContainsSensitiveWord reports the first banned word found in text. It returns
// ("", false) when the text is clean or when no word list is configured.
func ContainsSensitiveWord(text string) (string, bool) {
	if text == "" {
		return "", false
	}

	reloadSensitiveWordsIfChanged()

	sensitiveMutex.RLock()
	matcher := sensitiveMatcher
	sensitiveMutex.RUnlock()

	if matcher == nil {
		return "", false
	}
	return matcher.Match(text)
}

// MatchAllSensitiveWords reports every banned word found in text. Used by the
// offline scan, where an operator judging a false positive wants to see all of
// the hits rather than just the first one.
func MatchAllSensitiveWords(text string) []string {
	if text == "" {
		return nil
	}

	reloadSensitiveWordsIfChanged()

	sensitiveMutex.RLock()
	matcher := sensitiveMatcher
	sensitiveMutex.RUnlock()

	if matcher == nil {
		return nil
	}
	return matcher.MatchAll(text)
}

// SensitiveWordCount reports the size of the loaded word list, for diagnostics.
func SensitiveWordCount() int {
	sensitiveMutex.RLock()
	defer sensitiveMutex.RUnlock()
	return sensitiveMatcher.Size()
}
