package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CertificateGenerator handles TLS certificate generation
type CertificateGenerator struct {
	certDir string
}

// NewCertificateGenerator creates a new certificate generator
func NewCertificateGenerator(certDir string) *CertificateGenerator {
	return &CertificateGenerator{
		certDir: certDir,
	}
}

// GenerateSelfSignedCertificate generates a self-signed certificate for the given hostnames
func (g *CertificateGenerator) GenerateSelfSignedCertificate(hostnames []string) (certFile, keyFile string, err error) {
	// Ensure certificate directory exists
	if err := os.MkdirAll(g.certDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create certificate directory: %w", err)
	}

	// Generate private key with 4096-bit RSA for enhanced security
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"NoiseFS"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(3 * 365 * 24 * time.Hour), // 3 years
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{},
		DNSNames:     []string{},
	}

	// Add hostnames to certificate
	for _, hostname := range hostnames {
		if ip := net.ParseIP(hostname); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, hostname)
		}
	}

	// Add localhost IPs by default
	template.IPAddresses = append(template.IPAddresses, net.IPv4(127, 0, 0, 1))
	template.IPAddresses = append(template.IPAddresses, net.IPv6loopback)

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Define file paths
	certFile = filepath.Join(g.certDir, "server.crt")
	keyFile = filepath.Join(g.certDir, "server.key")

	// Save certificate
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return "", "", fmt.Errorf("failed to write certificate: %w", err)
	}

	// Save private key
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}); err != nil {
		return "", "", fmt.Errorf("failed to write private key: %w", err)
	}

	// Set secure permissions
	if err := os.Chmod(keyFile, 0600); err != nil {
		return "", "", fmt.Errorf("failed to set key file permissions: %w", err)
	}

	return certFile, keyFile, nil
}

// LoadOrGenerateCertificate loads existing certificate or generates a new one
func (g *CertificateGenerator) LoadOrGenerateCertificate(hostnames []string) (certFile, keyFile string, err error) {
	certFile = filepath.Join(g.certDir, "server.crt")
	keyFile = filepath.Join(g.certDir, "server.key")

	// Check if certificate files exist and are valid
	if g.certificateExists(certFile, keyFile) {
		if g.certificateValid(certFile, keyFile, hostnames) {
			return certFile, keyFile, nil
		}
	}

	// Generate new certificate
	return g.GenerateSelfSignedCertificate(hostnames)
}

// certificateExists checks if certificate files exist
func (g *CertificateGenerator) certificateExists(certFile, keyFile string) bool {
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)
	return certErr == nil && keyErr == nil
}

// certificateValid checks if the certificate is valid and covers the required hostnames
func (g *CertificateGenerator) certificateValid(certFile, keyFile string, hostnames []string) bool {
	// Load certificate
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return false
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return false
	}

	// Check if certificate is still valid (not expired)
	if time.Now().After(x509Cert.NotAfter) {
		return false
	}

	// Check if certificate covers all required hostnames
	for _, hostname := range hostnames {
		if ip := net.ParseIP(hostname); ip != nil {
			// Check IP addresses
			found := false
			for _, certIP := range x509Cert.IPAddresses {
				if certIP.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			// Check DNS names
			found := false
			for _, dnsName := range x509Cert.DNSNames {
				if dnsName == hostname {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// GetDefaultCertificateDir returns the default directory for storing certificates
func GetDefaultCertificateDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".noisefs", "certs"), nil
}