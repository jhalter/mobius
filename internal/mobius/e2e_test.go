package mobius

// End-to-end protocol regression tests. Each top-level test starts a fresh in-process server (see
// integration_test.go for the harness) and drives it with the real hotline.Client over TCP.

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustReadAll(r io.Reader) []byte {
	b, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return b
}

// readUploadedFile polls for an uploaded file to appear under the server's Files tree (the upload
// completes asynchronously on the transfer connection) and returns its data-fork content.
func readUploadedFile(t *testing.T, s *e2eServer, name string) []byte {
	t.Helper()
	path := filepath.Join(s.cfgDir, "Files", name)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("uploaded file %s never appeared", path)
	return nil
}

func newReq(tranType [2]byte, fields ...hotline.Field) hotline.Transaction {
	return hotline.NewTransaction(tranType, [2]byte{0, 0}, fields...)
}

func TestE2E_Handshake(t *testing.T) {
	s := startE2EServer(t)

	t.Run("valid handshake succeeds", func(t *testing.T) {
		c := hotline.NewClient("probe", NewTestLogger())
		conn, err := net.DialTimeout("tcp", s.addr, 5*time.Second)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		c.Connection = conn
		require.NoError(t, c.Handshake())
	})

	t.Run("garbage handshake is rejected", func(t *testing.T) {
		conn, err := net.DialTimeout("tcp", s.addr, 5*time.Second)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		_, err = conn.Write([]byte("not a valid hotline handshake!!!"))
		require.NoError(t, err)

		// The server closes the connection on a bad handshake; the read returns EOF.
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
		buf := make([]byte, 8)
		_, err = conn.Read(buf)
		require.Error(t, err)
	})
}

func TestE2E_Login(t *testing.T) {
	s := startE2EServer(t)

	t.Run("guest login and agreement flow", func(t *testing.T) {
		c := connectE2E(t, s.addr, "guest", "")
		// A successful, fully-established session answers a request. The user name list is not
		// access-gated, so a non-error reply proves login (and the agreement auto-reply) completed.
		reply := c.roundTrip(newReq(hotline.TranGetUserNameList))
		assert.False(t, isError(reply), "an established session should answer TranGetUserNameList")
	})

	t.Run("wrong password is rejected", func(t *testing.T) {
		c := hotline.NewClient("baduser", NewTestLogger())
		require.NoError(t, c.Connect(s.addr, e2eAdminLogin, "wrong-password"))
		// The server replies to the login with an error, then closes the connection.
		require.NoError(t, c.Connection.SetReadDeadline(time.Now().Add(3*time.Second)))
		buf := make([]byte, 512)
		n, err := c.Connection.Read(buf)
		if err == nil {
			// If a reply was delivered it must be an error reply.
			var reply hotline.Transaction
			_, werr := reply.Write(buf[:n])
			if werr == nil {
				assert.True(t, isError(reply), "login with wrong password should return an error reply")
			}
		}
		_ = c.Disconnect()
	})
}

func TestE2E_Chat(t *testing.T) {
	s := startE2EServer(t)

	alice := connectE2E(t, s.addr, "guest", "")
	bob := connectE2E(t, s.addr, "guest", "")

	// A completed round trip proves a session is published to the client manager (publication
	// happens before the server answers requests), so after these both peers are guaranteed to be
	// in the chat broadcast set. Waiting for a login notification instead would be order-dependent:
	// alice only hears about bob if bob logs in after alice is established.
	require.False(t, isError(alice.roundTrip(newReq(hotline.TranGetUserNameList))))
	require.False(t, isError(bob.roundTrip(newReq(hotline.TranGetUserNameList))))

	require.NoError(t, alice.client.Send(newReq(hotline.TranChatSend,
		hotline.NewField(hotline.FieldData, []byte("hello from alice")),
	)))

	msg := bob.waitFor(hotline.TranChatMsg)
	got := msg.GetField(hotline.FieldData)
	require.NotNil(t, got)
	assert.Contains(t, string(got.Data), "hello from alice")
}

