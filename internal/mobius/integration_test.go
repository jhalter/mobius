package mobius

// This file provides an in-process, protocol-level end-to-end harness: it builds a fully wired
// Hotline server (real YAML managers on a t.TempDir copy of test/config) on ephemeral ports and
// drives it with the in-repo hotline.Client. The individual regression tests live in e2e_test.go.

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// e2eServer is a running in-process server plus the metadata tests need to connect to it.
type e2eServer struct {
	srv      *hotline.Server
	addr     string // session port, host:port
	xferAddr string // file transfer port (session port + 1), host:port
	cfgDir   string // tempdir holding the copied config + Files tree
}

// startE2EServer builds and starts a fully-wired server on a free ephemeral port pair, returning
// once the session port accepts connections. The server is shut down via t.Cleanup.
//
// Binding is inherently racy: between probing for a free port pair and ListenAndServe claiming it,
// another process (a concurrently-tested package, or anything else on the machine) can steal a
// port. A stolen port surfaces as an early ListenAndServe error, so the serve attempt is retried
// on a fresh pair rather than failing the test.
func startE2EServer(t *testing.T) *e2eServer {
	t.Helper()

	cfgDir := t.TempDir()
	copyDirRecursiveTest(t, "test/config", cfgDir)

	cfg, err := LoadConfig(filepath.Join(cfgDir, "config.yaml"))
	require.NoError(t, err)
	// The fixture's FileRoot is a placeholder; point it at the copied Files tree.
	cfg.FileRoot = filepath.Join(cfgDir, "Files")

	// Wire the concrete managers exactly as cmd/mobius-hotline-server/main.go does.
	messageBoard, err := NewFlatNews(path.Join(cfgDir, "MessageBoard.txt"))
	require.NoError(t, err)

	banFile, err := NewBanFile(path.Join(cfgDir, "Banlist.yaml"))
	require.NoError(t, err)

	threadedNews, err := NewThreadedNewsYAML(path.Join(cfgDir, "ThreadedNews.yaml"))
	require.NoError(t, err)

	am, err := NewYAMLAccountManager(path.Join(cfgDir, "Users/"))
	require.NoError(t, err)

	agreement, err := NewAgreement(cfgDir, "\r")
	require.NoError(t, err)

	// Create an admin account with a known password so tests that need elevated rights can log in
	// deterministically. The client obfuscates passwords with EncodeString and the server bcrypt-
	// compares that obfuscated form directly (see ClientConn.Authenticate), so the stored account
	// must hash the obfuscated password — exactly what HandleNewUser does for real accounts.
	var adminAccess hotline.AccessBitmap
	for i := 0; i <= hotline.AccessSendPrivMsg; i++ {
		adminAccess.Set(i)
	}
	obfuscatedPass := string(hotline.EncodeString([]byte(e2eAdminPass)))
	require.NoError(t, am.Create(*hotline.NewAccount(e2eAdminLogin, "e2e admin", obfuscatedPass, adminAccess)))

	for attempt := 0; attempt < 5; attempt++ {
		port := findFreePortPairTest(t)

		srv, err := hotline.NewServer(
			hotline.WithInterface("127.0.0.1"),
			hotline.WithPort(port),
			hotline.WithConfig(*cfg),
			hotline.WithLogger(NewTestLogger()),
			// Disable per-IP connection throttling so multi-client tests don't pay 2s per connection.
			hotline.WithConnectionRateLimit(rate.Inf, 1),
		)
		require.NoError(t, err)

		srv.MessageBoard = messageBoard
		srv.BanList = banFile
		srv.ThreadedNewsMgr = threadedNews
		srv.AccountManager = am
		srv.Agreement = agreement
		RegisterHandlers(srv)

		ctx, cancel := context.WithCancel(context.Background())
		serveErr := make(chan error, 1)
		go func() { serveErr <- srv.ListenAndServe(ctx) }()

		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if !waitForListener(t, addr, serveErr) {
			cancel()
			continue
		}

		t.Cleanup(func() {
			cancel()
			select {
			case <-serveErr:
			case <-time.After(10 * time.Second):
				t.Error("server did not shut down within 10s")
			}
		})

		return &e2eServer{
			srv:      srv,
			addr:     addr,
			xferAddr: fmt.Sprintf("127.0.0.1:%d", port+1),
			cfgDir:   cfgDir,
		}
	}

	t.Fatal("could not start e2e server: probed port pairs kept being claimed by other processes")
	return nil
}

const (
	e2eAdminLogin = "e2e-admin"
	e2eAdminPass  = "e2e-admin-pw"
)

