package httpclient

import (
	"net/http"
	"time"

	"github.com/meowrain/localsend-go/internal/pkg/security"
)

// GetClient returns an HTTP client with TLS configuration
// protocol: "http" or "https"
func GetClient(protocol string, timeout time.Duration) *http.Client {
	if protocol == "http" {
		return &http.Client{
			Timeout: timeout,
		}
	}

	// For HTTPS, use TLS configuration that accepts self-signed certificates
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:    security.GetClientTLSConfig(),
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		},
	}
}

// GetTransport returns an HTTP transport with TLS configuration
func GetTransport(protocol string) *http.Transport {
	if protocol == "http" {
		return &http.Transport{
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		}
	}

	// For HTTPS, use TLS configuration that accepts self-signed certificates
	return &http.Transport{
		TLSClientConfig:    security.GetClientTLSConfig(),
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: true,
	}
}

// GetBasicClient returns a simple HTTP client for quick requests
func GetBasicClient(protocol string) *http.Client {
	return GetClient(protocol, 60*time.Second)
}
