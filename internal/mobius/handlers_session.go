package mobius

import (
	"encoding/binary"
	"io"
	"math/big"
	"time"

	"github.com/jhalter/mobius/hotline"
)

// HandleUserBroadcast broadcasts a message to all users on the server.
//
// Access: Broadcast (32)
//
// Fields used in the request:
//   - 101 Data  Required
//
// Fields used in the reply: None
func HandleUserBroadcast(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessBroadcast) {
		return cc.NewErrReply(t, ErrMsgNotAllowedSendBroadcast)
	}

	cc.SendAll(
		hotline.TranServerMsg,
		hotline.NewField(hotline.FieldData, t.GetField(hotline.FieldData).Data),
		hotline.NewField(hotline.FieldChatOptions, []byte{0}),
	)

	return append(res, cc.NewReply(t))
}

// HandleGetClientInfoText requests user information for a specific user.
//
// Access: Get Client Info (24)
//
// Fields used in the request:
//   - 103 User ID  Required
//
// Fields used in the reply:
//   - 102 User name  User's display name
//   - 101 Data       User info text string
func HandleGetClientInfoText(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessGetClientInfo) {
		return cc.NewErrReply(t, ErrMsgNotAllowedGetClientInfo)
	}

	clientID, ok := hotline.ClientIDFromBytes(t.GetField(hotline.FieldUserID).Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidUserID)
	}

	clientConn := cc.Server.ClientMgr.Get(clientID)
	if clientConn == nil {
		return cc.NewErrReply(t, ErrMsgUserNotFound)
	}

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldData, []byte(clientConn.String())),
		hotline.NewField(hotline.FieldUserName, clientConn.GetUserName()),
	))
}

// HandleGetUserNameList requests the list of all users connected to the current server.
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 300 User name with info  Repeated - User information for each connected client
func HandleGetUserNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	var fields []hotline.Field
	for _, c := range cc.Server.ClientMgr.List() {
		b, err := io.ReadAll(&hotline.User{
			ID:    c.ID,
			Icon:  c.GetIcon(),
			Flags: c.FlagBytes(),
			Name:  string(c.GetUserName()),
		})
		if err != nil {
			cc.Logger.Error("get user name list: read user info", "err", err)
			return cc.NewErrReply(t, ErrMsgGetUserList)
		}

		fields = append(fields, hotline.NewField(hotline.FieldUsernameWithInfo, b))
	}

	return []hotline.Transaction{cc.NewReply(t, fields...)}
}

// HandleTranAgreed notifies the server that the user accepted the server agreement.
//
// Fields used in the request:
//   - 102 User name           Display name
//   - 104 User icon ID        User icon identifier
//   - 113 Options             Bitmap: Automatic response (4), Refuse private chat (2), Refuse private message (1)
//   - 215 Automatic response  Optional - Auto-response string if options field indicates this feature
//
// Fields used in the reply: None
func HandleTranAgreed(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if t.GetField(hotline.FieldUserName).Data != nil {
		if cc.Authorize(hotline.AccessAnyName) {
			cc.SetUserName(t.GetField(hotline.FieldUserName).Data)
		} else {
			cc.SetUserName([]byte(cc.GetAccount().Name))
		}
	}

	login := cc.GetAccount().Login
	ip := cc.IP()

	if cc.Server.Presence != nil {
		cc.Server.Presence.UserRenamed(login, "", string(cc.GetUserName()), ip)
	}

	// Ban check for nickname
	if cc.Server.BanList != nil && cc.Server.BanList.IsNicknameBanned(string(cc.GetUserName())) {
		if cc.Server.Presence != nil {
			cc.Server.Presence.UserDisconnected(login, string(cc.GetUserName()), ip)
		}
		if err := cc.Server.BanList.Add(ip, nil); err != nil {
			cc.Logger.Error("Failed to ban IP for banned nickname", "ip", ip, "err", err)
		}
		cc.Disconnect()
		// Connection is being torn down; no reply is sent to the client.
		return res
	}

	cc.SetIcon(t.GetField(hotline.FieldUserIconID).Data)

	cc.Logger = cc.Logger.With("Name", string(cc.GetUserName()))
	cc.Logger.Info("Login successful")

	options := t.GetField(hotline.FieldOptions).Data
	optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

	// Check refuse private PM option
	cc.SetFlag(hotline.UserFlagRefusePM, optBitmap.Bit(hotline.UserOptRefusePM))

	// Check refuse private chat option
	cc.SetFlag(hotline.UserFlagRefusePChat, optBitmap.Bit(hotline.UserOptRefuseChat))

	// Check auto response
	if optBitmap.Bit(hotline.UserOptAutoResponse) == 1 {
		cc.SetAutoReply(t.GetField(hotline.FieldAutomaticResponse).Data)
	}

	trans := cc.NotifyOthers(
		hotline.NewTransaction(
			hotline.TranNotifyChangeUser, [2]byte{0, 0},
			hotline.NewField(hotline.FieldUserName, cc.GetUserName()),
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.GetIcon()),
			hotline.NewField(hotline.FieldUserFlags, cc.FlagBytes()),
		),
	)
	res = append(res, trans...)

	if cc.Server.Config.BannerFile != "" {
		bannerType := hotline.FileTypeFromFilename(cc.Server.Config.BannerFile).TypeCode
		res = append(res, hotline.NewTransaction(hotline.TranServerBanner, cc.ID, hotline.NewField(hotline.FieldBannerType, []byte(bannerType))))
	}

	res = append(res, cc.NewReply(t))

	return res
}

