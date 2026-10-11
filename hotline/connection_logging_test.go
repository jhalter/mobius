package hotline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capturedLogger() (*slog.Logger, *scriptedConn) {
	output := &scriptedConn{}
	return slog.New(slog.NewJSONHandler(output, nil)), output
}

func logRecords(t *testing.T, output *scriptedConn) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(output.written()), []byte("\n")) {
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	return records
}

type failingReadConn struct {
	*scriptedConn
	readErr error
}

func (c *failingReadConn) Read(p []byte) (int, error) {
	if c.in.Len() == 0 && c.readErr != nil {
		return 0, c.readErr
	}
	return c.scriptedConn.Read(p)
}

func TestConnectionLoggingBeforeLogin(t *testing.T) {
	oversized := make([]byte, bufio.MaxScanTokenSize)
	binary.BigEndian.PutUint32(oversized[12:16], bufio.MaxScanTokenSize)
	for _, tc := range []struct {
		name     string
		payload  []byte
		readErr  error
		wantErr  error
		reason   string
		level    string
		buffered int
		expected int
	}{
		{name: "handshake only", reason: "peer_closed", level: "INFO"},
		{name: "partial header", payload: []byte{0, 0, 0}, wantErr: io.ErrUnexpectedEOF,
			reason: "truncated_transaction", level: "WARN", buffered: 3, expected: 22},
		{name: "partial body", payload: sampleTransactionBytes[:22], wantErr: io.ErrUnexpectedEOF,
			reason: "truncated_transaction", level: "WARN", buffered: 22, expected: len(sampleTransactionBytes)},
		{name: "oversized", payload: oversized, wantErr: bufio.ErrTooLong,
			reason: "transaction_too_large", level: "WARN"},
		{name: "invalid size", payload: make([]byte, 20), wantErr: errTransactionRead,
			reason: "invalid_transaction_size", level: "WARN", buffered: 20, expected: 20},
		{name: "reset", readErr: syscall.ECONNRESET, wantErr: syscall.ECONNRESET,
			reason: "connection_reset", level: "INFO"},
		{name: "reset during packet", payload: []byte{0, 0, 0}, readErr: syscall.ECONNRESET,
			wantErr: syscall.ECONNRESET, reason: "connection_reset", level: "INFO"},
		{name: "other read failure", readErr: io.ErrClosedPipe, wantErr: io.ErrClosedPipe,
			reason: "read_error", level: "WARN"},
		{name: "reader reports truncation", readErr: io.ErrUnexpectedEOF, wantErr: io.ErrUnexpectedEOF,
			reason: "truncated_transaction", level: "WARN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLoginTestServer(nil)
			var output *scriptedConn
			srv.Logger, output = capturedLogger()
			input := bytes.NewBuffer(bytes.Clone(ClientHandshake))
			input.Write(tc.payload)
			conn := &failingReadConn{scriptedConn: &scriptedConn{in: input}, readErr: tc.readErr}
			err := srv.handleNewConnection(context.Background(), conn, "172.184.219.214:52256")
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			records := logRecords(t, output)
			require.Len(t, records, 2)
			closed := records[1]
			assert.Equal(t, "Connection closed", closed["msg"])
			assert.Equal(t, tc.reason, closed["reason"])
			assert.Equal(t, tc.level, closed["level"])
			assert.Equal(t, "login", closed["stage"])
			assert.Equal(t, true, closed["handshake_ok"])
			assert.Equal(t, false, closed["authenticated"])
			assert.Equal(t, float64(0), closed["requests_received"])
			assert.Equal(t, float64(0), closed["requests_handled"])
			assert.Equal(t, records[0]["remote_addr"], closed["remote_addr"])
			assert.Equal(t, "172.184.219.214", closed["ip"])
			assert.Equal(t, "172.184.219.214:52256", closed["remote_addr"])
			if tc.buffered != 0 {
				assert.Equal(t, float64(tc.buffered), closed["buffered_bytes"])
				assert.Equal(t, float64(tc.expected), closed["expected_bytes"])
			}
		})
	}
}

