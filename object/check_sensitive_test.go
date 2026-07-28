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

package object

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/casdoor/casdoor/util"
)

func loadTestWordList(t *testing.T) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(path, []byte("海洛因\n官方客服\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := util.InitSensitiveFilter(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = util.InitSensitiveFilter("") })
}

func TestCheckSensitiveUserFields(t *testing.T) {
	loadTestWordList(t)

	if msg := CheckSensitiveUserFields(&User{DisplayName: "售卖海洛因"}, "en"); msg == "" {
		t.Error("expected a banned display name to be rejected")
	}
	if msg := CheckSensitiveUserFields(&User{Bio: "官方客服"}, "en"); msg == "" {
		t.Error("expected a banned bio to be rejected: blocking only the display name moves the abuse elsewhere")
	}
	if msg := CheckSensitiveUserFields(&User{DisplayName: "张小明", Bio: "喜欢读书"}, "en"); msg != "" {
		t.Errorf("expected a clean user to pass, got %q", msg)
	}
	if msg := CheckSensitiveUserFields(nil, "en"); msg != "" {
		t.Errorf("a nil user must not be rejected, got %q", msg)
	}
}

// A user whose name predates the word list must not have unrelated writes
// rejected, or routine bookkeeping (last signin time, score, ...) would lock
// them out of the product entirely.
func TestCheckSensitiveUserFieldsUpdateIgnoresUnchangedFields(t *testing.T) {
	loadTestWordList(t)

	oldUser := &User{DisplayName: "售卖海洛因", Score: 0}
	newUser := &User{DisplayName: "售卖海洛因", Score: 10}

	if msg := CheckSensitiveUserFieldsUpdate(oldUser, newUser, []string{"score"}, "en"); msg != "" {
		t.Errorf("a pre-existing violation must not block an unrelated update, got %q", msg)
	}
	if msg := CheckSensitiveUserFieldsUpdate(oldUser, newUser, nil, "en"); msg != "" {
		t.Errorf("an unchanged violating field must not block a full update, got %q", msg)
	}
}

func TestCheckSensitiveUserFieldsUpdateRejectsNewViolation(t *testing.T) {
	loadTestWordList(t)

	oldUser := &User{DisplayName: "张小明"}
	newUser := &User{DisplayName: "售卖海洛因"}

	for _, columns := range [][]string{
		nil,                      // all columns
		{"displayName"},          // as the API receives it
		{"display_name"},         // as the controller forwards it
		{"score", "displayName"}, // alongside other columns
	} {
		if msg := CheckSensitiveUserFieldsUpdate(oldUser, newUser, columns, "en"); msg == "" {
			t.Errorf("expected rejection for columns %v", columns)
		}
	}
}

// A field that is not part of this write must not be judged.
func TestCheckSensitiveUserFieldsUpdateRespectsColumns(t *testing.T) {
	loadTestWordList(t)

	oldUser := &User{DisplayName: "张小明", Score: 0}
	newUser := &User{DisplayName: "售卖海洛因", Score: 10}

	if msg := CheckSensitiveUserFieldsUpdate(oldUser, newUser, []string{"score"}, "en"); msg != "" {
		t.Errorf("display name is not in columns, so it must not be judged, got %q", msg)
	}
}

func TestCheckSensitiveUserFieldsUpdateAllowsCleanRename(t *testing.T) {
	loadTestWordList(t)

	oldUser := &User{DisplayName: "售卖海洛因"}
	newUser := &User{DisplayName: "张小明"}

	if msg := CheckSensitiveUserFieldsUpdate(oldUser, newUser, []string{"displayName"}, "en"); msg != "" {
		t.Errorf("renaming away from a violation must be allowed, got %q", msg)
	}
}

func TestNormalizeColumnName(t *testing.T) {
	for _, in := range []string{"displayName", "display_name", "DisplayName", " display_name "} {
		if got := normalizeColumnName(in); got != "displayname" {
			t.Errorf("normalizeColumnName(%q) = %q, want %q", in, got, "displayname")
		}
	}
}

func TestIsColumnWritten(t *testing.T) {
	cases := []struct {
		columns []string
		column  string
		want    bool
	}{
		{nil, "display_name", true},                       // empty means all columns
		{[]string{}, "display_name", true},                // ditto
		{[]string{"display_name"}, "display_name", true},  // snake_case, as UpdateUser receives it
		{[]string{"displayName"}, "display_name", true},   // camelCase, as the API receives it
		{[]string{"score"}, "display_name", false},        // not part of this write
		{[]string{"score", "bio"}, "display_name", false}, // still not part of it
	}
	for _, c := range cases {
		if got := IsColumnWritten(c.columns, c.column); got != c.want {
			t.Errorf("IsColumnWritten(%v, %q) = %v, want %v", c.columns, c.column, got, c.want)
		}
	}
}
