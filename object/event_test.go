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
)

const evtOrg = "test-evt-org"

func TestEventTrackingAndStats(t *testing.T) {
	createDatabase = false
	InitConfig()
	ormer.Engine.Where("owner = ?", evtOrg).Delete(&Event{})

	// server-side (truth) events
	TrackServerEvent(evtOrg, "user_signup", "u1", "feiyu-app", map[string]interface{}{"signupMethod": "phone"})
	TrackServerEvent(evtOrg, "user_signup", "u2", "feiyu-app", nil)
	TrackServerEvent(evtOrg, "subscription_activated", "u1", "feiyu-app", map[string]interface{}{"plan": "pro-yearly", "source": "payment"})
	TrackServerEvent(evtOrg, "commission_earned", "ref1", "feiyu-app", map[string]interface{}{"amount": 5.6})

	// client-side batch
	n, err := AddEvents([]*Event{
		{Owner: evtOrg, Event: "invite_share", User: "ref1", Source: EventSourceClient, Platform: "iOS"},
		{Owner: evtOrg, Event: "page_view", DistinctId: "device-1", Source: EventSourceClient, Platform: "Android", Properties: `{"page":"home"}`},
	})
	if err != nil || n == 0 {
		t.Fatalf("AddEvents: n=%d err=%v", n, err)
	}

	// list
	cnt, err := GetEventCount(evtOrg, "", "")
	if err != nil || cnt != 6 {
		t.Fatalf("GetEventCount: want 6, got %d (err=%v)", cnt, err)
	}
	events, err := GetPaginationEvents(evtOrg, 0, 100, "", "", "", "")
	if err != nil || len(events) != 6 {
		t.Fatalf("GetPaginationEvents: want 6, got %d (err=%v)", len(events), err)
	}

	// stats + funnel
	stats, err := GetEventStats(evtOrg, 14)
	if err != nil {
		t.Fatalf("GetEventStats: %v", err)
	}
	funnel, ok := stats["funnel"].([]map[string]interface{})
	if !ok {
		t.Fatalf("funnel type: %T", stats["funnel"])
	}
	want := map[string]int64{"invite_share": 1, "user_signup": 2, "subscription_activated": 1, "commission_earned": 1}
	seen := map[string]bool{}
	for _, step := range funnel {
		ev := step["event"].(string)
		users := step["users"].(int64)
		seen[ev] = true
		if w, has := want[ev]; has && users != w {
			t.Fatalf("funnel %s: want users=%d, got %d", ev, w, users)
		}
	}
	for ev := range want {
		if !seen[ev] {
			t.Fatalf("funnel missing step: %s", ev)
		}
	}
	if stats["totalToday"].(int64) < 6 {
		t.Fatalf("totalToday should be >= 6, got %v", stats["totalToday"])
	}
}

func TestEventRetention(t *testing.T) {
	createDatabase = false
	InitConfig()
	owner := "test-evt-ret"
	ormer.Engine.Where("owner = ?", owner).Delete(&Event{})

	// one very old event, one recent
	ormer.Engine.Insert(&Event{Owner: owner, Name: "evt_old", Event: "x", Source: EventSourceServer, CreatedTime: "2020-01-01T00:00:00+08:00"})
	if _, err := AddEvents([]*Event{{Owner: owner, Event: "y", Source: EventSourceServer}}); err != nil {
		t.Fatalf("add recent: %v", err)
	}

	cutoff := daysAgoDate(90)
	deleted, err := DeleteEventsBefore(cutoff)
	if err != nil {
		t.Fatalf("delete before: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected to delete the old event, deleted=%d", deleted)
	}
	cnt, _ := ormer.Engine.Where("owner = ?", owner).Count(&Event{})
	if cnt != 1 {
		t.Fatalf("expected 1 recent event to remain, got %d", cnt)
	}
	ormer.Engine.Where("owner = ?", owner).Delete(&Event{})
}
