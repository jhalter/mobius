package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/internal/mobius"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

type reloadCertificatePair struct {
	certPEM, keyPEM []byte
	cert            *x509.Certificate
}

func newReloadCertificatePair(t *testing.T, serial int64) reloadCertificatePair {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return reloadCertificatePair{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		cert:    cert,
	}
}

func (p reloadCertificatePair) write(t *testing.T, certPath, keyPath string) {
	t.Helper()
	require.NoError(t, os.WriteFile(certPath, p.certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, p.keyPEM, 0600))
}

// Start the production TLS service paths on already-bound ephemeral listeners. Neither
// listener is closed until cleanup, so every certificate check uses the same addresses.
func startReloadTLSServices(t *testing.T, manager *mobius.CertificateManager) (*hotline.Server, []string) {
	t.Helper()
	srv, err := hotline.NewServer(
		hotline.WithTLS(&tls.Config{GetCertificate: manager.GetCertificate}, 0),
		hotline.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		hotline.WithConnectionRateLimit(rate.Inf, 1),
	)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var addresses []string
	for _, serve := range []func(context.Context, net.Listener) error{srv.ServeWithTLS, srv.ServeFileTransfersWithTLS} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addresses = append(addresses, ln.Addr().String())
		done := make(chan error, 1)
		go func() { done <- serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			_ = ln.Close()
			select {
			case err := <-done:
				assert.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Error("TLS service did not stop")
			}
		})
	}
	return srv, addresses
}

func dialReloadTLS(addr, serverName string, roots *x509.CertPool) (*tls.Conn, error) {
	// An IP address as ServerName verifies the IP SAN without sending SNI. No session
	// cache is configured: every dial performs a fresh handshake and verifies the peer.
	return tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr,
		&tls.Config{RootCAs: roots, ServerName: serverName})
}

func checkReloadCertificate(t *testing.T, addresses []string, roots *x509.CertPool, want *x509.Certificate) {
	t.Helper()
	for service, addr := range addresses {
		for _, serverName := range []string{"localhost", "127.0.0.1"} {
			conn, err := dialReloadTLS(addr, serverName, roots)
			require.NoError(t, err, "service %d, server name %s", service, serverName)
			assert.Equal(t, want.Raw, conn.ConnectionState().PeerCertificates[0].Raw)
			require.NoError(t, conn.Close())
		}
	}
}

func reloadRoundTrip(t *testing.T, conn net.Conn, request hotline.Transaction) hotline.Transaction {
	t.Helper()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err := io.Copy(conn, &request)
	require.NoError(t, err)
	for {
		var header [20]byte
		_, err := io.ReadFull(conn, header[:])
		require.NoError(t, err)
		body := make([]byte, binary.BigEndian.Uint32(header[16:20]))
		_, err = io.ReadFull(conn, body)
		require.NoError(t, err)
		var reply hotline.Transaction
		_, err = reply.Write(append(header[:], body...))
		require.NoError(t, err)
		if reply.IsReply == 1 && reply.ID == request.ID {
			require.Equal(t, [4]byte{}, reply.ErrorCode)
			return reply
		}
	}
}

