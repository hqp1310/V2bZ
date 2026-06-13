package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	"github.com/ZicBoard/ZicNode/common/reload"
	log "github.com/sirupsen/logrus"
)

const (
	domainRenewBefore = 30 * 24 * time.Hour
	ipRenewBefore     = 72 * time.Hour
)

type certMetadata struct {
	Target          string `json:"target"`
	Mode            string `json:"mode"`
	Source          string `json:"source"`
	SHA256          string `json:"sha256"`
	SHA256Hex       string `json:"sha256_hex,omitempty"`
	PublicKeySHA256 string `json:"public_key_sha256,omitempty"`
	NotAfter        int64  `json:"not_after"`
	UpdatedAt       int64  `json:"updated_at"`
}

func (c *Controller) renewCertTask(_ context.Context) error {
	if err := c.requestCertAndReport(true); err != nil {
		log.WithField("tag", c.tag).Info("renew cert error: ", err)
		return nil
	}
	return nil
}

func (c *Controller) requestCertAndReport(reloadOnChange bool) error {
	previous, _ := c.readCertMetadata()
	meta, err := c.requestCert()
	c.reportCertStatus(meta, err)
	if reloadOnChange && err == nil && certMetadataChanged(previous, meta) && c.server != nil && c.server.ReloadCh != nil {
		fields := log.Fields{
			"event":         "cert_auto_tls",
			"reason":        "fallback_cert_changed",
			"action":        "request_reload",
			"reload_reason": reload.ReasonCertMetadataChange,
			"tag":           c.tag,
		}
		details := map[string]string{}
		if meta != nil {
			fields["target"] = meta.Target
			fields["source"] = meta.Source
			fields["sha256_hex"] = shortHash(meta.SHA256Hex)
			fields["public_key_sha256"] = shortHash(meta.PublicKeySHA256)
			details["target"] = meta.Target
			details["source"] = meta.Source
			details["sha256_hex"] = shortHash(meta.SHA256Hex)
			details["public_key_sha256"] = shortHash(meta.PublicKeySHA256)
		}
		log.WithFields(fields).Error("cert metadata changed, request reload")
		select {
		case c.server.ReloadCh <- reload.Event{Reason: reload.ReasonCertMetadataChange, NodeTag: c.tag, Details: details}:
		default:
		}
	}
	return err
}

func (c *Controller) requestCert() (*certMetadata, error) {
	cert := c.info.Common.CertInfo
	cert.CertMode = strings.ToLower(strings.TrimSpace(cert.CertMode))
	cert.CertDomain = normalizeCertTarget(cert.CertDomain)
	if strings.TrimSpace(cert.Email) == "" {
		cert.Email = "contact@zicnet.vn"
	}

	switch cert.CertMode {
	case "none", "":
		return nil, nil
	case "file":
		if err := validateCertPaths(cert); err != nil {
			return basicCertMetadata(cert, "file"), err
		}
		if err := ensureCertFilesExist(cert); err != nil {
			return basicCertMetadata(cert, "file"), err
		}
		return metadataFromCertFile(cert, "file")
	case "dns", "http":
		return c.requestManagedCert(cert, cert.CertMode, managedCertSource(cert.CertMode, cert.CertDomain), false)
	case "auto":
		return c.requestAutoCert(cert)
	case "self":
		return c.requestSelfCert(cert)
	default:
		return basicCertMetadata(cert, ""), fmt.Errorf("unsupported certmode: %s", cert.CertMode)
	}
}

func (c *Controller) requestManagedCert(cert *panel.CertInfo, challengeMode, source string, allowSelfSigned bool) (*certMetadata, error) {
	if err := validateCertPaths(cert); err != nil {
		return basicCertMetadata(cert, source), err
	}
	if cert.CertDomain == "" {
		return basicCertMetadata(cert, source), fmt.Errorf("cert target is empty for certmode %s", cert.CertMode)
	}
	ready, _, err := certificateReady(cert, renewBefore(cert.CertDomain), allowSelfSigned)
	if err != nil {
		return basicCertMetadata(cert, source), err
	}
	if ready {
		meta, err := metadataFromCertFile(cert, source)
		if err == nil {
			_ = c.writeCertMetadata(meta)
		}
		return meta, err
	}

	x509Cert, err := issueLegoCert(cert, challengeMode)
	if err != nil {
		return basicCertMetadata(cert, source), fmt.Errorf("create lego cert error: %s", err)
	}
	meta := metadataFromCertificate(cert, x509Cert, source)
	_ = c.writeCertMetadata(meta)
	return meta, nil
}

