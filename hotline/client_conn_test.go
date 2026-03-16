package hotline

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockAccountMgr struct {
	accounts map[string]*Account
}

func (m *mockAccountMgr) Create(account Account) error                  { return nil }
func (m *mockAccountMgr) Update(account Account, newLogin string) error { return nil }
func (m *mockAccountMgr) Get(login string) *Account                     { return m.accounts[login] }
func (m *mockAccountMgr) List() []Account                               { return nil }
func (m *mockAccountMgr) Delete(login string) error                     { return nil }

func TestClientConn_IP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		wantIP     string
	}{
		{
			name:       "extracts IP from host:port",
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "extracts IPv6 address",
			remoteAddr: "[::1]:12345",
			wantIP:     "::1",
		},
		{
			name:       "returns empty string for missing port",
			remoteAddr: "192.168.1.1",
			wantIP:     "",
		},
		{
			name:       "returns empty string for empty RemoteAddr",
			remoteAddr: "",
			wantIP:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{RemoteAddr: tt.remoteAddr}
			assert.Equal(t, tt.wantIP, cc.IP())
		})
	}
}

func TestClientConn_FileRoot(t *testing.T) {
	tests := []struct {
		name            string
		accountFileRoot string
		serverFileRoot  string
		want            string
	}{
		{
			name:            "returns account FileRoot when set",
			accountFileRoot: "/home/user/files",
			serverFileRoot:  "/srv/files",
			want:            "/home/user/files",
		},
		{
			name:            "falls back to server Config FileRoot",
			accountFileRoot: "",
			serverFileRoot:  "/srv/files",
			want:            "/srv/files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{
				Account: &Account{FileRoot: tt.accountFileRoot},
				Server:  &Server{Config: Config{FileRoot: tt.serverFileRoot}},
			}
			assert.Equal(t, tt.want, cc.FileRoot())
		})
	}
}

func TestClientConn_Authenticate(t *testing.T) {
	password := []byte("secret123")
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.MinCost)
	require.NoError(t, err)

	mgr := &mockAccountMgr{
		accounts: map[string]*Account{
			"admin": {Login: "admin", Password: string(hash)},
		},
	}

	tests := []struct {
		name     string
		login    string
		password []byte
		want     bool
	}{
		{
			name:     "valid credentials return true",
			login:    "admin",
			password: password,
			want:     true,
		},
		{
			name:     "wrong password returns false",
			login:    "admin",
			password: []byte("wrongpassword"),
			want:     false,
		},
		{
			name:     "nonexistent login returns false",
			login:    "nobody",
			password: password,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{
				Server: &Server{AccountManager: mgr},
			}
			assert.Equal(t, tt.want, cc.Authenticate(tt.login, tt.password))
		})
	}
}

func TestClientConn_Authorize(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		access  int
		want    bool
	}{
		{
			name:    "returns false when Account is nil",
			account: nil,
			access:  AccessDeleteFile,
			want:    false,
		},
		{
			name: "returns true when access bit is set",
			account: func() *Account {
				a := &Account{}
				a.Access.Set(AccessUploadFile)
				return a
			}(),
			access: AccessUploadFile,
			want:   true,
		},
		{
			name:    "returns false when access bit is not set",
			account: &Account{},
			access:  AccessDownloadFile,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{Account: tt.account}
			assert.Equal(t, tt.want, cc.Authorize(tt.access))
		})
	}
}

func TestClientConn_NewReply(t *testing.T) {
	tests := []struct {
		name     string
		clientID ClientID
		tranID   [4]byte
		fields   []Field
	}{
		{
			name:     "reply with no fields",
			clientID: ClientID{0x00, 0x01},
			tranID:   [4]byte{0x00, 0x00, 0x00, 0x05},
			fields:   nil,
		},
		{
			name:     "reply with fields",
			clientID: ClientID{0x00, 0x02},
			tranID:   [4]byte{0x00, 0x00, 0x00, 0x0A},
			fields:   []Field{NewField(FieldError, []byte("test"))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{ID: tt.clientID}
			tran := &Transaction{ID: tt.tranID}

			reply := cc.NewReply(tran, tt.fields...)

			assert.Equal(t, byte(1), reply.IsReply)
			assert.Equal(t, tt.tranID, reply.ID)
			assert.Equal(t, tt.clientID, reply.ClientID)
			assert.Equal(t, tt.fields, reply.Fields)
		})
	}
}

