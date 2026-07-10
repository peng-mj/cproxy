package cert

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/peng-mj/scproxy/internal/logging"
)

// Manager manages domain leaf certificates: caches them in memory and on disk,
// and provides a GetCertificate callback for tls.Config.
type Manager struct {
	ca      *CA
	certDir string
	cache   map[string]*tls.Certificate
	mu      sync.RWMutex
	logger  *logging.Logger
}

// NewManager creates a certificate manager, loading or generating the root CA.
func NewManager(certDir string, logger *logging.Logger) (*Manager, error) {
	ca, err := LoadOrCreateCA(certDir, logger)
	if err != nil {
		return nil, err
	}
	return &Manager{
		ca:      ca,
		certDir: certDir,
		cache:   make(map[string]*tls.Certificate),
		logger:  logger,
	}, nil
}

// TLSConfig returns a *tls.Config using SNI-based certificate selection.
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

// GetCertificate returns a TLS certificate for the requested SNI domain.
// Checks memory cache → disk cache → generates and caches a new cert.
func (m *Manager) GetCertificate(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.ToLower(strings.TrimSpace(hi.ServerName))
	if domain == "" {
		return nil, fmt.Errorf("no SNI provided by client")
	}

	m.mu.RLock()
	if cert, ok := m.cache[domain]; ok {
		m.mu.RUnlock()
		return cert, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if cert, ok := m.cache[domain]; ok {
		return cert, nil
	}

	cert, err := m.loadOrSign(domain)
	if err != nil {
		return nil, err
	}
	m.cache[domain] = cert
	return cert, nil
}

// PreGenerate generates and caches certificates for all given domains at startup.
func (m *Manager) PreGenerate(domains []string) {
	for _, d := range domains {
		domain := strings.ToLower(d)
		m.mu.RLock()
		exists := false
		if _, ok := m.cache[domain]; ok {
			exists = true
		}
		m.mu.RUnlock()
		if exists {
			continue
		}

		m.mu.Lock()
		if _, ok := m.cache[domain]; !ok {
			cert, err := m.loadOrSign(domain)
			if err != nil {
				m.logger.Error("Failed to pre-generate certificate", "domain", domain, "error", err)
				m.mu.Unlock()
				continue
			}
			m.cache[domain] = cert
			m.logger.Info("Certificate pre-generated", "domain", domain)
		}
		m.mu.Unlock()
	}
}

// CACertPath returns the path to the root CA certificate file.
func (m *Manager) CACertPath() string {
	return filepath.Join(m.certDir, caCertFile)
}

// loadOrSign loads a domain cert from disk or signs a new one. Caller must hold m.mu.
func (m *Manager) loadOrSign(domain string) (*tls.Certificate, error) {
	certPath := filepath.Join(m.certDir, domain+".crt")
	keyPath := filepath.Join(m.certDir, domain+".key")

	if fileExists(certPath) && fileExists(keyPath) {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err == nil {
			m.logger.Debug("Domain certificate loaded from disk", "domain", domain)
			return &cert, nil
		}
		m.logger.Warn("Failed to load domain cert from disk, regenerating", "domain", domain, "error", err)
	}

	certDER, leafKey, err := m.ca.SignDomainCert(domain)
	if err != nil {
		return nil, err
	}

	if err := m.saveLeafCert(certPath, keyPath, certDER, leafKey); err != nil {
		m.logger.Warn("Failed to save domain cert to disk", "domain", domain, "error", err)
	}

	leafCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse leaf cert: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER, m.ca.CertDER()},
		PrivateKey:  leafKey,
		Leaf:        leafCert,
	}
	m.logger.Info("Domain certificate signed", "domain", domain)
	return tlsCert, nil
}

func (m *Manager) saveLeafCert(certPath, keyPath string, certDER []byte, key *rsa.PrivateKey) error {
	if err := os.MkdirAll(m.certDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644); err != nil {
		return err
	}
	return os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
}
