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
	"strings"
	"testing"
)

func TestValidateCustomPropertyKey(t *testing.T) {
	valid := []string{"deviceId", "device_id", "device.id", "device-id", "a", "a1", "theme2"}
	for _, key := range valid {
		if err := validateCustomPropertyKey(key, "en"); err != nil {
			t.Errorf("key %q should be valid, got %v", key, err)
		}
	}

	invalid := []string{
		"",           // empty
		"_leading",   // must start alphanumeric
		"-leading",   //
		".leading",   //
		"has space",  // would be awkward in URLs and logs
		"has/slash",  //
		"has\"quote", // would need JSON escaping in the lookup
		"中文键",        // keep keys ASCII so the reverse lookup stays predictable
		strings.Repeat("a", MaxCustomPropertyKeyLength+1),
	}
	for _, key := range invalid {
		if err := validateCustomPropertyKey(key, "en"); err == nil {
			t.Errorf("key %q should be rejected", key)
		}
	}
}

// The LIKE prefilter runs on a user-supplied value; a value containing a
// wildcard must not widen it to the whole table.
func TestEscapeLikePattern(t *testing.T) {
	cases := map[string]string{
		"abc":      "abc",
		"a%b":      `a\%b`,
		"a_b":      `a\_b`,
		`a\b`:      `a\\b`,
		"%":        `\%`,
		"100%_off": `100\%\_off`,
	}
	for in, want := range cases {
		if got := escapeLikePattern(in); got != want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCustomPropertyLimits(t *testing.T) {
	if MaxCustomPropertyCount <= 0 || MaxCustomPropertyKeyLength <= 0 || MaxCustomPropertyValueLength <= 0 {
		t.Fatal("the limits must be positive; they are the only thing bounding what a user can store")
	}
	if MaxCustomPropertyValueLength < 64 {
		t.Error("the value limit is too small to hold a device id")
	}
}

// checkCustomPropertyValue is the organization-schema gate on writes.
func TestCheckCustomPropertyValueDeclaration(t *testing.T) {
	items := []*CustomPropertyItem{
		{Name: "deviceId"},
		{Name: "theme"},
	}

	for _, key := range []string{"deviceId", "theme"} {
		if err := checkCustomPropertyValue(items, key, "anything", "en"); err != nil {
			t.Errorf("declared key %q was rejected: %v", key, err)
		}
	}
	if err := checkCustomPropertyValue(items, "isIdCardVerified", "true", "en"); err == nil {
		t.Error("an undeclared key must be rejected once the organization declares a schema")
	}

	// No schema means the organization has not opted in yet; existing
	// deployments must keep working rather than start rejecting every write.
	if err := checkCustomPropertyValue(nil, "anything", "x", "en"); err != nil {
		t.Errorf("with no declared items anything valid should pass, got %v", err)
	}
}

// An enumerated attribute that is not enforced ends up holding "zh", "zh-CN"
// and "中文" for the same thing.
func TestCheckCustomPropertyValueAllowedValues(t *testing.T) {
	items := []*CustomPropertyItem{
		{Name: "language", AllowedValues: "zh,en"},
		{Name: "mode", AllowedValues: "youth, child"},
		{Name: "deviceId"}, // free text
	}

	accepted := map[string]string{"language": "zh", "mode": "child"}
	for key, value := range accepted {
		if err := checkCustomPropertyValue(items, key, value, "en"); err != nil {
			t.Errorf("%s=%q is allowed but was rejected: %v", key, value, err)
		}
	}
	// Spacing in the declaration must not leak into the comparison.
	if err := checkCustomPropertyValue(items, "mode", "youth", "en"); err != nil {
		t.Errorf("a declaration written with spaces must still match: %v", err)
	}

	rejected := []struct{ key, value string }{
		{"language", "中文"},    // the localized label, not the value
		{"mode", "儿童"},        //
		{"language", "zh-CN"}, // a near miss
		{"language", "ZH"},    // matching is exact, not case-insensitive
		{"language", ""},      // empty is not a member of the set
		{"mode", "adult"},     // plausible but undeclared
	}
	for _, c := range rejected {
		if err := checkCustomPropertyValue(items, c.key, c.value, "en"); err == nil {
			t.Errorf("%s=%q is not in the allowed set but was accepted", c.key, c.value)
		}
	}

	// An item with no declared values stays free text.
	if err := checkCustomPropertyValue(items, "deviceId", "DEV-ANYTHING-123", "en"); err != nil {
		t.Errorf("an item without allowed values must stay free text, got %v", err)
	}
}

func TestIsCallerWhitelisted(t *testing.T) {
	const whitelist = "app/app-backoffice, built-in/admin"

	for _, caller := range []string{"app/app-backoffice", "built-in/admin"} {
		if !isCallerWhitelisted(whitelist, caller) {
			t.Errorf("caller %q is listed but was not matched", caller)
		}
	}
	for _, caller := range []string{"app/other", "gloopo/alice", "app/app-backoffice2"} {
		if isCallerWhitelisted(whitelist, caller) {
			t.Errorf("caller %q is not listed but matched", caller)
		}
	}
	// An empty caller must never match, including against an empty whitelist —
	// otherwise an unauthenticated request would slip through.
	if isCallerWhitelisted("", "") || isCallerWhitelisted(" , ", "") {
		t.Error("an empty caller must not match")
	}
}

func TestIsIpAllowed(t *testing.T) {
	// No list means the organization did not restrict by IP.
	if !isIpAllowed("", "8.8.8.8") {
		t.Error("an empty IP whitelist must not restrict")
	}

	const cidrs = "192.168.2.0/24, 10.1.0.0/16"
	for _, ip := range []string{"192.168.2.12", "10.1.5.7"} {
		if !isIpAllowed(cidrs, ip) {
			t.Errorf("IP %q is inside the whitelist but was refused", ip)
		}
	}
	for _, ip := range []string{"192.168.3.1", "8.8.8.8", "not-an-ip", ""} {
		if isIpAllowed(cidrs, ip) {
			t.Errorf("IP %q is outside the whitelist but was allowed", ip)
		}
	}

	// A malformed entry must not swallow the valid ones next to it.
	if !isIpAllowed("garbage, 127.0.0.0/8", "127.0.0.1") {
		t.Error("a malformed CIDR must be skipped, not abort the whole check")
	}
}

func TestUserTableNameUsesConfiguredPrefix(t *testing.T) {
	// The compare-and-swap is the one place using raw SQL, so it has to respect
	// tableNamePrefix rather than hardcoding "user".
	t.Setenv("tableNamePrefix", "casdoor_")
	if got := userTableName(); got != "casdoor_user" {
		t.Errorf("userTableName() = %q, want %q", got, "casdoor_user")
	}

	t.Setenv("tableNamePrefix", "")
	if got := userTableName(); got != "user" {
		t.Errorf("userTableName() = %q, want %q", got, "user")
	}
}
