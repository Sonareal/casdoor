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
// Traditional Chinese is folded to simplified (see traditionalToSimplified), so
// one simplified entry covers both spellings. Pinyin is NOT derived — add pinyin
// spellings to the word list as their own entries when they matter.

// traditionalToSimplified folds traditional Chinese characters to their
// simplified form during normalization, so that a single simplified word list
// entry also covers the traditional spelling. Without it, every entry would
// need a hand-written traditional twin — which does not scale: the list uses
// several hundred distinct characters.
//
// The table only needs to cover characters that appear in the word list. A
// character missing from it simply does not fold, which can cause a miss but
// never a false match — so growing the table is always safe, and omissions are
// the failure mode we can live with.
var traditionalToSimplified = map[rune]rune{
	'亂': '乱', '來': '来', '倉': '仓', '個': '个', '們': '们', '倫': '伦', '傳': '传', '兒': '儿',
	'動': '动', '務': '务', '員': '员', '單': '单', '嗎': '吗', '嘜': '麦', '國': '国', '圍': '围',
	'園': '园', '圖': '图', '團': '团', '報': '报', '場': '场', '姦': '奸', '婦': '妇', '媽': '妈',
	'學': '学', '實': '实', '寶': '宝', '屍': '尸', '廢': '废', '強': '强', '彈': '弹', '復': '复',
	'愛': '爱', '應': '应', '戀': '恋', '戶': '户', '掃': '扫', '搶': '抢', '擊': '击', '斬': '斩',
	'書': '书', '會': '会', '東': '东', '業': '业', '槍': '枪', '樂': '乐', '樓': '楼', '樣': '样',
	'機': '机', '殘': '残', '殺': '杀', '殼': '壳', '滅': '灭', '滾': '滚', '為': '为', '獎': '奖',
	'產': '产', '異': '异', '發': '发', '盤': '盘', '砲': '炮', '碼': '码', '種': '种', '穢': '秽',
	'筆': '笔', '約': '约', '級': '级', '組': '组', '結': '结', '絡': '络', '統': '统', '綁': '绑',
	'網': '网', '綵': '彩', '線': '线', '織': '织', '繫': '系', '罌': '罂', '習': '习', '聯': '联',
	'職': '职', '脫': '脱', '莖': '茎', '蕩': '荡', '薦': '荐', '藥': '药', '蘿': '萝', '號': '号',
	'製': '制', '褻': '亵', '襲': '袭', '視': '视', '訊': '讯', '記': '记', '註': '注', '話': '话',
	'認': '认', '請': '请', '謝': '谢', '證': '证', '護': '护', '讚': '赞', '豬': '猪', '貝': '贝',
	'貨': '货', '販': '贩', '買': '买', '費': '费', '資': '资', '賣': '卖', '賤': '贱', '賬': '账',
	'賭': '赌', '賽': '赛', '車': '车', '軍': '军', '轉': '转', '這': '这', '進': '进', '運': '运',
	'過': '过', '選': '选', '還': '还', '醯': '酯', '鏈': '链', '長': '长', '門': '门', '開': '开',
	'間': '间', '關': '关', '陰': '阴', '隊': '队', '雲': '云', '電': '电', '領': '领', '頻': '频',
	'顏': '颜', '風': '风', '馬': '马', '騷': '骚', '體': '体', '鬥': '斗', '鹼': '碱', '麥': '麦',
	'點': '点',
}

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
		if c, ok := traditionalToSimplified[r]; ok {
			r = c
		}
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
	length   int    // rune length of the normalized form, for span computation
}

func newACNode() *acNode {
	return &acNode{children: map[rune]*acNode{}}
}

// acAutomaton is an Aho-Corasick automaton: one O(n) pass over the text
// regardless of how many words it holds.
type acAutomaton struct {
	root  *acNode
	count int
}

type acSpan struct {
	start int // inclusive, in runes of the normalized text
	end   int // exclusive
	word  string
}

func newACAutomaton(words []string) *acAutomaton {
	a := &acAutomaton{root: newACNode()}

	for _, raw := range words {
		normalized := []rune(NormalizeForMatch(raw))
		if len(normalized) == 0 {
			continue
		}

		node := a.root
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
			node.length = len(normalized)
			a.count++
		}
	}

	a.buildFailLinks()
	return a
}

func (a *acAutomaton) buildFailLinks() {
	a.root.fail = a.root
	queue := make([]*acNode, 0, len(a.root.children))
	for _, child := range a.root.children {
		child.fail = a.root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for r, child := range node.children {
			fail := node.fail
			for fail != a.root {
				if _, ok := fail.children[r]; ok {
					break
				}
				fail = fail.fail
			}

			child.fail = a.root
			if next, ok := fail.children[r]; ok && next != child {
				child.fail = next
			}

			queue = append(queue, child)
		}
	}
}