// findUserID picks the user with the given display name out of a TranGetUserNameList reply.
func findUserID(t *testing.T, reply hotline.Transaction, name string) [2]byte {
	t.Helper()
	for _, f := range reply.Fields {
		if f.Type != hotline.FieldUsernameWithInfo {
			continue
		}
		var u hotline.User
		_, err := u.Write(f.Data)
		require.NoError(t, err)
		if u.Name == name {
			return u.ID
		}
	}
	t.Fatalf("user %q not in user list", name)
	return [2]byte{}
}

func TestE2E_PrivateChat(t *testing.T) {
	s := startE2EServer(t)

	alice := connectE2ENamed(t, s.addr, "alice", "guest", "")
	bob := connectE2ENamed(t, s.addr, "bob", "guest", "")

	// A completed round trip proves a session is published to the client manager, so bob is
	// guaranteed to be in alice's user list below.
	require.False(t, isError(bob.roundTrip(newReq(hotline.TranGetUserNameList))))
	list := alice.roundTrip(newReq(hotline.TranGetUserNameList))
	require.False(t, isError(list))
	bobID := findUserID(t, list, "bob")

	// Alice opens a private chat with bob; the reply carries the new chat ID.
	invite := alice.roundTrip(newReq(hotline.TranInviteNewChat,
		hotline.NewField(hotline.FieldUserID, bobID[:]),
	))
	require.False(t, isError(invite))
	chatIDField := invite.GetField(hotline.FieldChatID)
	require.NotNil(t, chatIDField)
	chatID := chatIDField.Data

	// Bob receives the invitation, carrying the same chat ID.
	inv := bob.waitFor(hotline.TranInviteToChat)
	require.Equal(t, chatID, inv.GetField(hotline.FieldChatID).Data)

	// Bob joins; the join reply lists alice as an existing member.
	join := bob.roundTrip(newReq(hotline.TranJoinChat,
		hotline.NewField(hotline.FieldChatID, chatID),
	))
	require.False(t, isError(join))
	var members []string
	for _, f := range join.Fields {
		if f.Type == hotline.FieldUsernameWithInfo {
			var u hotline.User
			_, err := u.Write(f.Data)
			require.NoError(t, err)
			members = append(members, u.Name)
		}
	}
	assert.Contains(t, members, "alice", "join reply should list the chat's existing members")

	// Alice is notified that bob joined.
	joined := alice.waitFor(hotline.TranNotifyChatChangeUser)
	assert.Equal(t, chatID, joined.GetField(hotline.FieldChatID).Data)

	// Alice sets the subject; bob is notified.
	require.NoError(t, alice.client.Send(newReq(hotline.TranSetChatSubject,
		hotline.NewField(hotline.FieldChatID, chatID),
		hotline.NewField(hotline.FieldChatSubject, []byte("secret plans")),
	)))
	subj := bob.waitFor(hotline.TranNotifyChatSubject)
	assert.Equal(t, "secret plans", string(subj.GetField(hotline.FieldChatSubject).Data))

	// A message sent with the chat ID goes to the room's members (including the sender's echo),
	// tagged with the chat ID.
	require.NoError(t, alice.client.Send(newReq(hotline.TranChatSend,
		hotline.NewField(hotline.FieldChatID, chatID),
		hotline.NewField(hotline.FieldData, []byte("psst")),
	)))
	msg := bob.waitFor(hotline.TranChatMsg)
	assert.Equal(t, chatID, msg.GetField(hotline.FieldChatID).Data)
	assert.Contains(t, string(msg.GetField(hotline.FieldData).Data), "psst")
	echo := alice.waitFor(hotline.TranChatMsg)
	assert.Contains(t, string(echo.GetField(hotline.FieldData).Data), "psst")

	// Bob leaves; alice is notified.
	require.NoError(t, bob.client.Send(newReq(hotline.TranLeaveChat,
		hotline.NewField(hotline.FieldChatID, chatID),
	)))
	left := alice.waitFor(hotline.TranNotifyChatDeleteUser)
	assert.Equal(t, chatID, left.GetField(hotline.FieldChatID).Data)

	// A declined invitation is announced to the chat's members.
	invite2 := alice.roundTrip(newReq(hotline.TranInviteNewChat,
		hotline.NewField(hotline.FieldUserID, bobID[:]),
	))
	require.False(t, isError(invite2))
	inv2 := bob.waitFor(hotline.TranInviteToChat)
	require.NoError(t, bob.client.Send(newReq(hotline.TranRejectChatInvite,
		hotline.NewField(hotline.FieldChatID, inv2.GetField(hotline.FieldChatID).Data),
	)))
	decline := alice.waitFor(hotline.TranChatMsg)
	assert.Contains(t, string(decline.GetField(hotline.FieldData).Data), "bob declined invitation to chat")
}

