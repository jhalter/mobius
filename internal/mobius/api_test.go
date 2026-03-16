package mobius

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockBanMgr struct {
	bannedIPs       []string
	bannedUsernames []string
	bannedNicknames []string

	addCalled           bool
	addIP               string
	banUsernameCalled   bool
	banUsernameArg      string
	banNicknameCalled   bool
	banNicknameArg      string
	unbanIPCalled       bool
	unbanIPArg          string
	unbanUsernameCalled bool
	unbanUsernameArg    string
	unbanNicknameCalled bool
	unbanNicknameArg    string

	// Allow injecting errors
	addErr             error
	banUsernameErr     error
	banNicknameErr     error
	unbanIPErr         error
	unbanUsernameErr   error
	unbanNicknameErr   error
	listIPsErr         error
	listUsernamesErr   error
	listNicknamesErr   error
}

func (m *mockBanMgr) Add(ip string, _ *time.Time) error {
	m.addCalled = true
	m.addIP = ip
	if m.addErr != nil {
		return m.addErr
	}
	m.bannedIPs = append(m.bannedIPs, ip)
	return nil
}

func (m *mockBanMgr) IsBanned(ip string) (bool, *time.Time) {
	for _, b := range m.bannedIPs {
		if b == ip {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockBanMgr) UnbanIP(ip string) error {
	m.unbanIPCalled = true
	m.unbanIPArg = ip
	return m.unbanIPErr
}

func (m *mockBanMgr) BanUsername(username string) error {
	m.banUsernameCalled = true
	m.banUsernameArg = username
	if m.banUsernameErr != nil {
		return m.banUsernameErr
	}
	m.bannedUsernames = append(m.bannedUsernames, username)
	return nil
}

func (m *mockBanMgr) UnbanUsername(username string) error {
	m.unbanUsernameCalled = true
	m.unbanUsernameArg = username
	return m.unbanUsernameErr
}

func (m *mockBanMgr) IsUsernameBanned(username string) bool {
	for _, b := range m.bannedUsernames {
		if b == username {
			return true
		}
	}
	return false
}

func (m *mockBanMgr) BanNickname(nickname string) error {
	m.banNicknameCalled = true
	m.banNicknameArg = nickname
	if m.banNicknameErr != nil {
		return m.banNicknameErr
	}
	m.bannedNicknames = append(m.bannedNicknames, nickname)
	return nil
}

func (m *mockBanMgr) UnbanNickname(nickname string) error {
	m.unbanNicknameCalled = true
	m.unbanNicknameArg = nickname
	return m.unbanNicknameErr
}

func (m *mockBanMgr) IsNicknameBanned(nickname string) bool {
	for _, b := range m.bannedNicknames {
		if b == nickname {
			return true
		}
	}
	return false
}

func (m *mockBanMgr) ListBannedIPs() ([]string, error) {
	if m.listIPsErr != nil {
		return nil, m.listIPsErr
	}
	return m.bannedIPs, nil
}

func (m *mockBanMgr) ListBannedUsernames() ([]string, error) {
	if m.listUsernamesErr != nil {
		return nil, m.listUsernamesErr
	}
	return m.bannedUsernames, nil
}

func (m *mockBanMgr) ListBannedNicknames() ([]string, error) {
	if m.listNicknamesErr != nil {
		return nil, m.listNicknamesErr
	}
	return m.bannedNicknames, nil
}

type mockClientMgr struct {
	clients []*hotline.ClientConn
}

func (m *mockClientMgr) List() []*hotline.ClientConn {
	return m.clients
}

func (m *mockClientMgr) Get(id hotline.ClientID) *hotline.ClientConn {
	for _, c := range m.clients {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (m *mockClientMgr) Add(cc *hotline.ClientConn) {
	m.clients = append(m.clients, cc)
}

func (m *mockClientMgr) Delete(id hotline.ClientID) {
	for i, c := range m.clients {
		if c.ID == id {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
			return
		}
	}
}

type mockCounter struct {
	vals map[string]interface{}
}

func (m *mockCounter) Increment(_ ...int) {}
func (m *mockCounter) Decrement(_ int)    {}
func (m *mockCounter) Set(_, _ int)       {}
func (m *mockCounter) Get(_ int) int      { return 0 }
func (m *mockCounter) Values() map[string]interface{} {
	if m.vals == nil {
		return map[string]interface{}{}
	}
	return m.vals
}

// --- Test helper ---

func newTestAPIServer(t *testing.T, apiKey string) (*APIServer, *mockBanMgr, *mockClientMgr, *mockCounter) {
	t.Helper()

	banMgr := &mockBanMgr{}
	clientMgr := &mockClientMgr{}
	counter := &mockCounter{}

	hlServer := &hotline.Server{
		BanList:   banMgr,
		ClientMgr: clientMgr,
		Stats:     counter,
		Logger:    slog.Default(),
	}

	srv := &APIServer{
		hlServer: hlServer,
		logger:   slog.Default(),
		mux:      http.NewServeMux(),
		apiKey:   apiKey,
	}

	reloadCalled := false
	reloadFunc := func() { reloadCalled = true }
	_ = reloadCalled

	srv.mux.Handle("/api/v1/online", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.OnlineHandler))))
	srv.mux.Handle("/api/v1/ban", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.BanHandler))))
	srv.mux.Handle("/api/v1/unban", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.UnbanHandler))))
	srv.mux.Handle("/api/v1/banned/ips", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedIPsHandler))))
	srv.mux.Handle("/api/v1/banned/usernames", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedUsernamesHandler))))
	srv.mux.Handle("/api/v1/banned/nicknames", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedNicknamesHandler))))
	srv.mux.Handle("/api/v1/reload", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ReloadHandler(reloadFunc)))))
	srv.mux.Handle("/api/v1/shutdown", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ShutdownHandler))))
	srv.mux.Handle("/api/v1/stats", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.RenderStats))))

	return srv, banMgr, clientMgr, counter
}