func (c *Controller) requestAutoCert(cert *panel.CertInfo) (*certMetadata, error) {
	if err := validateCertPaths(cert); err != nil {
		return basicCertMetadata(cert, "auto"), err
	}
	if err := validateAutoCertTarget(cert.CertDomain); err != nil {
		return basicCertMetadata(cert, "auto"), err
	}

	challengeMode, source := autoCertChallenge(cert)
	ready, _, err := certificateReady(cert, renewBefore(cert.CertDomain), false)
	if err != nil {
		return basicCertMetadata(cert, source), err
	}
	if ready {
		meta, err := metadataFromCertFile(cert, source)
		if err == nil {
			_ = c.writeCertMetadata(meta)
		}
		return meta, err
	}

	x509Cert, issueErr := issueLegoCert(cert, challengeMode)
	if issueErr == nil {
		meta := metadataFromCertificate(cert, x509Cert, source)
		_ = c.writeCertMetadata(meta)
		return meta, nil
	}

	if !cert.SelfFallback {
		return basicCertMetadata(cert, source), fmt.Errorf("create lego cert error: %s", issueErr)
	}
	log.WithFields(log.Fields{
		"event":  "cert_auto_tls",
		"reason": "acme_failed",
		"action": "fallback_self_signed",
		"tag":    c.tag,
		"target": cert.CertDomain,
		"mode":   challengeMode,
		"err":    issueErr,
	}).Error("ACME cert issue failed, falling back to self-signed cert")
	return c.generateAndStoreSelfCert(cert, "fallback_self")
}

func (c *Controller) requestSelfCert(cert *panel.CertInfo) (*certMetadata, error) {
	if err := validateCertPaths(cert); err != nil {
		return basicCertMetadata(cert, "self"), err
	}
	if cert.CertDomain == "" {
		return basicCertMetadata(cert, "self"), fmt.Errorf("cert target is empty for certmode self")
	}
	ready, _, err := certificateReady(cert, domainRenewBefore, true)
	if err != nil {
		return basicCertMetadata(cert, "self"), err
	}
	if ready {
		meta, err := metadataFromCertFile(cert, "self")
		if err == nil {
			_ = c.writeCertMetadata(meta)
		}
		return meta, err
	}
	return c.generateAndStoreSelfCert(cert, "self")
}

func (c *Controller) generateAndStoreSelfCert(cert *panel.CertInfo, source string) (*certMetadata, error) {
	if err := generateSelfSslCertificate(cert.CertDomain, cert.CertFile, cert.KeyFile); err != nil {
		return basicCertMetadata(cert, source), fmt.Errorf("generate self cert error: %s", err)
	}
	x509Cert, err := loadCertificate(cert)
	if err != nil {
		return basicCertMetadata(cert, source), err
	}
	meta := metadataFromCertificate(cert, x509Cert, source)
	_ = c.writeCertMetadata(meta)
	return meta, nil
}

func issueLegoCert(cert *panel.CertInfo, challengeMode string) (*x509.Certificate, error) {
	legoCert := *cert
	legoCert.CertMode = challengeMode
	l, err := NewLego(&legoCert)
	if err != nil {
		return nil, fmt.Errorf("create lego object error: %s", err)
	}
	if err := l.CreateCert(); err != nil {
		return nil, err
	}
	return loadCertificate(cert)
}

func validateCertPaths(cert *panel.CertInfo) error {
	if cert.CertFile == "" || cert.KeyFile == "" {
		return fmt.Errorf("cert file path or key file path not exist")
	}
	return nil
}

func ensureCertFilesExist(cert *panel.CertInfo) error {
	if _, err := os.Stat(cert.CertFile); err != nil {
		return fmt.Errorf("cert file not found %s: %w", cert.CertFile, err)
	}
	if _, err := os.Stat(cert.KeyFile); err != nil {
		return fmt.Errorf("key file not found %s: %w", cert.KeyFile, err)
	}
	return nil
}

func certificateReady(cert *panel.CertInfo, minValidity time.Duration, allowSelfSigned bool) (bool, *x509.Certificate, error) {
	if err := ensureCertFilesExist(cert); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil, nil
		}
		return false, nil, err
	}
	x509Cert, err := loadCertificate(cert)
	if err != nil {
		return false, nil, nil
	}
	now := time.Now()
	if now.Before(x509Cert.NotBefore) || time.Until(x509Cert.NotAfter) <= minValidity {
		return false, x509Cert, nil
	}
	if cert.CertDomain != "" {
		if err := x509Cert.VerifyHostname(cert.CertDomain); err != nil {
			return false, x509Cert, nil
		}
	}
	if !allowSelfSigned && isSelfSignedCertificate(x509Cert) {
		return false, x509Cert, nil
	}
	return true, x509Cert, nil
}