// findSpans reports every match in the normalized text, as rune offsets.
func (a *acAutomaton) findSpans(runes []rune) []acSpan {
	if a == nil || a.root == nil {
		return nil
	}

	spans := []acSpan{}
	node := a.root
	for i, r := range runes {
		for {
			if next, ok := node.children[r]; ok {
				node = next
				break
			}
			if node == a.root {
				break
			}
			node = node.fail
		}

		// Walk the fail chain so that words which are suffixes of the current
		// path are reported too (e.g. both "abc" and "bc" in the list).
		for t := node; t != a.root; t = t.fail {
			if t.word == "" {
				continue
			}
			spans = append(spans, acSpan{start: i + 1 - t.length, end: i + 1, word: t.word})
		}
	}

	return spans
}

// SensitiveMatcher holds the banned words plus an allow list of longer phrases
// that legitimately contain one of them.
//
// The allow list exists because Chinese compounds collide constantly: "大麻"
// (cannabis) is a substring of "大麻烦" (big trouble), "处女" of "处女座"
// (Virgo). Dropping the short word to dodge the collision would leave it
// usable on its own, so instead a match is discarded when it falls entirely
// inside an allowed phrase.
type SensitiveMatcher struct {
	banned  *acAutomaton
	allowed *acAutomaton
}

// NewSensitiveMatcher builds a matcher from raw word list entries. Entries are
// normalized here, so callers pass them in plain form. Entries that normalize
// to an empty string are skipped.
func NewSensitiveMatcher(words []string) *SensitiveMatcher {
	return NewSensitiveMatcherWithAllowList(words, nil)
}

// NewSensitiveMatcherWithAllowList also takes phrases that must never be
// treated as a violation, even when they contain a banned word.
func NewSensitiveMatcherWithAllowList(words []string, allowed []string) *SensitiveMatcher {
	return &SensitiveMatcher{
		banned:  newACAutomaton(words),
		allowed: newACAutomaton(allowed),
	}
}

// Size reports how many distinct banned words the matcher holds.
func (m *SensitiveMatcher) Size() int {
	if m == nil || m.banned == nil {
		return 0
	}
	return m.banned.count
}

// AllowListSize reports how many allow list phrases the matcher holds.
func (m *SensitiveMatcher) AllowListSize() int {
	if m == nil || m.allowed == nil {
		return 0
	}
	return m.allowed.count
}

// Match returns the first banned word found in text, if any. The text is
// normalized internally.
func (m *SensitiveMatcher) Match(text string) (string, bool) {
	hits := m.match(text, true)
	if len(hits) == 0 {
		return "", false
	}
	return hits[0], true
}

// MatchAll returns every distinct banned word found in text.
func (m *SensitiveMatcher) MatchAll(text string) []string {
	return m.match(text, false)
}

func (m *SensitiveMatcher) match(text string, stopAtFirst bool) []string {
	if m == nil || m.banned == nil {
		return nil
	}

	runes := []rune(NormalizeForMatch(text))
	if len(runes) == 0 {
		return nil
	}

	spans := m.banned.findSpans(runes)
	if len(spans) == 0 {
		return nil
	}

	// Mark the positions covered by an allowed phrase; a banned match sitting
	// entirely inside one of them is a false positive, not a violation.
	var allowedMask []bool
	if m.allowed != nil && m.allowed.count > 0 {
		allowedMask = make([]bool, len(runes))
		for _, span := range m.allowed.findSpans(runes) {
			for i := span.start; i < span.end; i++ {
				allowedMask[i] = true
			}
		}
	}

	var (
		hits []string
		seen = map[string]bool{}
	)
	for _, span := range spans {
		if isFullyAllowed(allowedMask, span) {
			continue
		}
		if seen[span.word] {
			continue
		}
		seen[span.word] = true
		hits = append(hits, span.word)
		if stopAtFirst {
			return hits
		}
	}

	return hits
}

func isFullyAllowed(allowedMask []bool, span acSpan) bool {
	if allowedMask == nil {
		return false
	}
	for i := span.start; i < span.end; i++ {
		if i < 0 || i >= len(allowedMask) || !allowedMask[i] {
			return false
		}
	}
	return true
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

// allowListSectionNames are the section headers that switch the parser over to
// the allow list. Both spellings are accepted so the file stays readable to
// whoever maintains it.
var allowListSectionNames = []string{"白名单", "例外", "allowlist", "allow list", "whitelist"}

func isAllowListSection(line string) bool {
	name := strings.ToLower(strings.TrimSpace(strings.Trim(line, "[]")))
	for _, candidate := range allowListSectionNames {
		if name == candidate {
			return true
		}
	}
	return false
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
	allowed := []string{}
	inAllowList := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// "[Category]" lines label a section. One section name is special: it
		// switches the following entries to the allow list.
		if strings.HasPrefix(line, "[") {
			inAllowList = isAllowListSection(line)
			continue
		}
		if inAllowList {
			allowed = append(allowed, line)
		} else {
			words = append(words, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	sensitiveMatcher = NewSensitiveMatcherWithAllowList(words, allowed)
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
