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
	"github.com/casdoor/casdoor/object"
)

// DeleteMyAccount
// @Title DeleteMyAccount
// @Description the signed-in user requests deletion of their OWN account (self-service 注销).
//   Requires re-verification: a correct `password` OR a fresh verification `code` (sent to `dest`).
//   The account is scheduled for physical deletion after a cooling-off period and stays
//   recoverable via cancel-my-account-deletion until then.
// @Param   password   query   string  false  "current password (either this or code is required)"
// @Param   code       query   string  false  "verification code (either this or password is required)"
// @Param   dest       query   string  false  "phone/email the code was sent to (defaults to the account's email or phone)"
// @Success 200 {object} controllers.Response
// @router /delete-my-account [post]
func (c *ApiController) DeleteMyAccount() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}

	password := c.Ctx.Input.Query("password")
	code := c.Ctx.Input.Query("code")
	dest := c.Ctx.Input.Query("dest")

	verified := false
	if password != "" {
		if err := object.CheckPassword(user, password, c.GetAcceptLanguage()); err == nil {
			verified = true
		}
	}
	if !verified && code != "" {
		checkDest := dest
		if checkDest == "" {
			if user.Email != "" {
				checkDest = user.Email
			} else {
				checkDest = user.Phone
			}
		}
		if err := object.CheckVerifyCodeWithLimit(user, checkDest, code, c.GetAcceptLanguage()); err == nil {
			verified = true
		}
	}
	if !verified {
		c.ResponseError(c.T("account:Please confirm with your password or a verification code before deleting your account"))
		return
	}

	scheduled, err := object.RequestAccountDeletion(user)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(map[string]interface{}{
		"scheduledDeletionTime": scheduled,
	})
}

// CancelMyAccountDeletion
// @Title CancelMyAccountDeletion
// @Description cancel a pending account deletion during the cooling-off period (撤销注销).
// @Success 200 {object} controllers.Response
// @router /cancel-my-account-deletion [post]
func (c *ApiController) CancelMyAccountDeletion() {
	user, ok := c.RequireSignedInUser()
	if !ok {
		return
	}
	if err := object.CancelAccountDeletion(user); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
