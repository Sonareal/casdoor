//go:build !skipCi

package object

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/casdoor/casdoor/util"
)

func makeValidCertPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "jwks-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// Regression: a cert of type x509 whose Certificate field is not PEM-encoded
// (e.g. a payment client key stored as a cert) must be skipped, not crash the
// JWKS endpoint. Before the fix, pem.Decode returned nil and the loop
// dereferenced it — a nil pointer panic on every /.well-known/jwks call.
func TestGetJsonWebKeySetSkipsNonPemCert(t *testing.T) {
	createDatabase = false
	InitConfig()

	good := "zz_jwks_good"
	bad := "zz_jwks_bad"
	ormer.Engine.Where("name = ? or name = ?", good, bad).Delete(&Cert{})
	now := util.GetCurrentTime()
	if _, err := ormer.Engine.Insert(&Cert{Owner: "admin", Name: good, CreatedTime: now, Type: "x509", CryptoAlgorithm: "RS256", Certificate: makeValidCertPEM(t)}); err != nil {
		t.Fatalf("insert good cert: %v", err)
	}
	// 40-char non-PEM value, exactly the shape that crashed production.
	if _, err := ormer.Engine.Insert(&Cert{Owner: "admin", Name: bad, CreatedTime: now, Type: "x509", CryptoAlgorithm: "RS256", Certificate: "1E005A9A195A999B135C22592FA653AABBCCDDEE"}); err != nil {
		t.Fatalf("insert bad cert: %v", err)
	}
	t.Cleanup(func() { ormer.Engine.Where("name = ? or name = ?", good, bad).Delete(&Cert{}) })

	// Must not panic.
	jwks, err := GetJsonWebKeySet("")
	if err != nil {
		t.Fatalf("GetJsonWebKeySet returned error: %v", err)
	}

	var sawGood, sawBad bool
	for _, k := range jwks.Keys {
		if k.KeyID == good {
			sawGood = true
		}
		if k.KeyID == bad {
			sawBad = true
		}
	}
	if !sawGood {
		t.Errorf("valid cert %q missing from JWKS", good)
	}
	if sawBad {
		t.Errorf("non-PEM cert %q should have been skipped but is in JWKS", bad)
	}
}
