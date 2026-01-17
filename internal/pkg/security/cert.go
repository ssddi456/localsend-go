package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"time"
)

// CertificateContext represents the security context for HTTPS communication
type CertificateContext struct {
	PrivateKey      *rsa.PrivateKey
	Certificate     *x509.Certificate
	CertPEM         []byte
	PrivateKeyPEM   []byte
	CertificateHash string // SHA-256 hash of certificate
}

// GenerateSelfSignedCert generates a self-signed certificate for LocalSend
func GenerateSelfSignedCert() (*CertificateContext, error) {
	// Generate RSA key pair (2048 bits like LocalSend)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// Certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour) // 10 years

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "LocalSend User",
			Organization: []string{""},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Calculate SHA-256 hash of certificate
	hash := sha256.Sum256(certDER)
	certHash := hex.EncodeToString(hash[:])

	return &CertificateContext{
		PrivateKey:      privateKey,
		Certificate:     cert,
		CertPEM:         certPEM,
		PrivateKeyPEM:   privateKeyPEM,
		CertificateHash: certHash,
	}, nil
}

// GetTLSConfig returns a TLS configuration for the server
func (c *CertificateContext) GetTLSConfig() (*tls.Config, error) {
	cert, err := tls.X509KeyPair(c.CertPEM, c.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// GetClientTLSConfig returns a TLS configuration for the client
// It accepts self-signed certificates
func GetClientTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // Accept self-signed certificates like LocalSend does
		MinVersion:         tls.VersionTLS12,
	}
}

// VerifyCertificate verifies a certificate against expected hash
func VerifyCertificate(cert *x509.Certificate, expectedHash string) bool {
	hash := sha256.Sum256(cert.Raw)
	certHash := hex.EncodeToString(hash[:])
	return certHash == expectedHash
}

// ExtractPublicKeyPEM extracts the public key from certificate in PEM format
func ExtractPublicKeyPEM(cert *x509.Certificate) ([]byte, error) {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return nil, err
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	return pubKeyPEM, nil
}
