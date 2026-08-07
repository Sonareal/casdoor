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
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/casdoor/casdoor/i18n"
	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

// Custom properties are application-defined key/value pairs a user may set on
// themselves with their own token — a device id, a preference, whatever the
// product needs.
//
// They live in their own column rather than in User.Properties on purpose.
// Properties is shared with values the server owns: OAuth access tokens,
// isIdCardVerified, and fields written by the LDAP/Okta syncers. Letting a user
// write into that map would let them forge their own identity-verification
// state. The boundary here is structural, not a naming convention.

// CustomProperty is one user-set attribute. UpdatedTime is stamped by the
// server on every write so that a reverse lookup can tell which of several
// accounts claimed a value most recently.
type CustomProperty struct {
	Value       string `json:"value"`
	UpdatedTime string `json:"updatedTime"`
}

// CustomPropertyItem declares one attribute an organization accepts. Configured
// on the organization's page in the admin UI, so adding an attribute does not
// need a code change.
//
// IsPrimary marks the one attribute the reverse lookup may search by. Without
// it any attribute would be searchable, which would turn every stored value —
// a theme, an app version — into a way to enumerate accounts. Keeping the
// searchable surface to a declared key is the point.
type CustomPropertyItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	IsPrimary   bool   `json:"isPrimary"`
}

const (
	MaxCustomPropertyCount       = 32
	MaxCustomPropertyKeyLength   = 64
	MaxCustomPropertyValueLength = 512
)

// Keys are restricted so they stay safe to use in URLs, log lines and JSON
// paths, and so that a reverse lookup query cannot be steered by a crafted key.
var reCustomPropertyKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func validateCustomPropertyKey(key string, lang string) error {
	if len(key) > MaxCustomPropertyKeyLength {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property key is too long (maximum is %d characters)"), MaxCustomPropertyKeyLength)
	}
	if !reCustomPropertyKey.MatchString(key) {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property key: %s is invalid, it may only contain letters, digits, and the characters _ . -"), key)
	}
	return nil
}

// SetUserCustomProperties merges updates into the user's custom properties and
// persists them. A nil value deletes the key, which is how a client removes an
// attribute without a separate endpoint.
//
// Returns the resulting property map.
func SetUserCustomProperties(user *User, updates map[string]*string, lang string) (map[string]*CustomProperty, error) {
	if user == nil {
		return nil, fmt.Errorf("the user is nil")
	}

	merged := map[string]*CustomProperty{}
	for key, property := range user.CustomProperties {
		merged[key] = property
	}

	items, err := GetCustomPropertyItems(user.Owner)
	if err != nil {
		return nil, err
	}

	now := util.GetCurrentTime()
	for key, value := range updates {
		key = strings.TrimSpace(key)
		if err := validateCustomPropertyKey(key, lang); err != nil {
			return nil, err
		}
		// Deleting a key the organization no longer declares must stay
		// possible, otherwise removing an item from the schema would strand
		// the values already stored under it.
		if value != nil {
			if err := checkCustomPropertyDeclared(items, key, lang); err != nil {
				return nil, err
			}
		}

		if value == nil {
			delete(merged, key)
			continue
		}
		if len(*value) > MaxCustomPropertyValueLength {
			return nil, fmt.Errorf(i18n.Translate(lang, "user:The value of the custom property: %s is too long (maximum is %d characters)"), key, MaxCustomPropertyValueLength)
		}

		merged[key] = &CustomProperty{Value: *value, UpdatedTime: now}
	}

	if len(merged) > MaxCustomPropertyCount {
		return nil, fmt.Errorf(i18n.Translate(lang, "user:A user may have at most %d custom properties"), MaxCustomPropertyCount)
	}

	user.CustomProperties = merged
	if _, err := ormer.Engine.ID(core.PK{user.Owner, user.Name}).
		Cols("custom_properties").Update(user); err != nil {
		return nil, err
	}

	return merged, nil
}

// GetCustomPropertyItems returns the attribute schema an organization declares.
// An empty result means the organization has not declared one, in which case
// any syntactically valid key is accepted.
func GetCustomPropertyItems(owner string) ([]*CustomPropertyItem, error) {
	organization, err := getOrganization("admin", owner)
	if err != nil {
		return nil, err
	}
	if organization == nil {
		return nil, nil
	}
	return organization.CustomPropertyItems, nil
}

// checkCustomPropertyDeclared enforces the organization's schema on a write.
// An organization with no declared items accepts anything valid, which keeps
// existing deployments working until someone fills the table in.
func checkCustomPropertyDeclared(items []*CustomPropertyItem, key string, lang string) error {
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item != nil && item.Name == key {
			return nil
		}
	}
	return fmt.Errorf(i18n.Translate(lang, "user:The custom property: %s is not declared in the organization settings"), key)
}

