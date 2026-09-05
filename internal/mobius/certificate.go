package mobius

import (
	"crypto/tls"
	"log/slog"
	"sync"
	"sync/atomic"
)

// CertificateManager reloads a TLS certificate pair without changing the listeners or
// established connections that use it. It must be constructed with NewCertificateManager.
type CertificateManager struct {
	certPath string
	keyPath  string
	logger   *slog.Logger

	reloadMu sync.Mutex
	current  atomic.Pointer[tls.Certificate]
}

func NewCertificateManager(certPath, keyPath string, logger *slog.Logger) (*CertificateManager, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	m := &CertificateManager{certPath: certPath, keyPath: keyPath, logger: logger}
	m.current.Store(&cert)
	return m, nil
}

// GetCertificate serves the current in-memory certificate for every ClientHello, including
// clients without SNI. The returned certificate must not be modified by callers.
func (m *CertificateManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.current.Load(), nil
}

// Reload publishes a replacement only after the entire pair has loaded successfully.
// Serialize both loading and publication so an older load cannot overwrite a newer one.
func (m *CertificateManager) Reload() error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	cert, err := tls.LoadX509KeyPair(m.certPath, m.keyPath)
	if err != nil {
		return err
	}
	m.current.Store(&cert)
	m.logger.Info("TLS certificate reloaded")
	return nil
}
