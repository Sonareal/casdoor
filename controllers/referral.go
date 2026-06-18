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

func maskUserName(name string) string {
	r := []rune(name)
	if len(r) <= 2 {
		return name
	}
	if len(r) <= 4 {
		return string(r[:1]) + "***"
	}
	return string(r[:2]) + "***" + string(r[len(r)-1:])
}

// GetMyReferralCode
// @Title GetMyReferralCode
// @Tag Referral API
// @Description get (and lazily create) the current user's personal referral code
// @Success 200 {object} controllers.Response The Response object
// @router /get-my-referral-code [get]
func (c *ApiController) GetMyReferralCode() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}

	inv, err := object.EnsurePersonalInvitation(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	invitedCount, _ := object.CountInvitedUsers(user.Owner, inv.Name)
	link := inv.GetInvitationLink(c.Ctx.Request.Host, user.SignupApplication)

	c.ResponseOk(map[string]interface{}{
		"code":         inv.Code,
		"link":         link,
		"invitedCount": invitedCount,
		"paidCount":    inv.PaidInviteCount,
	})
}

// GetMyReferralStats
// @Title GetMyReferralStats
// @Tag Referral API
// @Description get the current user's referral stats (rate/tier/progress + earnings)
// @Success 200 {object} controllers.Response The Response object
// @router /get-my-referral-stats [get]
func (c *ApiController) GetMyReferralStats() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}

	inv, err := object.EnsurePersonalInvitation(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	invitedCount, _ := object.CountInvitedUsers(user.Owner, inv.Name)

	rate, source, err := object.ResolveCommissionRate(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	commissions, _ := object.GetUserCommissions(user.Owner, user.Name)
	total := 0.0
	month := 0.0
	prefix := util.GetCurrentTime()[:7] // YYYY-MM
	for _, t := range commissions {
		total += t.Amount
		if len(t.CreatedTime) >= 7 && t.CreatedTime[:7] == prefix {
			month += t.Amount
		}
	}

	data := map[string]interface{}{
		"invitedCount":        invitedCount,
		"paidCount":           inv.PaidInviteCount,
		"balance":             user.Balance,
		"currency":            user.BalanceCurrency,
		"commissionTotal":     total,
		"commissionThisMonth": month,
		"effectiveRate":       rate,
		"rateSource":          source,
	}

	// auto-upgrade tier/progress (only when enabled)
	if policy, err := object.GetReferralPolicy(user.Owner); err == nil && policy.AutoUpgradeEnabled {
		cnt := inv.PaidInviteCount
		if policy.UpgradeBasis == "signup" {
			cnt = invitedCount
		}
		var curTier string
		var next *object.ReferralTier
		best := -1
		for _, t := range policy.Tiers {
			if cnt >= t.MinInvites && t.MinInvites > best {
				best = t.MinInvites
				curTier = t.Name
			}
		}
		for _, t := range policy.Tiers {
			if cnt < t.MinInvites && (next == nil || t.MinInvites < next.MinInvites) {
				next = t
			}
		}
		data["tier"] = curTier
		if next != nil {
			data["nextTier"] = map[string]interface{}{
				"name": next.Name, "minInvites": next.MinInvites, "rate": next.Rate,
				"remaining": next.MinInvites - cnt,
			}
		} else {
			data["nextTier"] = nil
		}
	}

	c.ResponseOk(data)
}

// GetMyInvitees
// @Title GetMyInvitees
// @Tag Referral API
// @Description list the users the current user invited (with paid status)
// @Success 200 {object} controllers.Response The Response object
// @router /get-my-invitees [get]
func (c *ApiController) GetMyInvitees() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	inv, err := object.EnsurePersonalInvitation(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	invitees, err := object.GetInvitees(user.Owner, inv.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	list := []map[string]interface{}{}
	for _, u := range invitees {
		list = append(list, map[string]interface{}{
			"user":           maskUserName(u.Name),
			"registeredTime": u.CreatedTime,
			"paid":           object.IsInviteePaid(user.Owner, u.GetId()),
		})
	}
	c.ResponseOk(list, len(list))
}

// GetMyCommissions
// @Title GetMyCommissions
// @Tag Referral API
// @Description list the current user's commission transactions
// @Success 200 {object} controllers.Response The Response object
// @router /get-my-commissions [get]
func (c *ApiController) GetMyCommissions() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	commissions, err := object.GetUserCommissions(user.Owner, user.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	total := 0.0
	list := []map[string]interface{}{}
	for _, t := range commissions {
		total += t.Amount
		list = append(list, map[string]interface{}{
			"time": t.CreatedTime, "amount": t.Amount, "currency": t.Currency,
			"type": t.Type, "subtype": t.Subtype, "payment": t.Payment, "state": t.State,
		})
	}
	c.ResponseOk(list, total)
}

// GetReferralPolicy
// @Title GetReferralPolicy
// @Tag Referral API
// @Description (admin) get the organization referral-commission policy
// @Param   owner     query    string  true        "The owner (organization)"
// @Success 200 {object} controllers.Response The Response object
// @router /get-referral-policy [get]
func (c *ApiController) GetReferralPolicy() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	owner := c.Ctx.Input.Query("owner")
	policy, err := object.GetReferralPolicy(owner)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(policy)
}

// UpdateReferralPolicy
// @Title UpdateReferralPolicy
// @Tag Referral API
// @Description (admin) update the organization referral-commission policy
// @Param   owner     query    string  true        "The owner (organization)"
// @Param   body      body     object.ReferralPolicy  true   "policy"
// @Success 200 {object} controllers.Response The Response object
// @router /update-referral-policy [post]
func (c *ApiController) UpdateReferralPolicy() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	owner := c.Ctx.Input.Query("owner")
	var policy object.ReferralPolicy
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &policy); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.Data["json"] = wrapActionResponse(object.UpdateReferralPolicy(owner, &policy, c.GetAcceptLanguage()))
	c.ServeJSON()
}

// GetReferralRate
// @Title GetReferralRate
// @Tag Referral API
// @Description (admin) get a user's per-user referral rate/tier and effective rate
// @Param   user     query    string  true        "The user id ( owner/name )"
// @Success 200 {object} controllers.Response The Response object
// @router /get-referral-rate [get]
func (c *ApiController) GetReferralRate() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	userId := c.Ctx.Input.Query("user")
	user, err := object.GetUser(userId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if user == nil {
		c.ResponseError(fmt.Sprintf(c.T("general:The user: %s doesn't exist"), userId))
		return
	}
	inv, err := object.EnsurePersonalInvitation(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	rate, source, _ := object.ResolveCommissionRate(user)
	c.ResponseOk(map[string]interface{}{
		"commissionRate":  inv.CommissionRate,
		"tier":            inv.Tier,
		"paidInviteCount": inv.PaidInviteCount,
		"effectiveRate":   rate,
		"rateSource":      source,
	})
}

// SetReferralRate
// @Title SetReferralRate
// @Tag Referral API
// @Description (admin) set a user's per-user override rate and/or group tier
// @Success 200 {object} controllers.Response The Response object
// @router /set-referral-rate [post]
func (c *ApiController) SetReferralRate() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var form struct {
		User string   `json:"user"`
		Rate *float64 `json:"rate"`
		Tier *string  `json:"tier"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	rate := float64(-2)
	if form.Rate != nil {
		rate = *form.Rate
	}
	tier := ""
	setTier := false
	if form.Tier != nil {
		tier = *form.Tier
		setTier = true
	}
	inv, err := object.SetReferralRate(form.User, rate, tier, setTier, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(map[string]interface{}{"commissionRate": inv.CommissionRate, "tier": inv.Tier})
}
