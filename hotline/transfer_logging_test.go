package hotline

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transferLogConn struct {
	net.Conn
	reader   io.Reader
	writeErr error
}

func (c *transferLogConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *transferLogConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return len(p), nil
}
func (c *transferLogConn) Close() error { return nil }
func (c *transferLogConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}
}
func (c *transferLogConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.0.2.2"), Port: 5601}
}

type transferLogListener struct {
	net.Listener
	conn net.Conn
}

func (l *transferLogListener) Accept() (net.Conn, error) {
	if l.conn == nil {
		return nil, net.ErrClosed
	}
	conn := l.conn
	l.conn = nil
	return conn, nil
}

func TestFileTransferLogging(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payload  []byte
		tls      bool
		pending  bool
		writeErr error
		level    string
		errText  string
	}{
		{name: "empty connection", level: "DEBUG", errText: "EOF"},
		{name: "partial header", payload: []byte("HT"), level: "DEBUG", errText: "EOF"},
		{name: "invalid protocol", payload: bytes.Repeat([]byte("x"), 16), level: "DEBUG", errText: "invalid protocol"},
		{name: "plaintext on TLS port", payload: []byte("GET / HTTP/1.0\r\n\r\n"), tls: true,
			level: "DEBUG", errText: "first record does not look like a TLS handshake"},
		{name: "unknown reference", payload: append([]byte("HTXF"), make([]byte, 12)...),
			level: "DEBUG", errText: "invalid transaction ID"},
		{name: "failure during transfer", pending: true, writeErr: io.EOF, level: "ERROR", errText: "banner download: EOF"},
		{name: "successful transfer", pending: true},
	} {
		for _, minLevel := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
			t.Run(tc.name+"/"+minLevel.String(), func(t *testing.T) {
				output := &scriptedConn{}
				srv := newLoginTestServer(nil)
				srv.Logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: minLevel}))
				srv.FileTransferMgr = NewMemFileTransferMgr()
				payload := tc.payload
				if tc.pending {
					srv.SetBanner([]byte("banner"))
					cc := &ClientConn{Server: srv, Account: &Account{Login: "guest"}, ClientFileTransferMgr: NewClientFileTransferMgr()}
					ft := cc.NewFileTransfer(BannerDownload, "", nil, nil, nil)
					var header bytes.Buffer
					require.NoError(t, binary.Write(&header, binary.BigEndian, transfer{Protocol: HTXF, ReferenceNumber: ft.RefNum}))
					payload = header.Bytes()
				}
				var conn net.Conn = &transferLogConn{reader: bytes.NewReader(payload), writeErr: tc.writeErr}
				if tc.tls {
					conn = tls.Server(conn, &tls.Config{})
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Skip the legacy transfer-close delay.
				require.ErrorIs(t, srv.ServeFileTransfers(ctx, &transferLogListener{conn: conn}), context.Canceled)
				srv.connWG.Wait()
				if tc.level == "" || (tc.level == "DEBUG" && minLevel == slog.LevelInfo) {
					assert.Empty(t, output.written())
					return
				}
				records := logRecords(t, output)
				require.Len(t, records, 1)
				assert.Equal(t, tc.level, records[0]["level"])
				assert.Equal(t, "file transfer error", records[0]["msg"])
				assert.Equal(t, "192.0.2.1:1234", records[0]["remote_addr"])
				assert.Equal(t, "192.0.2.2:5601", records[0]["local_addr"])
				assert.Contains(t, records[0]["err"], tc.errText)
			})
		}
	}
}
