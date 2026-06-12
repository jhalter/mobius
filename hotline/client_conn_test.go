package hotline

import (
	"bufio"
	"bytes"
	"fmt"
	"sync"
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

		cc := &ClientConn{
			ID:         ClientID{0, 1},
			Connection: &nopCloserRWC{Buffer: &bytes.Buffer{}},
			Server: &Server{
				ClientMgr: mockMgr,
				Logger:    NewTestLogger(),
			},
		}

		cc.Disconnect()

		mockMgr.AssertCalled(t, "Delete", ClientID{0, 1})
	})

	t.Run("notifies other clients", func(t *testing.T) {
		peer2 := &ClientConn{ID: ClientID{0, 2}}
		peer3 := &ClientConn{ID: ClientID{0, 3}}

		mockMgr := &MockClientMgr{}
		mockMgr.On("Delete", ClientID{0, 1}).Return()
		mockMgr.On("List").Return([]*ClientConn{
			{ID: ClientID{0, 1}},
			peer2,
			peer3,
		})
		mockMgr.On("Get", ClientID{0, 2}).Return(peer2)
		mockMgr.On("Get", ClientID{0, 3}).Return(peer3)

		cc := &ClientConn{
			ID:         ClientID{0, 1},
			Connection: &nopCloserRWC{Buffer: &bytes.Buffer{}},
			Server: &Server{
				ClientMgr: mockMgr,
				Logger:    NewTestLogger(),
			},
		}

		cc.Disconnect()

		assert.Len(t, peer2.sendCh, 1)
		assert.Len(t, peer3.sendCh, 1)
		mockMgr.AssertExpectations(t)
	})
}

