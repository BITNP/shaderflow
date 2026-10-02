package server

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type SecretRef struct {
	Env  string `json:"env,omitempty"`
	File string `json:"file,omitempty"`
}

func (r SecretRef) Resolve() (string, error) {
	if r.Env != "" && r.File != "" {
		return "", errors.New("secret reference must select env or file")
	}
	if r.Env != "" {
		value, ok := os.LookupEnv(r.Env)
		if !ok || value == "" {
			return "", fmt.Errorf("secret environment variable %s is empty", r.Env)
		}
		return value, nil
	}
	if r.File != "" {
		value, err := os.ReadFile(r.File)
		if err != nil {
			return "", err
		}
		result := strings.TrimSpace(string(value))
		if result == "" {
			return "", errors.New("secret file is empty")
		}
		return result, nil
	}
	return "", nil
}
func (cfg *Config) ResolveSecrets() error {
	if cfg.AdminPasswordHash == "" {
		value, err := cfg.AdminPasswordHashRef.Resolve()
		if err != nil {
			return fmt.Errorf("adminPasswordHashRef: %w", err)
		}
		cfg.AdminPasswordHash = value
	}
	if cfg.OIDC.ClientSecret == "" {
		value, err := cfg.OIDC.ClientSecretRef.Resolve()
		if err != nil {
			return fmt.Errorf("oidc.clientSecretRef: %w", err)
		}
		cfg.OIDC.ClientSecret = value
	}
	return nil
}