func loadCertificate(cert *panel.CertInfo) (*x509.Certificate, error) {
	data, err := os.ReadFile(cert.CertFile)
	if err != nil {
		return nil, fmt.Errorf("read cert file error: %s", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode cert file error: missing pem block")
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse cert file error: %s", err)
	}
	return x509Cert, nil
}

func isSelfSignedCertificate(cert *x509.Certificate) bool {
	if cert == nil || !bytes.Equal(cert.RawSubject, cert.RawIssuer) {
		return false
	}
	return cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

func renewBefore(target string) time.Duration {
	if net.ParseIP(target) != nil {
		return ipRenewBefore
	}
	return domainRenewBefore
}

func validateAutoCertTarget(target string) error {
	if target == "" {
		return fmt.Errorf("auto tls requires host or tls_settings.server_name")
	}
	if ip := net.ParseIP(target); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("auto tls target %s is not a public ip", target)
		}
		return nil
	}
	if strings.EqualFold(target, "localhost") || !strings.Contains(target, ".") {
		return fmt.Errorf("auto tls target %s is not a public domain", target)
	}
	if strings.ContainsAny(target, `/\\`) {
		return fmt.Errorf("auto tls target %s is not a valid hostname", target)
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast()
}

func normalizeCertTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		value = parsed.Hostname()
	} else if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	return strings.TrimSpace(value)
}

func autoCertChallenge(cert *panel.CertInfo) (string, string) {
	if net.ParseIP(cert.CertDomain) != nil {
		return "http", "acme_ip"
	}
	if strings.TrimSpace(cert.Provider) != "" && len(cert.DNSEnv) > 0 {
		return "dns", "acme_dns"
	}
	return "http", "acme_http"
}

func managedCertSource(mode, target string) string {
	if mode == "dns" {
		return "acme_dns"
	}
	if net.ParseIP(target) != nil {
		return "acme_ip"
	}
	return "acme_http"
}

func metadataFromCertFile(cert *panel.CertInfo, source string) (*certMetadata, error) {
	x509Cert, err := loadCertificate(cert)
	if err != nil {
		return basicCertMetadata(cert, source), err
	}
	return metadataFromCertificate(cert, x509Cert, source), nil
}

func metadataFromCertificate(cert *panel.CertInfo, x509Cert *x509.Certificate, source string) *certMetadata {
	return &certMetadata{
		Target:          cert.CertDomain,
		Mode:            cert.CertMode,
		Source:          source,
		SHA256:          certSHA256(x509Cert),
		SHA256Hex:       certSHA256Hex(x509Cert),
		PublicKeySHA256: certPublicKeySHA256(x509Cert),
		NotAfter:        x509Cert.NotAfter.Unix(),
		UpdatedAt:       time.Now().Unix(),
	}
}

func basicCertMetadata(cert *panel.CertInfo, source string) *certMetadata {
	if cert == nil {
		return nil
	}
	return &certMetadata{
		Target:    cert.CertDomain,
		Mode:      cert.CertMode,
		Source:    source,
		UpdatedAt: time.Now().Unix(),
	}
}

func certSHA256(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

func certSHA256Hex(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func certPublicKeySHA256(cert *x509.Certificate) string {
	publicKeyDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(publicKeyDER)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (c *Controller) certMetadataPath() string {
	return filepath.Join("/etc/zicnode", fmt.Sprintf("node-%d.certmeta.json", c.info.Id))
}

func (c *Controller) writeCertMetadata(meta *certMetadata) error {
	if meta == nil {
		return nil
	}
	if err := checkPath(c.certMetadataPath()); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.certMetadataPath(), data, 0644)
}

func (c *Controller) readCertMetadata() (*certMetadata, error) {
	data, err := os.ReadFile(c.certMetadataPath())
	if err != nil {
		return nil, err
	}
	var meta certMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func shortHash(value string) string {
	const maxLen = 12
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}
func certMetadataChanged(previous, next *certMetadata) bool {
	if next == nil {
		return false
	}
	if previous == nil {
		return true
	}
	return previous.Target != next.Target ||
		previous.Mode != next.Mode ||
		previous.Source != next.Source ||
		previous.SHA256 != next.SHA256 ||
		previous.SHA256Hex != next.SHA256Hex ||
		previous.PublicKeySHA256 != next.PublicKeySHA256 ||
		previous.NotAfter != next.NotAfter
}

func (c *Controller) reportCertStatus(meta *certMetadata, issueErr error) {
	if c.apiClient == nil || meta == nil {
		return
	}
	status := "ok"
	errorMessage := ""
	if issueErr != nil {
		status = "error"
		errorMessage = issueErr.Error()
	}
	report := &panel.CertReport{
		Status:          status,
		Target:          meta.Target,
		Mode:            meta.Mode,
		Source:          meta.Source,
		SHA256:          meta.SHA256,
		SHA256Hex:       meta.SHA256Hex,
		PublicKeySHA256: meta.PublicKeySHA256,
		NotAfter:        meta.NotAfter,
		Error:           errorMessage,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.apiClient.ReportCertStatus(ctx, report); err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Warn("Report cert status failed")
	}
}

func generateSelfSslCertificate(domain, certPath, keyPath string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		Version:      3,
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName: domain,
		},
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(30, 0, 0),
	}
	if ip := net.ParseIP(domain); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{domain}
	}
	cert, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return err
	}
	if err := checkPath(certPath); err != nil {
		return err
	}
	f, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	err = pem.Encode(f, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
	if err != nil {
		return err
	}
	if err := checkPath(keyPath); err != nil {
		return err
	}
	f, err = os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	err = pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err != nil {
		return err
	}
	return nil
}