// IsPrimaryCustomPropertyKey reports whether key is the searchable attribute of
// any organization — or of `owner` specifically when one is given.
//
// Restricting the reverse lookup to declared primary keys keeps the searchable
// surface to what an operator chose. Otherwise every stored attribute, down to
// a UI theme, would double as a way to enumerate accounts.
func IsPrimaryCustomPropertyKey(owner string, key string) (bool, error) {
	organizations := []*Organization{}
	session := ormer.Engine.NewSession()
	defer session.Close()
	if owner != "" {
		session = session.Where("name = ?", owner)
	}
	if err := session.Find(&organizations); err != nil {
		return false, err
	}

	for _, organization := range organizations {
		for _, item := range organization.CustomPropertyItems {
			if item != nil && item.IsPrimary && item.Name == key {
				return true, nil
			}
		}
	}
	return false, nil
}

// getLookupSettings reads the reverse-lookup access control. It lives on the
// built-in organization rather than in app.conf so that an operator can change
// it from the admin UI and have it take effect immediately, with no file edit
// and no container restart.
func getLookupSettings() (whitelist string, ipWhitelist string, err error) {
	organization, err := getOrganization("admin", "built-in")
	if err != nil {
		return "", "", err
	}
	if organization == nil {
		return "", "", nil
	}
	return strings.TrimSpace(organization.CustomPropertyLookupWhitelist),
		strings.TrimSpace(organization.CustomPropertyLookupIpWhitelist), nil
}

// CheckCustomPropertyLookupAllowed gates the reverse lookup.
//
// The lookup spans every organization, so it can turn an opaque value such as a
// device id into an account in any product line. That is too much reach to hand
// out on the strength of an admin token alone, so access is deny-by-default:
// with no whitelist configured the endpoint is off entirely, rather than open
// to every admin.
//
// Both lists are configured on the built-in organization in the admin UI.
func CheckCustomPropertyLookupAllowed(callerId string, clientIp string, lang string) error {
	whitelist, ipWhitelist, err := getLookupSettings()
	if err != nil {
		return err
	}

	if whitelist == "" {
		return errors.New(i18n.Translate(lang, "user:The custom property lookup is not enabled; configure it on the built-in organization"))
	}

	allowed := false
	for _, candidate := range strings.Split(whitelist, ",") {
		if strings.TrimSpace(candidate) == callerId {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf(i18n.Translate(lang, "user:The caller: %s is not allowed to look up users by custom property"), callerId)
	}

	if ipWhitelist == "" {
		return nil
	}

	entryIp := net.ParseIP(clientIp)
	if entryIp == nil {
		return fmt.Errorf(i18n.Translate(lang, "check:Failed to parse client IP: %s"), clientIp)
	}
	for _, cidr := range strings.Split(ipWhitelist, ",") {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			return err
		}
		if ipNet != nil && ipNet.Contains(entryIp) {
			return nil
		}
	}

	return fmt.Errorf(i18n.Translate(lang, "user:The client IP: %s is not in the lookup IP whitelist"), clientIp)
}

// GetUsersByCustomProperty finds the users whose custom property `key` holds
// `value`, most recently updated first.
//
// owner narrows the search to one organization; empty searches every one. The
// global form is what a back office needs when it only has a device id and does
// not yet know which product line it belongs to — which is also why callers are
// gated by CheckCustomPropertyLookupAllowed.
//
// A value may legitimately belong to several accounts — the same device used by
// more than one login — so this returns all of them, newest claim first, and
// lets the caller pick rather than guessing here.
func GetUsersByCustomProperty(owner string, key string, value string) ([]*User, error) {
	if key == "" {
		return nil, fmt.Errorf("the key should not be empty")
	}

	users := []*User{}

	// Narrow the scan in SQL before verifying in Go. The column is JSON text,
	// so LIKE can only prefilter: it may match a value belonging to a different
	// key, and it cannot account for JSON escaping. The exact check below is
	// what decides. Works on every supported database; if this ever becomes a
	// bottleneck, PostgreSQL can index it with
	//   CREATE INDEX ... USING gin ((custom_properties::jsonb) jsonb_path_ops)
	// and match with the @> containment operator instead.
	session := ormer.Engine.Where("custom_properties LIKE ?", "%"+escapeLikePattern(value)+"%")
	if owner != "" {
		session = session.And("owner = ?", owner)
	}

	err := session.Find(&users)
	if err != nil {
		return nil, err
	}

	matched := []*User{}
	for _, user := range users {
		if property, ok := user.CustomProperties[key]; ok && property != nil && property.Value == value {
			matched = append(matched, user)
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].CustomProperties[key].UpdatedTime > matched[j].CustomProperties[key].UpdatedTime
	})

	return matched, nil
}

// escapeLikePattern neutralises the LIKE wildcards in a user-supplied value so
// that a value containing "%" cannot widen the prefilter to the whole table.
func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
