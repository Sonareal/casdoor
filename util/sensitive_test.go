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
	"os"
	"path/filepath"
	"testing"
)

func TestSensitiveMatcherEvasions(t *testing.T) {
	matcher := NewSensitiveMatcher([]string{"海洛因", "heroin", "casino", "加微信", "官方客服"})

	blocked := []struct {
		name string
		text string
	}{
		{"plain", "售卖海洛因"},
		{"spaces", "售卖 海 洛 因"},
		{"punctuation", "售卖-海*洛*因"},
		{"zero width", "售卖海​洛​因"},
		{"emoji separator", "海🌟洛🌟因"},
		{"full width latin", "ｈｅｒｏｉｎ"},
		{"mixed case", "HeRoIn"},
		{"leetspeak digit", "her0in"},
		{"leetspeak symbol", "c@sin0"},
		{"embedded", "我的名字叫海洛因你好"},
		{"ad word", "加微信 abc123"},
		{"impersonation", "官方客服"},
	}
	for _, c := range blocked {
		t.Run("blocked/"+c.name, func(t *testing.T) {
			if word, ok := matcher.Match(c.text); !ok {
				t.Errorf("expected %q to be blocked, but it passed", c.text)
			} else if word == "" {
				t.Errorf("expected a non-empty matched word for %q", c.text)
			}
		})
	}

	allowed := []struct {
		name string
		text string
	}{
		{"normal chinese", "张小明"},
		{"normal latin", "Alice Wang"},
		{"partial only", "海洋"},
		{"digits", "user2024"},
		{"empty", ""},
	}
	for _, c := range allowed {
		t.Run("allowed/"+c.name, func(t *testing.T) {
			if word, ok := matcher.Match(c.text); ok {
				t.Errorf("expected %q to pass, but it matched %q", c.text, word)
			}
		})
	}
}

func TestSensitiveMatcherReportsOriginalWord(t *testing.T) {
	matcher := NewSensitiveMatcher([]string{"海洛因"})

	word, ok := matcher.Match("售卖海​洛因")
	if !ok {
		t.Fatal("expected a match")
	}
	if word != "海洛因" {
		t.Errorf("expected the word list entry %q to be reported, got %q", "海洛因", word)
	}
}

// A word that is a suffix of another must still be reported.
func TestSensitiveMatcherSuffixWord(t *testing.T) {
	matcher := NewSensitiveMatcher([]string{"abcd", "bc"})

	hits := matcher.MatchAll("xxabcdxx")
	found := map[string]bool{}
	for _, h := range hits {
		found[h] = true
	}
	if !found["abcd"] || !found["bc"] {
		t.Errorf("expected both %q and %q to be reported, got %v", "abcd", "bc", hits)
	}
}

func TestSensitiveMatcherEmptyList(t *testing.T) {
	matcher := NewSensitiveMatcher(nil)
	if _, ok := matcher.Match("海洛因"); ok {
		t.Error("an empty word list must not match anything")
	}
	if matcher.Size() != 0 {
		t.Errorf("expected size 0, got %d", matcher.Size())
	}
}

func TestSensitiveMatcherSkipsBlankEntries(t *testing.T) {
	// Entries that normalize to nothing (punctuation only) must not become a
	// catch-all that blocks every name.
	matcher := NewSensitiveMatcher([]string{"   ", "***", "海洛因"})
	if matcher.Size() != 1 {
		t.Fatalf("expected only 1 usable word, got %d", matcher.Size())
	}
	if _, ok := matcher.Match("张小明"); ok {
		t.Error("blank word list entries must not match clean text")
	}
}

func TestInitSensitiveFilterFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "words.txt")

	content := "# comment line\n[Drugs]\n海洛因\n\nheroin\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := InitSensitiveFilter(path); err != nil {
		t.Fatal(err)
	}
	if got := SensitiveWordCount(); got != 2 {
		t.Fatalf("expected 2 words (comments and section headers skipped), got %d", got)
	}

	if _, ok := ContainsSensitiveWord("售卖海洛因"); !ok {
		t.Error("expected the loaded word list to block the text")
	}
	if _, ok := ContainsSensitiveWord("张小明"); ok {
		t.Error("expected clean text to pass")
	}
}

// A missing word list disables filtering instead of failing the boot.
func TestInitSensitiveFilterMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.txt")

	if err := InitSensitiveFilter(path); err != nil {
		t.Fatalf("a missing word list must not be an error, got %v", err)
	}
	if _, ok := ContainsSensitiveWord("海洛因"); ok {
		t.Error("filtering must be disabled when no word list is present")
	}

	// Leave the package-level state clean for other tests.
	_ = InitSensitiveFilter("")
}

func TestNormalizeForMatch(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "helloworld"},
		{"ＡＢＣ", "abc"},
		{"a​b", "ab"},
		{"a-b_c", "abc"},
		{"张 小明", "张小明"},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := NormalizeForMatch(c.in); got != c.want {
			t.Errorf("NormalizeForMatch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
