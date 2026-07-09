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
	"time"

	"github.com/casdoor/casdoor/conf"
)

// accountDeletionCoolingDays is the cooling-off window (in days) between a user
// requesting account deletion and the account being physically removed.
// Configurable via conf key `accountDeletionCoolingDays` (default 15).
func accountDeletionCoolingDays() int {
	n, err := conf.GetConfigInt64("accountDeletionCoolingDays")
	if err != nil || n <= 0 {
		return 15
	}
	return int(n)
}

// RequestAccountDeletion schedules the user's own account for deletion after the
// cooling-off period. Returns the scheduled deletion time (RFC3339). The account
// stays recoverable (via CancelAccountDeletion) until then.
func RequestAccountDeletion(user *User) (string, error) {
	scheduled := time.Now().AddDate(0, 0, accountDeletionCoolingDays()).Format(time.RFC3339)
	user.DeleteScheduledTime = scheduled
	_, err := ormer.Engine.Where("owner = ? and name = ?", user.Owner, user.Name).
		Cols("delete_scheduled_time").Update(user)
	if err != nil {
		return "", err
	}
	return scheduled, nil
}

// CancelAccountDeletion clears a pending deletion during the cooling-off period.
func CancelAccountDeletion(user *User) error {
	user.DeleteScheduledTime = ""
	_, err := ormer.Engine.Where("owner = ? and name = ?", user.Owner, user.Name).
		Cols("delete_scheduled_time").Update(user)
	return err
}

// findExpiredAccounts returns every account whose cooling-off period has elapsed
// (built-in/admin is always excluded as a safety guard).
func findExpiredAccounts() []*User {
	users := []*User{}
	if err := ormer.Engine.Where("delete_scheduled_time != ?", "").Find(&users); err != nil {
		return nil
	}
	now := time.Now()
	expired := []*User{}
	for _, u := range users {
		t, perr := time.Parse(time.RFC3339, u.DeleteScheduledTime)
		if perr != nil || !now.After(t) {
			continue
		}
		if u.Owner == "built-in" && u.Name == "admin" {
			continue
		}
		expired = append(expired, u)
	}
	return expired
}

// purgeExpiredAccounts physically deletes every account whose cooling-off period
// has elapsed. Returns the number of accounts removed.
func purgeExpiredAccounts() int {
	removed := 0
	for _, u := range findExpiredAccounts() {
		if ok, err := DeleteUser(u); err == nil && ok {
			removed++
		}
	}
	return removed
}

// StartAccountDeletion runs purgeExpiredAccounts once a day (best-effort).
func StartAccountDeletion() {
	for {
		purgeExpiredAccounts()
		time.Sleep(24 * time.Hour)
	}
}