func TestConnectionLoggingSession(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "modern", true: "legacy"}[legacy], func(t *testing.T) {
			srv := newLoginTestServer(map[string]*Account{
				"guest": accountWithPassword(t, "guest", "secret-password", AccessAnyName),
			})
			var output *scriptedConn
			srv.Logger, output = capturedLogger()
			srv.handlers = map[TranType]HandlerFunc{
				TranGetUserNameList: func(c *ClientConn, tr *Transaction) []Transaction { return []Transaction{c.NewReply(tr)} },
				TranChatSend:        func(c *ClientConn, tr *Transaction) []Transaction { return c.NewErrReply(tr, "Denied") },
				TranGetMsgs:         func(*ClientConn, *Transaction) []Transaction { panic("test handler failure") },
			}
			var fields []Field
			if legacy {
				fields = append(fields, NewField(FieldUserName, []byte("Tester")))
			}
			input := loginInput(t, "guest", "secret-password", fields...)
			for _, typ := range []TranType{TranGetUserNameList, TranChatSend, TranGetMsgs, TranType{0xff, 0xff}} {
				input.Write(serializeTransaction(t, NewTransaction(typ, ClientID{})))
			}
			// An incomplete next request must not disappear silently when the session ends.
			input.Write([]byte{0, 0, 0})
			err := srv.handleNewConnection(context.Background(), &scriptedConn{in: input}, "192.0.2.1:1234")
			srv.connWG.Wait()
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			records := logRecords(t, output)
			counts := map[string]int{}
			outcomes := map[string]int{}
			for _, record := range records {
				counts[record["msg"].(string)]++
				if outcome, ok := record["outcome"].(string); ok {
					outcomes[outcome]++
					assert.NotNil(t, record["transaction_id"])
				}
				assert.Equal(t, "192.0.2.1:1234", record["remote_addr"])
			}
			assert.Equal(t, 1, counts["Authentication succeeded"])
			assert.Zero(t, counts["Login successful"])
			assert.Equal(t, map[string]int{"returned": 1, "error_reply": 1, "panic": 1, "unsupported_transaction": 1}, outcomes)
			closed := records[len(records)-1]
			assert.Equal(t, "Connection closed", closed["msg"])
			assert.Equal(t, "session", closed["stage"])
			assert.Equal(t, true, closed["authenticated"])
			assert.Equal(t, float64(4), closed["requests_received"])
			assert.Equal(t, float64(2), closed["requests_handled"])
			assert.Equal(t, "truncated_transaction", closed["reason"])
			assert.NotContains(t, string(output.written()), "secret-password")
			assert.NotContains(t, string(output.written()), string(EncodeString([]byte("secret-password"))))
		})
	}
}

func TestConnectionLoggingAuthenticationFailure(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{"guest": accountWithPassword(t, "guest", "correct")})
	var output *scriptedConn
	srv.Logger, output = capturedLogger()
	err := srv.handleNewConnection(context.Background(), &scriptedConn{in: loginInput(t, "guest", "wrong")}, "192.0.2.1:1234")
	srv.connWG.Wait()
	require.NoError(t, err)
	records := logRecords(t, output)
	require.Len(t, records, 3)
	assert.Equal(t, "Authentication failed", records[1]["msg"])
	assert.Equal(t, "authentication_failed", records[2]["reason"])
	assert.Equal(t, false, records[2]["authenticated"])
}

func TestConnectionLoggingHandshakeFailure(t *testing.T) {
	for _, input := range [][]byte{nil, []byte("TRTP"), []byte("XXXXYYYY0000")} {
		srv := newLoginTestServer(nil)
		var output *scriptedConn
		srv.Logger, output = capturedLogger()
		err := srv.handleNewConnection(context.Background(), &scriptedConn{in: bytes.NewBuffer(input)}, "192.0.2.1:1234")
		require.Error(t, err)
		records := logRecords(t, output)
		require.Len(t, records, 2)
		assert.Equal(t, "handshake", records[1]["stage"])
		assert.Equal(t, false, records[1]["handshake_ok"])
		if len(input) == 0 {
			assert.Equal(t, "peer_closed", records[1]["reason"])
		} else if errors.Is(err, io.ErrUnexpectedEOF) {
			assert.Equal(t, "truncated_handshake", records[1]["reason"])
		} else {
			assert.Equal(t, "invalid_handshake", records[1]["reason"])
		}
	}
}

func TestConnectionLoggingServe(t *testing.T) {
	logger, output := capturedLogger()
	srv, err := NewServer(WithLogger(logger), WithConnectionRateLimit(0, 1))
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		_ = ln.Close()
		<-done
		srv.connWG.Wait()
	})
	var remoteAddrs []string
	for i := range 2 {
		conn, err := net.Dial("tcp", ln.Addr().String())
		require.NoError(t, err)
		remoteAddrs = append(remoteAddrs, conn.LocalAddr().String())
		t.Cleanup(func() { _ = conn.Close() })
		require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
		if i == 0 {
			_, err = conn.Write(ClientHandshake)
			require.NoError(t, err)
			response := make([]byte, len(ServerHandshake))
			_, err = io.ReadFull(conn, response)
			require.NoError(t, err)
			assert.Equal(t, ServerHandshake, response)
			require.NoError(t, conn.(*net.TCPConn).CloseWrite())
		}
		_, err = io.ReadAll(conn)
		require.NoError(t, err)
		srv.connWG.Wait()
	}
	records := logRecords(t, output)
	require.Len(t, records, 4, "each connection has exactly one opening and closing event")
	for i, remoteAddr := range remoteAddrs {
		assert.Equal(t, remoteAddr, records[2*i]["remote_addr"])
		assert.Equal(t, remoteAddr, records[2*i+1]["remote_addr"])
	}
	assert.Equal(t, "peer_closed", records[1]["reason"])
	assert.Equal(t, "rate_limited", records[3]["reason"])
}

