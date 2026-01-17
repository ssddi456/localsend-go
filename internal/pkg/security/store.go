package security

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"

	"github.com/meowrain/localsend-go/internal/utils/logger"
)

// StoredSecurityContext represents persisted security data
type StoredSecurityContext struct {
	PrivateKeyPEM   string `json:"private_key"`
	CertificatePEM  string `json:"certificate"`
	CertificateHash string `json:"certificate_hash"`
}

var (
	securityContext *CertificateContext
	mu              sync.RWMutex
	configDir       = ".localsend"
	certFile        = "security.json"
)

// Initialize initializes or loads the security context
func Initialize() error {
	mu.Lock()
	defer mu.Unlock()

	// Try to load existing certificate
	stored, err := loadStoredContext()
	if err == nil && stored != nil {
		// Parse stored certificate
		ctx, err := parseStoredContext(stored)
		if err == nil {
			securityContext = ctx
			logger.Info("Loaded existing certificate")
			logger.Debugf("Certificate hash: %s", ctx.CertificateHash)
			return nil
		}
		logger.Warnf("Failed to parse stored certificate: %v", err)
	}

	// Generate new certificate
	logger.Info("Generating new self-signed certificate...")
	ctx, err := GenerateSelfSignedCert()
	if err != nil {
		return err
	}

	// Save certificate
	err = saveContext(ctx)
	if err != nil {
		logger.Warnf("Failed to save certificate: %v", err)
	}

	securityContext = ctx
	logger.Info("Generated new certificate")
	logger.Debugf("Certificate hash: %s", ctx.CertificateHash)

	return nil
}

// GetSecurityContext returns the current security context
func GetSecurityContext() *CertificateContext {
	mu.RLock()
	defer mu.RUnlock()
	return securityContext
}

// ResetSecurityContext generates a new certificate and saves it
func ResetSecurityContext() error {
	mu.Lock()
	defer mu.Unlock()

	logger.Info("Resetting security context...")
	ctx, err := GenerateSelfSignedCert()
	if err != nil {
		return err
	}

	err = saveContext(ctx)
	if err != nil {
		logger.Warnf("Failed to save new certificate: %v", err)
	}

	securityContext = ctx
	logger.Info("Reset complete. New certificate generated")
	logger.Debugf("Certificate hash: %s", ctx.CertificateHash)

	return nil
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(homeDir, configDir)
	err = os.MkdirAll(configPath, 0700)
	if err != nil {
		return "", err
	}

	return filepath.Join(configPath, certFile), nil
}

func loadStoredContext() (*StoredSecurityContext, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var stored StoredSecurityContext
	err = json.Unmarshal(data, &stored)
	if err != nil {
		return nil, err
	}

	return &stored, nil
}

func saveContext(ctx *CertificateContext) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	stored := StoredSecurityContext{
		PrivateKeyPEM:   string(ctx.PrivateKeyPEM),
		CertificatePEM:  string(ctx.CertPEM),
		CertificateHash: ctx.CertificateHash,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func parseStoredContext(stored *StoredSecurityContext) (*CertificateContext, error) {
	// Parse private key
	block, _ := pem.Decode([]byte(stored.PrivateKeyPEM))
	if block == nil {
		return nil, os.ErrInvalid
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Parse certificate
	certBlock, _ := pem.Decode([]byte(stored.CertificatePEM))
	if certBlock == nil {
		return nil, os.ErrInvalid
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}

	return &CertificateContext{
		PrivateKey:      privateKey,
		Certificate:     cert,
		CertPEM:         []byte(stored.CertificatePEM),
		PrivateKeyPEM:   []byte(stored.PrivateKeyPEM),
		CertificateHash: stored.CertificateHash,
	}, nil
}
