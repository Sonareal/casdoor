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
	"strconv"

	"github.com/beego/beego/v2/core/utils/pagination"
	"github.com/casdoor/casdoor/conf"
	"github.com/casdoor/casdoor/object"
	"github.com/casdoor/casdoor/util"
)

func withdrawalMinAmount() float64 {
	s := conf.GetConfigString("withdrawalMinAmount")
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// ApplyWithdrawal
// @Title ApplyWithdrawal
// @Tag Withdrawal API
// @Description the current user applies to withdraw balance
// @Success 200 {object} controllers.Response The Response object
// @router /apply-withdrawal [post]
func (c *ApiController) ApplyWithdrawal() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	var w object.Withdrawal
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &w); err != nil {
		c.ResponseError(err.Error())
		return
	}
	// force applicant identity from the token (cannot withdraw on behalf of others)
	w.Owner = user.Owner
	w.User = user.Name

	affected, err := object.ApplyWithdrawal(&w, withdrawalMinAmount(), c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !affected {
		c.ResponseError("failed to create withdrawal")
		return
	}
	c.ResponseOk(map[string]interface{}{"name": w.Name, "state": w.State})
}

// GetMyWithdrawals
// @Title GetMyWithdrawals
// @Tag Withdrawal API
// @Description list the current user's withdrawal records
// @Success 200 {object} controllers.Response The Response object
// @router /get-my-withdrawals [get]
func (c *ApiController) GetMyWithdrawals() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	withdrawals, err := object.GetUserWithdrawals(user.Owner, user.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(withdrawals)
}

// GetWithdrawals
// @Title GetWithdrawals
// @Tag Withdrawal API
// @Description (admin) list withdrawals for review
// @Param   owner     query    string  true        "The owner (organization)"
// @Success 200 {object} controllers.Response The Response object
// @router /get-withdrawals [get]
func (c *ApiController) GetWithdrawals() {
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
		withdrawals, err := object.GetWithdrawals(owner)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		c.ResponseOk(withdrawals)
	} else {
		limitInt := util.ParseInt(limit)
		count, err := object.GetWithdrawalCount(owner, field, value)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		paginator := pagination.NewPaginator(c.Ctx.Request, limitInt, count)
		withdrawals, err := object.GetPaginationWithdrawals(owner, paginator.Offset(), limitInt, field, value, sortField, sortOrder)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		c.ResponseOk(withdrawals, paginator.Nums())
	}
}

// ReviewWithdrawal
// @Title ReviewWithdrawal
// @Tag Withdrawal API
// @Description (admin) approve or reject a withdrawal
// @Success 200 {object} controllers.Response The Response object
// @router /review-withdrawal [post]
func (c *ApiController) ReviewWithdrawal() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var form struct {
		Id     string `json:"id"`
		Action string `json:"action"`
		Remark string `json:"remark"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	operator := c.GetSessionUsername()
	affected, err := object.ReviewWithdrawal(form.Id, form.Action, operator, form.Remark, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !affected {
		c.ResponseError("failed to review withdrawal")
		return
	}
	w, _ := object.GetWithdrawal(form.Id)
	c.ResponseOk(map[string]interface{}{"state": w.State})
}

// MarkWithdrawalPaid
// @Title MarkWithdrawalPaid
// @Tag Withdrawal API
// @Description (admin) mark an approved withdrawal as paid (or failed) after manual transfer
// @Success 200 {object} controllers.Response The Response object
// @router /mark-withdrawal-paid [post]
func (c *ApiController) MarkWithdrawalPaid() {
	if !c.IsAdmin() {
		c.ResponseError(c.T("auth:Unauthorized operation"))
		return
	}
	var form struct {
		Id                 string `json:"id"`
		Action             string `json:"action"`
		ExternalTransferNo string `json:"externalTransferNo"`
		FailReason         string `json:"failReason"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	operator := c.GetSessionUsername()
	affected, err := object.MarkWithdrawalPaid(form.Id, form.Action, form.ExternalTransferNo, form.FailReason, operator, c.GetAcceptLanguage())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !affected {
		c.ResponseError("failed to mark withdrawal")
		return
	}
	w, _ := object.GetWithdrawal(form.Id)
	c.ResponseOk(map[string]interface{}{"state": w.State})
}