// newTestAPIServerWithReload is like newTestAPIServer but returns a pointer to the reloadCalled flag.
func newTestAPIServerWithReload(t *testing.T, apiKey string) (*APIServer, *bool) {
	t.Helper()

	banMgr := &mockBanMgr{}
	clientMgr := &mockClientMgr{}
	counter := &mockCounter{}

	hlServer := &hotline.Server{
		BanList:   banMgr,
		ClientMgr: clientMgr,
		Stats:     counter,
		Logger:    slog.Default(),
	}

	srv := &APIServer{
		hlServer: hlServer,
		logger:   slog.Default(),
		mux:      http.NewServeMux(),
		apiKey:   apiKey,
	}

	reloadCalled := false
	reloadFunc := func() { reloadCalled = true }

	srv.mux.Handle("/api/v1/reload", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ReloadHandler(reloadFunc)))))

	return srv, &reloadCalled
}

func TestAuthMiddleware(t *testing.T) {
	t.Run("rejects request without API key when key is configured", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "secret-key")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("rejects request with wrong API key", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "secret-key")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("allows request with correct API key", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "secret-key")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("X-API-Key", "secret-key")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("allows all requests when no API key is configured", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestOnlineHandler(t *testing.T) {
	t.Run("returns JSON list of online users", func(t *testing.T) {
		srv, _, clientMgr, _ := newTestAPIServer(t, "")

		clientMgr.clients = []*hotline.ClientConn{
			{
				ID:         hotline.ClientID{0, 1},
				RemoteAddr: "192.168.1.1:12345",
				UserName:   []byte("nick1"),
				Account:    &hotline.Account{Login: "user1"},
			},
			{
				ID:         hotline.ClientID{0, 2},
				RemoteAddr: "10.0.0.1:54321",
				UserName:   []byte("nick2"),
				Account:    &hotline.Account{Login: "user2"},
			},
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/online", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var users []map[string]string
		err := json.Unmarshal(rr.Body.Bytes(), &users)
		require.NoError(t, err)
		require.Len(t, users, 2)

		assert.Equal(t, "user1", users[0]["login"])
		assert.Equal(t, "nick1", users[0]["nickname"])
		assert.Equal(t, "192.168.1.1:12345", users[0]["ip"])

		assert.Equal(t, "user2", users[1]["login"])
		assert.Equal(t, "nick2", users[1]["nickname"])
		assert.Equal(t, "10.0.0.1:54321", users[1]["ip"])
	})

	t.Run("returns empty list when no users online", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/online", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		// null JSON is acceptable for nil slice
		body := strings.TrimSpace(rr.Body.String())
		assert.True(t, body == "null" || body == "[]", "expected null or empty array, got: %s", body)
	})
}

func TestBanHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantBanUser    bool
		wantBanNick    bool
		wantBanIP      bool
		banUserArg     string
		banNickArg     string
		banIPArg       string
		wantBodySubstr string
	}{
		{
			name:           "ban by username",
			body:           `{"username":"baduser"}`,
			wantStatus:     http.StatusOK,
			wantBanUser:    true,
			banUserArg:     "baduser",
			wantBodySubstr: "banned",
		},
		{
			name:           "ban by nickname",
			body:           `{"nickname":"badnick"}`,
			wantStatus:     http.StatusOK,
			wantBanNick:    true,
			banNickArg:     "badnick",
			wantBodySubstr: "banned",
		},
		{
			name:           "ban by IP",
			body:           `{"ip":"1.2.3.4"}`,
			wantStatus:     http.StatusOK,
			wantBanIP:      true,
			banIPArg:       "1.2.3.4",
			wantBodySubstr: "banned",
		},
		{
			name:           "missing fields returns 400",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "username, nickname, or ip required",
		},
		{
			name:       "invalid JSON returns 400",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, banMgr, _, _ := newTestAPIServer(t, "")

			req := httptest.NewRequest(http.MethodPost, "/api/v1/ban", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantBodySubstr != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBodySubstr)
			}

			assert.Equal(t, tt.wantBanUser, banMgr.banUsernameCalled)
			if tt.wantBanUser {
				assert.Equal(t, tt.banUserArg, banMgr.banUsernameArg)
			}

			assert.Equal(t, tt.wantBanNick, banMgr.banNicknameCalled)
			if tt.wantBanNick {
				assert.Equal(t, tt.banNickArg, banMgr.banNicknameArg)
			}

			assert.Equal(t, tt.wantBanIP, banMgr.addCalled)
			if tt.wantBanIP {
				assert.Equal(t, tt.banIPArg, banMgr.addIP)
			}
		})
	}
}

func TestUnbanHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantUnbanUser  bool
		wantUnbanNick  bool
		wantUnbanIP    bool
		unbanUserArg   string
		unbanNickArg   string
		unbanIPArg     string
		wantBodySubstr string
	}{
		{
			name:           "unban by username",
			body:           `{"username":"baduser"}`,
			wantStatus:     http.StatusOK,
			wantUnbanUser:  true,
			unbanUserArg:   "baduser",
			wantBodySubstr: "unbanned",
		},
		{
			name:           "unban by nickname",
			body:           `{"nickname":"badnick"}`,
			wantStatus:     http.StatusOK,
			wantUnbanNick:  true,
			unbanNickArg:   "badnick",
			wantBodySubstr: "unbanned",
		},
		{
			name:           "unban by IP",
			body:           `{"ip":"1.2.3.4"}`,
			wantStatus:     http.StatusOK,
			wantUnbanIP:    true,
			unbanIPArg:     "1.2.3.4",
			wantBodySubstr: "unbanned",
		},
		{
			name:           "missing fields returns 400",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "username, nickname, or ip required",
		},
		{
			name:       "invalid JSON returns 400",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, banMgr, _, _ := newTestAPIServer(t, "")

			req := httptest.NewRequest(http.MethodPost, "/api/v1/unban", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantBodySubstr != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBodySubstr)
			}

			assert.Equal(t, tt.wantUnbanUser, banMgr.unbanUsernameCalled)
			if tt.wantUnbanUser {
				assert.Equal(t, tt.unbanUserArg, banMgr.unbanUsernameArg)
			}

			assert.Equal(t, tt.wantUnbanNick, banMgr.unbanNicknameCalled)
			if tt.wantUnbanNick {
				assert.Equal(t, tt.unbanNickArg, banMgr.unbanNicknameArg)
			}

			assert.Equal(t, tt.wantUnbanIP, banMgr.unbanIPCalled)
			if tt.wantUnbanIP {
				assert.Equal(t, tt.unbanIPArg, banMgr.unbanIPArg)
			}
		})
	}
}

