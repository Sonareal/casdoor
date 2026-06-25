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
	"strings"

	"github.com/casdoor/casdoor/util"
	"github.com/xorm-io/core"
)

const (
	WithdrawalStateRequested = "Requested"
	WithdrawalStateApproved  = "Approved"
	WithdrawalStatePaid      = "Paid"
	WithdrawalStateRejected  = "Rejected"
	WithdrawalStateFailed    = "Failed"
)

// Withdrawal is a user request to cash out referral-commission balance. Approval & payout are manual (admin).
type Withdrawal struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`

	User     string  `xorm:"varchar(100) index" json:"user"`
	Amount   float64 `json:"amount"`
	Currency string  `xorm:"varchar(100)" json:"currency"`

	Channel      string `xorm:"varchar(100)" json:"channel"` // WechatBalance | Alipay | Bank
	PayeeName    string `xorm:"varchar(100)" json:"payeeName"`
	PayeeAccount string `xorm:"varchar(200)" json:"payeeAccount"`

	State    string `xorm:"varchar(100)" json:"state"`
	Operator string `xorm:"varchar(100)" json:"operator"`
	Remark   string `xorm:"varchar(500)" json:"remark"`

	ReviewedTime       string `xorm:"varchar(100)" json:"reviewedTime"`
	PaidTime           string `xorm:"varchar(100)" json:"paidTime"`
	ExternalTransferNo string `xorm:"varchar(200)" json:"externalTransferNo"`
	FailReason         string `xorm:"varchar(500)" json:"failReason"`
}

func (w *Withdrawal) GetId() string {
	return fmt.Sprintf("%s/%s", w.Owner, w.Name)
}

func GetWithdrawalCount(owner, field, value string) (int64, error) {
	session := GetSession(owner, -1, -1, field, value, "", "")
	return session.Count(&Withdrawal{})
}

func GetWithdrawals(owner string) ([]*Withdrawal, error) {
	withdrawals := []*Withdrawal{}
	err := ormer.Engine.Desc("created_time").Find(&withdrawals, &Withdrawal{Owner: owner})
	return withdrawals, err
}

func GetPaginationWithdrawals(owner string, offset, limit int, field, value, sortField, sortOrder string) ([]*Withdrawal, error) {
	withdrawals := []*Withdrawal{}
	session := GetSession(owner, offset, limit, field, value, sortField, sortOrder)
	err := session.Find(&withdrawals, &Withdrawal{Owner: owner})
	return withdrawals, err
}

func GetUserWithdrawals(owner, user string) ([]*Withdrawal, error) {
	withdrawals := []*Withdrawal{}
	err := ormer.Engine.Desc("created_time").Find(&withdrawals, &Withdrawal{Owner: owner, User: user})
	return withdrawals, err
}

func getWithdrawal(owner, name string) (*Withdrawal, error) {
	if owner == "" || name == "" {
		return nil, nil
	}
	w := Withdrawal{Owner: owner, Name: name}
	existed, err := ormer.Engine.Get(&w)
	if err != nil {
		return nil, err
	}
	if existed {
		return &w, nil
	}
	return nil, nil
}

func GetWithdrawal(id string) (*Withdrawal, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return nil, err
	}
	return getWithdrawal(owner, name)
}

func withdrawalTransaction(w *Withdrawal, amount float64, subtype string, lang string) error {
	t := &Transaction{
		Owner:       w.Owner,
		CreatedTime: util.GetCurrentTime(),
		Category:    TransactionCategoryWithdrawal,
		Type:        "Withdrawal",
		Subtype:     subtype,
		User:        w.User,
		Tag:         "User",
		Amount:      amount,
		Currency:    w.Currency,
		State:       w.State,
		Domain:      "withdrawal:" + w.Name,
	}
	_, _, err := AddTransaction(t, lang, false)
	return err
}

// ApplyWithdrawal validates the request, atomically deducts balance, and records a Requested withdrawal.
func ApplyWithdrawal(w *Withdrawal, minAmount float64, lang string) (bool, error) {
	if w.Amount <= 0 {
		return false, errors.New("Below minimum withdrawal amount")
	}
	if w.Amount < minAmount {
		return false, errors.New("Below minimum withdrawal amount")
	}
	if w.PayeeName == "" {
		return false, errors.New("Payee info required")
	}

	user, err := getUser(w.Owner, w.User)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, fmt.Errorf("the user: %s/%s does not exist", w.Owner, w.User)
	}
	if user.Balance < w.Amount {
		return false, errors.New("Insufficient balance")
	}
	if w.Currency == "" {
		w.Currency = user.BalanceCurrency
	}

	w.Name = "wd_" + strings.ReplaceAll(util.GenerateId(), "-", "")
	w.CreatedTime = util.GetCurrentTime()
	w.State = WithdrawalStateRequested

	// deduct balance first (negative transaction); if it fails, do not create the withdrawal
	if err = withdrawalTransaction(w, -w.Amount, "apply", lang); err != nil {
		return false, err
	}

	affected, err := ormer.Engine.Insert(w)
	if err != nil {
		// best-effort refund if the withdrawal row could not be created
		_ = withdrawalTransaction(w, w.Amount, "rollback", lang)
		return false, err
	}
	TrackServerEvent(w.Owner, "withdrawal_requested", w.User, "", map[string]interface{}{"amount": w.Amount, "channel": w.Channel})
	return affected != 0, nil
}

func saveWithdrawal(w *Withdrawal) (bool, error) {
	affected, err := ormer.Engine.ID(core.PK{w.Owner, w.Name}).AllCols().Update(w)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

// ReviewWithdrawal: action "approve" -> Approved; "reject" -> Rejected + refund balance.
func ReviewWithdrawal(id, action, operator, remark, lang string) (bool, error) {
	w, err := GetWithdrawal(id)
	if err != nil {
		return false, err
	}
	if w == nil {
		return false, fmt.Errorf("the withdrawal: %s does not exist", id)
	}
	if w.State != WithdrawalStateRequested {
		return false, errors.New("Withdrawal not in reviewable state")
	}
	w.Operator = operator
	w.Remark = remark
	w.ReviewedTime = util.GetCurrentTime()

	switch action {
	case "approve":
		w.State = WithdrawalStateApproved
	case "reject":
		w.State = WithdrawalStateRejected
		if err = withdrawalTransaction(w, w.Amount, "refund-reject", lang); err != nil {
			return false, err
		}
	default:
		return false, errors.New("invalid action")
	}
	return saveWithdrawal(w)
}

// MarkWithdrawalPaid: action "paid" -> Paid (needs externalTransferNo); "fail" -> Failed + refund balance.
func MarkWithdrawalPaid(id, action, externalTransferNo, failReason, operator, lang string) (bool, error) {
	w, err := GetWithdrawal(id)
	if err != nil {
		return false, err
	}
	if w == nil {
		return false, fmt.Errorf("the withdrawal: %s does not exist", id)
	}
	if w.State != WithdrawalStateApproved {
		return false, errors.New("Withdrawal not in reviewable state")
	}
	w.Operator = operator
	w.PaidTime = util.GetCurrentTime()

	switch action {
	case "", "paid":
		if externalTransferNo == "" {
			return false, errors.New("externalTransferNo is required")
		}
		w.State = WithdrawalStatePaid
		w.ExternalTransferNo = externalTransferNo
		TrackServerEvent(w.Owner, "withdrawal_paid", w.User, "", map[string]interface{}{"amount": w.Amount, "channel": w.Channel})
	case "fail":
		w.State = WithdrawalStateFailed
		w.FailReason = failReason
		if err = withdrawalTransaction(w, w.Amount, "refund-fail", lang); err != nil {
			return false, err
		}
	default:
		return false, errors.New("invalid action")
	}
	return saveWithdrawal(w)
}