func TestTLSCertificateReloadServices(t *testing.T) {
	first, second, third := newReloadCertificatePair(t, 1), newReloadCertificatePair(t, 2), newReloadCertificatePair(t, 3)
	roots := x509.NewCertPool()
	for _, pair := range []reloadCertificatePair{first, second, third} {
		roots.AddCert(pair.cert)
	}
	certPath, keyPath := filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key")
	first.write(t, certPath, keyPath)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	manager, err := mobius.NewCertificateManager(certPath, keyPath, logger)
	require.NoError(t, err)
	srv, addresses := startReloadTLSServices(t, manager)

	// Use real login and request handlers for the established Hotline session.
	cfgDir := t.TempDir()
	require.NoError(t, copyDir("mobius/config", cfgDir))
	accounts, err := mobius.NewYAMLAccountManager(filepath.Join(cfgDir, "Users"))
	require.NoError(t, err)
	guest := accounts.Get("guest")
	guest.Access.Set(hotline.AccessNoAgreement)
	hash, err := bcrypt.GenerateFromPassword(nil, bcrypt.MinCost)
	require.NoError(t, err)
	guest.Password = string(hash)
	require.NoError(t, accounts.Update(*guest, guest.Login))
	srv.AccountManager = accounts
	mobius.RegisterHandlers(srv)

	session, err := dialReloadTLS(addresses[0], "localhost", roots)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	require.NoError(t, session.SetDeadline(time.Now().Add(5*time.Second)))
	client := hotline.NewClient("reload test", srv.Logger)
	client.Connection = session
	require.NoError(t, client.Handshake())
	reloadRoundTrip(t, session, hotline.NewTransaction(hotline.TranLogin, [2]byte{},
		hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("guest"))),
		hotline.NewField(hotline.FieldUserPassword, nil)))
	checkSession := func() {
		t.Helper()
		reply := reloadRoundTrip(t, session, hotline.NewTransaction(hotline.TranGetUserNameList, [2]byte{}))
		require.NotEmpty(t, reply.Fields)
	}
	checkSession()

	// A folder transfer waits for client acknowledgements between files. This keeps a real
	// transfer active across each reload without relying on socket buffering or sleeps.
	fileRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(fileRoot, "batch"), 0700))
	for i := range 5 {
		require.NoError(t, srv.FS.WriteFile(filepath.Join(fileRoot, "batch", fmt.Sprintf("%d.txt", i)), []byte(fmt.Sprintf("reload payload %d", i)), 0600))
	}
	clients := srv.ClientMgr.List()
	require.Len(t, clients, 1)
	transfer := clients[0].NewFileTransfer(hotline.FolderDownload, fileRoot, []byte("batch"), nil, []byte{0, 0, 0, 0})
	xfer, err := dialReloadTLS(addresses[1], "127.0.0.1", roots)
	require.NoError(t, err)
	t.Cleanup(func() { _ = xfer.Close() })
	require.NoError(t, xfer.SetDeadline(time.Now().Add(5*time.Second)))
	var htxf [16]byte
	copy(htxf[:4], hotline.HTXF[:])
	copy(htxf[4:8], transfer.RefNum[:])
	_, err = xfer.Write(htxf[:])
	require.NoError(t, err)
	downloadNext := func(index int) {
		t.Helper()
		require.NoError(t, xfer.SetDeadline(time.Now().Add(5*time.Second)))
		_, err := xfer.Write([]byte{0, hotline.DlFldrActionNextFile})
		require.NoError(t, err)
		var headerSize [2]byte
		_, err = io.ReadFull(xfer, headerSize[:])
		require.NoError(t, err)
		header := make([]byte, binary.BigEndian.Uint16(headerSize[:]))
		_, err = io.ReadFull(xfer, header)
		require.NoError(t, err)
		_, err = xfer.Write([]byte{0, hotline.DlFldrActionSendFile})
		require.NoError(t, err)
		var size [4]byte
		_, err = io.ReadFull(xfer, size[:])
		require.NoError(t, err)
		payload := make([]byte, binary.BigEndian.Uint32(size[:]))
		_, err = io.ReadFull(xfer, payload)
		require.NoError(t, err)
		assert.Contains(t, string(payload), fmt.Sprintf("reload payload %d", index))
	}
	downloadNext(0)
	checkReloadCertificate(t, addresses, roots, first.cert)

	otherReloads := 0
	reload := newReloadFunc([]namedReloader{
		{"TLS certificate", manager},
		{"other component", mobius.ReloaderFunc(func() error { otherReloads++; return nil })},
	}, logger)
	api := mobius.NewAPIServer(srv, nil, reload, logger, "")
	apiReload := func() {
		t.Helper()
		rr := httptest.NewRecorder()
		api.ReloadHandler(reload)(rr, httptest.NewRequest(http.MethodPost, "/api/v1/reload", nil))
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, `{ "msg": "config reloaded" }`, rr.Body.String())
	}

	second.write(t, certPath, keyPath)
	reload() // The callback used by SIGHUP.
	assert.Contains(t, logs.String(), "TLS certificate reloaded")
	checkReloadCertificate(t, addresses, roots, second.cert)
	checkSession()
	downloadNext(1)

	apiReload() // Reloading an unchanged pair also succeeds.
	checkReloadCertificate(t, addresses, roots, second.cert)
	checkSession()
	downloadNext(2)

	logs.Reset()
	require.NoError(t, os.Remove(keyPath))
	apiReload()
	assert.Contains(t, logs.String(), "Error reloading TLS certificate")
	assert.NotContains(t, logs.String(), "TLS certificate reloaded")
	checkReloadCertificate(t, addresses, roots, second.cert)
	checkSession()
	downloadNext(3)

	third.write(t, certPath, keyPath)
	apiReload()
	checkReloadCertificate(t, addresses, roots, third.cert)
	checkSession()
	downloadNext(4)
	_, err = xfer.Write([]byte{0, hotline.DlFldrActionNextFile})
	require.NoError(t, err)
	assert.Equal(t, 4, otherReloads, "a failed certificate reload must not skip other reloaders")
	assert.Equal(t, first.cert.Raw, session.ConnectionState().PeerCertificates[0].Raw)
	assert.Equal(t, first.cert.Raw, xfer.ConnectionState().PeerCertificates[0].Raw)
}