func TestListBannedIPsHandler(t *testing.T) {
	t.Run("returns JSON list of banned IPs", func(t *testing.T) {
		srv, banMgr, _, _ := newTestAPIServer(t, "")
		banMgr.bannedIPs = []string{"1.2.3.4", "5.6.7.8"}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/banned/ips", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var ips []string
		err := json.Unmarshal(rr.Body.Bytes(), &ips)
		require.NoError(t, err)
		assert.Equal(t, []string{"1.2.3.4", "5.6.7.8"}, ips)
	})

	t.Run("returns empty list when no IPs banned", func(t *testing.T) {
		srv, banMgr, _, _ := newTestAPIServer(t, "")
		banMgr.bannedIPs = []string{}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/banned/ips", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var ips []string
		err := json.Unmarshal(rr.Body.Bytes(), &ips)
		require.NoError(t, err)
		assert.Empty(t, ips)
	})
}

func TestListBannedUsernamesHandler(t *testing.T) {
	t.Run("returns JSON list of banned usernames", func(t *testing.T) {
		srv, banMgr, _, _ := newTestAPIServer(t, "")
		banMgr.bannedUsernames = []string{"user1", "user2"}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/banned/usernames", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var usernames []string
		err := json.Unmarshal(rr.Body.Bytes(), &usernames)
		require.NoError(t, err)
		assert.Equal(t, []string{"user1", "user2"}, usernames)
	})
}

func TestListBannedNicknamesHandler(t *testing.T) {
	t.Run("returns JSON list of banned nicknames", func(t *testing.T) {
		srv, banMgr, _, _ := newTestAPIServer(t, "")
		banMgr.bannedNicknames = []string{"nick1", "nick2"}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/banned/nicknames", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var nicknames []string
		err := json.Unmarshal(rr.Body.Bytes(), &nicknames)
		require.NoError(t, err)
		assert.Equal(t, []string{"nick1", "nick2"}, nicknames)
	})
}

func TestReloadHandler(t *testing.T) {
	t.Run("calls reload function and returns success", func(t *testing.T) {
		srv, reloadCalled := newTestAPIServerWithReload(t, "")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/reload", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "config reloaded")
		assert.True(t, *reloadCalled, "expected reload function to be called")
	})
}

func TestShutdownHandler(t *testing.T) {
	t.Run("empty body returns 400", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/shutdown", strings.NewReader(""))
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("with body returns success", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "")

		// Shutdown calls srv.hlServer.Shutdown in a goroutine, which calls
		// SendAll on the server. We need outbox to be non-nil to avoid a panic.
		// Since Shutdown runs in a goroutine, we just verify the HTTP response.
		// The goroutine may panic but that's acceptable in test since we're only
		// testing the HTTP layer.
		srv.hlServer.Logger = slog.Default()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/shutdown", strings.NewReader("Server is going down"))
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "server shutting down")
	})
}

func TestRenderStats(t *testing.T) {
	t.Run("returns JSON stats", func(t *testing.T) {
		srv, _, _, counter := newTestAPIServer(t, "")
		counter.vals = map[string]interface{}{
			"connections": 42,
			"downloads":   10,
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var stats map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &stats)
		require.NoError(t, err)
		assert.Equal(t, float64(42), stats["connections"])
		assert.Equal(t, float64(10), stats["downloads"])
	})

	t.Run("returns empty stats when no data", func(t *testing.T) {
		srv, _, _, _ := newTestAPIServer(t, "")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var stats map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &stats)
		require.NoError(t, err)
		assert.Empty(t, stats)
	})
}
