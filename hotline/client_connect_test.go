package hotline

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acceptHandshakeServer starts a TCP listener that, for a single connection, plays the server side
// of the handshake (optionally sending a bad response) and then reads the login transaction the
// client sends.  The received login bytes are delivered on loginCh.  It returns the listener's
// address so the client can dial it.
func acceptHandshakeServer(t *testing.T, goodHandshake bool) (addr string, loginCh <-chan []byte) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan []byte, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// Read the client handshake.
		hs := make([]byte, handshakeSize)
		if _, err := io.ReadFull(conn, hs); err != nil {
			return
		}

		if goodHandshake {
			_, _ = conn.Write(ServerHandshake)
		} else {
			_, _ = conn.Write([]byte{0, 0, 0, 0, 0, 0, 0, 1})
			return
		}

		// Read the login transaction that Connect sends next.
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		ch <- buf[:n]

		// Hold the connection open briefly so the client's keepalive goroutine has a live socket.
		time.Sleep(50 * time.Millisecond)
	}()

	return ln.Addr().String(), ch
}

func TestClient_Connect(t *testing.T) {
	t.Run("completes handshake and sends login", func(t *testing.T) {
		addr, loginCh := acceptHandshakeServer(t, true)

		c := newTestClient()
		c.Pref.Username = "testuser"

		require.NoError(t, c.Connect(addr, "admin", "password"))
		defer func() { _ = c.Disconnect() }()

		// Connect must establish the connection and arm the keepalive done channel.
		assert.NotNil(t, c.Connection)
		assert.NotNil(t, c.done)

		// The server should have received a well-formed login transaction carrying the
		// obfuscated credentials and the username.
		select {
		case raw := <-loginCh:
			var login Transaction
			_, err := login.Write(raw)
			require.NoError(t, err)

			assert.Equal(t, TranLogin, login.Type)
			assert.Equal(t, "admin", login.GetField(FieldUserLogin).DecodeObfuscatedString())
			assert.Equal(t, "password", login.GetField(FieldUserPassword).DecodeObfuscatedString())
			assert.Equal(t, []byte("testuser"), login.GetField(FieldUserName).Data)
		case <-time.After(2 * time.Second):
			t.Fatal("server did not receive login transaction")
		}
	})

	t.Run("returns error when dial fails", func(t *testing.T) {
		// Reserve a port, then close the listener so nothing is accepting on it.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		require.NoError(t, ln.Close())

		c := newTestClient()
		err = c.Connect(addr, "admin", "password")
		assert.Error(t, err)
	})

	t.Run("returns error on bad handshake response", func(t *testing.T) {
		addr, _ := acceptHandshakeServer(t, false)

		c := newTestClient()
		err := c.Connect(addr, "admin", "password")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handshake")
	})
}

func TestClient_HandleTransactions(t *testing.T) {
	t.Run("dispatches queued transactions then reports termination", func(t *testing.T) {
		// Two server-initiated transactions back to back, followed by EOF.
		var buf bytes.Buffer
		buf.Write(serializeTransaction(t, NewTransaction(TranChatMsg, [2]byte{0, 0}, NewField(FieldData, []byte("hi")))))
		buf.Write(serializeTransaction(t, NewTransaction(TranChatMsg, [2]byte{0, 0}, NewField(FieldData, []byte("there")))))

		c := newTestClient()
		c.Connection = &clientMockConn{RBuf: &buf, WBuf: &bytes.Buffer{}}

		var got []string
		c.HandleFunc(TranChatMsg, func(_ context.Context, _ *Client, tr *Transaction) ([]Transaction, error) {
			got = append(got, string(tr.GetField(FieldData).Data))
			return nil, nil
		})

		err := c.HandleTransactions(context.Background())

		// At EOF the scanner stops and HandleTransactions reports the connection is gone.
		assert.EqualError(t, err, "connection terminated")
		assert.Equal(t, []string{"hi", "there"}, got)
	})

	t.Run("returns scanner error", func(t *testing.T) {
		wantErr := errors.New("read boom")
		c := newTestClient()
		c.Connection = &errorConn{err: wantErr}

		err := c.HandleTransactions(context.Background())
		assert.ErrorIs(t, err, wantErr)
	})
}

func TestClient_keepalive_stopsOnDone(t *testing.T) {
	c := newTestClient()
	done := make(chan struct{})

	errCh := make(chan error, 1)
	go func() { errCh <- c.keepalive(done) }()

	// Closing done must cause keepalive to return promptly (well before the 300s tick).
	close(done)

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("keepalive did not return after done was closed")
	}
}

// errorConn is a net.Conn whose Read always fails with a non-EOF error, so a bufio.Scanner over it
// surfaces the error via Scanner.Err().
type errorConn struct {
	err error
}

func (e *errorConn) Read([]byte) (int, error)         { return 0, e.err }
func (e *errorConn) Write(b []byte) (int, error)      { return len(b), nil }
func (e *errorConn) Close() error                     { return nil }
func (e *errorConn) LocalAddr() net.Addr              { return nil }
func (e *errorConn) RemoteAddr() net.Addr             { return nil }
func (e *errorConn) SetDeadline(time.Time) error      { return nil }
func (e *errorConn) SetReadDeadline(time.Time) error  { return nil }
func (e *errorConn) SetWriteDeadline(time.Time) error { return nil }