func TestTLSCertificateReloadConcurrentHandshakes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pairs := []reloadCertificatePair{newReloadCertificatePair(t, 1), newReloadCertificatePair(t, 2)}
	roots := x509.NewCertPool()
	for _, pair := range pairs {
		roots.AddCert(pair.cert)
	}
	certPath, keyPath := filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key")
	pairs[0].write(t, certPath, keyPath)
	manager, err := mobius.NewCertificateManager(certPath, keyPath, logger)
	require.NoError(t, err)
	_, addresses := startReloadTLSServices(t, manager)

	var handshakes sync.WaitGroup
	start := make(chan struct{})
	for worker := range 8 {
		handshakes.Go(func() {
			<-start
			for range 40 {
				name := "localhost"
				if worker%2 == 0 {
					name = "127.0.0.1"
				}
				conn, err := dialReloadTLS(addresses[(worker/2)%2], name, roots)
				if !assert.NoError(t, err) {
					return
				}
				peer := conn.ConnectionState().PeerCertificates[0]
				assert.True(t, bytes.Equal(peer.Raw, pairs[0].cert.Raw) || bytes.Equal(peer.Raw, pairs[1].cert.Raw))
				assert.NoError(t, conn.Close())
			}
		})
	}
	close(start)
	defer handshakes.Wait()
	for round := range 30 {
		// Finish both writes before concurrent reload requests start, as operators must do.
		pairs[round%2].write(t, certPath, keyPath)
		var reloads sync.WaitGroup
		for range 4 {
			reloads.Go(func() { assert.NoError(t, manager.Reload()) })
		}
		reloads.Wait()
	}
	handshakes.Wait()
	checkReloadCertificate(t, addresses, roots, pairs[1].cert)
}

func TestReloadFuncContinuesAfterError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	called := false
	reload := newReloadFunc([]namedReloader{
		{"broken component", mobius.ReloaderFunc(func() error { return errors.New("test failure") })},
		{"next component", mobius.ReloaderFunc(func() error { called = true; return nil })},
	}, logger)
	reload()
	assert.True(t, called)
	assert.Contains(t, logs.String(), "Error reloading broken component")
	assert.Contains(t, logs.String(), "test failure")
}
