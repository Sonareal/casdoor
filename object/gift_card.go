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
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

const (
	GiftCardStateUnused   = "Unused"
	GiftCardStateUsed     = "Used"
	GiftCardStateDisabled = "Disabled"

	giftCardAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	giftCardCodeLen  = 16
)

// GiftCard is a redeemable code that grants a subscription (tied to a Plan) without payment.
type GiftCard struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`

	Batch   string `xorm:"varchar(100) index" json:"batch"`   // batch label, for batch management
	Code    string `xorm:"varchar(100) index" json:"code"`    // 32-char uppercase, unique
	Pricing string `xorm:"varchar(100)" json:"pricing"`       // optional, informational
	Plan    string `xorm:"varchar(100)" json:"plan"`          // bound plan -> period + role
	Product string `xorm:"varchar(100)" json:"product"`       // product code (type follows this)

	State        string `xorm:"varchar(100)" json:"state"` // Unused | Used | Disabled
	UsedBy       string `xorm:"varchar(100)" json:"usedBy"`
	UsedTime     string `xorm:"varchar(100)" json:"usedTime"`
	Subscription string `xorm:"varchar(100)" json:"subscription"` // created subscription name
	SubEndTime   string `xorm:"varchar(100)" json:"subEndTime"`   // granted subscription end time (set at redeem)
	ExpireTime   string `xorm:"varchar(100)" json:"expireTime"`   // empty = permanent (unredeemed)
}

func (gc *GiftCard) GetId() string {
	return fmt.Sprintf("%s/%s", gc.Owner, gc.Name)
}

func GetGiftCardCount(owner, field, value string) (int64, error) {
	session := GetSession(owner, -1, -1, field, value, "", "")
	return session.Count(&GiftCard{})
}

func GetGiftCards(owner string) ([]*GiftCard, error) {
	giftCards := []*GiftCard{}
	err := ormer.Engine.Desc("created_time").Find(&giftCards, &GiftCard{Owner: owner})
	return giftCards, err
}

func GetPaginationGiftCards(owner string, offset, limit int, field, value, sortField, sortOrder string) ([]*GiftCard, error) {
	giftCards := []*GiftCard{}
	session := GetSession(owner, offset, limit, field, value, sortField, sortOrder)
	err := session.Find(&giftCards, &GiftCard{Owner: owner})
	return giftCards, err
}

func getGiftCard(owner, name string) (*GiftCard, error) {
	if owner == "" || name == "" {
		return nil, nil
	}
	gc := GiftCard{Owner: owner, Name: name}
	existed, err := ormer.Engine.Get(&gc)
	if err != nil {
		return nil, err
	}
	if existed {
		return &gc, nil
	}
	return nil, nil
}

func GetGiftCard(id string) (*GiftCard, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return nil, err
	}
	return getGiftCard(owner, name)
}

func getGiftCardByCode(owner, code string) (*GiftCard, error) {
	if owner == "" || code == "" {
		return nil, nil
	}
	gc := GiftCard{Owner: owner, Code: code}
	existed, err := ormer.Engine.Get(&gc)
	if err != nil {
		return nil, err
	}
	if existed {
		return &gc, nil
	}
	return nil, nil
}

func genGiftCardCode() (string, error) {
	for i := 0; i < 5; i++ {
		b := make([]byte, giftCardCodeLen)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		out := make([]byte, giftCardCodeLen)
		for j := range b {
			out[j] = giftCardAlphabet[int(b[j])%len(giftCardAlphabet)]
		}
		code := string(out)
		existed, err := ormer.Engine.Get(&GiftCard{Code: code})
		if err != nil {
			return "", err
		}
		if !existed {
			return code, nil
		}
	}
	return "", errors.New("failed to generate a unique gift card code")
}

// GenerateGiftCards batch-creates `quantity` unused gift cards bound to a plan. Returns the created cards.
func GenerateGiftCards(owner, batch, planName, pricing, product string, quantity int, lang string) ([]*GiftCard, error) {
	if quantity <= 0 || quantity > 10000 {
		return nil, errors.New("quantity must be between 1 and 10000")
	}
	plan, err := GetPlan(util.GetId(owner, planName))
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("the plan: %s does not exist", planName)
	}
	if product == "" {
		product = plan.Product
	}

	now := util.GetCurrentTime()
	cards := make([]*GiftCard, 0, quantity)
	for i := 0; i < quantity; i++ {
		code, err := genGiftCardCode()
		if err != nil {
			return nil, err
		}
		gc := &GiftCard{
			Owner:       owner,
			Name:        "gc_" + strings.ReplaceAll(util.GenerateId(), "-", ""),
			CreatedTime: now,
			Batch:       batch,
			Code:        code,
			Pricing:     pricing,
			Plan:        planName,
			Product:     product,
			State:       GiftCardStateUnused,
		}
		if _, err := ormer.Engine.Insert(gc); err != nil {
			return nil, err
		}
		cards = append(cards, gc)
	}
	return cards, nil
}

func UpdateGiftCard(id string, gc *GiftCard) (bool, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return false, err
	}
	if old, err := getGiftCard(owner, name); err != nil || old == nil {
		return false, err
	}
	affected, err := ormer.Engine.ID(core.PK{owner, name}).AllCols().Update(gc)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func DeleteGiftCard(gc *GiftCard) (bool, error) {
	affected, err := ormer.Engine.ID(core.PK{gc.Owner, gc.Name}).Delete(&GiftCard{})
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

// BatchDeleteGiftCards deletes cards by explicit names, or (if names empty) the whole batch.
func BatchDeleteGiftCards(owner string, names []string, batch string) (int64, error) {
	if owner == "" {
		return 0, errors.New("owner is required")
	}
	if len(names) > 0 {
		return ormer.Engine.In("name", names).Where("owner = ?", owner).Delete(&GiftCard{})
	}
	if batch != "" {
		return ormer.Engine.Where("owner = ? and batch = ?", owner, batch).Delete(&GiftCard{})
	}
	return 0, errors.New("names or batch is required")
}

// BatchDisableGiftCards sets Unused cards to Disabled, by explicit names or whole batch.
func BatchDisableGiftCards(owner string, names []string, batch string) (int64, error) {
	if owner == "" {
		return 0, errors.New("owner is required")
	}
	upd := &GiftCard{State: GiftCardStateDisabled}
	if len(names) > 0 {
		return ormer.Engine.In("name", names).Where("owner = ? and state = ?", owner, GiftCardStateUnused).Cols("state").Update(upd)
	}
	if batch != "" {
		return ormer.Engine.Where("owner = ? and batch = ? and state = ?", owner, batch, GiftCardStateUnused).Cols("state").Update(upd)
	}
	return 0, errors.New("names or batch is required")
}

// RedeemGiftCard atomically claims an unused card for the user and grants the bound subscription (no payment).
func RedeemGiftCard(owner, code, userName, lang string) (*Subscription, error) {
	gc, err := getGiftCardByCode(owner, code)
	if err != nil {
		return nil, err
	}
	if gc == nil {
		return nil, errors.New("Gift card not found")
	}
	if gc.State == GiftCardStateUsed {
		return nil, errors.New("Gift card already used")
	}
	if gc.State == GiftCardStateDisabled {
		return nil, errors.New("Gift card is disabled")
	}
	if gc.ExpireTime != "" {
		if exp, perr := time.Parse(time.RFC3339, gc.ExpireTime); perr == nil && exp.Before(time.Now()) {
			return nil, errors.New("Gift card has expired")
		}
	}

	plan, err := GetPlan(util.GetId(owner, gc.Plan))
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("the plan: %s does not exist", gc.Plan)
	}

	now := util.GetCurrentTime()
	// 1) atomically claim the card (only succeeds if still Unused) to prevent double-redeem
	claim := &GiftCard{State: GiftCardStateUsed, UsedBy: userName, UsedTime: now}
	affected, err := ormer.Engine.ID(core.PK{gc.Owner, gc.Name}).
		Cols("state", "used_by", "used_time").
		Where("state = ?", GiftCardStateUnused).
		Update(claim)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, errors.New("Gift card already used")
	}

	// 2) create the Active subscription (no payment), mirroring a purchased subscription
	sub, err := NewSubscription(owner, userName, gc.Plan, "", plan.Period)
	if err != nil {
		// revert the claim
		_, _ = ormer.Engine.ID(core.PK{gc.Owner, gc.Name}).Cols("state", "used_by", "used_time").
			Update(&GiftCard{State: GiftCardStateUnused, UsedBy: "", UsedTime: ""})
		return nil, err
	}
	sub.Pricing = gc.Pricing
	sub.State = SubStateActive
	sub.DisplayName = "Gift card: " + gc.Code
	if _, err = AddSubscription(sub); err != nil {
		_, _ = ormer.Engine.ID(core.PK{gc.Owner, gc.Name}).Cols("state", "used_by", "used_time").
			Update(&GiftCard{State: GiftCardStateUnused, UsedBy: "", UsedTime: ""})
		return nil, err
	}

	// 3) link the subscription back to the card
	_, _ = ormer.Engine.ID(core.PK{gc.Owner, gc.Name}).Cols("subscription", "sub_end_time").Update(&GiftCard{Subscription: sub.Name, SubEndTime: sub.EndTime})

	// analytics (best-effort)
	TrackServerEvent(owner, "gift_card_redeemed", userName, "", map[string]interface{}{"plan": gc.Plan, "period": plan.Period, "batch": gc.Batch})
	TrackServerEvent(owner, "subscription_activated", userName, "", map[string]interface{}{"plan": gc.Plan, "period": plan.Period, "source": "giftcard"})
	return sub, nil
}
