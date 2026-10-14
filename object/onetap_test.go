//go:build !skipCi

package object

import (
	"testing"

	"github.com/casdoor/casdoor/util"
)

const oneTapOrg = "test-onetap-org"

func setupOneTapOrg(t *testing.T) *Application {
	t.Helper()
	createDatabase = false
	InitConfig()

	ormer.Engine.Where("owner = ?", oneTapOrg).Delete(&User{})
	ormer.Engine.Where("owner = ? and name = ?", "admin", oneTapOrg).Delete(&Organization{})
	ormer.Engine.Where("owner = ?", oneTapOrg).Delete(&Provider{})

	now := util.GetCurrentTime()
	org := &Organization{Owner: "admin", Name: oneTapOrg, DisplayName: oneTapOrg, CreatedTime: now, DefaultAvatar: "https://example.com/a.png", InitScore: 20}
	if _, err := ormer.Engine.Insert(org); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	appName := "app-" + oneTapOrg
	ormer.Engine.Where("owner = ? and name = ?", "admin", appName).Delete(&Application{})
	app := &Application{Owner: "admin", Name: appName, Organization: oneTapOrg, CreatedTime: now}
	if _, err := ormer.Engine.Insert(app); err != nil {
		t.Fatalf("insert app: %v", err)
	}
	app.OrganizationObj = org
	return app
}

// A first-time number creates an account, following the u<phone>_<idPrefix>
// convention and tagged so one-tap signups are distinguishable; a second call
// with the same number returns that same account rather than a duplicate.
func TestResolveOrCreateOneTapUser(t *testing.T) {
	app := setupOneTapOrg(t)
	mobile := "13800138000"

	u1, created1, err := ResolveOrCreateOneTapUser(app, mobile, "CN", "")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if !created1 {
		t.Errorf("first call should report created=true")
	}
	if u1.Phone != mobile || u1.CountryCode != "CN" {
		t.Errorf("phone/country not set: %q / %q", u1.Phone, u1.CountryCode)
	}
	if u1.RegisterType != "OneTap Signup" {
		t.Errorf("register type = %q, want OneTap Signup", u1.RegisterType)
	}
	if u1.Type != "normal-user" {
		t.Errorf("type = %q", u1.Type)
	}
	if len(u1.Name) < len("u13800138000_") || u1.Name[:13] != "u13800138000_" {
		t.Errorf("username %q does not follow u<phone>_<id> convention", u1.Name)
	}

	u2, created2, err := ResolveOrCreateOneTapUser(app, mobile, "CN", "")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if created2 {
		t.Errorf("second call should report created=false")
	}
	if u2.Id != u1.Id || u2.Name != u1.Name {
		t.Errorf("second call created a different user: %q vs %q", u2.Name, u1.Name)
	}

	cnt, _ := ormer.Engine.Where("owner = ? and phone = ?", oneTapOrg, mobile).Count(&User{})
	if cnt != 1 {
		t.Errorf("expected exactly 1 user for the phone, got %d", cnt)
	}
}

// An existing account (e.g. created earlier via the app's SMS signup) is
// matched by phone and logged in, not duplicated.
func TestResolveOneTapUserMatchesExisting(t *testing.T) {
	app := setupOneTapOrg(t)
	mobile := "13912345678"

	existing := &User{Owner: oneTapOrg, Name: "u13912345678_pre", Id: util.GenerateId(), CreatedTime: util.GetCurrentTime(), Type: "normal-user", Phone: mobile, CountryCode: "CN"}
	if _, err := ormer.Engine.Insert(existing); err != nil {
		t.Fatalf("insert existing: %v", err)
	}

	got, created, err := ResolveOrCreateOneTapUser(app, mobile, "CN", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created {
		t.Errorf("matching an existing user should report created=false")
	}
	if got.Name != "u13912345678_pre" {
		t.Errorf("did not match existing user, got %q", got.Name)
	}
}

// The provider is resolved from the application and only when it is the
// number-authentication type and actually enabled on the app.
func TestGetOneTapProvider(t *testing.T) {
	app := setupOneTapOrg(t)
	now := util.GetCurrentTime()

	// A plain SMS provider must NOT be picked for one-tap.
	if _, err := ormer.Engine.Insert(&Provider{Owner: oneTapOrg, Name: "plain-sms", Category: "SMS", Type: "Aliyun SMS", CreatedTime: now}); err != nil {
		t.Fatalf("insert sms: %v", err)
	}
	app.Providers = []*ProviderItem{{Name: "plain-sms"}}
	if _, err := GetOneTapProvider(app, ""); err == nil {
		t.Errorf("expected error when only a plain SMS provider is present")
	}

	// A PNVS provider that exists but is not attached to the app must not count.
	if _, err := ormer.Engine.Insert(&Provider{Owner: oneTapOrg, Name: "pnvs", Category: "SMS", Type: OneTapProviderType, ClientId: "ak", ClientSecret: "sk", RegionId: "cn-hangzhou", CreatedTime: now}); err != nil {
		t.Fatalf("insert pnvs: %v", err)
	}
	if _, err := GetOneTapProvider(app, ""); err == nil {
		t.Errorf("expected error when PNVS provider is not enabled on the app")
	}

	// Attached and correct type -> resolved.
	app.Providers = []*ProviderItem{{Name: "plain-sms"}, {Name: "pnvs"}}
	p, err := GetOneTapProvider(app, "")
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	if p.Name != "pnvs" || p.Type != OneTapProviderType {
		t.Errorf("resolved wrong provider: %q (%q)", p.Name, p.Type)
	}
}
