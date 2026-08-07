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
	"fmt"

	"github.com/casdoor/casdoor/object"
	"github.com/casdoor/casdoor/util"
)

// resolveCustomPropertyTarget picks the user a custom-property request applies
// to. Without "id" it is the caller themselves, which is the case an app hits
// with a plain user token. With "id" it is someone else, which only an admin
// may do.
func (c *ApiController) resolveCustomPropertyTarget() (*object.User, bool) {
	callerId := c.GetSessionUsername()
	if callerId == "" {
		c.ResponseError(c.T("general:Please login first"))
		return nil, false
	}

	id := c.Ctx.Input.Query("id")

	// An application credential (clientId/clientSecret) authenticates as
	// "app/<name>", which is not a row in the user table. Such a caller has no
	// "self" to default to, so it has to name the target explicitly — this is
	// the back office path.
	if object.IsAppUser(callerId) {
		if id == "" {
			c.ResponseError(c.T("general:Missing parameter") + ": id")
			return nil, false
		}
		return c.loadCustomPropertyUser(id)
	}

	if id == "" || id == callerId {
		caller, ok := c.RequireSignedInUser()
		if !ok {
			return nil, false
		}
		return caller, true
	}

	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return nil, false
	}

	return c.loadCustomPropertyUser(id)
}

func (c *ApiController) loadCustomPropertyUser(id string) (*object.User, bool) {
	target, err := object.GetUser(id)
	if err != nil {
		c.ResponseError(err.Error())
		return nil, false
	}
	if target == nil {
		c.ResponseError(fmt.Sprintf(c.T("general:The user: %s doesn't exist"), id))
		return nil, false
	}

	return target, true
}

// GetCustomProperties
// @Title GetCustomProperties
// @Tag User API
// @Description get the custom properties of a user; defaults to the caller
// @Param   id   query   string  false  "The id ( owner/name ) of the user; admin only"
// @Success 200 {object} controllers.Response The Response object
// @router /get-custom-properties [get]
func (c *ApiController) GetCustomProperties() {
	user, ok := c.resolveCustomPropertyTarget()
	if !ok {
		return
	}

	properties := user.CustomProperties
	if properties == nil {
		properties = map[string]*object.CustomProperty{}
	}
	c.ResponseOk(properties)
}

// SetCustomProperties
// @Title SetCustomProperties
// @Tag User API
// @Description merge custom properties into a user; defaults to the caller.
// @Description A null value deletes the key. The server stamps updatedTime.
// @Param   id     query   string  false  "The id ( owner/name ) of the user; admin only"
// @Param   body   body    object  true   "A map of key to value; null deletes"
// @Success 200 {object} controllers.Response The Response object
// @router /set-custom-properties [post]
func (c *ApiController) SetCustomProperties() {
	user, ok := c.resolveCustomPropertyTarget()
	if !ok {
		return
	}

	// A pointer value lets a client distinguish "set to empty" from "delete",
	// which a map[string]string could not express.
	updates := map[string]*string{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &updates); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(updates) == 0 {
		c.ResponseError(c.T("general:Missing parameter"))
		return
	}

	properties, err := object.SetUserCustomProperties(user, updates, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(properties)
}

// GetUsersByCustomProperty
// @Title GetUsersByCustomProperty
// @Tag User API
// @Description look up the users in an organization whose custom property holds a value,
// @Description most recently updated first. Admin only.
// @Param   owner   query   string  true   "The organization to search"
// @Param   key     query   string  true   "The custom property name, e.g. deviceId"
// @Param   value   query   string  true   "The value to match exactly"
// @Success 200 {object} controllers.Response The Response object
// @router /get-users-by-custom-property [get]
func (c *ApiController) GetUsersByCustomProperty() {
	// Three gates, because this one endpoint can resolve an opaque value such
	// as a device id back to an account in ANY organization:
	//   1. admin only — an ordinary token must never reach it
	//   2. an explicit caller whitelist, deny-by-default (app.conf)
	//   3. an optional source-IP whitelist on top
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}

	clientIp := util.GetClientIpFromRequest(c.Ctx.Request)
	if err := object.CheckCustomPropertyLookupAllowed(c.GetSessionUsername(), clientIp, c.GetAcceptLanguage()); err != nil {
		c.ResponseError(err.Error())
		return
	}

	// owner is optional: empty searches every organization.
	owner := c.Ctx.Input.Query("owner")
	key := c.Ctx.Input.Query("key")
	value := c.Ctx.Input.Query("value")
	if key == "" || value == "" {
		c.ResponseError(c.T("general:Missing parameter"))
		return
	}

	// Only an attribute an operator marked as the primary key may be searched.
	// Otherwise every stored attribute — a theme, an app version — would double
	// as a way to enumerate accounts.
	isPrimary, err := object.IsPrimaryCustomPropertyKey(owner, key)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !isPrimary {
		c.ResponseError(fmt.Sprintf(c.T("user:The custom property: %s is not configured as a primary key for lookup"), key))
		return
	}

	users, err := object.GetUsersByCustomProperty(owner, key, value)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	type match struct {
		Owner            string                            `json:"owner"`
		Name             string                            `json:"name"`
		Id               string                            `json:"id"`
		DisplayName      string                            `json:"displayName"`
		MatchedAt        string                            `json:"matchedAt"`
		CustomProperties map[string]*object.CustomProperty `json:"customProperties"`
	}

	matches := []*match{}
	for _, user := range users {
		matches = append(matches, &match{
			Owner:            user.Owner,
			Name:             user.Name,
			Id:               user.Id,
			DisplayName:      user.DisplayName,
			MatchedAt:        user.CustomProperties[key].UpdatedTime,
			CustomProperties: user.CustomProperties,
		})
	}

	c.ResponseOk(matches, len(matches))
}
