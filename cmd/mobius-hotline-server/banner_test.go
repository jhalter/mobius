package main

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/internal/mobius"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise production announcements, reload callbacks, download requests, and
// transfer sockets together, with both image formats alive across pool changes.
func TestBannerSessionReloadAndTransfer(t *testing.T) {
	dir := t.TempDir()
	banners := filepath.Join(dir, "banners")
	require.NoError(t, os.Mkdir(banners, 0700))
	jpeg := []byte("original JPEG banner")
	gif := []byte("new GIF banner, a different size")
	require.NoError(t, os.WriteFile(filepath.Join(banners, "one.jpg"), jpeg, 0600))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := hotline.NewServer(hotline.WithConfig(hotline.Config{BannerDirectory: "banners"}), hotline.WithLogger(logger))
	require.NoError(t, err)
	loader := mobius.NewBannerReloader(dir, srv)
	require.NoError(t, loader.Reload())
	reload := newReloadFunc([]namedReloader{{"banner", loader}}, logger)
	api := mobius.NewAPIServer(srv, nil, reload, logger, "")

	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- srv.ServeFileTransfers(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		_ = ln.Close()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Error("banner transfer service did not stop")
		}
	})
	newSession := func() *hotline.ClientConn {
		return &hotline.ClientConn{
			Server: srv, Account: &hotline.Account{Login: "guest", Name: "Guest"}, Logger: logger,
			ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
		}
	}
	announce := func(cc *hotline.ClientConn, want string) {
		t.Helper()
		request := hotline.NewTransaction(hotline.TranAgreed, cc.ID)
		responses := mobius.HandleTranAgreed(cc, &request)
		var types []string
		for _, response := range responses {
			if response.Type == hotline.TranServerBanner {
				types = append(types, string(response.GetField(hotline.FieldBannerType).Data))
			}
		}
		require.Equal(t, []string{want}, types)
	}
	requestDownload := func(cc *hotline.ClientConn) hotline.Transaction {
		t.Helper()
		request := hotline.NewTransaction(hotline.TranDownloadBanner, cc.ID)
		responses := mobius.HandleDownloadBanner(cc, &request)
		require.Len(t, responses, 1)
		return responses[0]
	}
	oldSession := newSession()
	announce(oldSession, "JPEG")
	// Change both format and size after announcement, before requesting download.
	require.NoError(t, os.Remove(filepath.Join(banners, "one.jpg")))
	require.NoError(t, os.WriteFile(filepath.Join(banners, "two.gif"), gif, 0600))
	reload() // The same callback used by SIGHUP.
	oldReply := requestDownload(oldSession)
	newClient := newSession()
	newReply := requestDownload(newClient) // Download before announcement selects lazily.
	announce(newClient, "GIFf")
	announce(oldSession, "JPEG")

	// Reload through the API after transfer registration, before opening sockets.
	require.NoError(t, os.WriteFile(filepath.Join(banners, "two.gif"), []byte("third image"), 0600))
	rr := httptest.NewRecorder()
	api.ReloadHandler(reload)(rr, httptest.NewRequest(http.MethodPost, "/api/v1/reload", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "third image", string(newSession().Banner().Data))
	for _, test := range []struct {
		reply hotline.Transaction
		want  []byte
	}{{oldReply, jpeg}, {newReply, gif}, {requestDownload(oldSession), jpeg}, {requestDownload(newClient), gif}} {
		assert.Equal(t, uint32(len(test.want)), binary.BigEndian.Uint32(test.reply.GetField(hotline.FieldTransferSize).Data))
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		header := make([]byte, 16)
		copy(header, "HTXF")
		copy(header[4:8], test.reply.GetField(hotline.FieldRefNum).Data)
		_, err = conn.Write(header)
		require.NoError(t, err)
		payload := make([]byte, len(test.want))
		_, err = io.ReadFull(conn, payload)
		require.NoError(t, err)
		assert.Equal(t, test.want, payload)
		require.NoError(t, conn.Close())
	}
}
