package node

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
)

func TestCertSHAFormats(t *testing.T) {
	cert := testCertificate(t)

	derSum := sha256.Sum256(cert.Raw)
	wantHex := hex.EncodeToString(derSum[:])
	wantColon := colonUpper(wantHex)

	publicKeyDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicKeySum := sha256.Sum256(publicKeyDER)
	wantPublicKey := base64.StdEncoding.EncodeToString(publicKeySum[:])

	if got := certSHA256(cert); got != wantColon {
		t.Fatalf("certSHA256 got %q, want %q", got, wantColon)
	}
	if got := certSHA256Hex(cert); got != wantHex {
		t.Fatalf("certSHA256Hex got %q, want %q", got, wantHex)
	}
	if got := certPublicKeySHA256(cert); got != wantPublicKey {
		t.Fatalf("certPublicKeySHA256 got %q, want %q", got, wantPublicKey)
	}
}

func TestMetadataFromCertificateIncludesPinHashes(t *testing.T) {
	cert := testCertificate(t)
	meta := metadataFromCertificate(&panel.CertInfo{CertDomain: "example.com", CertMode: "self"}, cert, "self")

	if meta.SHA256 == "" || meta.SHA256Hex == "" || meta.PublicKeySHA256 == "" {
		t.Fatalf("metadata missing hashes: %#v", meta)
	}
	if meta.Target != "example.com" || meta.Mode != "self" || meta.Source != "self" {
		t.Fatalf("unexpected metadata identity fields: %#v", meta)
	}
	if !certMetadataChanged(&certMetadata{SHA256: meta.SHA256}, meta) {
		t.Fatal("metadata change should include sha256_hex/public_key_sha256 differences")
	}
}

func testCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Unix(1700000000, 0),
		NotAfter:     time.Unix(1800000000, 0),
		DNSNames:     []string{"example.com"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func colonUpper(hexValue string) string {
	parts := make([]string, 0, len(hexValue)/2)
	for i := 0; i < len(hexValue); i += 2 {
		parts = append(parts, strings.ToUpper(hexValue[i:i+2]))
	}
	return strings.Join(parts, ":")
}

func TestAutoCertChallengeUsesDNSForDomainTarget(t *testing.T) {
	cert := &panel.CertInfo{
		CertDomain: "www.apple.com",
		Provider:   "cloudflare",
		DNSEnv: map[string]string{
			"CF_DNS_API_TOKEN": "token",
		},
	}

	mode, source := autoCertChallenge(cert)
	if mode != "dns" || source != "acme_dns" {
		t.Fatalf("autoCertChallenge got mode=%q source=%q, want dns/acme_dns", mode, source)
	}
}

func TestAutoCertChallengeKeepsIPTargetOnHTTPPath(t *testing.T) {
	cert := &panel.CertInfo{
		CertDomain: "8.8.8.8",
		Provider:   "cloudflare",
		DNSEnv: map[string]string{
			"CF_DNS_API_TOKEN": "token",
		},
	}

	mode, source := autoCertChallenge(cert)
	if mode != "http" || source != "acme_ip" {
		t.Fatalf("autoCertChallenge got mode=%q source=%q, want http/acme_ip", mode, source)
	}
}