// waitForListener retry-dials until the server accepts a TCP connection, returning true on
// success. It returns false if ListenAndServe returned early instead — a bind failure, meaning
// another process claimed a probed port first — which the caller treats as a retry signal. A
// dial can even "succeed" against that other process's listener, so a successful dial only counts
// after the bind error has had a moment to surface. A server that neither accepts nor errors
// within the deadline fails the test.
func waitForListener(t *testing.T, addr string, serveErr <-chan error) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-serveErr:
			return false
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			select {
			case <-serveErr:
				return false
			case <-time.After(20 * time.Millisecond):
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s never became ready", addr)
	return false
}

// findFreePortPairTest finds a port p such that both p and p+1 are free on the loopback interface.
// Duplicated from hotline/server_test.go's findFreePortPair (that copy is in-package and unexported).
func findFreePortPairTest(t *testing.T) int {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		l0, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		p := l0.Addr().(*net.TCPAddr).Port
		l1, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
		_ = l0.Close()
		if err != nil {
			continue
		}
		_ = l1.Close()
		return p
	}
	t.Fatal("could not find a free consecutive port pair")
	return 0
}

// copyDirRecursiveTest copies a directory tree from src into dst, creating dst subdirectories as
// needed. It mirrors copyDirRecursive in cmd/mobius-hotline-server (not importable from here).
func copyDirRecursiveTest(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dst, 0755))

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDirRecursiveTest(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(srcPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dstPath, data, 0644))
	}
}

// --- E2E client driver -----------------------------------------------------------------------

// e2eClient wraps hotline.Client with request/reply correlation and collection of unsolicited
// server-pushed transactions, so tests can drive the protocol synchronously.
type e2eClient struct {
	t      *testing.T
	client *hotline.Client
	cancel context.CancelFunc

	mu       sync.Mutex
	waiters  map[[4]byte]chan hotline.Transaction // reply ID -> waiter
	incoming []hotline.Transaction                // unsolicited (IsReply==0) transactions
	incCh    chan hotline.Transaction             // signals a new unsolicited transaction
}

// connectE2E performs handshake + login and starts the read loop. It auto-replies to the agreement
// prompt so the session becomes fully established. The login doubles as the display name.
func connectE2E(t *testing.T, addr, login, pass string) *e2eClient {
	t.Helper()
	return connectE2ENamed(t, addr, login, login, pass)
}

// connectE2ENamed is connectE2E with a display name distinct from the login, so tests that run
// several sessions on the same account can tell them apart in user lists and notifications.
func connectE2ENamed(t *testing.T, addr, name, login, pass string) *e2eClient {
	t.Helper()

	c := hotline.NewClient(name, NewTestLogger())
	ec := &e2eClient{
		t:       t,
		client:  c,
		waiters: map[[4]byte]chan hotline.Transaction{},
		incCh:   make(chan hotline.Transaction, 64),
	}

	// Register the same dispatcher for every transaction type the tests care about. Replies are
	// re-typed to their request type by the client before dispatch, so one handler covers both
	// request-reply and server-push flows.
	for _, tt2 := range dispatchTypes {
		c.HandleFunc(tt2, ec.dispatch)
	}

	require.NoError(t, c.Connect(addr, login, pass))

	ctx, cancel := context.WithCancel(context.Background())
	ec.cancel = cancel
	go func() { _ = c.HandleTransactions(ctx) }()

	t.Cleanup(func() {
		cancel()
		_ = c.Disconnect()
	})

	return ec
}

// dispatchTypes is the set of transaction types the e2e dispatcher is registered for: every request
// type the tests send (replies arrive re-typed to these) plus server-pushed types.
var dispatchTypes = [][2]byte{
	hotline.TranLogin, hotline.TranAgreed, hotline.TranShowAgreement,
	hotline.TranChatSend, hotline.TranChatMsg,
	hotline.TranGetMsgs, hotline.TranOldPostNews,
	hotline.TranGetNewsCatNameList, hotline.TranGetNewsArtNameList,
	hotline.TranGetNewsArtData, hotline.TranPostNewsArt,
	hotline.TranGetFileNameList, hotline.TranDownloadFile, hotline.TranUploadFile,
	hotline.TranGetUser, hotline.TranNewUser, hotline.TranSetUser, hotline.TranDeleteUser,
	hotline.TranListUsers, hotline.TranGetUserNameList,
	hotline.TranNotifyChangeUser, hotline.TranNotifyDeleteUser,
	hotline.TranServerMsg, hotline.TranDisconnectMsg, hotline.TranUserAccess,
	hotline.TranInviteNewChat, hotline.TranInviteToChat, hotline.TranJoinChat,
	hotline.TranNotifyChatChangeUser, hotline.TranNotifyChatDeleteUser, hotline.TranNotifyChatSubject,
}

