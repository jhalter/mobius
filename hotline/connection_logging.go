package hotline

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"syscall"
	"time"
)

// connectionLog is owned by the connection's reader goroutine. Its logger is also
// shared with the client writer, so the logger itself must remain immutable.
type connectionLog struct {
	logger        *slog.Logger
	started       time.Time
	stage         string
	handshakeOK   bool
	authenticated bool
	login         string
	requests      int
	handled       int
	reason        string
}

var errInvalidTransaction = errors.New("invalid transaction")
var errTransactionRead = errors.New("transaction read failed")

func newConnectionLog(logger *slog.Logger, remoteAddr string) *connectionLog {
	ip, _, _ := net.SplitHostPort(remoteAddr)
	return &connectionLog{
		logger:  logger.With("conn_id", rand.Text(), "ip", ip, "remote_addr", remoteAddr),
		started: time.Now(),
		stage:   "handshake",
	}
}

func (c *connectionLog) closed(ctx context.Context, err error) {
	level, reason := connectionCloseReason(err)
	if err == nil && c.reason != "" {
		reason = c.reason
	}
	if ctx.Err() != nil && (err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled)) {
		level, reason = slog.LevelInfo, "server_shutdown"
	}
	attrs := []any{
		"stage", c.stage, "handshake_ok", c.handshakeOK,
		"authenticated", c.authenticated, "requests_received", c.requests,
		"requests_handled", c.handled, "duration_ms", time.Since(c.started).Milliseconds(),
		"reason", reason,
	}
	if c.login != "" {
		attrs = append(attrs, "login", c.login)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		attrs = append(attrs, "err", err)
	}
	var framing *transactionFramingError
	if errors.As(err, &framing) {
		attrs = append(attrs, "buffered_bytes", framing.buffered, "expected_bytes", framing.expected)
	}
	c.logger.Log(ctx, level, "Connection closed", attrs...)
}

func connectionCloseReason(err error) (slog.Level, string) {
	var framing *transactionFramingError
	var netErr net.Error
	switch {
	case err == nil, errors.Is(err, io.EOF):
		return slog.LevelInfo, "peer_closed"
	case errors.As(err, &framing):
		return slog.LevelWarn, framing.reason
	case errors.Is(err, bufio.ErrTooLong):
		return slog.LevelWarn, "transaction_too_large"
	case errors.Is(err, io.ErrUnexpectedEOF):
		if errors.Is(err, errTransactionRead) {
			return slog.LevelWarn, "truncated_transaction"
		}
		return slog.LevelWarn, "truncated_handshake"
	case errors.Is(err, errInvalidHandshake):
		return slog.LevelWarn, "invalid_handshake"
	case errors.Is(err, errInvalidTransaction):
		return slog.LevelWarn, "invalid_transaction"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return slog.LevelInfo, "connection_reset"
	case errors.Is(err, net.ErrClosed):
		return slog.LevelInfo, "connection_closed"
	case errors.As(err, &netErr) && netErr.Timeout():
		return slog.LevelInfo, "timeout"
	case errors.Is(err, errTransactionRead):
		return slog.LevelWarn, "read_error"
	default:
		return slog.LevelError, "internal_error"
	}
}

// Scanner calls its split function with atEOF=true for any read error, not only
// EOF. Preserve a reset/timeout rather than replacing it with a truncation error.
type transactionReader struct {
	io.Reader
	err error
}

func (r *transactionReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil {
		r.err = err
	}
	return n, err
}

func (r *transactionReader) scanError(scanner *bufio.Scanner) error {
	if r.err != nil && !errors.Is(r.err, io.EOF) {
		return r.err
	}
	return scanner.Err()
}
