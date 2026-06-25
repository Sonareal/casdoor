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
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
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
func GetEventStats(owner string, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 14
	}
	res := map[string]interface{}{}

	type row struct {
		K string `xorm:"k" json:"key"`
		C int64  `xorm:"c" json:"count"`
		U int64  `xorm:"u" json:"users"`
	}

	ownerCond := ""
	args := []interface{}{}
	if owner != "" {
		ownerCond = "owner = ? AND "
		args = append(args, owner)
	}

	// daily event counts (last `days`)
	daily := []row{}
	sql := fmt.Sprintf("SELECT substr(created_time,1,10) AS k, count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %screated_time >= ? GROUP BY substr(created_time,1,10) ORDER BY k", ownerCond)
	sinceDay := util.GetCurrentTime()[:10]
	_ = sinceDay
	if err := ormer.Engine.SQL(sql, append(append([]interface{}{}, args...), daysAgoDate(days))...).Find(&daily); err != nil {
		return nil, err
	}
	res["daily"] = daily

	// top events (last `days`)
	top := []row{}
	sql = fmt.Sprintf("SELECT event AS k, count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %screated_time >= ? GROUP BY event ORDER BY c DESC LIMIT 20", ownerCond)
	if err := ormer.Engine.SQL(sql, append(append([]interface{}{}, args...), daysAgoDate(days))...).Find(&top); err != nil {
		return nil, err
	}
	res["topEvents"] = top

	// totals + DAU today
	today := util.GetCurrentTime()[:10]
	var totalToday, dauToday int64
	cntRow := row{}
	sql = fmt.Sprintf("SELECT count(*) AS c, count(distinct \"user\") AS u FROM event WHERE %ssubstr(created_time,1,10) = ?", ownerCond)
	if _, err := ormer.Engine.SQL(sql, append(append([]interface{}{}, args...), today)...).Get(&cntRow); err == nil {
		totalToday = cntRow.C
		dauToday = cntRow.U
	}
	res["totalToday"] = totalToday
	res["dauToday"] = dauToday

	// conversion funnel: distinct users per step (last `days`)
	funnelEvents := []string{"invite_share", "user_signup", "subscription_activated", "commission_earned"}
	funnel := []map[string]interface{}{}
	for _, ev := range funnelEvents {
		r := row{}
		sql = fmt.Sprintf("SELECT count(distinct \"user\") AS u, count(*) AS c FROM event WHERE %sevent = ? AND created_time >= ?", ownerCond)
		_, _ = ormer.Engine.SQL(sql, append(append([]interface{}{}, args...), ev, daysAgoDate(days))...).Get(&r)
		funnel = append(funnel, map[string]interface{}{"event": ev, "users": r.U, "count": r.C})
	}
	res["funnel"] = funnel
	res["days"] = days

	return res, nil
}
