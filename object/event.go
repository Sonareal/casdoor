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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor/conf"
	"github.com/casdoor/casdoor/util"
)

func daysAgoDate(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02")
}

const (
	EventSourceServer = "server"
	EventSourceClient = "client"
)

// Event is a lightweight analytics / tracking event (both server-side conversions and client-side behavior).
type Event struct {
	Owner       string `xorm:"varchar(100) notnull pk index" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100) index" json:"createdTime"` // server receive time
	ClientTime  string `xorm:"varchar(100)" json:"clientTime"`        // client event time (optional)

	Event       string `xorm:"varchar(100) index" json:"event"` // event name, snake_case
	Source      string `xorm:"varchar(20)" json:"source"`       // server | client
	User        string `xorm:"varchar(100) index" json:"user"`  // owner/name when logged in
	DistinctId  string `xorm:"varchar(100)" json:"distinctId"`  // anonymous/device id
	Application string `xorm:"varchar(100)" json:"application"`
	Platform    string `xorm:"varchar(50)" json:"platform"`
	AppVersion  string `xorm:"varchar(50)" json:"appVersion"`
	SessionId   string `xorm:"varchar(100)" json:"sessionId"`
	ClientIp    string `xorm:"varchar(100)" json:"clientIp"`
	Region      string `xorm:"varchar(100)" json:"region"`

	Properties string `xorm:"mediumtext" json:"properties"` // JSON object as text
}

func (e *Event) GetId() string {
	return fmt.Sprintf("%s/%s", e.Owner, e.Name)
}

// AddEvents bulk-inserts events (lightweight write path). Names are auto-assigned.
func AddEvents(events []*Event) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}
	now := util.GetCurrentTime()
	for _, e := range events {
		if e.Name == "" {
			e.Name = "evt_" + strings.ReplaceAll(util.GenerateId(), "-", "")
		}
		if e.CreatedTime == "" {
			e.CreatedTime = now
		}
		if e.Source == "" {
			e.Source = EventSourceClient
		}
	}
	return ormer.Engine.Insert(&events)
}

// TrackServerEvent records a server-side (truth) event. Best-effort: errors are logged, never propagated,
// so analytics never breaks a business flow. Use inside payment/redeem/signup/etc. hooks.
func TrackServerEvent(owner, event, user, application string, props map[string]interface{}) {
	propsStr := ""
	if len(props) > 0 {
		if b, err := json.Marshal(props); err == nil {
			propsStr = string(b)
		}
	}
	e := &Event{
		Owner:       owner,
		Event:       event,
		Source:      EventSourceServer,
		User:        user,
		Application: application,
		Properties:  propsStr,
	}
	if _, err := AddEvents([]*Event{e}); err != nil {
		logs.Warning(fmt.Sprintf("TrackServerEvent(%s/%s) failed: %v", owner, event, err))
	}
}

// DeleteEventsBefore removes events with created_time older than the given date (YYYY-MM-DD).
func DeleteEventsBefore(date string) (int64, error) {
	return ormer.Engine.Where("created_time < ?", date).Delete(&Event{})
}