func (ec *e2eClient) dispatch(_ context.Context, _ *hotline.Client, t *hotline.Transaction) ([]hotline.Transaction, error) {
	if t.IsReply == 1 {
		ec.mu.Lock()
		ch := ec.waiters[t.ID]
		delete(ec.waiters, t.ID)
		ec.mu.Unlock()
		if ch != nil {
			ch <- *t
		}
		return nil, nil
	}

	// Unsolicited server push.
	if t.Type == hotline.TranShowAgreement {
		// Accept the agreement so login completes.
		return []hotline.Transaction{
			hotline.NewTransaction(hotline.TranAgreed, [2]byte{},
				hotline.NewField(hotline.FieldUserName, []byte(ec.client.Pref.Username)),
				hotline.NewField(hotline.FieldUserIconID, ec.client.Pref.IconBytes()),
				hotline.NewField(hotline.FieldOptions, []byte{0, 0}),
			),
		}, nil
	}

	ec.mu.Lock()
	ec.incoming = append(ec.incoming, *t)
	ec.mu.Unlock()
	select {
	case ec.incCh <- *t:
	default:
	}
	return nil, nil
}

// roundTrip sends a request and waits for the matching reply (correlated by transaction ID).
func (ec *e2eClient) roundTrip(req hotline.Transaction) hotline.Transaction {
	ec.t.Helper()

	ch := make(chan hotline.Transaction, 1)
	ec.mu.Lock()
	ec.waiters[req.ID] = ch
	ec.mu.Unlock()

	require.NoError(ec.t, ec.client.Send(req))

	select {
	case reply := <-ch:
		return reply
	case <-time.After(15 * time.Second):
		ec.t.Fatalf("timed out waiting for reply to %v", req.Type)
		return hotline.Transaction{}
	}
}

// waitFor blocks until an unsolicited transaction of the given type arrives, or the test fails.
// It polls the buffer on a short interval as well as waking on incCh, so a signal that races with
// the buffer scan can never cause a lost wakeup.
func (ec *e2eClient) waitFor(tranType [2]byte) hotline.Transaction {
	ec.t.Helper()

	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Scan anything already buffered.
		ec.mu.Lock()
		for i, tr := range ec.incoming {
			if tr.Type == tranType {
				ec.incoming = append(ec.incoming[:i], ec.incoming[i+1:]...)
				ec.mu.Unlock()
				return tr
			}
		}
		ec.mu.Unlock()

		select {
		case <-ec.incCh:
			// Woke on a new arrival; loop and re-scan.
		case <-ticker.C:
			// Periodic re-scan guards against a dropped incCh signal.
		case <-deadline:
			ec.t.Fatalf("timed out waiting for unsolicited %v", tranType)
			return hotline.Transaction{}
		}
	}
}

// isError reports whether a reply carries a Hotline error (non-zero ErrorCode).
func isError(t hotline.Transaction) bool {
	return t.ErrorCode != [4]byte{0, 0, 0, 0}
}

// --- transfer-port helpers -------------------------------------------------------------------

// htxfHeader builds the 16-byte HTXF transfer header (protocol + reference number + data size).
func htxfHeader(refNum [4]byte, dataSize uint32) []byte {
	b := make([]byte, 16)
	copy(b[0:4], hotline.HTXF[:])
	copy(b[4:8], refNum[:])
	b[8] = byte(dataSize >> 24)
	b[9] = byte(dataSize >> 16)
	b[10] = byte(dataSize >> 8)
	b[11] = byte(dataSize)
	return b
}

// downloadOverTransferPort dials the transfer port, sends the HTXF header for refNum, and returns
// all bytes the server streams back (the flattened file object).
func downloadOverTransferPort(t *testing.T, xferAddr string, refNum [4]byte) []byte {
	t.Helper()
	conn, err := net.DialTimeout("tcp", xferAddr, 5*time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Write(htxfHeader(refNum, 0))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	data, err := io.ReadAll(conn)
	// A read deadline or server-initiated close both surface here; the payload collected so far is
	// what we assert on.
	if err != nil && !isTimeout(err) {
		require.ErrorIs(t, err, io.EOF)
	}
	return data
}

// uploadOverTransferPort dials the transfer port, sends the HTXF header, then streams payload.
func uploadOverTransferPort(t *testing.T, xferAddr string, refNum [4]byte, payload []byte) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", xferAddr, 5*time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Write(htxfHeader(refNum, uint32(len(payload))))
	require.NoError(t, err)
	_, err = conn.Write(payload)
	require.NoError(t, err)

	// Give the server a moment to persist before the caller asserts.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _ = io.ReadAll(conn)
}

func isTimeout(err error) bool {
	var ne net.Error
	if e, ok := err.(net.Error); ok {
		ne = e
	}
	return ne != nil && ne.Timeout()
}
