package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretRefResolvesEnvironmentAndFile(t *testing.T) {
	t.Setenv("SHADERFLOW_TEST_SECRET", "from-env")
	if got, err := (SecretRef{Env: "SHADERFLOW_TEST_SECRET"}).Resolve(); err != nil || got != "from-env" {
		t.Fatalf("env=%q %v", got, err)
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := (SecretRef{File: path}).Resolve(); err != nil || got != "from-file" {
		t.Fatalf("file=%q %v", got, err)
	}
}

func TestProductionResolvesSecretReferences(t *testing.T) {
	t.Setenv("ADMIN_HASH", "$2a$10$012345678901234567890uK7i14ZkmO.Bm65rNlf.ZXIvFKRm")
	t.Setenv("OIDC_SECRET", "secret")
	cfg := Config{Environment: "production", PublicURL: "https://app.example", AdminPasswordHashRef: SecretRef{Env: "ADMIN_HASH"}, TLSCert: "cert", TLSKey: "key", OIDC: OIDC{Issuer: "https://id.example", ClientID: "client", ClientSecretRef: SecretRef{Env: "OIDC_SECRET"}, RedirectURL: "https://app.example/callback"}}
	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatal(err)
	}
	if cfg.AdminPasswordHash == "" || cfg.OIDC.ClientSecret == "" {
		t.Fatalf("secrets unresolved: %#v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
