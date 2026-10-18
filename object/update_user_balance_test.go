//go:build !skipCi

package object

import (
	"testing"

	"github.com/casdoor/casdoor/util"
)

const balOrg = "test-bal-org"

// Regression: a non-admin caller must not be able to set monetary/privilege
// fields (balance, score, type, ...) on a user, regardless of the org's
// per-account-item ViewRule. An admin still can.
func TestCheckPermissionPinsBalanceForNonAdmin(t *testing.T) {
	createDatabase = false
	InitConfig()
	ormer.Engine.Where("owner = ?", balOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", balOrg).Delete(&Organization{})
	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: balOrg, DisplayName: balOrg, CreatedTime: now}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	old := &User{Owner: balOrg, Name: "u", Id: util.GenerateId(), CreatedTime: now, Balance: 0, Score: 0, Type: "normal-user"}
	if _, err := ormer.Engine.Insert(old); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Non-admin tries to raise balance/score and change type -> all pinned.
	mk := func() (*User, *User) {
		o, _ := getUser(balOrg, "u")
		n := *o
		n.Balance = 1000000
		n.BalanceCredit = 5000
		n.Score = 99999
		n.Type = "paid-user"
		return o, &n
	}
	o, n := mk()
	if pass, msg := CheckPermissionForUpdateUser(o, n, false, false, ""); !pass {
		t.Fatalf("unexpected reject: %s", msg)
	}
	if n.Balance != 0 || n.BalanceCredit != 0 || n.Score != 0 || n.Type != "normal-user" {
		t.Errorf("non-admin was able to change protected fields: balance=%v credit=%v score=%v type=%q", n.Balance, n.BalanceCredit, n.Score, n.Type)
	}

	// Admin may change them.
	o2, n2 := mk()
	if pass, msg := CheckPermissionForUpdateUser(o2, n2, true, false, ""); !pass {
		t.Fatalf("admin reject: %s", msg)
	}
	if n2.Balance != 1000000 {
		t.Errorf("admin should be able to set balance, got %v", n2.Balance)
	}
	ormer.Engine.Where("owner = ?", balOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", balOrg).Delete(&Organization{})
}
