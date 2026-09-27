//go:build !skipCi

package object

import (
	"testing"

	"github.com/casdoor/casdoor/util"
)

const privescOrg = "test-privesc-org"

// Regression for the privilege-escalation flaw where a non-admin caller could
// promote themselves by naming a privilege flag in ?columns= (which makes
// UpdateUser skip its column whitelist), because CheckPermissionForUpdateUser
// did not pin those flags for non-admins.
//
// The organization here has NO account items on purpose: userVisible treats a
// missing item as visible, so the per-item checks fail open. The fix must hold
// regardless of that configuration.
func TestUpdateUserPrivilegeFlagsNonAdmin(t *testing.T) {
	createDatabase = false
	InitConfig()

	ormer.Engine.Where("owner = ?", privescOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", privescOrg).Delete(&Organization{})

	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: privescOrg, DisplayName: privescOrg, CreatedTime: now}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	seed := &User{Owner: privescOrg, Name: "victim", Id: util.GenerateId(), CreatedTime: now, IsAdmin: false, IsForbidden: true, IsDeleted: false}
	if _, err := ormer.Engine.Insert(seed); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id := privescOrg + "/victim"

	// A non-admin self-update that tries to grant admin, clear the forbidden
	// flag, and also legitimately change the display name, naming everything in
	// columns so the whitelist would be skipped.
	edited, _ := GetUser(id)
	edited.IsAdmin = true
	edited.IsForbidden = false
	edited.DisplayName = "new name"
	cols := []string{"is_admin", "is_forbidden", "display_name"}

	if pass, msg := CheckPermissionForUpdateUser(seedCopy(edited), edited, false, false, ""); !pass {
		t.Fatalf("permission check unexpectedly rejected a legitimate display-name change: %s", msg)
	}
	if _, err := UpdateUser(id, edited, cols, false); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := GetUser(id)
	if got.IsAdmin {
		t.Errorf("提权未被拦截：非管理员经 columns 把自己改成了 is_admin=true")
	}
	if !got.IsForbidden {
		t.Errorf("非管理员经 columns 清除了 is_forbidden（应保持 true）")
	}
	if got.DisplayName != "new name" {
		t.Errorf("合法的 display_name 修改被误伤：got %q", got.DisplayName)
	}
}

// An actual admin must still be able to set the flags.
func TestUpdateUserPrivilegeFlagsAdmin(t *testing.T) {
	createDatabase = false
	InitConfig()

	ormer.Engine.Where("owner = ?", privescOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", privescOrg).Delete(&Organization{})

	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: privescOrg, DisplayName: privescOrg, CreatedTime: now}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := ormer.Engine.Insert(&User{Owner: privescOrg, Name: "target", Id: util.GenerateId(), CreatedTime: now}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id := privescOrg + "/target"

	edited, _ := GetUser(id)
	edited.IsAdmin = true
	if pass, msg := CheckPermissionForUpdateUser(seedCopy(edited), edited, true, false, ""); !pass {
		t.Fatalf("admin permission check rejected: %s", msg)
	}
	if _, err := UpdateUser(id, edited, []string{"is_admin"}, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := GetUser(id)
	if !got.IsAdmin {
		t.Errorf("管理员设置 is_admin=true 失败")
	}
}

// Soft-deletion goes through UpdateUser with isAdmin=false and must still be
// able to set is_deleted — proof the fix stays at the permission layer and does
// not pin flags in the data layer.
func TestSoftDeleteStillSetsIsDeleted(t *testing.T) {
	createDatabase = false
	InitConfig()

	ormer.Engine.Where("owner = ?", privescOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", privescOrg).Delete(&Organization{})

	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Organization{Owner: "admin", Name: privescOrg, DisplayName: privescOrg, CreatedTime: now}); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := ormer.Engine.Insert(&User{Owner: privescOrg, Name: "goner", Id: util.GenerateId(), CreatedTime: now}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id := privescOrg + "/goner"

	u, _ := GetUser(id)
	u.IsDeleted = true
	u.DeletedTime = util.GetCurrentTime()
	if _, err := UpdateUser(id, u, []string{"is_deleted", "deleted_time"}, false); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	got, _ := GetUser(id)
	if !got.IsDeleted {
		t.Errorf("软删除被破坏：is_deleted 未被写入")
	}
}

// seedCopy makes the oldUser argument CheckPermissionForUpdateUser needs — a
// snapshot of the stored row before the in-memory edits.
func seedCopy(edited *User) *User {
	stored, _ := getUser(edited.Owner, edited.Name)
	return stored
}
