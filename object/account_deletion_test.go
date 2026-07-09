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

//go:build !skipCi

package object

import (
	"testing"
	"time"
)

func TestAccountDeletionCoolingAndPurge(t *testing.T) {
	createDatabase = false
	InitConfig()

	org := "test-del-org"
	ormer.Engine.Where("owner = ?", org).Delete(&User{})

	u := &User{Owner: org, Name: "delme", CreatedTime: GetCurrentTimeForTest(), Email: "d@e.com"}
	if _, err := ormer.Engine.Insert(u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// request deletion -> schedules a future time, account still present
	scheduled, err := RequestAccountDeletion(u)
	if err != nil || scheduled == "" {
		t.Fatalf("request: err=%v scheduled=%q", err, scheduled)
	}
	got, _ := getUser(org, "delme")
	if got == nil || got.DeleteScheduledTime == "" {
		t.Fatalf("scheduled time not stored: %+v", got)
	}
	// cooling-off in the future -> NOT selected for purge
	if exp := findExpiredAccounts(); len(exp) != 0 {
		t.Fatalf("account selected too early, got %d", len(exp))
	}

	// cancel -> clears the schedule
	if err := CancelAccountDeletion(got); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ = getUser(org, "delme")
	if got.DeleteScheduledTime != "" {
		t.Fatalf("cancel did not clear schedule: %q", got.DeleteScheduledTime)
	}

	// force an already-elapsed schedule -> selected for purge
	got.DeleteScheduledTime = time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
	ormer.Engine.Where("owner = ? and name = ?", org, "delme").Cols("delete_scheduled_time").Update(got)
	exp := findExpiredAccounts()
	found := false
	for _, u := range exp {
		if u.Owner == org && u.Name == "delme" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expired account not selected for purge, got %d expired", len(exp))
	}

	ormer.Engine.Where("owner = ?", org).Delete(&User{})
}

// GetCurrentTimeForTest is a tiny helper so the test doesn't import util directly.
func GetCurrentTimeForTest() string {
	return time.Now().Format(time.RFC3339)
}
