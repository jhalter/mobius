package hotline

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/encoding/charmap"
)

// scriptedConn is an io.ReadWriteCloser that serves a fixed script of bytes to the server (the
// handshake plus a login transaction) and captures everything the server writes back.  Reads are
// driven by handleNewConnection's main goroutine while writes come from the ClientConn.writeLoop
// goroutine, so the write buffer is mutex-protected for -race.
type scriptedConn struct {
	in  *bytes.Buffer
	mu  sync.Mutex
	out bytes.Buffer
}

func (c *scriptedConn) Read(p []byte) (int, error) { return c.in.Read(p) }

func (c *scriptedConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.Write(p)
}

func (c *scriptedConn) Close() error { return nil }

func (c *scriptedConn) written() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.out.Bytes())
}

// serializeTransaction renders a transaction to its wire bytes.
func serializeTransaction(t *testing.T, tran Transaction) []byte {
	t.Helper()
	var buf bytes.Buffer
	_, err := io.Copy(&buf, &tran)
	require.NoError(t, err)
	return buf.Bytes()
}

// loginInput builds the byte script a client sends on connect: the handshake followed by a
// TranLogin transaction carrying the given credentials and optional extra fields.
func loginInput(t *testing.T, login, password string, extraFields ...Field) *bytes.Buffer {
	t.Helper()

	fields := append([]Field{
		NewField(FieldUserLogin, EncodeString([]byte(login))),
		NewField(FieldUserPassword, EncodeString([]byte(password))),
	}, extraFields...)

	buf := bytes.NewBuffer(bytes.Clone(ClientHandshake))
	buf.Write(serializeTransaction(t, NewTransaction(TranLogin, [2]byte{0, 0}, fields...)))
	return buf
}

// newLoginTestServer builds a Server wired with the minimum dependencies handleNewConnection needs
// to run a login to completion.
func newLoginTestServer(accounts map[string]*Account) *Server {
	return &Server{
		Config:         Config{Name: "Test Server"},
		Logger:         NewTestLogger(),
		Stats:          NewStats(),
		ClientMgr:      NewMemClientMgr(),
		AccountManager: &mockAccountMgr{accounts: accounts},
		Agreement:      bytes.NewReader([]byte("Welcome to the server")),
		TextEncoder:    charmap.Macintosh.NewEncoder(),
		TextDecoder:    charmap.Macintosh.NewDecoder(),
	}
}

// accountWithPassword returns an account whose stored password verifies against the given
// plaintext.  The server authenticates against the obfuscated bytes carried on the wire (login
// never de-obfuscates FieldUserPassword), and account creation hashes those same obfuscated bytes,
// so the stored hash must cover EncodeString(password), not the plaintext.
func accountWithPassword(t *testing.T, login, password string, access ...int) *Account {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword(EncodeString([]byte(password)), bcrypt.MinCost)
	require.NoError(t, err)

	acc := &Account{Login: login, Name: login, Password: string(hash)}
	for _, a := range access {
		acc.Access.Set(a)
	}
	return acc
}

func TestServer_handleNewConnection_successfulLogin(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		"admin": accountWithPassword(t, "admin", "password"),
	})

	conn := &scriptedConn{in: loginInput(t, "admin", "password",
		NewField(FieldVersion, []byte{0, 0xc8}),
	)}

	err := srv.handleNewConnection(context.Background(), conn, "192.168.1.5:1234")
	require.NoError(t, err)

	// Wait for the writeLoop goroutine to flush the queued replies and exit.
	srv.connWG.Wait()

	out := conn.written()

	// The server must complete the handshake first.
	require.GreaterOrEqual(t, len(out), len(ServerHandshake))
	assert.Equal(t, ServerHandshake, out[:len(ServerHandshake)])

	// The login reply, user-access, and agreement transactions should all have been sent, so the
	// server name and agreement text land in the output stream.
	assert.Contains(t, string(out), "Test Server")
	assert.Contains(t, string(out), "Welcome to the server")

	// The connection should have been removed from the client manager on disconnect.
	assert.Empty(t, srv.ClientMgr.List())
}

func TestServer_handleNewConnection_legacyLoginFlowNotifiesPresence(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		// AccessAnyName lets the client keep the name it supplied at login.
		"admin": accountWithPassword(t, "admin", "password", AccessAnyName),
	})
	presence := &stubPresence{}
	srv.Presence = presence

	// A username supplied in the login (and no version field) is the 1.2.3 login flow, which sets
	// the nickname immediately and fires the presence/notify block.
	conn := &scriptedConn{in: loginInput(t, "admin", "password",
		NewField(FieldUserName, []byte("Administrator")),
	)}

	require.NoError(t, srv.handleNewConnection(context.Background(), conn, "172.16.0.9:4000"))
	srv.connWG.Wait()

	assert.Equal(t, []string{"admin"}, presence.connected)
	assert.Equal(t, []string{"admin:Administrator"}, presence.renamed)
	assert.Equal(t, []string{"admin:Administrator"}, presence.disconnected)
}

