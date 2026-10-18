//go:build !skipCi

package object

import "testing"

// Regression: refineUser must strip credential and MFA secrets so they never
// land in a JWT access/id token (MFA bypass + offline hash cracking otherwise).
func TestRefineUserScrubsSecrets(t *testing.T) {
	u := &User{
		Owner: "o", Name: "n",
		Password: "p", PasswordSalt: "salt", PasswordType: "plain",
		Hash: "h", PreHash: "ph",
		TotpSecret: "JBSWY3DPEHPK3PXP", RecoveryCodes: []string{"abc", "def"},
	}
	got := refineUser(u)
	if got.Password != "" || got.PasswordSalt != "" || got.PasswordType != "" ||
		got.Hash != "" || got.PreHash != "" || got.TotpSecret != "" || len(got.RecoveryCodes) != 0 {
		t.Errorf("refineUser left a secret: password=%q salt=%q type=%q hash=%q prehash=%q totp=%q recovery=%v",
			got.Password, got.PasswordSalt, got.PasswordType, got.Hash, got.PreHash, got.TotpSecret, got.RecoveryCodes)
	}
}