func TestE2E_MessageBoard(t *testing.T) {
	s := startE2EServer(t)
	c := connectE2E(t, s.addr, "guest", "")

	reply := c.roundTrip(newReq(hotline.TranGetMsgs))
	require.False(t, isError(reply))
	data := reply.GetField(hotline.FieldData)
	require.NotNil(t, data)
	assert.Contains(t, string(data.Data), "Test News Post", "message board should return the fixture content")
}

func TestE2E_ThreadedNews(t *testing.T) {
	s := startE2EServer(t)
	c := connectE2E(t, s.addr, "guest", "")

	reply := c.roundTrip(newReq(hotline.TranGetNewsCatNameList,
		hotline.NewField(hotline.FieldNewsPath, []byte{}),
	))
	require.False(t, isError(reply))
	// The fixture ThreadedNews.yaml defines a "TestBundle" category at the root.
	var found bool
	for _, f := range reply.Fields {
		if bytes.Contains(f.Data, []byte("TestBundle")) {
			found = true
		}
	}
	assert.True(t, found, "root news category list should include the fixture bundle")
}

func TestE2E_FileList(t *testing.T) {
	s := startE2EServer(t)
	c := connectE2E(t, s.addr, "guest", "")

	reply := c.roundTrip(newReq(hotline.TranGetFileNameList,
		hotline.NewField(hotline.FieldFilePath, []byte{}),
	))
	require.False(t, isError(reply))

	var found bool
	for _, f := range reply.Fields {
		if f.Type == hotline.FieldFileNameWithInfo && bytes.Contains(f.Data, []byte("testfile.txt")) {
			found = true
		}
	}
	assert.True(t, found, "file list should include the fixture testfile.txt")
}

func TestE2E_FileDownload(t *testing.T) {
	s := startE2EServer(t)
	c := connectE2E(t, s.addr, "guest", "")

	reply := c.roundTrip(newReq(hotline.TranDownloadFile,
		hotline.NewField(hotline.FieldFileName, []byte("testfile.txt")),
		hotline.NewField(hotline.FieldFilePath, []byte{}),
	))
	require.False(t, isError(reply))

	refField := reply.GetField(hotline.FieldRefNum)
	require.NotNil(t, refField, "download reply must carry a reference number")
	var refNum [4]byte
	copy(refNum[:], refField.Data)

	data := downloadOverTransferPort(t, s.xferAddr, refNum)
	assert.Contains(t, string(data), "Hello, I'm a test file!", "downloaded payload should contain the fixture data fork")
}

func TestE2E_FileUpload(t *testing.T) {
	s := startE2EServer(t)
	// Upload to the file root requires AccessUploadAnywhere, which the admin account has and guest
	// does not.
	c := connectE2E(t, s.addr, e2eAdminLogin, e2eAdminPass)

	content := []byte("uploaded by the e2e suite")
	payload := buildUploadFFO("uploaded.txt", content)

	reply := c.roundTrip(newReq(hotline.TranUploadFile,
		hotline.NewField(hotline.FieldFileName, []byte("uploaded.txt")),
		hotline.NewField(hotline.FieldFilePath, []byte{}),
		hotline.NewField(hotline.FieldTransferSize, u32(uint32(len(payload)))),
	))
	require.False(t, isError(reply))

	refField := reply.GetField(hotline.FieldRefNum)
	require.NotNil(t, refField)
	var refNum [4]byte
	copy(refNum[:], refField.Data)

	uploadOverTransferPort(t, s.xferAddr, refNum, payload)

	// The uploaded file should land under the server's Files tree with the expected content.
	uploaded := readUploadedFile(t, s, "uploaded.txt")
	assert.Equal(t, content, uploaded)
}

