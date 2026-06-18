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

	"github.com/casdoor/casdoor/util"
)

const gcOrg = "test-gc-org"

func TestGiftCardRedeem(t *testing.T) {
	createDatabase = false
	InitConfig()

	ormer.Engine.Where("owner = ?", gcOrg).Delete(&User{})
	ormer.Engine.Where("owner = ?", gcOrg).Delete(&GiftCard{})
	ormer.Engine.Where("owner = ?", gcOrg).Delete(&Subscription{})
	ormer.Engine.Where("owner = ?", gcOrg).Delete(&Plan{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", gcOrg).Delete(&Organization{})

	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: gcOrg, DisplayName: gcOrg, CreatedTime: now}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := ormer.Engine.Insert(&User{Owner: gcOrg, Name: "u1", Id: util.GenerateId(), CreatedTime: now}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := ormer.Engine.Insert(&Plan{Owner: gcOrg, Name: "pro-yearly", CreatedTime: now, Period: PeriodYearly, Role: "pro", Product: "prod-pro", IsEnabled: true}); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	// batch generate 3 cards bound to the yearly plan
	cards, err := GenerateGiftCards(gcOrg, "batch1", "pro-yearly", "pro", "prod-pro", 3, "en")
	if err != nil || len(cards) != 3 {
		t.Fatalf("generate: err=%v n=%d", err, len(cards))
	}
	code := cards[0].Code
	if len(code) != 16 {
		t.Fatalf("code length: want 16, got %d (%s)", len(code), code)
	}
	for _, ch := range code {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
			t.Fatalf("code not uppercase-alnum: %s", code)
		}
	}

	// redeem -> Active subscription matching the plan period
	sub, err := RedeemGiftCard(gcOrg, code, "u1", "en")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if sub.State != SubStateActive || sub.Plan != "pro-yearly" || sub.Period != PeriodYearly || sub.User != "u1" || sub.Payment != "" {
		t.Fatalf("subscription wrong: %+v", sub)
	}

	// card marked Used + bound to user + linked to subscription
	gc, _ := getGiftCardByCode(gcOrg, code)
	if gc.State != GiftCardStateUsed || gc.UsedBy != "u1" || gc.Subscription != sub.Name {
		t.Fatalf("card not properly used: %+v", gc)
	}

	// double redeem must fail (one-time)
	if _, err := RedeemGiftCard(gcOrg, code, "u1", "en"); err == nil {
		t.Fatalf("double redeem should fail")
	}

	// unknown code must fail
	if _, err := RedeemGiftCard(gcOrg, "THISCODEDOESNOTEXIST000000000000", "u1", "en"); err == nil {
		t.Fatalf("unknown code should fail")
	}

	// the user now has exactly one active subscription for the plan
	subs, _ := GetSubscriptionsByUser(gcOrg, "u1")
	active := 0
	for _, s := range subs {
		if s.State == SubStateActive && s.Plan == "pro-yearly" {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("want 1 active subscription, got %d", active)
	}
}