func TestServer_handleNewConnection_noAgreementAccess(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		"admin": accountWithPassword(t, "admin", "password", AccessNoAgreement),
	})

	conn := &scriptedConn{in: loginInput(t, "admin", "password",
		NewField(FieldVersion, []byte{0, 0xc8}),
	)}

	require.NoError(t, srv.handleNewConnection(context.Background(), conn, "10.0.0.1:5000"))
	srv.connWG.Wait()

	// Accounts with AccessNoAgreement never receive the agreement body.
	assert.NotContains(t, string(conn.written()), "Welcome to the server")
}

func TestServer_handleNewConnection_incorrectLogin(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		"admin": accountWithPassword(t, "admin", "password"),
	})

	conn := &scriptedConn{in: loginInput(t, "admin", "wrongpassword")}

	require.NoError(t, srv.handleNewConnection(context.Background(), conn, "192.168.1.5:1234"))
	srv.connWG.Wait()

	assert.Contains(t, string(conn.written()), "Incorrect login")
	assert.Empty(t, srv.ClientMgr.List())
}

func TestServer_handleNewConnection_bannedUsername(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		"baduser": accountWithPassword(t, "baduser", "password"),
	})
	ban := &stubBanMgr{usernameBanned: true}
	srv.BanList = ban

	conn := &scriptedConn{in: loginInput(t, "baduser", "password")}

	require.NoError(t, srv.handleNewConnection(context.Background(), conn, "1.2.3.4:9999"))
	srv.connWG.Wait()

	assert.Contains(t, string(conn.written()), "banned")
	// The offending IP should have been added to the ban list.
	assert.Equal(t, []string{"1.2.3.4"}, ban.added)
	// A banned user never becomes a live connection.
	assert.Empty(t, srv.ClientMgr.List())
}

func TestServer_handleNewConnection_bannedIP(t *testing.T) {
	srv := newLoginTestServer(map[string]*Account{
		"admin": accountWithPassword(t, "admin", "password"),
	})
	srv.BanList = &stubBanMgr{ipBanned: true} // permaban (nil expiry)

	conn := &scriptedConn{in: loginInput(t, "admin", "password")}

	require.NoError(t, srv.handleNewConnection(context.Background(), conn, "5.6.7.8:1111"))
	srv.connWG.Wait()

	assert.Contains(t, string(conn.written()), "permanently banned")
	assert.Empty(t, srv.ClientMgr.List())
}

func TestServer_handleNewConnection_handshakeFailure(t *testing.T) {
	srv := newLoginTestServer(nil)

	// Wrong protocol bytes: handshake must fail before any login processing.
	conn := &scriptedConn{in: bytes.NewBuffer([]byte("XXXXYYYY\x00\x01\x00\x02"))}

	err := srv.handleNewConnection(context.Background(), conn, "9.9.9.9:2222")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handshake")
}

// stubPresence records PresenceTracker callbacks for assertion.
type stubPresence struct {
	connected    []string
	renamed      []string
	disconnected []string
}

func (s *stubPresence) UserConnected(login, ip string) {
	s.connected = append(s.connected, login)
}
func (s *stubPresence) UserRenamed(login, oldNickname, newNickname, ip string) {
	s.renamed = append(s.renamed, login+":"+newNickname)
}
func (s *stubPresence) UserDisconnected(login, nickname, ip string) {
	s.disconnected = append(s.disconnected, login+":"+nickname)
}

// stubBanMgr is a minimal BanMgr for exercising the ban branches of handleNewConnection.
type stubBanMgr struct {
	usernameBanned bool
	ipBanned       bool
	banUntil       *time.Time
	added          []string
}

func (s *stubBanMgr) Add(ip string, until *time.Time) error {
	s.added = append(s.added, ip)
	return nil
}
func (s *stubBanMgr) IsBanned(ip string) (bool, *time.Time) { return s.ipBanned, s.banUntil }
func (s *stubBanMgr) UnbanIP(ip string) error               { return nil }
func (s *stubBanMgr) BanUsername(username string) error     { return nil }
func (s *stubBanMgr) UnbanUsername(username string) error   { return nil }
func (s *stubBanMgr) IsUsernameBanned(username string) bool { return s.usernameBanned }
func (s *stubBanMgr) BanNickname(nickname string) error     { return nil }
func (s *stubBanMgr) UnbanNickname(nickname string) error   { return nil }
func (s *stubBanMgr) IsNicknameBanned(nickname string) bool { return false }
func (s *stubBanMgr) ListBannedIPs() ([]string, error)      { return nil, nil }
func (s *stubBanMgr) ListBannedUsernames() ([]string, error) { return nil, nil }
func (s *stubBanMgr) ListBannedNicknames() ([]string, error) { return nil, nil }