func TestE2E_AccountAdmin(t *testing.T) {
	s := startE2EServer(t)

	t.Run("admin can create, read, and delete a user", func(t *testing.T) {
		admin := connectE2E(t, s.addr, e2eAdminLogin, e2eAdminPass)

		create := admin.roundTrip(newReq(hotline.TranNewUser,
			hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("newbie"))),
			hotline.NewField(hotline.FieldUserName, []byte("New Bie")),
			hotline.NewField(hotline.FieldUserPassword, hotline.EncodeString([]byte("pw"))),
			hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
		))
		require.False(t, isError(create), "admin should be allowed to create a user")

		get := admin.roundTrip(newReq(hotline.TranGetUser,
			hotline.NewField(hotline.FieldUserLogin, []byte("newbie")),
		))
		require.False(t, isError(get))
		name := get.GetField(hotline.FieldUserName)
		require.NotNil(t, name)
		assert.Equal(t, "New Bie", string(name.Data))

		del := admin.roundTrip(newReq(hotline.TranDeleteUser,
			hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("newbie"))),
		))
		assert.False(t, isError(del))
	})

	t.Run("guest cannot create a user", func(t *testing.T) {
		guest := connectE2E(t, s.addr, "guest", "")
		reply := guest.roundTrip(newReq(hotline.TranNewUser,
			hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("sneaky"))),
			hotline.NewField(hotline.FieldUserName, []byte("Sneaky")),
			hotline.NewField(hotline.FieldUserPassword, hotline.EncodeString([]byte("pw"))),
			hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
		))
		assert.True(t, isError(reply), "guest lacks CreateUser access and must be rejected")
	})
}

func TestE2E_DisconnectNotifiesPeers(t *testing.T) {
	s := startE2EServer(t)

	alice := connectE2E(t, s.addr, "guest", "")
	bob := connectE2E(t, s.addr, "guest", "")

	// Ensure both sessions are fully established (and thus both in the client manager) before alice
	// leaves, so bob is guaranteed to be a peer that receives the delete notification.
	require.False(t, isError(alice.roundTrip(newReq(hotline.TranGetUserNameList))))
	require.False(t, isError(bob.roundTrip(newReq(hotline.TranGetUserNameList))))

	alice.cancel()
	_ = alice.client.Disconnect()

	del := bob.waitFor(hotline.TranNotifyDeleteUser)
	assert.Equal(t, hotline.TranNotifyDeleteUser, hotline.TranType(del.Type))
}

func TestE2E_ShutdownBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Shutdown pays a 3s broadcast-flush sleep")
	}
	s := startE2EServer(t)
	c := connectE2E(t, s.addr, "guest", "")
	// Ensure login has completed before triggering shutdown.
	require.False(t, isError(c.roundTrip(newReq(hotline.TranGetUserNameList))))

	go s.srv.Shutdown([]byte("server going down"))

	msg := c.waitFor(hotline.TranDisconnectMsg)
	data := msg.GetField(hotline.FieldData)
	require.NotNil(t, data)
	assert.Contains(t, string(data.Data), "server going down")
}

// --- upload payload construction -------------------------------------------------------------

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// buildUploadFFO assembles the flattened file object bytes a client streams during an upload:
// FlatFileHeader + INFO fork header + info fork + DATA fork header + data fork. It mirrors the
// layout hotline.flattenedFileObject.ReadFrom expects (that type is unexported, so we build the
// bytes by hand from the exported information fork).
func buildUploadFFO(name string, content []byte) []byte {
	infoFork := hotline.NewFlatFileInformationFork(name, [8]byte{}, "TEXT", "TTXT")
	infoBody := mustReadAll(&infoFork)

	var buf bytes.Buffer
	// FlatFileHeader: "FILP" + version 1 + 16 reserved + fork count 2.
	buf.WriteString("FILP")
	buf.Write([]byte{0, 1})
	buf.Write(make([]byte, 16))
	buf.Write([]byte{0, 2})

	// INFO fork header + body.
	buf.WriteString("INFO")
	buf.Write(make([]byte, 8)) // compression + reserved
	buf.Write(u32(uint32(len(infoBody))))
	buf.Write(infoBody)

	// DATA fork header + body.
	buf.WriteString("DATA")
	buf.Write(make([]byte, 8))
	buf.Write(u32(uint32(len(content))))
	buf.Write(content)

	return buf.Bytes()
}
