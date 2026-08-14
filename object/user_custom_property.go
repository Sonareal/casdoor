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
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/casdoor/casdoor/conf"
	"github.com/casdoor/casdoor/i18n"
	"github.com/casdoor/casdoor/util"
)

// Device-scoped application attributes: an account is used on several devices,
// and each device carries its own values — the same account can be in Chinese
// on the tablet and English on the phone.
//
// They live in their own column rather than in User.Properties. Properties is
// shared with values the server owns: OAuth access tokens, isIdCardVerified,
// and fields written by the LDAP/Okta syncers. Letting a user write into that
// map would let them forge their own identity-verification state. The boundary
// is structural, not a naming convention.
//
// Shape:
//
//	customProperties = {
//	  "<device>": { "<key>": {"value": ..., "updatedTime": ...}, ... },
//	  ...
//	}

// CustomProperty is one attribute recorded for an account on one device.
type CustomProperty struct {
	Value string `json:"value"`
	// Stamped when the account's own token writes; left untouched when the back
	// office writes. It answers "when did the user last do this on this device",
	// which is what the back office sorts by — a back-office write refreshing it
	// would make every record it touched look freshly visited.
	UpdatedTime string `json:"updatedTime"`
}

// DeviceProperties are the attributes recorded for one account on one device.
type DeviceProperties map[string]*CustomProperty

// CustomPropertyItem declares one attribute an organization accepts on a
// device. Configured on the organization's page in the admin UI, so adding an
// attribute does not need a code change.
type CustomPropertyItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	// Comma-separated set of accepted values. Empty means free text. Declaring
	// it turns a convention into a constraint: an enumerated attribute that is
	// not enforced ends up holding "zh", "zh-CN" and "中文" for the same thing,
	// and the back office has to guess which the client meant.
	AllowedValues string `json:"allowedValues"`
}

const (
	MaxDevicesPerUser            = 32
	MaxCustomPropertyCount       = 32
	MaxCustomPropertyKeyLength   = 64
	MaxCustomPropertyValueLength = 512
)

// Keys and device identifiers are restricted so they stay safe in URLs, log
// lines and JSON paths, and so a lookup cannot be steered by a crafted value.
var (
	reCustomPropertyKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	reDeviceId          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)
)

func validateCustomPropertyKey(key string, lang string) error {
	if len(key) > MaxCustomPropertyKeyLength {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property key is too long (maximum is %d characters)"), MaxCustomPropertyKeyLength)
	}
	if !reCustomPropertyKey.MatchString(key) {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property key: %s is invalid, it may only contain letters, digits, and the characters _ . -"), key)
	}
	return nil
}

