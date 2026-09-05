package mobius

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDisconnectUserReportsBanSaveFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		option byte
	}{
		{name: "temporary", option: 1},
		{name: "permanent", option: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The missing parent directory makes saving fail on every storage permission setup.
			bans, err := NewBanFile(filepath.Join(t.TempDir(), "missing", "bans.yaml"))
			require.NoError(t, err)
			srv := &hotline.Server{
				ClientMgr: hotline.NewMemClientMgr(),
				BanList:   bans,
				Logger:    NewTestLogger(),
			}
			conn := &banTestConn{closed: make(chan struct{}, 1)}
			target := &hotline.ClientConn{
				Server:     srv,
				Connection: conn,
				RemoteAddr: "10.0.0.1:12345",
				Account:    &hotline.Account{Login: "target"},
			}
			srv.ClientMgr.Add(target)
			admin := &hotline.ClientConn{
				ID:      hotline.ClientID{0, 9},
				Server:  srv,
				Logger:  srv.Logger,
				Account: &hotline.Account{Login: "admin"},
			}
			admin.Account.Access.Set(hotline.AccessDisconUser)
			req := hotline.NewTransaction(hotline.TranDisconnectUser, admin.ID,
				hotline.NewField(hotline.FieldUserID, target.ID[:]),
				hotline.NewField(hotline.FieldOptions, []byte{0, tc.option}),
			)

			replies := HandleDisconnectUser(admin, &req)

			require.Len(t, replies, 1, "return only the administrator's error, without a ban notification")
			assert.Equal(t, admin.ID, replies[0].ClientID)
			assert.Equal(t, req.ID, replies[0].ID)
			assert.Equal(t, byte(1), replies[0].IsReply)
			assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
			assert.Equal(t, "Error saving ban. User was not disconnected.", string(replies[0].GetField(hotline.FieldError).Data))
			banned, _ := bans.IsBanned(target.IP())
			assert.False(t, banned)

			select {
			case <-conn.closed:
				t.Fatal("a failed ban must not schedule the user's disconnection")
			case <-time.After(1100 * time.Millisecond):
			}
			assert.Same(t, target, srv.ClientMgr.Get(target.ID))
		})
	}
}

type banTestConn struct {
	nopReadWriteCloser
	closed chan struct{}
}

func (c *banTestConn) Close() error {
	select {
	case c.closed <- struct{}{}:
	default:
	}
	return nil
}