type failingWriteConn struct{ *scriptedConn }

func (c *failingWriteConn) Write([]byte) (int, error) { return 0, syscall.EPIPE }

func TestTransactionWriteFailureLogging(t *testing.T) {
	for _, closing := range []bool{false, true} {
		t.Run(map[bool]string{false: "draining", true: "closing"}[closing], func(t *testing.T) {
			logger, output := capturedLogger()
			cc := &ClientConn{Logger: logger.With("remote_addr", "192.0.2.1:1234"), Connection: &failingWriteConn{&scriptedConn{}}}
			tr := NewTransaction(TranGetUserNameList, ClientID{})
			tr.ID = [4]byte{0, 0, 0, 42}
			tr.IsReply = 1
			cc.Send(tr)
			cc.closeSendQueue()
			if closing {
				require.NoError(t, cc.closeConnection())
			}
			cc.writeLoop()
			records := logRecords(t, output)
			require.Len(t, records, 1)
			assert.Equal(t, "Transaction write failed", records[0]["msg"])
			assert.Equal(t, "WARN", records[0]["level"], "a broken pipe is not a local closed-connection error")
			assert.Equal(t, float64(42), records[0]["transaction_id"])
			assert.Equal(t, true, records[0]["is_reply"])
			assert.Equal(t, "192.0.2.1:1234", records[0]["remote_addr"])
		})
	}
}

func TestTransactionWriteLoggingDuringClosure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		close func(*ClientConn)
		level string
	}{
		{"disconnect", func(cc *ClientConn) { cc.Disconnect() }, "DEBUG"},
		{"server shutdown", func(cc *ClientConn) { cc.Server.closeActiveConns() }, "DEBUG"},
		{"send queue overflow", func(cc *ClientConn) {
			for range sendQueueDepth {
				cc.Send(NewTransaction(TranServerMsg, ClientID{}))
			}
		}, "DEBUG"},
		{"unexpected closure while draining", func(cc *ClientConn) {
			cc.closeSendQueue()
			_ = cc.Connection.Close()
		}, "WARN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &scriptedConn{}
			logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			conn, peer := net.Pipe()
			defer conn.Close()
			defer peer.Close()
			srv := &Server{ClientMgr: NewMemClientMgr(), Logger: logger}
			require.True(t, srv.trackConn(conn))
			cc := &ClientConn{Connection: conn, Server: srv, Logger: logger}
			cc.Send(NewTransaction(TranServerMsg, ClientID{}))
			// Cleanup is allowed to run before the writer consumes its queued message.
			tc.close(cc)
			cc.writeLoop()
			var failures []map[string]any
			for _, record := range logRecords(t, output) {
				if record["msg"] == "Transaction write failed" {
					failures = append(failures, record)
				}
			}
			require.Len(t, failures, 1)
			assert.Equal(t, tc.level, failures[0]["level"])
		})
	}
}

type signaledWriteConn struct {
	net.Conn
	writing chan struct{}
}

func (c *signaledWriteConn) Write(p []byte) (int, error) {
	close(c.writing)
	return c.Conn.Write(p)
}

func TestTransactionWriteLoggingWhenDisconnectInterruptsWrite(t *testing.T) {
	output := &scriptedConn{}
	logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	writing := make(chan struct{})
	cc := &ClientConn{
		Connection: &signaledWriteConn{Conn: conn, writing: writing}, Logger: logger,
		Server: &Server{ClientMgr: NewMemClientMgr(), Logger: logger},
	}
	cc.Send(NewTransaction(TranServerMsg, ClientID{}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		cc.writeLoop()
	}()
	select {
	case <-writing:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not start")
	}
	cc.Disconnect()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect did not unblock writer")
	}
	records := logRecords(t, output)
	require.Len(t, records, 1)
	assert.Equal(t, "Transaction write failed", records[0]["msg"])
	assert.Equal(t, "DEBUG", records[0]["level"])
}