// ValidateDeviceId checks the device identifier a caller addresses. MAC
// addresses are the expected form, so ":" is allowed on top of the key charset.
func ValidateDeviceId(device string, lang string) error {
	if device == "" {
		return errors.New(i18n.Translate(lang, "user:The device identifier is required"))
	}
	if len(device) > MaxCustomPropertyKeyLength {
		return fmt.Errorf(i18n.Translate(lang, "user:The custom property key is too long (maximum is %d characters)"), MaxCustomPropertyKeyLength)
	}
	if !reDeviceId.MatchString(device) {
		return fmt.Errorf(i18n.Translate(lang, "user:The device identifier: %s is invalid, it may only contain letters, digits, and the characters _ . : -"), device)
	}
	return nil
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

func findCustomPropertyItem(items []*CustomPropertyItem, key string) *CustomPropertyItem {
	for _, item := range items {
		if item != nil && item.Name == key {
			return item
		}
	}
	return nil
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

	declared := findCustomPropertyItem(items, key)
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

// userTableName is the user table with the configured prefix applied, for the
// one place that needs raw SQL.
func userTableName() string {
	return conf.GetConfigString("tableNamePrefix") + "user"
}

// maxCustomPropertyWriteAttempts bounds the compare-and-swap retry. Contention
// on one account is between that user's app and a back-office call, so a
// handful of attempts is far more than enough; the bound exists so a
// pathological case fails loudly instead of spinning.
const maxCustomPropertyWriteAttempts = 8

// SetUserDeviceProperties merges updates into one device's attributes on one
// account. A nil value deletes the key; deleting the last key drops the device.
//
// refreshTime distinguishes the two callers. The account's own token is
// recording activity, so it stamps the current time. The back office is
// correcting data, so it leaves the existing timestamp alone — otherwise every
// record it touched would look freshly visited, and the back office sorts by
// exactly that.
//
// All devices share one column, so any write rewrites the whole structure.
// Merging onto whatever the caller loaded earlier would let two concurrent
// writers drop each other's changes. So the write is a compare-and-swap:
// re-read the column, merge onto that, and update only if the stored value is
// still what was read, retrying against the winner otherwise. Done in SQL
// rather than with SELECT ... FOR UPDATE so the guarantee does not depend on
// the database supporting row locks — SQLite, used for local testing, does not.
func SetUserDeviceProperties(user *User, device string, updates map[string]*string, refreshTime bool, lang string) (map[string]DeviceProperties, error) {
	if user == nil {
		return nil, errors.New("the user is nil")
	}
	if err := ValidateDeviceId(device, lang); err != nil {
		return nil, err
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

		merged, err := decodeCustomProperties(stored)
		if err != nil {
			return nil, err
		}

		properties := DeviceProperties{}
		for key, property := range merged[device] {
			properties[key] = property
		}

		now := util.GetCurrentTime()
		for key, value := range updates {
			key = strings.TrimSpace(key)
			if value == nil {
				delete(properties, key)
				continue
			}

			// A back-office write keeps whatever timestamp is already there, and
			// leaves it empty when creating the key: the field means "last seen
			// from the user", and the back office has not seen anything.
			updatedTime := ""
			if previous, ok := properties[key]; ok && previous != nil {
				updatedTime = previous.UpdatedTime
			}
			if refreshTime {
				updatedTime = now
			}

			properties[key] = &CustomProperty{Value: *value, UpdatedTime: updatedTime}
		}

		if len(properties) > MaxCustomPropertyCount {
			return nil, fmt.Errorf(i18n.Translate(lang, "user:A user may have at most %d custom properties"), MaxCustomPropertyCount)
		}

		if len(properties) == 0 {
			delete(merged, device)
		} else {
			merged[device] = properties
		}

		if len(merged) > MaxDevicesPerUser {
			return nil, fmt.Errorf(i18n.Translate(lang, "user:A user may have at most %d devices"), MaxDevicesPerUser)
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

	return nil, errors.New(i18n.Translate(lang, "user:The custom properties are being updated too frequently, please retry"))
}

// decodeCustomProperties parses the stored column. A column holding the literal
// "null" (an empty map serialized before anything was written) or an empty
// string is an empty structure rather than an error.
func decodeCustomProperties(stored string) (map[string]DeviceProperties, error) {
	merged := map[string]DeviceProperties{}
	stored = strings.TrimSpace(stored)
	if stored == "" || stored == "null" {
		return merged, nil
	}
	if err := json.Unmarshal([]byte(stored), &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		merged = map[string]DeviceProperties{}
	}
	return merged, nil
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
// resolve devices inside it, so a caller's reach is exactly the union of the
// organizations that named it — never "everything, because the token is admin".
// A lookup with no owner is still global, but global means "every organization
// that granted this caller access", a bounded set the operator chose one org at
// a time.
//
// An empty result is always an error: with nothing configured the endpoint
// stays closed rather than open.
func ResolveCustomPropertyLookupScope(callerId string, clientIp string, owner string, lang string) ([]string, error) {
	organizations := []*Organization{}
	if err := ormer.Engine.Find(&organizations); err != nil {
		return nil, err
	}

	var (
		whitelistedAnywhere bool
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
			continue
		}
		scope = append(scope, organization.Name)
	}

	if len(scope) > 0 {
		sort.Strings(scope)
		return scope, nil
	}

	// Distinguish the reasons: "you have no access" and "your address is not
	// allowed" send an operator to very different places.
	if !whitelistedAnywhere {
		return nil, fmt.Errorf(i18n.Translate(lang, "user:The caller: %s is not allowed to look up users by custom property"), callerId)
	}
	return nil, fmt.Errorf(i18n.Translate(lang, "user:The client IP: %s is not in the lookup IP whitelist"), clientIp)
}

// DeviceMatch is one account that has recorded the searched device.
type DeviceMatch struct {
	Owner            string           `json:"owner"`
	Name             string           `json:"name"`
	UserId           string           `json:"userId"`
	Id               string           `json:"id"`
	DisplayName      string           `json:"displayName"`
	MatchedAt        string           `json:"matchedAt"`
	DeviceProperties DeviceProperties `json:"deviceProperties"`
}

// GetUsersByDevice finds the accounts that have recorded `device`, most
// recently active on it first.
//
// owners is the set of organizations to search, as resolved by
// ResolveCustomPropertyLookupScope — never caller-supplied directly, so a
// caller cannot widen its own reach.
//
// A device legitimately belongs to several accounts — successive logins on one
// phone — so this returns all of them and lets the caller decide, rather than
// guessing which is current.
func GetUsersByDevice(owners []string, device string) ([]*DeviceMatch, error) {
	if device == "" {
		return nil, errors.New("the device identifier should not be empty")
	}
	if len(owners) == 0 {
		return []*DeviceMatch{}, nil
	}

	users := []*User{}
	// Narrow the scan in SQL before verifying in Go. The column is JSON text, so
	// LIKE can only prefilter: it may match a device name appearing elsewhere in
	// the document, and cannot account for JSON escaping. The exact map lookup
	// below is what decides.
	err := ormer.Engine.
		Where("custom_properties LIKE ?", "%"+escapeLikePattern(device)+"%").
		In("owner", owners).
		Find(&users)
	if err != nil {
		return nil, err
	}

	matches := []*DeviceMatch{}
	for _, user := range users {
		properties, ok := user.CustomProperties[device]
		if !ok || len(properties) == 0 {
			continue
		}

		matches = append(matches, &DeviceMatch{
			Owner:            user.Owner,
			Name:             user.Name,
			UserId:           user.GetId(),
			Id:               user.Id,
			DisplayName:      user.DisplayName,
			MatchedAt:        latestUpdatedTime(properties),
			DeviceProperties: properties,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].MatchedAt > matches[j].MatchedAt
	})

	return matches, nil
}

// latestUpdatedTime is the most recent user activity across a device's
// attributes. Back-office writes leave the field empty, so a device only ever
// touched by the back office sorts last rather than looking freshly visited.
func latestUpdatedTime(properties DeviceProperties) string {
	latest := ""
	for _, property := range properties {
		if property != nil && property.UpdatedTime > latest {
			latest = property.UpdatedTime
		}
	}
	return latest
}

// escapeLikePattern neutralises the LIKE wildcards in a caller-supplied value
// so that a value containing "%" cannot widen the prefilter to the whole table.
func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
