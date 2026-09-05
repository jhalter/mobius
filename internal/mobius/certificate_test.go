package mobius

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func certificatePair(t *testing.T, serial int64) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func TestCertificateManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	certPEM, keyPEM := certificatePair(t, 1)
	nextCertPEM, nextKeyPEM := certificatePair(t, 2)
	certPath, keyPath := filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key")
	writePair := func(cert, key []byte) {
		t.Helper()
		require.NoError(t, os.WriteFile(certPath, cert, 0600))
		require.NoError(t, os.WriteFile(keyPath, key, 0600))
	}
	writePair(certPEM, keyPEM)

	m, err := NewCertificateManager(certPath, keyPath, logger)
	require.NoError(t, err)
	initial, err := m.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), initial.Leaf.SerialNumber.Int64())

	writePair(nextCertPEM, nextKeyPEM)
	beforeReload, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "localhost"})
	require.NoError(t, err)
	assert.Same(t, initial, beforeReload, "replacement files alone do not publish a certificate")
	for range 3 {
		require.NoError(t, m.Reload())
		current, err := m.GetCertificate(nil)
		require.NoError(t, err)
		assert.Equal(t, int64(2), current.Leaf.SerialNumber.Int64())
	}
	assert.Equal(t, int64(1), initial.Leaf.SerialNumber.Int64(), "previous snapshots remain immutable")

	// A handshake callback must not access the filesystem.
	require.NoError(t, os.Remove(certPath))
	require.NoError(t, os.Remove(keyPath))
	current, err := m.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), current.Leaf.SerialNumber.Int64())
}

func TestCertificateManagerFailureRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	certPEM, keyPEM := certificatePair(t, 1)
	nextCertPEM, nextKeyPEM := certificatePair(t, 2)
	for _, failure := range []string{"missing certificate", "missing key", "malformed certificate", "malformed key", "mismatched key"} {
		t.Run(failure, func(t *testing.T) {
			certPath, keyPath := filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key")
			require.NoError(t, os.WriteFile(certPath, certPEM, 0600))
			require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))
			m, err := NewCertificateManager(certPath, keyPath, logger)
			require.NoError(t, err)
			initial, err := m.GetCertificate(nil)
			require.NoError(t, err)

			switch failure {
			case "missing certificate":
				require.NoError(t, os.Remove(certPath))
			case "missing key":
				require.NoError(t, os.Remove(keyPath))
			case "malformed certificate":
				require.NoError(t, os.WriteFile(certPath, []byte("invalid PEM"), 0600))
			case "malformed key":
				require.NoError(t, os.WriteFile(keyPath, []byte("invalid PEM"), 0600))
			case "mismatched key":
				require.NoError(t, os.WriteFile(keyPath, nextKeyPEM, 0600))
			}
			failed, err := NewCertificateManager(certPath, keyPath, logger)
			require.Error(t, err, "startup must reject an invalid pair")
			assert.Nil(t, failed)
			for range 2 {
				require.Error(t, m.Reload())
				current, err := m.GetCertificate(nil)
				require.NoError(t, err)
				assert.Same(t, initial, current, "failed reload must preserve the active pair")
			}

			require.NoError(t, os.WriteFile(certPath, nextCertPEM, 0600))
			require.NoError(t, os.WriteFile(keyPath, nextKeyPEM, 0600))
			require.NoError(t, m.Reload())
			current, err := m.GetCertificate(nil)
			require.NoError(t, err)
			assert.Equal(t, int64(2), current.Leaf.SerialNumber.Int64())
		})
	}
}