func TestClientConn_NewErrReply(t *testing.T) {
	tests := []struct {
		name     string
		clientID ClientID
		tranID   [4]byte
		errMsg   string
	}{
		{
			name:     "returns error transaction",
			clientID: ClientID{0x00, 0x01},
			tranID:   [4]byte{0x00, 0x00, 0x00, 0x01},
			errMsg:   "access denied",
		},
		{
			name:     "returns error with empty message",
			clientID: ClientID{0x00, 0x03},
			tranID:   [4]byte{0x00, 0x00, 0x00, 0x02},
			errMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{ID: tt.clientID}
			tran := &Transaction{ID: tt.tranID}

			result := cc.NewErrReply(tran, tt.errMsg)

			require.Len(t, result, 1)
			errReply := result[0]

			assert.Equal(t, byte(1), errReply.IsReply)
			assert.Equal(t, tt.tranID, errReply.ID)
			assert.Equal(t, tt.clientID, errReply.ClientID)
			assert.Equal(t, [4]byte{0, 0, 0, 1}, errReply.ErrorCode)

			require.Len(t, errReply.Fields, 1)
			assert.Equal(t, FieldError, errReply.Fields[0].Type)
			assert.Equal(t, []byte(tt.errMsg), errReply.Fields[0].Data)
		})
	}
}

func TestClientConn_NotifyOthers(t *testing.T) {
	tests := []struct {
		name        string
		selfID      ClientID
		otherConns  []*ClientConn
		wantCount   int
		wantClients []ClientID
	}{
		{
			name:   "sends to other clients, excludes self",
			selfID: ClientID{0x00, 0x01},
			otherConns: []*ClientConn{
				{ID: ClientID{0x00, 0x01}},
				{ID: ClientID{0x00, 0x02}},
				{ID: ClientID{0x00, 0x03}},
			},
			wantCount:   2,
			wantClients: []ClientID{{0x00, 0x02}, {0x00, 0x03}},
		},
		{
			name:   "returns nil when only self is connected",
			selfID: ClientID{0x00, 0x01},
			otherConns: []*ClientConn{
				{ID: ClientID{0x00, 0x01}},
			},
			wantCount:   0,
			wantClients: nil,
		},
		{
			name:        "returns nil when no clients are connected",
			selfID:      ClientID{0x00, 0x01},
			otherConns:  []*ClientConn{},
			wantCount:   0,
			wantClients: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMgr := &MockClientMgr{}
			mockMgr.On("List").Return(tt.otherConns)

			cc := &ClientConn{
				ID:     tt.selfID,
				Server: &Server{ClientMgr: mockMgr},
			}

			tran := NewTransaction(TranChatSend, ClientID{}, NewField(FieldError, []byte("test")))
			result := cc.NotifyOthers(tran)

			assert.Len(t, result, tt.wantCount)

			if tt.wantClients != nil {
				var gotIDs []ClientID
				for _, r := range result {
					gotIDs = append(gotIDs, r.ClientID)
				}
				assert.Equal(t, tt.wantClients, gotIDs)
			}

			mockMgr.AssertExpectations(t)
		})
	}
}

func TestClientConn_String(t *testing.T) {
	cc := &ClientConn{
		UserName:              []byte("TestUser"),
		RemoteAddr:            "192.168.1.10:54321",
		Account:               &Account{Name: "Test Account", Login: "testlogin"},
		ClientFileTransferMgr: NewClientFileTransferMgr(),
	}

	result := cc.String()

	// The method replaces \n with \r
	assert.Contains(t, result, "TestUser")
	assert.Contains(t, result, "Test Account")
	assert.Contains(t, result, "testlogin")
	assert.Contains(t, result, "192.168.1.10:54321")
	assert.Contains(t, result, "None.")
	assert.NotContains(t, result, "\n", "all newlines should be replaced with carriage returns")
	assert.Contains(t, result, "\r")
}

func TestClientConn_Disconnect(t *testing.T) {
	t.Run("removes client and closes connection", func(t *testing.T) {
		mockMgr := &MockClientMgr{}
		mockMgr.On("Delete", ClientID{0, 1}).Return()
		mockMgr.On("List").Return([]*ClientConn{})

		outbox := make(chan Transaction, 10)
		cc := &ClientConn{
			ID:         ClientID{0, 1},
			Connection: &nopCloserRWC{Buffer: &bytes.Buffer{}},
			Server: &Server{
				ClientMgr: mockMgr,
				outbox:    outbox,
				Logger:    NewTestLogger(),
			},
		}

		cc.Disconnect()

		mockMgr.AssertCalled(t, "Delete", ClientID{0, 1})
		assert.Empty(t, outbox) // No other clients to notify
	})

	t.Run("notifies other clients", func(t *testing.T) {
		mockMgr := &MockClientMgr{}
		mockMgr.On("Delete", ClientID{0, 1}).Return()
		mockMgr.On("List").Return([]*ClientConn{
			{ID: ClientID{0, 1}},
			{ID: ClientID{0, 2}},
			{ID: ClientID{0, 3}},
		})

		outbox := make(chan Transaction, 10)
		cc := &ClientConn{
			ID:         ClientID{0, 1},
			Connection: &nopCloserRWC{Buffer: &bytes.Buffer{}},
			Server: &Server{
				ClientMgr: mockMgr,
				outbox:    outbox,
				Logger:    NewTestLogger(),
			},
		}

		cc.Disconnect()

		assert.Len(t, outbox, 2)
		mockMgr.AssertExpectations(t)
	})
}

