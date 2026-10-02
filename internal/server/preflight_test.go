package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestCertificate(t *testing.T, root string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "shaderflow.test"}, DNSNames: []string{"shaderflow.test"}, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(root, "cert.pem")
	keyPath := filepath.Join(root, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestPreflightChecksStaticTLSAndStorage(t *testing.T) {
	root := t.TempDir()
	static := filepath.Join(root, "web")
	if err := os.Mkdir(static, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "style.css", "admin/index.html", "player/index.html"} {
		path := filepath.Join(static, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cert, key := writeTestCertificate(t, root, time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	report := Preflight(Config{StaticDir: static, DataDir: filepath.Join(root, "data"), TLSCert: cert, TLSKey: key})
	if !report.Ready || len(report.Checks) != 5 {
		t.Fatalf("bad preflight: %#v", report)
	}
	if report.Certificate == nil || report.Certificate.DNSNames[0] != "shaderflow.test" {
		t.Fatalf("certificate facts missing: %#v", report.Certificate)
	}
	bad := Preflight(Config{StaticDir: filepath.Join(root, "missing"), DataDir: filepath.Join(root, "data2"), TLSCert: cert, TLSKey: key})
	if bad.Ready {
		t.Fatal("missing static directory accepted")
	}
}

func TestPreflightRejectsCertificateWithoutPublicHostname(t *testing.T) {
	root := t.TempDir()
	static := filepath.Join(root, "web")
	for _, name := range []string{"index.html", "style.css", "admin/index.html", "player/index.html"} {
		path := filepath.Join(static, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte("ok"), 0600)
	}
	cert, key := writeTestCertificate(t, root, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	report := Preflight(Config{PublicURL: "https://other.example", StaticDir: static, DataDir: filepath.Join(root, "data"), TLSCert: cert, TLSKey: key})
	if report.Ready {
		t.Fatal("certificate hostname mismatch accepted")
	}
}

func TestDevelopmentPreflightAllowsHTTPWithoutTLS(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"index.html", "style.css", "admin/index.html", "player/index.html"} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte("ok"), 0600)
	}
	report := Preflight(Config{Environment: "development", StaticDir: root, DataDir: filepath.Join(root, "data")})
	if !report.Ready {
		t.Fatalf("development HTTP preflight failed: %#v", report)
	}
}

func TestPreflightRejectsExpiredAndMismatchedTLS(t *testing.T) {
	root := t.TempDir()
	cert, key := writeTestCertificate(t, root, time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
	report := Preflight(Config{StaticDir: root, DataDir: filepath.Join(root, "data"), TLSCert: cert, TLSKey: key})
	if report.Ready {
		t.Fatal("expired certificate accepted")
	}
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	_, otherKey := writeTestCertificate(t, other, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	report = Preflight(Config{StaticDir: root, DataDir: filepath.Join(root, "data2"), TLSCert: cert, TLSKey: otherKey})
	if report.Ready {
		t.Fatal("mismatched TLS key accepted")
	}
}