func eventRetentionDays() int {
	s := conf.GetConfigString("eventRetentionDays")
	if s == "" {
		return 90
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return 90
}

// StartEventRetention runs a daily cleanup that deletes events older than `eventRetentionDays`
// (default 90; set <= 0 in conf to disable). Best-effort; safe to run as a background goroutine.
func StartEventRetention() {
	run := func() {
		days := eventRetentionDays()
		if days <= 0 {
			return
		}
		cutoff := daysAgoDate(days)
		if affected, err := DeleteEventsBefore(cutoff); err != nil {
			logs.Warning(fmt.Sprintf("event retention cleanup failed: %v", err))
		} else if affected > 0 {
			logs.Info(fmt.Sprintf("event retention: deleted %d events older than %s", affected, cutoff))
		}
	}
	run() // once at startup
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

func GetEventCount(owner, field, value string) (int64, error) {
	session := GetSession(owner, -1, -1, field, value, "", "")
	return session.Count(&Event{})
}

func GetPaginationEvents(owner string, offset, limit int, field, value, sortField, sortOrder string) ([]*Event, error) {
	events := []*Event{}
	if sortField == "" {
		sortField = "created_time"
		sortOrder = "descend"
	}
	session := GetSession(owner, offset, limit, field, value, sortField, sortOrder)
	err := session.Find(&events, &Event{Owner: owner})
	return events, err
}

// GetEventStats returns summary analytics for the last `days` days:
// daily event counts, top events, DAU today, and a fixed conversion funnel.
// StatRow is a generic {key,count,users} aggregation row.
type StatRow struct {
	Key   string `xorm:"k" json:"key"`
	Count int64  `xorm:"c" json:"count"`
	Users int64  `xorm:"u" json:"users"`
}

func eventOwnerClause(owner string) (string, []interface{}) {
	if owner == "" {
		return "", nil
	}
	return "owner = ? AND ", []interface{}{owner}
}

// eventGroupBy aggregates count/users grouped by an expression over the last `days`.
func eventGroupBy(owner, groupExpr, orderBy string, days, limit int) ([]StatRow, error) {
	oc, args := eventOwnerClause(owner)
	sql := fmt.Sprintf("SELECT %s AS k, count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %screated_time >= ? GROUP BY %s ORDER BY %s", groupExpr, oc, groupExpr, orderBy)
	if limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows := []StatRow{}
	err := ormer.Engine.SQL(sql, append(append([]interface{}{}, args...), daysAgoDate(days))...).Find(&rows)
	return rows, err
}

// eventCountWhere returns (events, distinctUsers) matching an extra WHERE over the last `days`.
func eventCountWhere(owner, extraWhere string, extraArgs []interface{}, days int) (int64, int64) {
	oc, ocArgs := eventOwnerClause(owner)
	args := append([]interface{}{}, ocArgs...)
	args = append(args, extraArgs...)
	args = append(args, daysAgoDate(days))
	sql := fmt.Sprintf("SELECT count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %s%screated_time >= ?", oc, extraWhere)
	r := StatRow{}
	_, _ = ormer.Engine.SQL(sql, args...).Get(&r)
	return r.Count, r.Users
}

func eventCount(owner, event string, days int) int64 {
	c, _ := eventCountWhere(owner, "event = ? AND ", []interface{}{event}, days)
	return c
}

// sumEventAmount sums the numeric "amount" property across events of a name (DB-agnostic; parsed in Go).
func sumEventAmount(owner, event string, days int) float64 {
	events := []*Event{}
	sess := ormer.Engine.Where("event = ? AND created_time >= ?", event, daysAgoDate(days))
	if owner != "" {
		sess = sess.And("owner = ?", owner)
	}
	if err := sess.Cols("properties").Find(&events); err != nil {
		return 0
	}
	sum := 0.0
	for _, e := range events {
		if e.Properties == "" {
			continue
		}
		var p map[string]interface{}
		if json.Unmarshal([]byte(e.Properties), &p) == nil {
			if f, ok := p["amount"].(float64); ok {
				sum += f
			}
		}
	}
	return sum
}

// GetEventStats returns a comprehensive operations dashboard payload for the last `days` days.
func GetEventStats(owner string, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 14
	}
	res := map[string]interface{}{"days": days}

	// daily trend (ordered by date)
	daily, err := eventGroupBy(owner, "substr(created_time,1,10)", "k", days, 0)
	if err != nil {
		return nil, err
	}
	res["daily"] = daily

	// breakdowns
	if top, err := eventGroupBy(owner, "event", "c DESC", days, 20); err == nil {
		res["topEvents"] = top
	}
	if byPlat, err := eventGroupBy(owner, "platform", "c DESC", days, 0); err == nil {
		res["byPlatform"] = byPlat
	}
	if bySrc, err := eventGroupBy(owner, "source", "c DESC", days, 0); err == nil {
		res["bySource"] = bySrc
	}

	// today + period totals
	oc, ocArgs := eventOwnerClause(owner)
	today := util.GetCurrentTime()[:10]
	tRow := StatRow{}
	_, _ = ormer.Engine.SQL(fmt.Sprintf("SELECT count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %ssubstr(created_time,1,10) = ?", oc), append(append([]interface{}{}, ocArgs...), today)...).Get(&tRow)
	res["totalToday"] = tRow.Count
	res["dauToday"] = tRow.Users

	totalEvents, activeUsers := eventCountWhere(owner, "", nil, days)
	res["totalEvents"] = totalEvents
	res["activeUsers"] = activeUsers

	// operations metrics (period)
	res["metrics"] = map[string]interface{}{
		"newUsers":         eventCount(owner, "user_signup", days),
		"payments":         eventCount(owner, "payment_paid", days),
		"revenue":          sumEventAmount(owner, "payment_paid", days),
		"subscriptions":    eventCount(owner, "subscription_activated", days),
		"giftRedeems":      eventCount(owner, "gift_card_redeemed", days),
		"commissions":      eventCount(owner, "commission_earned", days),
		"commissionAmount": sumEventAmount(owner, "commission_earned", days),
		"withdrawals":      eventCount(owner, "withdrawal_requested", days),
		"activeUsers":      activeUsers,
		"events":           totalEvents,
	}

	// conversion funnel: distinct users per step (frontend computes step-to-step rate)
	funnelEvents := []string{"invite_share", "user_signup", "subscription_activated", "commission_earned"}
	funnel := []map[string]interface{}{}
	for _, ev := range funnelEvents {
		u, c := func() (int64, int64) { c, u := eventCountWhere(owner, "event = ? AND ", []interface{}{ev}, days); return u, c }()
		funnel = append(funnel, map[string]interface{}{"event": ev, "users": u, "count": c})
	}
	res["funnel"] = funnel

	return res, nil
}