func TestClientConn_handleTransaction(t *testing.T) {
	t.Run("dispatches to registered handler", func(t *testing.T) {
		outbox := make(chan Transaction, 10)
		mockMgr := &MockClientMgr{}

		cc := &ClientConn{
			ID:      ClientID{0, 1},
			Account: &Account{},
			Logger:  NewTestLogger(),
			Server: &Server{
				outbox:    outbox,
				ClientMgr: mockMgr,
				handlers: map[TranType]HandlerFunc{
					TranChatSend: func(cc *ClientConn, t *Transaction) []Transaction {
						return []Transaction{cc.NewReply(t)}
					},
				},
			},
		}

		cc.handleTransaction(NewTransaction(TranChatSend, ClientID{0, 1}))

		assert.Len(t, outbox, 1)
		assert.Equal(t, 0, cc.IdleTime)
	})

	t.Run("keepalive does not reset idle time", func(t *testing.T) {
		outbox := make(chan Transaction, 10)

		cc := &ClientConn{
			ID:       ClientID{0, 1},
			Account:  &Account{},
			IdleTime: 100,
			Logger:   NewTestLogger(),
			Server: &Server{
				outbox: outbox,
				handlers: map[TranType]HandlerFunc{
					TranKeepAlive: func(cc *ClientConn, t *Transaction) []Transaction {
						return []Transaction{cc.NewReply(t)}
					},
				},
			},
		}

		cc.handleTransaction(NewTransaction(TranKeepAlive, ClientID{0, 1}))

		assert.Equal(t, 100, cc.IdleTime)
	})

	t.Run("non-keepalive clears away flag", func(t *testing.T) {
		outbox := make(chan Transaction, 10)
		mockMgr := &MockClientMgr{}
		mockMgr.On("List").Return([]*ClientConn{
			{ID: ClientID{0, 1}},
		})

		cc := &ClientConn{
			ID:       ClientID{0, 1},
			Account:  &Account{},
			UserName: []byte("test"),
			Icon:     []byte{0, 0},
			IdleTime: 50,
			Logger:   NewTestLogger(),
			Server: &Server{
				outbox:    outbox,
				ClientMgr: mockMgr,
				handlers: map[TranType]HandlerFunc{
					TranChatSend: func(cc *ClientConn, t *Transaction) []Transaction {
						return nil
					},
				},
			},
		}
		cc.Flags.Set(UserFlagAway, 1)

		cc.handleTransaction(NewTransaction(TranChatSend, ClientID{0, 1}))

		assert.Equal(t, 0, cc.IdleTime)
		assert.False(t, cc.Flags.IsSet(UserFlagAway))
		// SendAll should have sent TranNotifyChangeUser
		assert.Greater(t, len(outbox), 0)
	})
}

func TestClientConn_SendAll(t *testing.T) {
	mockMgr := &MockClientMgr{}
	mockMgr.On("List").Return([]*ClientConn{
		{ID: ClientID{0, 1}},
		{ID: ClientID{0, 2}},
		{ID: ClientID{0, 3}},
	})

	outbox := make(chan Transaction, 10)
	cc := &ClientConn{
		ID: ClientID{0, 1},
		Server: &Server{
			ClientMgr: mockMgr,
			outbox:    outbox,
		},
	}

	cc.SendAll(TranChatMsg, NewField(FieldData, []byte("hello")))

	assert.Len(t, outbox, 3)

	clientIDs := make(map[ClientID]bool)
	for range 3 {
		tran := <-outbox
		clientIDs[tran.ClientID] = true
		assert.Equal(t, TranChatMsg, tran.Type)
	}
	assert.True(t, clientIDs[ClientID{0, 1}])
	assert.True(t, clientIDs[ClientID{0, 2}])
	assert.True(t, clientIDs[ClientID{0, 3}])

	mockMgr.AssertExpectations(t)
}