// HandleDisconnectUser disconnects a user from the current server.
//
// Access: Disconnect User (22)
//
// Fields used in the request:
//   - 103 User ID   Required
//   - 113 Options   Optional - Ban options
//   - 101 Data      Optional
//
// Fields used in the reply: None
func HandleDisconnectUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDisconUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDisconnectUsers)
	}

	clientID, ok := hotline.ClientIDFromBytes(t.GetField(hotline.FieldUserID).Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidUserID)
	}
	clientConn := cc.Server.ClientMgr.Get(clientID)
	if clientConn == nil {
		return cc.NewErrReply(t, ErrMsgUserNotFound)
	}

	if clientConn.Authorize(hotline.AccessCannotBeDiscon) {
		return cc.NewErrReply(t, clientConn.GetAccount().Login+" is not allowed to be disconnected.")
	}

	// If FieldOptions is set, then the client IP is banned in addition to disconnected.
	// 00 01 = temporary ban
	// 00 02 = permanent ban
	if options := t.GetField(hotline.FieldOptions).Data; len(options) > 1 {
		switch options[1] {
		case 1:
			// send message: "You are temporarily banned on this server"
			cc.Logger.Info("Disconnect & temporarily ban user", "username", string(clientConn.GetUserName()))

			res = append(res, hotline.NewTransaction(
				hotline.TranServerMsg,
				clientConn.ID,
				hotline.NewField(hotline.FieldData, []byte(ErrMsgTemporaryBan)),
				hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
			))

			banUntil := time.Now().Add(hotline.BanDuration)
			ip := clientConn.IP()

			err := cc.Server.BanList.Add(ip, &banUntil)
			if err != nil {
				cc.Logger.Error("Error saving ban", "err", err)
				// TODO
			}
		case 2:
			// send message: "You are permanently banned on this server"
			cc.Logger.Info("Disconnect & ban user", "username", string(clientConn.GetUserName()))

			res = append(res, hotline.NewTransaction(
				hotline.TranServerMsg,
				clientConn.ID,
				hotline.NewField(hotline.FieldData, []byte(ErrMsgPermanentBan)),
				hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
			))

			ip := clientConn.IP()

			err := cc.Server.BanList.Add(ip, nil)
			if err != nil {
				cc.Logger.Error("Error saving ban", "err", err)
			}
		}
	}

	go func() {
		time.Sleep(1 * time.Second)
		clientConn.Disconnect()
	}()

	return append(res, cc.NewReply(t))
}

// HandleSetClientUserInfo sets user preferences on the server.
//
// Fields used in the request:
//   - 102 User name           Optional
//   - 104 User icon ID        Optional
//   - 113 Options             Bitmap: Automatic response (4), Refuse private chat (2), Refuse private message (1)
//   - 215 Automatic response  Optional - Auto-response string if options field indicates this feature
//
// Reply is not expected.
func HandleSetClientUserInfo(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if len(t.GetField(hotline.FieldUserIconID).Data) == 4 {
		cc.SetIcon(t.GetField(hotline.FieldUserIconID).Data[2:])
	} else {
		cc.SetIcon(t.GetField(hotline.FieldUserIconID).Data)
	}
	if cc.Authorize(hotline.AccessAnyName) {
		oldNickname := string(cc.GetUserName())
		newNickname := string(t.GetField(hotline.FieldUserName).Data)
		cc.SetUserName(t.GetField(hotline.FieldUserName).Data)

		login := cc.GetAccount().Login
		ip := cc.IP()

		if cc.Server.Presence != nil {
			cc.Server.Presence.UserRenamed(login, oldNickname, newNickname, ip)
		}

		// Ban check for nickname
		if cc.Server.BanList != nil && cc.Server.BanList.IsNicknameBanned(newNickname) {
			if cc.Server.Presence != nil {
				cc.Server.Presence.UserDisconnected(login, newNickname, ip)
			}
			if err := cc.Server.BanList.Add(ip, nil); err != nil {
				cc.Logger.Error("Failed to ban IP for banned nickname", "ip", ip, "err", err)
			}
			cc.Disconnect()
			// Connection is being torn down; no reply is sent to the client.
			return res
		}
	}

	// the options field is only passed by the client versions > 1.2.3.
	options := t.GetField(hotline.FieldOptions).Data
	if options != nil {
		optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

		cc.SetFlag(hotline.UserFlagRefusePM, optBitmap.Bit(hotline.UserOptRefusePM))
		cc.SetFlag(hotline.UserFlagRefusePChat, optBitmap.Bit(hotline.UserOptRefuseChat))

		// Check auto response
		if optBitmap.Bit(hotline.UserOptAutoResponse) == 1 {
			cc.SetAutoReply(t.GetField(hotline.FieldAutomaticResponse).Data)
		} else {
			cc.SetAutoReply([]byte{})
		}
	}

	for _, c := range cc.Server.ClientMgr.List() {
		res = append(res, hotline.NewTransaction(
			hotline.TranNotifyChangeUser,
			c.ID,
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.GetIcon()),
			hotline.NewField(hotline.FieldUserFlags, cc.FlagBytes()),
			hotline.NewField(hotline.FieldUserName, cc.GetUserName()),
		))
	}

	return res
}

// HandleKeepAlive responds to client keepalive messages to maintain the connection.
//
// Fields used in the request: None
//
// Fields used in the reply: None
func HandleKeepAlive(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	res = append(res, cc.NewReply(t))

	return res
}
