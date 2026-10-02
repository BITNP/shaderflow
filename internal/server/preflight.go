package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"shaderflow/internal/archive"
)

type PreflightCheck struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
type CertificateFacts struct {
	Subject   string    `json:"subject"`
	DNSNames  []string  `json:"dnsNames,omitempty"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
}
type PreflightReport struct {
	Ready       bool              `json:"ready"`
	Checks      []PreflightCheck  `json:"checks"`
	Certificate *CertificateFacts `json:"certificate,omitempty"`
}

func Preflight(cfg Config) PreflightReport {
	report := PreflightReport{Ready: true}
	add := func(name string, err error) {
		check := PreflightCheck{Name: name, OK: err == nil}
		if err != nil {
			check.Error = err.Error()
			report.Ready = false
		}
		report.Checks = append(report.Checks, check)
	}
	staticErr := error(nil)
	for _, name := range []string{"index.html", "style.css", "admin/index.html", "player/index.html"} {
		if _, err := os.Stat(filepath.Join(cfg.StaticDir, name)); err != nil {
			staticErr = errors.Join(staticErr, err)
		}
	}
	add("static.assets", staticErr)
	if cfg.TLSCert == "" && cfg.TLSKey == "" && cfg.Environment != "production" {
		add("tls.optional", nil)
		add("archive.writable", archive.NewFilesystem(cfg.DataDir).Health())
		return report
	}
	_, certFileErr := os.Stat(cfg.TLSCert)
	add("tls.certificate", certFileErr)
	_, keyFileErr := os.Stat(cfg.TLSKey)
	add("tls.key", keyFileErr)
	var pairErr error
	if certFileErr == nil && keyFileErr == nil {
		pair, pairLoadErr := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		pairErr = pairLoadErr
		if pairLoadErr == nil && len(pair.Certificate) > 0 {
			cert, parseErr := x509.ParseCertificate(pair.Certificate[0])
			if parseErr != nil {
				pairErr = parseErr
			} else {
				report.Certificate = &CertificateFacts{Subject: cert.Subject.String(), DNSNames: cert.DNSNames, NotBefore: cert.NotBefore, NotAfter: cert.NotAfter}
				now := time.Now()
				if now.Before(cert.NotBefore) {
					pairErr = errors.New("TLS certificate is not valid yet")
				}
				if now.After(cert.NotAfter) {
					pairErr = errors.New("TLS certificate has expired")
				}
				if cfg.PublicURL != "" {
					public, parseURL := url.Parse(cfg.PublicURL)
					if parseURL != nil || public.Hostname() == "" {
						pairErr = errors.New("invalid publicUrl")
					} else if verifyErr := cert.VerifyHostname(public.Hostname()); verifyErr != nil {
						pairErr = verifyErr
					}
				}
			}
		}
	} else {
		pairErr = errors.New("TLS certificate and key files unavailable")
	}
	add("tls.keypair", pairErr)
	add("archive.writable", archive.NewFilesystem(cfg.DataDir).Health())
	return report
}
