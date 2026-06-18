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

const refTestOrg = "test-ref-org"

func refSeed(t *testing.T) (referrer *User, payer *User, inv *Invitation) {
	// clean slate
	ormer.Engine.Where("owner = ?", refTestOrg).Delete(&User{})
	ormer.Engine.Where("owner = ?", refTestOrg).Delete(&Invitation{})
	ormer.Engine.Where("owner = ?", refTestOrg).Delete(&Transaction{})
	ormer.Engine.Where("owner = ?", refTestOrg).Delete(&ReferralPolicy{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", refTestOrg).Delete(&Organization{})

	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: refTestOrg, DisplayName: refTestOrg, CreatedTime: now, BalanceCurrency: "CNY"}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	referrer = &User{Owner: refTestOrg, Name: "ref1", Id: util.GenerateId(), CreatedTime: now, BalanceCurrency: "CNY"}
	if _, err := ormer.Engine.Insert(referrer); err != nil {
		t.Fatalf("insert referrer: %v", err)
	}
	var err error
	inv, err = EnsurePersonalInvitation(referrer)
	if err != nil {
		t.Fatalf("ensure invitation: %v", err)
	}
	payer = &User{Owner: refTestOrg, Name: "pay1", Id: util.GenerateId(), CreatedTime: now, BalanceCurrency: "CNY", Invitation: inv.Name}
	if _, err := ormer.Engine.Insert(payer); err != nil {
		t.Fatalf("insert payer: %v", err)
	}
	return referrer, payer, inv
}

func refBalance(t *testing.T, name string) float64 {
	u, err := getUser(refTestOrg, name)
	if err != nil || u == nil {
		t.Fatalf("get user %s: %v", name, err)
	}
	return u.Balance
}

func TestReferralCommissionFlow(t *testing.T) {
	createDatabase = false
	InitConfig()

	_, _, inv := refSeed(t)
	if _, err := UpdateReferralPolicy(refTestOrg, &ReferralPolicy{Enabled: true, DefaultRate: 0.2, UpgradeBasis: "paid", MaxRate: 0.5}, "en"); err != nil {
		t.Fatalf("update policy: %v", err)
	}

	order := &Order{Owner: refTestOrg, Name: "order1", Currency: "CNY", User: "pay1", Price: 100}
	payment := &Payment{Owner: refTestOrg, Name: "paytxn1", User: "pay1", Price: 100, Currency: "CNY", Order: "order1"}

	// 1) first order -> 20 commission (0.2 * 100)
	if err := GrantReferralCommission(payment, order, "en"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if got := refBalance(t, "ref1"); got != 20 {
		t.Fatalf("after first order: want balance 20, got %v", got)
	}

	// 2) idempotent on the same payment
	if err := GrantReferralCommission(payment, order, "en"); err != nil {
		t.Fatalf("grant idempotent: %v", err)
	}
	if got := refBalance(t, "ref1"); got != 20 {
		t.Fatalf("idempotency broken: want 20, got %v", got)
	}

	// 3) first-order only: a second payment by the same payer must not pay again
	payment2 := &Payment{Owner: refTestOrg, Name: "paytxn2", User: "pay1", Price: 100, Currency: "CNY", Order: "order1"}
	if err := GrantReferralCommission(payment2, order, "en"); err != nil {
		t.Fatalf("grant second: %v", err)
	}
	if got := refBalance(t, "ref1"); got != 20 {
		t.Fatalf("first-order guard broken: want 20, got %v", got)
	}

	// PaidInviteCount incremented exactly once
	refInv, _ := GetInvitationByInviter(refTestOrg, util.GetId(refTestOrg, "ref1"))
	if refInv == nil || refInv.PaidInviteCount != 1 {
		t.Fatalf("PaidInviteCount: want 1, got %v", refInv)
	}
	_ = inv
}

func TestResolveCommissionRatePrecedence(t *testing.T) {
	createDatabase = false
	InitConfig()
	referrer, _, _ := refSeed(t)

	// default
	if _, err := UpdateReferralPolicy(refTestOrg, &ReferralPolicy{
		Enabled: true, DefaultRate: 0.2, UpgradeBasis: "paid", MaxRate: 0.5,
		GroupRates: map[string]float64{"vip": 0.3},
		Tiers:      []*ReferralTier{{Name: "T1", MinInvites: 1, Rate: 0.25}},
	}, "en"); err != nil {
		t.Fatalf("policy: %v", err)
	}

	rate, src, _ := ResolveCommissionRate(referrer)
	if rate != 0.2 || src != "default" {
		t.Fatalf("default: want 0.2/default, got %v/%v", rate, src)
	}

	// group
	if _, err := SetReferralRate(util.GetId(refTestOrg, "ref1"), -2, "vip", true, "en"); err != nil {
		t.Fatalf("set tier: %v", err)
	}
	referrer, _ = getUser(refTestOrg, "ref1")
	rate, src, _ = ResolveCommissionRate(referrer)
	if rate != 0.3 || src != "group" {
		t.Fatalf("group: want 0.3/group, got %v/%v", rate, src)
	}

	// auto-upgrade beats group (toggle on, count >= tier)
	policy, _ := GetReferralPolicy(refTestOrg)
	policy.AutoUpgradeEnabled = true
	UpdateReferralPolicy(refTestOrg, policy, "en")
	refInv, _ := GetInvitationByInviter(refTestOrg, util.GetId(refTestOrg, "ref1"))
	refInv.PaidInviteCount = 5
	ormer.Engine.ID([]interface{}{refInv.Owner, refInv.Name}).Cols("paid_invite_count").Update(refInv)
	rate, src, _ = ResolveCommissionRate(referrer)
	if rate != 0.25 || src != "autoUpgrade" {
		t.Fatalf("autoUpgrade: want 0.25/autoUpgrade, got %v/%v", rate, src)
	}

	// per-user override beats everything (incl. explicit 0%)
	if _, err := SetReferralRate(util.GetId(refTestOrg, "ref1"), 0.4, "", false, "en"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	rate, src, _ = ResolveCommissionRate(referrer)
	if rate != 0.4 || src != "override" {
		t.Fatalf("override: want 0.4/override, got %v/%v", rate, src)
	}

	// explicit 0% override (rate=0 must be honored, not treated as unset)
	if _, err := SetReferralRate(util.GetId(refTestOrg, "ref1"), 0, "", false, "en"); err != nil {
		t.Fatalf("set 0%%: %v", err)
	}
	rate, src, _ = ResolveCommissionRate(referrer)
	if rate != 0 || src != "override" {
		t.Fatalf("explicit 0%%: want 0/override, got %v/%v", rate, src)
	}

	// total switch off
	policy, _ = GetReferralPolicy(refTestOrg)
	policy.Enabled = false
	UpdateReferralPolicy(refTestOrg, policy, "en")
	rate, src, _ = ResolveCommissionRate(referrer)
	if rate != 0 || src != "disabled" {
		t.Fatalf("disabled: want 0/disabled, got %v/%v", rate, src)
	}
}

func TestWithdrawalFlow(t *testing.T) {
	createDatabase = false
	InitConfig()
	referrer, _, _ := refSeed(t)
	// give the referrer a starting balance
	referrer.Balance = 100
	ormer.Engine.ID([]interface{}{referrer.Owner, referrer.Name}).Cols("balance").Update(referrer)

	// apply 50 (min 10) -> Requested, balance 100 -> 50
	w := &Withdrawal{Owner: refTestOrg, User: "ref1", Amount: 50, Currency: "CNY", Channel: "WechatBalance", PayeeName: "Zhang"}
	if ok, err := ApplyWithdrawal(w, 10, "en"); err != nil || !ok {
		t.Fatalf("apply: ok=%v err=%v", ok, err)
	}
	if got := refBalance(t, "ref1"); got != 50 {
		t.Fatalf("after apply: want 50, got %v", got)
	}

	// below min
	bad := &Withdrawal{Owner: refTestOrg, User: "ref1", Amount: 5, Currency: "CNY", PayeeName: "Zhang"}
	if _, err := ApplyWithdrawal(bad, 10, "en"); err == nil {
		t.Fatalf("below-min should fail")
	}

	// approve -> Approved, then mark paid
	if ok, err := ReviewWithdrawal(w.GetId(), "approve", "admin", "", "en"); err != nil || !ok {
		t.Fatalf("approve: ok=%v err=%v", ok, err)
	}
	if ok, err := MarkWithdrawalPaid(w.GetId(), "paid", "TRANSFER-001", "", "admin", "en"); err != nil || !ok {
		t.Fatalf("mark paid: ok=%v err=%v", ok, err)
	}
	paid, _ := GetWithdrawal(w.GetId())
	if paid.State != WithdrawalStatePaid || paid.ExternalTransferNo != "TRANSFER-001" {
		t.Fatalf("paid state wrong: %+v", paid)
	}
	if got := refBalance(t, "ref1"); got != 50 {
		t.Fatalf("after paid: balance must stay 50, got %v", got)
	}

	// apply 30 then reject -> balance refunded back to 50
	w2 := &Withdrawal{Owner: refTestOrg, User: "ref1", Amount: 30, Currency: "CNY", PayeeName: "Zhang"}
	if ok, err := ApplyWithdrawal(w2, 10, "en"); err != nil || !ok {
		t.Fatalf("apply2: ok=%v err=%v", ok, err)
	}
	if got := refBalance(t, "ref1"); got != 20 {
		t.Fatalf("after apply2: want 20, got %v", got)
	}
	if ok, err := ReviewWithdrawal(w2.GetId(), "reject", "admin", "bad payee", "en"); err != nil || !ok {
		t.Fatalf("reject: ok=%v err=%v", ok, err)
	}
	if got := refBalance(t, "ref1"); got != 50 {
		t.Fatalf("after reject refund: want 50, got %v", got)
	}
}

func TestReferralSelfReferralGuard(t *testing.T) {
	createDatabase = false
	InitConfig()
	referrer, _, inv := refSeed(t)
	UpdateReferralPolicy(refTestOrg, &ReferralPolicy{Enabled: true, DefaultRate: 0.2, MaxRate: 0.5}, "en")

	// make the referrer their own invitee (self-referral)
	referrer.Invitation = inv.Name
	ormer.Engine.ID([]interface{}{referrer.Owner, referrer.Name}).Cols("invitation").Update(referrer)

	order := &Order{Owner: refTestOrg, Name: "o", Currency: "CNY", User: "ref1", Price: 100}
	payment := &Payment{Owner: refTestOrg, Name: "selftxn", User: "ref1", Price: 100, Currency: "CNY", Order: "o"}
	if err := GrantReferralCommission(payment, order, "en"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if got := refBalance(t, "ref1"); got != 0 {
		t.Fatalf("self-referral must not pay: got %v", got)
	}
}
