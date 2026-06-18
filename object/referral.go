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
	"math"
	"strings"

	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

const referralCommissionFromPrefix = "referralFrom:"

func errCommissionRate(lang string) error {
	return errors.New("Commission rate exceeds limit")
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// GetInvitationByInviter returns the personal invitation owned by the given user (inviter id = "owner/name").
func GetInvitationByInviter(owner string, inviterId string) (*Invitation, error) {
	if owner == "" || inviterId == "" {
		return nil, nil
	}
	invitation := Invitation{Owner: owner, Inviter: inviterId}
	existed, err := ormer.Engine.Get(&invitation)
	if err != nil {
		return nil, err
	}
	if existed {
		return &invitation, nil
	}
	return nil, nil
}

func genReferralCode() (string, error) {
	// derive an uppercase alphanumeric code from a uuid; retry on (unlikely) collision
	for i := 0; i < 5; i++ {
		raw := strings.ToUpper(strings.ReplaceAll(util.GenerateId(), "-", ""))
		code := raw[:8]
		existed, err := ormer.Engine.Get(&Invitation{Code: code})
		if err != nil {
			return "", err
		}
		if !existed {
			return code, nil
		}
	}
	return "", errors.New("failed to generate a unique referral code")
}

// EnsurePersonalInvitation returns the user's personal referral invitation, creating it if absent.
func EnsurePersonalInvitation(user *User) (*Invitation, error) {
	if user == nil {
		return nil, errors.New("user is nil")
	}
	inv, err := GetInvitationByInviter(user.Owner, user.GetId())
	if err != nil {
		return nil, err
	}
	if inv != nil {
		return inv, nil
	}

	code, err := genReferralCode()
	if err != nil {
		return nil, err
	}
	now := util.GetCurrentTime()
	inv = &Invitation{
		Owner:           user.Owner,
		Name:            "ref_" + user.Name,
		CreatedTime:     now,
		UpdatedTime:     now,
		DisplayName:     "Referral of " + user.Name,
		Code:            code,
		DefaultCode:     code,
		IsRegexp:        false,
		Quota:           1 << 30,
		UsedCount:       0,
		Application:     "All",
		State:           "Active",
		Inviter:         user.GetId(),
		CommissionRate:  -1,
		PaidInviteCount: 0,
	}
	if _, err = ormer.Engine.Insert(inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// SetReferralRate sets a user's per-user override rate and/or group tier on their personal invitation.
// rate == -2 means "leave unchanged"; -1 clears the override; >=0 sets it (0 = explicit 0%). Validated against policy max.
func SetReferralRate(userId string, rate float64, tier string, setTier bool, lang string) (*Invitation, error) {
	owner, name := util.GetOwnerAndNameFromIdNoCheck(userId)
	user, err := getUser(owner, name)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("the user: %s does not exist", userId)
	}
	inv, err := EnsurePersonalInvitation(user)
	if err != nil {
		return nil, err
	}

	if rate != -2 {
		if rate >= 0 {
			policy, perr := GetReferralPolicy(owner)
			if perr != nil {
				return nil, perr
			}
			if rate > policy.effectiveMaxRate() {
				return nil, errCommissionRate(lang)
			}
		} else {
			rate = -1 // normalize any negative to "unset"
		}
		inv.CommissionRate = rate
	}
	if setTier {
		inv.Tier = tier
	}
	inv.UpdatedTime = util.GetCurrentTime()
	if _, err = ormer.Engine.ID(core.PK{inv.Owner, inv.Name}).Cols("commission_rate", "tier", "updated_time").Update(inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// GetReferrerOfUser returns the user who referred the given user (via the invitation code used at signup).
func GetReferrerOfUser(user *User) (*User, error) {
	if user == nil || user.Invitation == "" {
		return nil, nil
	}
	inv, err := getInvitation(user.Owner, user.Invitation)
	if err != nil || inv == nil || inv.Inviter == "" {
		return nil, err
	}
	refOwner, refName := util.GetOwnerAndNameFromIdNoCheck(inv.Inviter)
	if refOwner == user.Owner && refName == user.Name {
		return nil, nil // self-referral
	}
	return getUser(refOwner, refName)
}

// ResolveCommissionRate computes the effective rate for a referrer.
// Precedence: per-user override > auto-upgrade tier > group rate > default. Returns (rate, source).
func ResolveCommissionRate(referrer *User) (float64, string, error) {
	policy, err := GetReferralPolicy(referrer.Owner)
	if err != nil {
		return 0, "", err
	}
	if !policy.Enabled {
		return 0, "disabled", nil
	}

	inv, err := GetInvitationByInviter(referrer.Owner, referrer.GetId())
	if err != nil {
		return 0, "", err
	}

	// ① per-user override (-1 = unset; 0 = explicit 0%)
	if inv != nil && inv.CommissionRate >= 0 {
		return inv.CommissionRate, "override", nil
	}

	// ② auto-upgrade by invite count (toggle)
	if policy.AutoUpgradeEnabled && len(policy.Tiers) > 0 {
		cnt := 0
		if policy.UpgradeBasis == "signup" {
			cnt = countSignupInvites(referrer.Owner, inv)
		} else if inv != nil {
			cnt = inv.PaidInviteCount
		}
		best := -1
		var bestRate float64
		for _, t := range policy.Tiers {
			if cnt >= t.MinInvites && t.MinInvites > best {
				best = t.MinInvites
				bestRate = t.Rate
			}
		}
		if best >= 0 {
			return bestRate, "autoUpgrade", nil
		}
	}

	// ③ group rate
	if inv != nil && inv.Tier != "" {
		if r, ok := policy.GroupRates[inv.Tier]; ok {
			return r, "group", nil
		}
	}

	// ④ default
	return policy.DefaultRate, "default", nil
}

func countSignupInvites(owner string, inv *Invitation) int {
	if inv == nil {
		return 0
	}
	cnt, err := ormer.Engine.Count(&User{Owner: owner, Invitation: inv.Name})
	if err != nil {
		return 0
	}
	return int(cnt)
}

func getCommissionByPayment(owner string, paymentName string) (*Transaction, error) {
	t := Transaction{Owner: owner, Payment: paymentName, Category: TransactionCategoryCommission}
	existed, err := ormer.Engine.Get(&t)
	if err != nil {
		return nil, err
	}
	if existed {
		return &t, nil
	}
	return nil, nil
}

func getCommissionByPayer(owner string, payerId string) (*Transaction, error) {
	t := Transaction{Owner: owner, Category: TransactionCategoryCommission, Domain: referralCommissionFromPrefix + payerId}
	existed, err := ormer.Engine.Get(&t)
	if err != nil {
		return nil, err
	}
	if existed {
		return &t, nil
	}
	return nil, nil
}

// CountInvitedUsers returns how many users registered with the given personal invitation.
func CountInvitedUsers(owner string, invitationName string) (int, error) {
	if owner == "" || invitationName == "" {
		return 0, nil
	}
	cnt, err := ormer.Engine.Count(&User{Owner: owner, Invitation: invitationName})
	return int(cnt), err
}

// GetInvitees returns the users who registered with the given personal invitation.
func GetInvitees(owner string, invitationName string) ([]*User, error) {
	users := []*User{}
	if owner == "" || invitationName == "" {
		return users, nil
	}
	err := ormer.Engine.Desc("created_time").Find(&users, &User{Owner: owner, Invitation: invitationName})
	return users, err
}

// IsInviteePaid reports whether the given invitee already produced a commission.
func IsInviteePaid(owner string, inviteeId string) bool {
	t, err := getCommissionByPayer(owner, inviteeId)
	return err == nil && t != nil
}

// GetUserCommissions returns commission transactions credited to the user (newest first).
func GetUserCommissions(owner string, user string) ([]*Transaction, error) {
	transactions := []*Transaction{}
	err := ormer.Engine.Desc("created_time").Find(&transactions, &Transaction{Owner: owner, User: user, Category: TransactionCategoryCommission})
	return transactions, err
}

// GrantReferralCommission credits the referrer of the paying user.
// Single-level, first-order only, idempotent, self-referral guarded. Returns nil if nothing to do.
// Callers MUST treat errors as non-fatal (must not break payment processing).
func GrantReferralCommission(payment *Payment, order *Order, lang string) error {
	if payment == nil || order == nil {
		return nil
	}
	payer, err := getUser(payment.Owner, payment.User)
	if err != nil || payer == nil {
		return err
	}
	payerId := payer.GetId()

	referrer, err := GetReferrerOfUser(payer)
	if err != nil || referrer == nil {
		return err
	}

	// idempotency: this exact payment already commissioned?
	if existing, err := getCommissionByPayment(payment.Owner, payment.Name); err != nil {
		return err
	} else if existing != nil {
		return nil
	}
	// first-order only: this payer already produced a commission before?
	if prior, err := getCommissionByPayer(payment.Owner, payerId); err != nil {
		return err
	} else if prior != nil {
		return nil
	}

	rate, source, err := ResolveCommissionRate(referrer)
	if err != nil {
		return err
	}
	if rate <= 0 {
		return nil
	}
	commission := round2(payment.Price * rate)
	if commission <= 0 {
		return nil
	}

	now := util.GetCurrentTime()
	t := &Transaction{
		Owner:       payment.Owner,
		CreatedTime: now,
		Application: referrer.SignupApplication,
		Category:    TransactionCategoryCommission,
		Type:        "Referral",
		Subtype:     source,
		User:        referrer.Name,
		Tag:         "User", // routes balance credit to referrer
		Amount:      commission,
		Currency:    order.Currency,
		Payment:     payment.Name,
		Domain:      referralCommissionFromPrefix + payerId,
		State:       "Paid",
	}
	affected, _, err := AddTransaction(t, lang, false)
	if err != nil {
		return err
	}
	if !affected {
		return fmt.Errorf("failed to add commission transaction for payment %s", payment.Name)
	}

	// maintain auto-upgrade counter on the referrer's personal invitation
	refInv, err := GetInvitationByInviter(referrer.Owner, referrer.GetId())
	if err == nil && refInv != nil {
		refInv.PaidInviteCount += 1
		refInv.UpdatedTime = now
		_, _ = ormer.Engine.ID(core.PK{refInv.Owner, refInv.Name}).Cols("paid_invite_count", "updated_time").Update(refInv)
	}
	return nil
}
