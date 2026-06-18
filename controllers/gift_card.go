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

	"github.com/beego/beego/v2/core/utils/pagination"
	"github.com/casdoor/casdoor/object"
	"github.com/casdoor/casdoor/util"
)

// GetGiftCards
// @Title GetGiftCards
// @Tag GiftCard API
// @Description (admin) list gift cards
// @Param   owner   query   string  true  "owner (organization)"
// @Success 200 {array} object.GiftCard The Response object
// @router /get-gift-cards [get]
func (c *ApiController) GetGiftCards() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	owner := c.Ctx.Input.Query("owner")
	limit := c.Ctx.Input.Query("pageSize")
	page := c.Ctx.Input.Query("p")
	field := c.Ctx.Input.Query("field")
	value := c.Ctx.Input.Query("value")
	sortField := c.Ctx.Input.Query("sortField")
	sortOrder := c.Ctx.Input.Query("sortOrder")

	if limit == "" || page == "" {
		giftCards, err := object.GetGiftCards(owner)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		c.ResponseOk(giftCards)
	} else {
		limitInt := util.ParseInt(limit)
		count, err := object.GetGiftCardCount(owner, field, value)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		paginator := pagination.NewPaginator(c.Ctx.Request, limitInt, count)
		giftCards, err := object.GetPaginationGiftCards(owner, paginator.Offset(), limitInt, field, value, sortField, sortOrder)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		c.ResponseOk(giftCards, paginator.Nums())
	}
}

// GenerateGiftCards
// @Title GenerateGiftCards
// @Tag GiftCard API
// @Description (admin) batch-generate gift cards bound to a plan
// @Success 200 {object} controllers.Response The Response object
// @router /generate-gift-cards [post]
func (c *ApiController) GenerateGiftCards() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var form struct {
		Owner    string `json:"owner"`
		Batch    string `json:"batch"`
		Plan     string `json:"plan"`
		Pricing  string `json:"pricing"`
		Product  string `json:"product"`
		Quantity int    `json:"quantity"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	cards, err := object.GenerateGiftCards(form.Owner, form.Batch, form.Plan, form.Pricing, form.Product, form.Quantity, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(cards, len(cards))
}

// UpdateGiftCard
// @Title UpdateGiftCard
// @Tag GiftCard API
// @Description (admin) update a gift card (e.g. disable)
// @Param   id   query   string  true  "id (owner/name)"
// @Success 200 {object} controllers.Response The Response object
// @router /update-gift-card [post]
func (c *ApiController) UpdateGiftCard() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	id := c.Ctx.Input.Query("id")
	var giftCard object.GiftCard
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &giftCard); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.Data["json"] = wrapActionResponse(object.UpdateGiftCard(id, &giftCard))
	c.ServeJSON()
}

// DeleteGiftCard
// @Title DeleteGiftCard
// @Tag GiftCard API
// @Description (admin) delete a gift card
// @Success 200 {object} controllers.Response The Response object
// @router /delete-gift-card [post]
func (c *ApiController) DeleteGiftCard() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var giftCard object.GiftCard
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &giftCard); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.Data["json"] = wrapActionResponse(object.DeleteGiftCard(&giftCard))
	c.ServeJSON()
}

// BatchGiftCards
// @Title BatchGiftCards
// @Tag GiftCard API
// @Description (admin) batch delete/disable gift cards by names or by batch
// @Success 200 {object} controllers.Response The Response object
// @router /batch-gift-cards [post]
func (c *ApiController) BatchGiftCards() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var form struct {
		Owner  string   `json:"owner"`
		Action string   `json:"action"` // "delete" | "disable"
		Names  []string `json:"names"`
		Batch  string   `json:"batch"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}

	var affected int64
	var err error
	switch form.Action {
	case "delete":
		affected, err = object.BatchDeleteGiftCards(form.Owner, form.Names, form.Batch)
	case "disable":
		affected, err = object.BatchDisableGiftCards(form.Owner, form.Names, form.Batch)
	default:
		c.ResponseError("invalid action")
		return
	}
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(affected)
}

// RedeemGiftCard
// @Title RedeemGiftCard
// @Tag GiftCard API
// @Description the current user redeems a gift card to activate a subscription
// @Success 200 {object} controllers.Response The Response object
// @router /redeem-gift-card [post]
func (c *ApiController) RedeemGiftCard() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	var form struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	sub, err := object.RedeemGiftCard(user.Owner, form.Code, user.Name, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(map[string]interface{}{
		"plan":      sub.Plan,
		"period":    sub.Period,
		"startTime": sub.StartTime,
		"endTime":   sub.EndTime,
		"state":     sub.State,
	})
}
