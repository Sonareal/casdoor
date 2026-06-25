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

package controllers

import (
	"encoding/json"

	"github.com/beego/beego/v2/core/utils/pagination"
	"github.com/casdoor/casdoor/object"
	"github.com/casdoor/casdoor/util"
)

// trackEventForm is one client-reported event. `properties` is an arbitrary JSON object.
type trackEventForm struct {
	Event       string                 `json:"event"`
	ClientTime  string                 `json:"clientTime"`
	DistinctId  string                 `json:"distinctId"`
	Application string                 `json:"application"`
	Platform    string                 `json:"platform"`
	AppVersion  string                 `json:"appVersion"`
	SessionId   string                 `json:"sessionId"`
	Properties  map[string]interface{} `json:"properties"`
}

// Track
// @Title Track
// @Tag Event API
// @Description ingest client analytics events (batch). Auth optional (Bearer or anonymous distinctId).
// @router /track [post]
func (c *ApiController) Track() {
	var body struct {
		Events []trackEventForm `json:"events"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(body.Events) == 0 || len(body.Events) > 200 {
		c.ResponseError("events must contain 1..200 items")
		return
	}

	// attribute to the signed-in user if a valid session/token is present (else anonymous)
	userId := c.GetSessionUsername()
	owner := ""
	if userId != "" {
		owner, _ = util.GetOwnerAndNameFromIdNoCheck(userId)
	}
	clientIp := util.GetClientIp(c.Ctx.Request)

	events := make([]*object.Event, 0, len(body.Events))
	for _, f := range body.Events {
		if f.Event == "" {
			continue
		}
		o := owner
		if o == "" {
			o = f.Application // fall back to app/org context for anonymous events
		}
		propsStr := ""
		if len(f.Properties) > 0 {
			if b, err := json.Marshal(f.Properties); err == nil {
				propsStr = string(b)
			}
		}
		events = append(events, &object.Event{
			Owner:       o,
			Event:       f.Event,
			Source:      object.EventSourceClient,
			User:        userId,
			DistinctId:  f.DistinctId,
			Application: f.Application,
			Platform:    f.Platform,
			AppVersion:  f.AppVersion,
			SessionId:   f.SessionId,
			ClientTime:  f.ClientTime,
			ClientIp:    clientIp,
			Properties:  propsStr,
		})
	}

	if _, err := object.AddEvents(events); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(len(events))
}

// GetEvents
// @Title GetEvents
// @Tag Event API
// @Description (admin) list analytics events
// @Param   owner   query   string  true  "owner (organization)"
// @router /get-events [get]
func (c *ApiController) GetEvents() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	owner := c.Ctx.Input.Query("owner")
	limit := c.Ctx.Input.Query("pageSize")
	page := c.Ctx.Input.Query("p")
	field := c.Ctx.Input.Query("field")
	value := c.Ctx.Input.Query("value")
	sortField := c.Ctx.Input.Query("sortField")
	sortOrder := c.Ctx.Input.Query("sortOrder")

	limitInt := util.ParseInt(limit)
	if limitInt == 0 {
		limitInt = 25
	}
	count, err := object.GetEventCount(owner, field, value)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	paginator := pagination.NewPaginator(c.Ctx.Request, limitInt, count)
	events, err := object.GetPaginationEvents(owner, paginator.Offset(), limitInt, field, value, sortField, sortOrder)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(events, paginator.Nums())
	_ = page
}

// GetEventStats
// @Title GetEventStats
// @Tag Event API
// @Description (admin) summary analytics: daily counts, top events, DAU, conversion funnel
// @Param   owner   query   string  true  "owner (organization)"
// @Param   days    query   int     false "lookback days (default 14)"
// @router /get-event-stats [get]
func (c *ApiController) GetEventStats() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	owner := c.Ctx.Input.Query("owner")
	days := util.ParseInt(c.Ctx.Input.Query("days"))
	stats, err := object.GetEventStats(owner, days)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(stats)
}
