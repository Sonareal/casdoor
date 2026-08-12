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
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/casdoor/casdoor/conf"
	"github.com/casdoor/casdoor/i18n"
	"github.com/casdoor/casdoor/util"
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
	// Comma-separated set of accepted values. Empty means free text. Declaring
	// it turns a convention into a constraint: an enumerated attribute that is
	// not enforced ends up holding "zh", "zh-CN" and "中文" for the same thing,
	// and the back office has to guess which the client meant.
	AllowedValues string `json:"allowedValues"`
	IsPrimary     bool   `json:"isPrimary"`
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

// userTableName is the user table with the configured prefix applied, for the
// one place that needs raw SQL.
func userTableName() string {
	return conf.GetConfigString("tableNamePrefix") + "user"
}

// maxCustomPropertyWriteAttempts bounds the compare-and-swap retry. Contention
// on one account is between that user's app and a back-office call, so a
// handful of attempts is far more than enough; the bound exists so a pathological
// case fails loudly instead of spinning.
const maxCustomPropertyWriteAttempts = 8

// SetUserCustomProperties merges updates into the user's custom properties and
// persists them. A nil value deletes the key, which is how a client removes an
// attribute without a separate endpoint.
//
// Custom properties share one column, so any write rewrites all of them. Merging
// onto whatever the caller loaded earlier would let two concurrent writers drop
// each other's changes — the back office setting a MAC address while the app
// sets a language, and one of the two silently vanishing.
//
// So the write is a compare-and-swap: re-read the column, merge onto that, and
// update only if the stored value is still what we read. A racing writer that
// got there first makes the swap affect zero rows, and we retry against its
// result. This is done in SQL rather than with SELECT ... FOR UPDATE so the
// guarantee does not depend on the database supporting row locks — SQLite, used
// for local testing, does not.
//
// Returns the resulting property map.
func SetUserCustomProperties(user *User, updates map[string]*string, lang string) (map[string]*CustomProperty, error) {
	if user == nil {
		return nil, fmt.Errorf("the user is nil")
	}

	// Validate before touching the database: a bad value should be rejected the
	// same way regardless of contention.
	items, err := GetCustomPropertyItems(user.Owner)
	if err != nil {
		return nil, err
	}
	for key, value := range updates {
		key = strings.TrimSpace(key)
		if err := validateCustomPropertyKey(key, lang); err != nil {
			return nil, err
		}
		if value == nil {
			// Deleting a key the organization no longer declares must stay
			// possible, otherwise removing an item from the schema would strand
			// the values already stored under it.
			continue
		}
		if len(*value) > MaxCustomPropertyValueLength {
			return nil, fmt.Errorf(i18n.Translate(lang, "user:The value of the custom property: %s is too long (maximum is %d characters)"), key, MaxCustomPropertyValueLength)
		}
		if err := checkCustomPropertyValue(items, key, *value, lang); err != nil {
			return nil, err
		}
	}

	for attempt := 0; attempt < maxCustomPropertyWriteAttempts; attempt++ {
		stored, err := readRawCustomProperties(user.Owner, user.Name)
		if err != nil {
			return nil, err
		}

		current := map[string]*CustomProperty{}
		if strings.TrimSpace(stored) != "" {
			if err := json.Unmarshal([]byte(stored), &current); err != nil {
				return nil, err
			}
		}

		merged := map[string]*CustomProperty{}
		for key, property := range current {
			merged[key] = property
		}

		now := util.GetCurrentTime()
		for key, value := range updates {
			key = strings.TrimSpace(key)
			if value == nil {
				delete(merged, key)
				continue
			}
			merged[key] = &CustomProperty{Value: *value, UpdatedTime: now}
		}

		if len(merged) > MaxCustomPropertyCount {
			return nil, fmt.Errorf(i18n.Translate(lang, "user:A user may have at most %d custom properties"), MaxCustomPropertyCount)
		}

		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}

		swapped, err := swapCustomProperties(user.Owner, user.Name, stored, string(encoded))
		if err != nil {
			return nil, err
		}
		if swapped {
			user.CustomProperties = merged
			return merged, nil
		}
		// Someone else committed between the read and the swap; retry against
		// their result rather than overwriting it.
	}

	return nil, fmt.Errorf(i18n.Translate(lang, "user:The custom properties are being updated too frequently, please retry"))
}

// readRawCustomProperties reads the column as stored, so the compare in
// swapCustomProperties is against the exact bytes rather than a re-encoding
// (map key order is not stable, so a round-trip would not compare equal).
func readRawCustomProperties(owner string, name string) (string, error) {
	results, err := ormer.Engine.QueryString(
		"SELECT custom_properties FROM "+userTableName()+" WHERE owner = ? AND name = ?", owner, name)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", fmt.Errorf("the user: %s/%s is not found", owner, name)
	}
	return results[0]["custom_properties"], nil
}