func TestClientConn_handleTransaction(t *testing.T) {
	t.Run("dispatches to registered handler", func(t *testing.T) {
		mockMgr := &MockClientMgr{}

		cc := &ClientConn{
			ID:      ClientID{0, 1},
			Account: &Account{},
			Logger:  NewTestLogger(),
			Server: &Server{
				ClientMgr: mockMgr,
				handlers: map[TranType]HandlerFunc{
					TranChatSend: func(cc *ClientConn, t *Transaction) []Transaction {
						return []Transaction{cc.NewReply(t)}
					},
				},
			},
		}
		mockMgr.On("Get", ClientID{0, 1}).Return(cc)

		cc.handleTransaction(NewTransaction(TranChatSend, ClientID{0, 1}))

		assert.Len(t, cc.sendCh, 1)
		assert.Equal(t, 0, cc.IdleTime)
	})

	t.Run("keepalive does not reset idle time", func(t *testing.T) {
		mockMgr := &MockClientMgr{}

		cc := &ClientConn{
			ID:       ClientID{0, 1},
			Account:  &Account{},
			IdleTime: 100,
			Logger:   NewTestLogger(),
			Server: &Server{
				ClientMgr: mockMgr,
				handlers: map[TranType]HandlerFunc{
					TranKeepAlive: func(cc *ClientConn, t *Transaction) []Transaction {
						return []Transaction{cc.NewReply(t)}
					},
				},
			},
		}
		mockMgr.On("Get", ClientID{0, 1}).Return(cc)

		cc.handleTransaction(NewTransaction(TranKeepAlive, ClientID{0, 1}))

		assert.Equal(t, 100, cc.IdleTime)
	})

	t.Run("non-keepalive clears away flag", func(t *testing.T) {
		peer := &ClientConn{ID: ClientID{0, 1}}
		mockMgr := &MockClientMgr{}
		mockMgr.On("List").Return([]*ClientConn{peer})

		cc := &ClientConn{
			ID:       ClientID{0, 1},
			Account:  &Account{},
			UserName: []byte("test"),
			Icon:     []byte{0, 0},
			IdleTime: 50,
			Logger:   NewTestLogger(),
			Server: &Server{
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
		assert.Greater(t, len(peer.sendCh), 0)
	})
}

func TestClientConn_SendAll(t *testing.T) {
	peers := []*ClientConn{
		{ID: ClientID{0, 1}},
		{ID: ClientID{0, 2}},
		{ID: ClientID{0, 3}},
	}
	mockMgr := &MockClientMgr{}
	mockMgr.On("List").Return(peers)

	cc := &ClientConn{
		ID: ClientID{0, 1},
		Server: &Server{
			ClientMgr: mockMgr,
		},
	}

	cc.SendAll(TranChatMsg, NewField(FieldData, []byte("hello")))

	for _, peer := range peers {
		assert.Len(t, peer.sendCh, 1)

		tran := <-peer.sendCh
		assert.Equal(t, peer.ID, tran.ClientID)
		assert.Equal(t, TranChatMsg, tran.Type)
	}

	mockMgr.AssertExpectations(t)
}

// closeRecorderRWC records whether Close was called.
type closeRecorderRWC struct {
	*bytes.Buffer
	closed bool
}

func (c *closeRecorderRWC) Close() error {
	c.closed = true
	return nil
}

// TestClientConn_writeLoop_ordering verifies that transactions are written to the connection in
// the order they were enqueued.
func TestClientConn_writeLoop_ordering(t *testing.T) {
	buf := &bytes.Buffer{}
	cc := &ClientConn{
		ID:         ClientID{0, 1},
		Connection: &nopCloserRWC{Buffer: buf},
		Logger:     NewTestLogger(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cc.writeLoop()
	}()

	const numTrans = 50
	for i := range numTrans {
		cc.Send(NewTransaction(TranChatMsg, cc.ID, NewField(FieldData, fmt.Appendf(nil, "msg-%03d", i))))
	}

	// Closing the queue stops writeLoop after it drains the remaining transactions.
	cc.closeSendQueue()
	<-done

	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	scanner.Split(transactionScanner)

	var count int
	for scanner.Scan() {
		var tran Transaction
		_, err := tran.Write(scanner.Bytes())
		require.NoError(t, err, "transaction %d is malformed", count)

		assert.Equal(t, fmt.Sprintf("msg-%03d", count), string(tran.GetField(FieldData).Data))
		count++
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, numTrans, count)
}

// TestClientConn_writeLoop_noInterleaving verifies that concurrent senders cannot interleave bytes
// within the connection's transaction framing.
func TestClientConn_writeLoop_noInterleaving(t *testing.T) {
	buf := &bytes.Buffer{}
	cc := &ClientConn{
		ID:         ClientID{0, 1},
		Connection: &nopCloserRWC{Buffer: buf},
		Logger:     NewTestLogger(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cc.writeLoop()
	}()

	// Total sends must stay within sendQueueDepth so the queue cannot overflow even if writeLoop
	// has not started draining yet.
	const senders, transPerSender = 4, 10

	var wg sync.WaitGroup
	for s := range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range transPerSender {
				cc.Send(NewTransaction(TranChatMsg, cc.ID, NewField(FieldData, fmt.Appendf(nil, "sender-%d-msg-%d", s, i))))
			}
		}()
	}
	wg.Wait()

	cc.closeSendQueue()
	<-done

	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	scanner.Split(transactionScanner)

	var count int
	for scanner.Scan() {
		var tran Transaction
		_, err := tran.Write(scanner.Bytes())
		require.NoError(t, err, "transaction %d is malformed", count)
		assert.Equal(t, TranChatMsg, tran.Type)
		count++
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, senders*transPerSender, count)
}

// TestClientConn_Send_slowClientDisconnected verifies that a client whose send queue overflows has
// its connection closed and that subsequent sends are dropped without panicking.
func TestClientConn_Send_slowClientDisconnected(t *testing.T) {
	conn := &closeRecorderRWC{Buffer: &bytes.Buffer{}}
	cc := &ClientConn{
		ID:         ClientID{0, 1},
		Connection: conn,
		Logger:     NewTestLogger(),
	}

	// No writeLoop is running, so the queue fills up after sendQueueDepth sends.
	for i := range sendQueueDepth + 1 {
		cc.Send(NewTransaction(TranChatMsg, cc.ID, NewField(FieldData, fmt.Appendf(nil, "msg-%d", i))))
	}

	assert.True(t, conn.closed, "connection should be closed when the send queue overflows")

	// Further sends must be silently dropped.
	cc.Send(NewTransaction(TranChatMsg, cc.ID))
}

// TestClientConn_SendDisconnectRace exercises concurrent Send and Disconnect calls.  Run with
// -race to detect data races and unsynchronized channel closes.
func TestClientConn_SendDisconnectRace(t *testing.T) {
	mockMgr := &MockClientMgr{}
	mockMgr.On("Delete", ClientID{0, 1}).Return()
	mockMgr.On("List").Return([]*ClientConn{})

	cc := &ClientConn{
		ID:         ClientID{0, 1},
		Connection: &nopCloserRWC{Buffer: &bytes.Buffer{}},
		Logger:     NewTestLogger(),
		Server: &Server{
			ClientMgr: mockMgr,
			Logger:    NewTestLogger(),
		},
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				cc.Send(NewTransaction(TranChatMsg, cc.ID, NewField(FieldData, fmt.Appendf(nil, "msg-%d", i))))
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		cc.Disconnect()
	}()

	wg.Wait()
}

func TestClientConn_incrementIdleTime(t *testing.T) {
	cc := &ClientConn{}

	// Increment until just below the idle threshold: not yet away.
	for i := 0; i < userIdleSeconds/idleCheckInterval; i++ {
		assert.False(t, cc.incrementIdleTime(idleCheckInterval))
	}
	assert.False(t, cc.IsFlagSet(UserFlagAway))

	// The increment that crosses the threshold marks the client away exactly once.
	assert.True(t, cc.incrementIdleTime(idleCheckInterval))
	assert.True(t, cc.IsFlagSet(UserFlagAway))
	assert.False(t, cc.incrementIdleTime(idleCheckInterval), "already-away client should not be marked away again")
}

func TestClientConn_clearIdleAndAway(t *testing.T) {
	cc := &ClientConn{IdleTime: 500}

	// Not away: idle timer resets, no notification needed.
	assert.False(t, cc.clearIdleAndAway())
	assert.Equal(t, 0, cc.IdleTime)

	// Away: flag clears and the caller is told to notify.
	cc.SetFlag(UserFlagAway, 1)
	assert.True(t, cc.clearIdleAndAway())
	assert.False(t, cc.IsFlagSet(UserFlagAway))
	assert.False(t, cc.clearIdleAndAway(), "second clear should report no change")
}

// TestClientConn_sessionStateRace exercises concurrent access to the mutable session state through
// the accessor methods.  Run with -race.
func TestClientConn_sessionStateRace(t *testing.T) {
	cc := &ClientConn{}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				cc.SetUserName(fmt.Appendf(nil, "user-%d", i))
				_ = cc.GetUserName()
				cc.SetIcon([]byte{0, byte(i)})
				_ = cc.GetIcon()
				cc.SetAutoReply([]byte("brb"))
				_ = cc.GetAutoReply()
				cc.SetFlag(UserFlagRefusePM, uint(i%2))
				_ = cc.IsFlagSet(UserFlagRefusePM)
				_ = cc.FlagBytes()
				_ = cc.incrementIdleTime(idleCheckInterval)
				_ = cc.clearIdleAndAway()
			}
		}()
	}
	wg.Wait()
}
