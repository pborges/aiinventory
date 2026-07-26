// Package tlscert generates and caches a self-signed TLS certificate for
// aiinventory's optional TLS_ENABLED mode — needed for camera access over
// HTTPS from a phone on the LAN, since iOS requires a secure context for
// getUserMedia on anything other than localhost.
package tlscert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	settingCert = "tls_cert"
	settingKey  = "tls_key"
)

// SettingsStore is the narrow slice of store.Store this package needs —
// the same generic key/value settings table SESSION_SECRET is cached in.
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

// LoadOrGenerate returns a cached certificate from settings if one exists
// and hasn't expired, otherwise generates a new self-signed one and
// persists it so it survives restarts (avoiding re-triggering the
// browser's "untrusted certificate" prompt on every boot).
func LoadOrGenerate(ctx context.Context, s SettingsStore) (tls.Certificate, error) {
	if cert, ok := loadCached(ctx, s); ok {
		return cert, nil
	}

	certPEM, keyPEM, err := generate()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed certificate: %w", err)
	}
	if err := s.SetSetting(ctx, settingCert, string(certPEM)); err != nil {
		return tls.Certificate{}, err
	}
	if err := s.SetSetting(ctx, settingKey, string(keyPEM)); err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

func loadCached(ctx context.Context, s SettingsStore) (tls.Certificate, bool) {
	certPEM, certOK, err := s.GetSetting(ctx, settingCert)
	if err != nil || !certOK || certPEM == "" {
		return tls.Certificate{}, false
	}
	keyPEM, keyOK, err := s.GetSetting(ctx, settingKey)
	if err != nil || !keyOK || keyPEM == "" {
		return tls.Certificate{}, false
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return tls.Certificate{}, false
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil || time.Now().After(leaf.NotAfter) {
		return tls.Certificate{}, false
	}
	return cert, true
}

// generate creates a fresh self-signed ECDSA certificate covering
// localhost, loopback, and every non-loopback IPv4 address currently
// assigned to this machine — so it validates when a phone connects via
// the LAN IP, not just localhost.
func generate() (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "aiinventory (self-signed)"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           localIPs(),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, nil
}

func localIPs() []net.IP {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			ips = append(ips, ip4)
		}
	}
	return ips
}
