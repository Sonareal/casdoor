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
	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

// SensitiveHit is one user whose stored text violates the current word list.
type SensitiveHit struct {
	Owner       string   `json:"owner"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Field       string   `json:"field"`
	Words       []string `json:"words"`
	Flagged     bool     `json:"flagged"`
}

// ScanSensitiveUsers walks existing users and reports those carrying a banned
// word — accounts created before the word list existed, or before a word was
// added to it.
//
// With apply=false nothing is written, so it is safe to run for a report.
// With apply=true every hit gets NeedUpdateDisplayName set, which is the
// client's cue to force a rename before the user can continue. The offending
// text itself is deliberately left alone: overwriting it would destroy the
// original value and make a false positive unrecoverable.
//
// owner scopes the scan to one organization; "" scans all of them.
func ScanSensitiveUsers(owner string, apply bool) ([]*SensitiveHit, error) {
	users := []*User{}

	session := ormer.Engine.NewSession()
	defer session.Close()
	if owner != "" {
		session = session.Where("owner = ?", owner)
	}
	if err := session.Find(&users); err != nil {
		return nil, err
	}

	hits := []*SensitiveHit{}
	for _, user := range users {
		for _, field := range sensitiveUserFields {
			value := field.get(user)
			if value == "" {
				continue
			}

			words := util.MatchAllSensitiveWords(value)
			if len(words) == 0 {
				continue
			}

			hit := &SensitiveHit{
				Owner:       user.Owner,
				Name:        user.Name,
				DisplayName: user.DisplayName,
				Field:       field.label,
				Words:       words,
			}

			if apply && !user.NeedUpdateDisplayName {
				user.NeedUpdateDisplayName = true
				affected, err := ormer.Engine.ID(core.PK{user.Owner, user.Name}).
					Cols("need_update_display_name").Update(user)
				if err != nil {
					return nil, err
				}
				hit.Flagged = affected > 0
			} else {
				hit.Flagged = user.NeedUpdateDisplayName
			}

			hits = append(hits, hit)
		}
	}

	return hits, nil
}