// swapCustomProperties writes newValue only if the column still holds oldValue.
// Reports whether the swap happened.
func swapCustomProperties(owner string, name string, oldValue string, newValue string) (bool, error) {
	var (
		result sql.Result
		err    error
	)
	// A NULL column never compares equal with "=", so the two cases need
	// different SQL. A fresh user has NULL here until the first write.
	if oldValue == "" {
		result, err = ormer.Engine.Exec(
			"UPDATE "+userTableName()+" SET custom_properties = ? WHERE owner = ? AND name = ? AND (custom_properties IS NULL OR custom_properties = '')",
			newValue, owner, name)
	} else {
		result, err = ormer.Engine.Exec(
			"UPDATE "+userTableName()+" SET custom_properties = ? WHERE owner = ? AND name = ? AND custom_properties = ?",
			newValue, owner, name, oldValue)
	}
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
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

// checkCustomPropertyValue enforces the organization's schema on a write: the
// key must be declared, and the value must be one the item accepts.
//
// An organization with no declared items accepts anything valid, which keeps
// existing deployments working until someone fills the table in.
func checkCustomPropertyValue(items []*CustomPropertyItem, key string, value string, lang string) error {
	if len(items) == 0 {
		return nil
	}

	var declared *CustomPropertyItem
	for _, item := range items {
		if item != nil && item.Name == key {
			declared = item
			break
		}
	}
	if declared == nil {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property: %s is not declared in the organization settings"), key)
	}

	allowedValues := strings.TrimSpace(declared.AllowedValues)
	if allowedValues == "" {
		return nil
	}
	for _, allowed := range strings.Split(allowedValues, ",") {
		if strings.TrimSpace(allowed) == value {
			return nil
		}
	}
	return fmt.Errorf(i18n.Translate(lang, "user:The value of the custom property: %s must be one of: %s"), key, allowedValues)
}

func isCallerWhitelisted(whitelist string, callerId string) bool {
	for _, candidate := range strings.Split(whitelist, ",") {
		if strings.TrimSpace(candidate) == callerId && callerId != "" {
			return true
		}
	}
	return false
}

func isIpAllowed(ipWhitelist string, clientIp string) bool {
	ipWhitelist = strings.TrimSpace(ipWhitelist)
	if ipWhitelist == "" {
		return true
	}

	entryIp := net.ParseIP(clientIp)
	if entryIp == nil {
		return false
	}
	for _, cidr := range strings.Split(ipWhitelist, ",") {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil || ipNet == nil {
			continue
		}
		if ipNet.Contains(entryIp) {
			return true
		}
	}
	return false
}

// ResolveCustomPropertyLookupScope works out which organizations a reverse
// lookup may actually touch.
//
// Permission follows the organization: each one lists the callers allowed to
// resolve values inside it, so a caller's reach is exactly the union of the
// organizations that named it — never "everything, because the token is admin".
// A lookup with no owner is still global, but global here means "every
// organization that granted this caller access", which is a bounded set the
// operator chose one org at a time.
//
// An organization is in scope only if it also declares `key` as its primary
// lookup attribute, so a caller cannot search by an attribute that happens to
// exist there but was never meant to be searchable.
//
// Returns the organizations to search. An empty result is always an error: with
// nothing configured the endpoint stays closed rather than open.
func ResolveCustomPropertyLookupScope(callerId string, clientIp string, key string, owner string, lang string) ([]string, error) {
	organizations := []*Organization{}
	if err := ormer.Engine.Find(&organizations); err != nil {
		return nil, err
	}

	var (
		whitelistedAnywhere bool
		blockedByIp         bool
		scope               []string
	)

	for _, organization := range organizations {
		if owner != "" && organization.Name != owner {
			continue
		}
		if !isCallerWhitelisted(organization.CustomPropertyLookupWhitelist, callerId) {
			continue
		}
		whitelistedAnywhere = true

		if !isIpAllowed(organization.CustomPropertyLookupIpWhitelist, clientIp) {
			blockedByIp = true
			continue
		}

		for _, item := range organization.CustomPropertyItems {
			if item != nil && item.IsPrimary && item.Name == key {
				scope = append(scope, organization.Name)
				break
			}
		}
	}

	if len(scope) > 0 {
		sort.Strings(scope)
		return scope, nil
	}

	// Distinguish the reasons: "you have no access" and "that key is not
	// searchable" send an operator to very different places.
	if !whitelistedAnywhere {
		return nil, fmt.Errorf(i18n.Translate(lang, "user:The caller: %s is not allowed to look up users by custom property"), callerId)
	}
	if blockedByIp {
		return nil, fmt.Errorf(i18n.Translate(lang, "user:The client IP: %s is not in the lookup IP whitelist"), clientIp)
	}
	return nil, fmt.Errorf(i18n.Translate(lang, "user:The custom property: %s is not configured as a primary key for lookup"), key)
}

// GetUsersByCustomProperty finds the users whose custom property `key` holds
// `value`, most recently updated first.
//
// owners is the set of organizations to search, as resolved by
// ResolveCustomPropertyLookupScope — never caller-supplied directly, so a
// caller cannot widen its own reach.
//
// A value may legitimately belong to several accounts — the same device used by
// more than one login — so this returns all of them, newest claim first, and
// lets the caller pick rather than guessing here.
func GetUsersByCustomProperty(owners []string, key string, value string) ([]*User, error) {
	if key == "" {
		return nil, fmt.Errorf("the key should not be empty")
	}
	if len(owners) == 0 {
		return []*User{}, nil
	}

	users := []*User{}

	// Narrow the scan in SQL before verifying in Go. The column is JSON text,
	// so LIKE can only prefilter: it may match a value belonging to a different
	// key, and it cannot account for JSON escaping. The exact check below is
	// what decides. Works on every supported database; if this ever becomes a
	// bottleneck, PostgreSQL can index it with
	//   CREATE INDEX ... USING gin ((custom_properties::jsonb) jsonb_path_ops)
	// and match with the @> containment operator instead.
	err := ormer.Engine.
		Where("custom_properties LIKE ?", "%"+escapeLikePattern(value)+"%").
		In("owner", owners).
		Find(&users)
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
