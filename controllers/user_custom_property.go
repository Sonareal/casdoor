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
	"net"
	"net/http"
	"strings"

	"github.com/casdoor/casdoor/object"
)

// clientIpForIpWhitelist resolves the address to match against an IP whitelist.
//
// It deliberately does NOT reuse util.GetClientIpFromRequest, which takes the
// FIRST entry of X-Forwarded-For. The reverse proxy in front of Casdoor is
// configured with $proxy_add_x_forwarded_for, which APPENDS the real peer to
// whatever the caller sent — so the first entry is caller-controlled, and a
// request carrying "X-Forwarded-For: 10.0.0.1" would pass any whitelist. That
// is fine for a log line; it is not fine for an access control.
//
// The last entry is the one our own proxy appended, so it is the peer as the
// proxy saw it. This assumes a single trusted proxy hop, which matches the
// nginx -> casdoor deployment; adding another hop in front would need this to
// count back further.
func clientIpForIpWhitelist(req *http.Request) string {
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if host, _, err := net.SplitHostPort(last); err == nil {
			last = host
		}
		if ip := strings.Trim(last, "[]"); ip != "" {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return strings.Trim(req.RemoteAddr, "[]")
	}
	return strings.Trim(host, "[]")
}

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

	// Accept the account UUID too, since that is what the reverse lookup shows
	// most prominently; resolving it here saves the caller from having to
	// rebuild an "owner/name" string.
	if id == "" {
		if uuid := c.Ctx.Input.Query("userId"); uuid != "" {
			owner := c.Ctx.Input.Query("owner")
			if owner == "" {
				c.ResponseError(c.T("general:Missing parameter") + ": owner")
				return nil, false
			}
			target, err := object.GetUserByUserId(owner, uuid)
			if err != nil {
				c.ResponseError(err.Error())
				return nil, false
			}
			if target == nil {
				c.ResponseError(fmt.Sprintf(c.T("general:The user: %s doesn't exist"), owner+"/"+uuid))
				return nil, false
			}
			id = target.GetId()
		}
	}

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

	// Multi-value attributes append by default; replace=true hands the whole
	// list over to the caller instead.
	replace := c.Ctx.Input.Query("replace") == "true"
	properties, err := object.SetUserCustomProperties(user, updates, replace, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(properties)
}

// GetUsersByCustomProperty
// @Title GetUsersByCustomProperty
// @Tag User API
// @Description look up users whose custom property holds a value, most recently
// @Description updated first. Searches the organizations that whitelist the caller.
// @Param   owner   query   string  false  "Narrow to one organization; must be one the caller is allowed in"
// @Param   key     query   string  true   "The custom property name, e.g. deviceId"
// @Param   value   query   string  true   "The value to match exactly"
// @Success 200 {object} controllers.Response The Response object
// @router /get-users-by-custom-property [get]
func (c *ApiController) GetUsersByCustomProperty() {
	// Permission follows the organization: each one lists the callers allowed
	// to resolve values inside it. A caller's reach is therefore the union of
	// the organizations that named it — bounded by construction, rather than
	// "everything, because the token is admin".
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}

	// owner is optional: empty means every organization the caller is allowed in.
	owner := c.Ctx.Input.Query("owner")
	key := c.Ctx.Input.Query("key")
	value := c.Ctx.Input.Query("value")
	if key == "" || value == "" {
		c.ResponseError(c.T("general:Missing parameter"))
		return
	}

	clientIp := clientIpForIpWhitelist(c.Ctx.Request)
	scope, err := object.ResolveCustomPropertyLookupScope(c.GetSessionUsername(), clientIp, key, owner, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	users, err := object.GetUsersByCustomProperty(scope, key, value)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	type match struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
		// The identifier to pass straight back to ?id= when writing. "id" below
		// is the account UUID, which reads like the thing to use but is not —
		// spelling both out saves a round of guessing.
		UserId           string                            `json:"userId"`
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
			UserId:           user.GetId(),
			Id:               user.Id,
			DisplayName:      user.DisplayName,
			MatchedAt:        user.CustomProperties[key].UpdatedTime,
			CustomProperties: user.CustomProperties,
		})
	}

	c.ResponseOk(matches, len(matches))
}
