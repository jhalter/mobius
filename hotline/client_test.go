package hotline

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient() *Client {
	return NewClient("testuser", slog.New(slog.NewTextHandler(os.Stdout, nil)))
}

func TestClient_Handshake(t *testing.T) {
	t.Run("successful handshake", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn

		// Server side: read the client handshake and respond
		go func() {
			buf := make([]byte, 12)
			_, _ = io.ReadFull(serverConn, buf)
			assert.Equal(t, ClientHandshake, buf)
			_, _ = serverConn.Write(ServerHandshake)
		}()

		err := c.Handshake()
		assert.NoError(t, err)
	})

	t.Run("server returns error response", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn

		go func() {
			buf := make([]byte, 12)
			_, _ = io.ReadFull(serverConn, buf)
			// Send a non-standard response
			_, _ = serverConn.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
		}()

		err := c.Handshake()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected handshake response")
	})

	t.Run("connection closed during read", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()

		c := newTestClient()
		c.Connection = clientConn

		go func() {
			buf := make([]byte, 12)
			_, _ = io.ReadFull(serverConn, buf)
			serverConn.Close()
		}()

		err := c.Handshake()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handshake read err")
	})
}

func TestClient_Send(t *testing.T) {
	t.Run("sends non-reply transaction and tracks it", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn

		tran := NewTransaction(TranChatSend, [2]byte{0, 1}, NewField(FieldData, []byte("hello")))

		// Read what client sends
		go func() {
			buf := make([]byte, 4096)
			_, _ = serverConn.Read(buf)
		}()

		err := c.Send(tran)
		assert.NoError(t, err)

		// Verify the transaction was added to activeTasks
		assert.Contains(t, c.activeTasks, tran.ID)
	})

	t.Run("reply transactions are not tracked", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn

		tran := NewTransaction(TranChatSend, [2]byte{0, 1})
		tran.IsReply = 1

		go func() {
			buf := make([]byte, 4096)
			_, _ = serverConn.Read(buf)
		}()

		err := c.Send(tran)
		assert.NoError(t, err)
		assert.NotContains(t, c.activeTasks, tran.ID)
	})
}

func TestClient_HandleTransaction(t *testing.T) {
	t.Run("dispatches to registered handler", func(t *testing.T) {
		c := newTestClient()
		c.Connection = &clientMockConn{WBuf: &bytes.Buffer{}}

		handlerCalled := false
		c.HandleFunc(TranChatMsg, func(ctx context.Context, client *Client, t *Transaction) ([]Transaction, error) {
			handlerCalled = true
			return nil, nil
		})

		tran := &Transaction{Type: TranChatMsg}
		err := c.HandleTransaction(context.Background(), tran)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
	})

	t.Run("reply matches original request type", func(t *testing.T) {
		c := newTestClient()
		c.Connection = &clientMockConn{WBuf: &bytes.Buffer{}}

		// Register a handler for the original type
		var receivedType TranType
		c.HandleFunc(TranGetUserNameList, func(ctx context.Context, client *Client, t *Transaction) ([]Transaction, error) {
			receivedType = t.Type
			return nil, nil
		})

		// Simulate sending a request first
		origTran := NewTransaction(TranGetUserNameList, [2]byte{0, 1})
		c.activeTasks[origTran.ID] = &origTran

		// Create a reply with the same ID
		reply := &Transaction{
			ID:      origTran.ID,
			IsReply: 1,
		}

		err := c.HandleTransaction(context.Background(), reply)
		assert.NoError(t, err)
		assert.Equal(t, TranGetUserNameList, receivedType)
	})

	t.Run("reply with no matching request returns error", func(t *testing.T) {
		c := newTestClient()
		c.Connection = &clientMockConn{WBuf: &bytes.Buffer{}}

		reply := &Transaction{
			ID:      [4]byte{0, 0, 0, 99},
			IsReply: 1,
		}

		err := c.HandleTransaction(context.Background(), reply)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no matching request")
	})

	t.Run("unhandled transaction type does not error", func(t *testing.T) {
		c := newTestClient()
		c.Connection = &clientMockConn{WBuf: &bytes.Buffer{}}

		tran := &Transaction{Type: [2]byte{0xFF, 0xFF}}
		err := c.HandleTransaction(context.Background(), tran)
		assert.NoError(t, err)
	})
}

func TestClient_Disconnect(t *testing.T) {
	t.Run("closes connection and done channel", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn
		c.done = make(chan struct{})
		doneCh := c.done // save reference before Disconnect sets it to nil

		err := c.Disconnect()
		assert.NoError(t, err)

		// Verify done channel is closed
		select {
		case <-doneCh:
			// expected - channel is closed
		default:
			t.Error("done channel should be closed")
		}

		// Verify connection is closed by attempting a write
		_, err = clientConn.Write([]byte("test"))
		assert.Error(t, err)
	})

	t.Run("handles nil done channel", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer serverConn.Close()

		c := newTestClient()
		c.Connection = clientConn
		// done is nil by default (before Connect is called)

		err := c.Disconnect()
		assert.NoError(t, err)
	})
}

func TestClient_HandleFunc(t *testing.T) {
	c := newTestClient()

	handler := func(ctx context.Context, client *Client, t *Transaction) ([]Transaction, error) {
		return nil, nil
	}

	c.HandleFunc(TranChatMsg, handler)

	_, ok := c.Handlers[TranChatMsg]
	require.True(t, ok, "handler should be registered")
}

func TestNewClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	c := NewClient("myuser", logger)

	assert.Equal(t, "myuser", c.Pref.Username)
	assert.NotNil(t, c.Handlers)
	assert.NotNil(t, c.activeTasks)
}

// clientMockConn implements net.Conn for testing Send without a net.Pipe.
type clientMockConn struct {
	RBuf *bytes.Buffer
	WBuf *bytes.Buffer
}

func (mc *clientMockConn) Read(b []byte) (n int, err error) {
	if mc.RBuf == nil {
		return 0, io.EOF
	}
	return mc.RBuf.Read(b)
}

func (mc *clientMockConn) Write(b []byte) (n int, err error) {
	return mc.WBuf.Write(b)
}

func (mc *clientMockConn) Close() error                        { return nil }
func (mc *clientMockConn) LocalAddr() net.Addr                 { return nil }
func (mc *clientMockConn) RemoteAddr() net.Addr                { return nil }
func (mc *clientMockConn) SetDeadline(_ time.Time) error       { return nil }
func (mc *clientMockConn) SetReadDeadline(_ time.Time) error   { return nil }
func (mc *clientMockConn) SetWriteDeadline(_ time.Time) error  { return nil }
