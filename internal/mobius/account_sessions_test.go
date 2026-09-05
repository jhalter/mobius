package mobius

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func accountUpdateField(t *testing.T, oldLogin, newLogin string, access hotline.AccessBitmap) hotline.Field {
	t.Helper()
	fields := []hotline.Field{
		hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte(newLogin))),
		hotline.NewField(hotline.FieldUserName, []byte("Updated account")),
		hotline.NewField(hotline.FieldUserPassword, []byte{0}),
		hotline.NewField(hotline.FieldUserAccess, access[:]),
	}
	if oldLogin != newLogin {
		fields = append(fields, hotline.NewField(hotline.FieldData, hotline.EncodeString([]byte(oldLogin))))
	}
	block := make([]byte, 2)
	binary.BigEndian.PutUint16(block, uint16(len(fields)))
	for _, f := range fields {
		encoded, err := io.ReadAll(&f)
		require.NoError(t, err)
		block = append(block, encoded...)
	}
	return hotline.NewField(hotline.FieldData, block)
}

func TestE2E_AccountPermissionUpdates(t *testing.T) {
	s := startE2EServer(t)
	var enabled hotline.AccessBitmap
	enabled.Set(hotline.AccessDownloadFile)
	enabled.Set(hotline.AccessDisconUser)
	enabled.Set(hotline.AccessAnyName)
	enabled.Set(hotline.AccessNoAgreement)
	require.NoError(t, s.srv.AccountManager.Create(*hotline.NewAccount("editable", "Editable", "", enabled)))
	admin := connectE2E(t, s.addr, e2eAdminLogin, e2eAdminPass)
	clients := []*e2eClient{
		connectE2ENamed(t, s.addr, "first", "editable", ""),
		connectE2ENamed(t, s.addr, "second", "editable", ""),
	}
	for _, c := range clients {
		require.False(t, isError(c.roundTrip(newReq(hotline.TranGetUserNameList))))
		c.waitFor(hotline.TranUserAccess) // Consume the initial login access notification.
	}

	for _, step := range []struct {
		name, oldLogin, newLogin string
		access                   hotline.AccessBitmap
		failLater                bool
	}{
		{"rename and revoke before a later batch error", "editable", "renamed", hotline.AccessBitmap{}, true},
		{"grant permissions to the renamed account", "renamed", "renamed", enabled, false},
	} {
		t.Log(step.name)
		fields := []hotline.Field{accountUpdateField(t, step.oldLogin, step.newLogin, step.access)}
		if step.failLater {
			fields = append(fields, hotline.NewField(hotline.FieldData, []byte{0}))
		}
		reply := admin.roundTrip(newReq(hotline.TranUpdateUser, fields...))
		require.Equal(t, step.failLater, isError(reply))
		require.Equal(t, step.access, s.srv.AccountManager.Get(step.newLogin).Access)
		for _, c := range clients {
			reply := c.roundTrip(newReq(hotline.TranDownloadFile, hotline.NewField(hotline.FieldFileName, []byte("testfile.txt"))))
			require.Equal(t, !step.access.IsSet(hotline.AccessDownloadFile), isError(reply), "permissions must apply without reconnecting")
			update := c.waitFor(hotline.TranUserAccess)
			assert.Equal(t, step.access[:], update.GetField(hotline.FieldUserAccess).Data)
		}
		list := admin.roundTrip(newReq(hotline.TranGetUserNameList))
		for _, nickname := range []string{"first", "second"} {
			id := findUserID(t, list, nickname)
			client := s.srv.ClientMgr.Get(id)
			assert.Equal(t, step.newLogin, client.GetAccount().Login)
			assert.Equal(t, step.access.IsSet(hotline.AccessDisconUser), client.IsFlagSet(hotline.UserFlagAdmin))
		}
		// Updating the target account must not revoke another account's permissions.
		reply = admin.roundTrip(newReq(hotline.TranDownloadFile, hotline.NewField(hotline.FieldFileName, []byte("testfile.txt"))))
		assert.False(t, isError(reply))
	}
}

type failingAccountUpdateManager struct {
	hotline.AccountManager
}

func (failingAccountUpdateManager) Update(hotline.Account, string) error {
	return errors.New("cannot persist account")
}

func TestHandleUpdateUserPreservesSessionsOnSaveFailure(t *testing.T) {
	am := &YAMLAccountManager{accountDir: t.TempDir(), accounts: map[string]hotline.Account{}}
	account := hotline.Account{Login: "editable"}
	account.Access.Set(hotline.AccessDownloadFile)
	require.NoError(t, am.Create(account))
	srv, err := hotline.NewServer(hotline.WithLogger(NewTestLogger()))
	require.NoError(t, err)
	srv.AccountManager = failingAccountUpdateManager{am}
	admin := srv.NewClientConn(nopReadWriteCloser{}, "127.0.0.1:1")
	admin.Logger = srv.Logger
	admin.Account = &hotline.Account{Login: "admin"}
	admin.Account.Access.Set(hotline.AccessModifyUser)
	target := srv.NewClientConn(nopReadWriteCloser{}, "127.0.0.1:2")
	target.SetAccount(am.Get("editable"))
	srv.ClientMgr.Add(target)
	req := newReq(hotline.TranUpdateUser, accountUpdateField(t, "editable", "renamed", hotline.AccessBitmap{}))
	replies := HandleUpdateUser(admin, &req)
	require.Len(t, replies, 1)
	assert.True(t, isError(replies[0]))
	assert.True(t, target.Authorize(hotline.AccessDownloadFile))
	assert.Equal(t, "editable", target.GetAccount().Login)
	assert.Equal(t, account.Access, am.Get("editable").Access)
}
