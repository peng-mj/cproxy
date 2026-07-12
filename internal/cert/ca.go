// Package cert provides self-signed root CA generation and domain
// certificate signing for the HTTPS reverse proxy.
package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/peng-mj/scproxy/internal/logging"
)

const (
	caCertFile = "ca.crt"
	caKeyFile  = "ca.key"

	caValidity   = 10 * 365 * 24 * time.Hour // 10 years
	leafValidity = 365 * 24 * time.Hour      // 1 year
	rsaKeyBits   = 2048
)

// CA represents a root certificate authority used to sign domain leaf certs.
type CA struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	certDER []byte
}

// LoadOrCreateCA loads an existing root CA from certDir, or generates a new
// self-signed root CA if the files don't exist.
func LoadOrCreateCA(certDir string, logger *logging.Logger) (*CA, error) {
	certPath := filepath.Join(certDir, caCertFile)
	keyPath := filepath.Join(certDir, caKeyFile)

	if fileExists(certPath) && fileExists(keyPath) {
		ca, err := loadCA(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA: %w", err)
		}
		logger.Info("Root CA loaded", "dir", certDir, "subject", ca.Cert.Subject.CommonName)
		return ca, nil
	}

	logger.Info("Root CA not found, generating new self-signed root CA", "dir", certDir)
	ca, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA: %w", err)
	}
	if err := saveCA(ca, certPath, keyPath); err != nil {
		return nil, fmt.Errorf("failed to save CA: %w", err)
	}
	logger.Info("Root CA generated and saved", "cert", certPath, "key", keyPath, "subject", ca.Cert.Subject.CommonName)
	return ca, nil
}

// SignDomainCert signs a leaf certificate for the given domain. Returns the
// DER-encoded certificate bytes and the leaf private key.
func (ca *CA) SignDomainCert(domain string) ([]byte, *rsa.PrivateKey, error) {
	leafKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"scproxy"},
		},
		DNSNames:              []string{domain},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(leafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, &leafKey.PublicKey, ca.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign leaf certificate: %w", err)
	}
	return certDER, leafKey, nil
}

// CertDER returns the DER bytes of the CA certificate (used for chaining in leaf certs).
func (ca *CA) CertDER() []byte { return ca.certDER }

func generateCA() (*CA, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "scproxy Root CA",
			Organization: []string{"scproxy"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	return &CA{Cert: caCert, Key: caKey, certDER: certDER}, nil
}

func loadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA key: %w", err)
	}

	return &CA{Cert: caCert, Key: caKey, certDER: block.Bytes}, nil
}

func saveCA(ca *CA, certPath, keyPath string) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certDER})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write CA cert: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(ca.Key)})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write CA key: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
