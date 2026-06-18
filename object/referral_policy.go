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
	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

// ReferralTier is one auto-upgrade tier: reaching MinInvites paid invites grants Rate.
type ReferralTier struct {
	Name       string  `json:"name"`
	MinInvites int     `json:"minInvites"`
	Rate       float64 `json:"rate"`
}

// ReferralPolicy is the per-organization referral-commission policy.
// One row per organization, Name fixed to "default".
type ReferralPolicy struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	UpdatedTime string `xorm:"varchar(100)" json:"updatedTime"`

	Enabled     bool    `json:"enabled"`     // master switch
	DefaultRate float64 `json:"defaultRate"` // fallback rate

	GroupRates map[string]float64 `xorm:"mediumtext" json:"groupRates"` // tier label -> rate (group management)

	AutoUpgradeEnabled bool            `json:"autoUpgradeEnabled"` // toggle: auto-upgrade rate by invite count
	UpgradeBasis       string          `xorm:"varchar(100)" json:"upgradeBasis"` // "paid" (default) | "signup"
	Tiers              []*ReferralTier `xorm:"mediumtext" json:"tiers"`

	MaxRate float64 `json:"maxRate"` // upper bound for any configured rate (safety, e.g. 0.5)
}

const referralPolicyName = "default"

func getReferralPolicy(owner string) (*ReferralPolicy, error) {
	if owner == "" {
		return nil, nil
	}
	policy := ReferralPolicy{Owner: owner, Name: referralPolicyName}
	existed, err := ormer.Engine.Get(&policy)
	if err != nil {
		return nil, err
	}
	if existed {
		return &policy, nil
	}
	return nil, nil
}

// GetReferralPolicy returns the org policy, or a safe in-memory default if none configured.
// The default has Enabled=true but DefaultRate=0, so nothing pays out until an admin sets a rate.
func GetReferralPolicy(owner string) (*ReferralPolicy, error) {
	policy, err := getReferralPolicy(owner)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		return policy, nil
	}
	return &ReferralPolicy{
		Owner:              owner,
		Name:               referralPolicyName,
		Enabled:            true,
		DefaultRate:        0,
		GroupRates:         map[string]float64{},
		AutoUpgradeEnabled: false,
		UpgradeBasis:       "paid",
		Tiers:              []*ReferralTier{},
		MaxRate:            0.5,
	}, nil
}

func (policy *ReferralPolicy) effectiveMaxRate() float64 {
	if policy.MaxRate > 0 {
		return policy.MaxRate
	}
	return 0.5
}

// UpdateReferralPolicy upserts the org policy (creates the row if absent). Validates rates against MaxRate.
func UpdateReferralPolicy(owner string, policy *ReferralPolicy, lang string) (bool, error) {
	policy.Owner = owner
	policy.Name = referralPolicyName
	policy.UpdatedTime = util.GetCurrentTime()

	max := policy.effectiveMaxRate()
	if policy.DefaultRate < 0 || policy.DefaultRate > max {
		return false, errCommissionRate(lang)
	}
	for _, r := range policy.GroupRates {
		if r < 0 || r > max {
			return false, errCommissionRate(lang)
		}
	}
	for _, t := range policy.Tiers {
		if t.Rate < 0 || t.Rate > max {
			return false, errCommissionRate(lang)
		}
	}

	existing, err := getReferralPolicy(owner)
	if err != nil {
		return false, err
	}
	if existing == nil {
		policy.CreatedTime = util.GetCurrentTime()
		affected, err := ormer.Engine.Insert(policy)
		if err != nil {
			return false, err
		}
		return affected != 0, nil
	}

	affected, err := ormer.Engine.ID(core.PK{owner, referralPolicyName}).AllCols().Update(policy)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}
