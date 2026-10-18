//go:build !skipCi

package object

import "testing"

// Regression for the 2026-10 secret-disclosure incident: an application
// credential (app/*) is treated as a global admin for ordinary operations, but
// must NOT be able to read application secrets back in plaintext. Only a real
// admin user may. Anonymous callers are masked as before.
func TestGetMaskedApplicationDoesNotUnmaskForApp(t *testing.T) {
	createDatabase = false
	InitConfig()

	newApp := func() *Application {
		return &Application{
			Owner: "admin", Name: "app-mask-test",
			Organization: "built-in",
			ClientId:     "cid-123", ClientSecret: "super-secret-value", Cert: "cert-built-in",
		}
	}

	// Anonymous -> masked.
	if got := GetMaskedApplication(newApp(), ""); got.ClientSecret != "***" {
		t.Errorf("anonymous: clientSecret should be masked, got %q", got.ClientSecret)
	}

	// Application credential -> masked (the fix).
	if got := GetMaskedApplication(newApp(), "app/app-built-in"); got.ClientSecret != "***" {
		t.Errorf("app credential: clientSecret must be masked, got %q", got.ClientSecret)
	}

	// Real global admin -> unmasked (unchanged behaviour).
	if got := GetMaskedApplication(newApp(), "built-in/admin"); got.ClientSecret != "super-secret-value" {
		t.Errorf("admin: clientSecret should be visible, got %q", got.ClientSecret)
	}

	// Plural path: app credential must not get the unmasked list.
	apps := GetMaskedApplications([]*Application{newApp()}, "app/app-built-in")
	if apps[0].ClientSecret != "***" {
		t.Errorf("app credential (list): clientSecret must be masked, got %q", apps[0].ClientSecret)
	}
}
