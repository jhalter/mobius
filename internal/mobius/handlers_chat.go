package mobius

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/jhalter/mobius/hotline"
)

// HandleChatSend sends a chat message to the chat.
//
// Access: Send Chat (10)
//
// Fields used in the request:
//   - 109 Chat options  Optional - Normal (0) or alternate (1) chat message
//   - 114 Chat ID       Optional
//   - 101 Data          Chat message string
//
// Reply is not expected.
func HandleChatSend(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessSendChat) {
		return cc.NewErrReply(t, ErrMsgNotAllowedParticipateChat)
	}

	// Truncate long usernames
	// %13.13s: This means a string that is right-aligned in a field of 13 characters.
	// If the string is longer than 13 characters, it will be truncated to 13 characters.
	formattedMsg := fmt.Sprintf("\r%13.13s:  %s", cc.UserName, t.GetField(hotline.FieldData).Data)

	// By holding the option key, Hotline chat allows users to send /me formatted messages like:
	// *** Halcyon does stuff
	// This is indicated by the presence of the optional field FieldChatOptions set to a value of 1.
	// Most clients do not send this option for normal chat messages.
	if t.GetField(hotline.FieldChatOptions).Data != nil && bytes.Equal(t.GetField(hotline.FieldChatOptions).Data, []byte{0, 1}) {
		formattedMsg = fmt.Sprintf("\r*** %s %s", cc.UserName, t.GetField(hotline.FieldData).Data)
	}

	// Truncate the message to the limit.  This does not handle the edge case of a string ending on multibyte character.
	formattedMsg = formattedMsg[:min(len(formattedMsg), hotline.LimitChatMsg)]

	// The ChatID field is used to identify messages as belonging to a private chat.
	// All clients *except* Frogblast omit this field for public chat, but Frogblast sends a value of 00 00 00 00.
	chatID := t.GetField(hotline.FieldChatID).Data
	if chatID != nil && !bytes.Equal([]byte{0, 0, 0, 0}, chatID) {
		privChatID, ok := hotline.ChatIDFromBytes(chatID)
		if !ok {
			return cc.NewErrReply(t, ErrMsgInvalidChatID)
		}

		// send the message to all connected clients of the private chat
		for _, c := range cc.Server.ChatMgr.Members(privChatID) {
			res = append(res, hotline.NewTransaction(
				hotline.TranChatMsg,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID),
				hotline.NewField(hotline.FieldData, []byte(formattedMsg)),
			))
		}
		return res
	}

	//cc.Server.mux.Lock()
	for _, c := range cc.Server.ClientMgr.List() {
		if c == nil || cc.Account == nil {
			continue
		}
		// Skip clients that do not have the read chat permission.
		if c.Authorize(hotline.AccessReadChat) {
			res = append(res, hotline.NewTransaction(hotline.TranChatMsg, c.ID, hotline.NewField(hotline.FieldData, []byte(formattedMsg))))
		}
	}
	//cc.Server.mux.Unlock()

	return res
}

// HandleSendInstantMsg sends an instant message to a user on the current server.
//
// Fields used in the request:
//   - 103 User ID          Target user
//   - 113 Options          myOpt_UserMessage (1), myOpt_RefuseMessage (2), myOpt_RefuseChat (3), myOpt_AutomaticResponse (4)
//   - 101 Data             Optional
//   - 214 Quoting message  Optional
//
// Fields used in the reply: None
func HandleSendInstantMsg(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessSendPrivMsg) {
		return cc.NewErrReply(t, ErrMsgNotAllowedSendPrivateMsg)
	}

	msg := t.GetField(hotline.FieldData)
	userID := t.GetField(hotline.FieldUserID)

	targetID, ok := hotline.ClientIDFromBytes(userID.Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidUserID)
	}

	reply := hotline.NewTransaction(
		hotline.TranServerMsg,
		targetID,
		hotline.NewField(hotline.FieldData, msg.Data),
		hotline.NewField(hotline.FieldUserName, cc.UserName),
		hotline.NewField(hotline.FieldUserID, cc.ID[:]),
		hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
	)

	// Later versions of Hotline include the original message in the FieldQuotingMsg field so
	//  the receiving client can display both the received message and what it is in reply to
	if t.GetField(hotline.FieldQuotingMsg).Data != nil {
		reply.Fields = append(reply.Fields, hotline.NewField(hotline.FieldQuotingMsg, t.GetField(hotline.FieldQuotingMsg).Data))
	}

	otherClient := cc.Server.ClientMgr.Get(targetID)
	if otherClient == nil {
		// Target user is no longer connected. The protocol defines no reply for
		// this transaction, so there is nothing to send back.
		return res
	}

	// Check if target user has "Refuse private messages" flag
	if otherClient.Flags.IsSet(hotline.UserFlagRefusePM) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranServerMsg,
				cc.ID,
				hotline.NewField(hotline.FieldData, []byte(fmt.Sprintf(ErrMsgDoesNotAcceptTemplate, string(otherClient.UserName), "private messages"))),
				hotline.NewField(hotline.FieldUserName, otherClient.UserName),
				hotline.NewField(hotline.FieldUserID, otherClient.ID[:]),
				hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res, reply)
	}

	// Respond with auto reply if other client has it enabled
	if len(otherClient.AutoReply) > 0 {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranServerMsg,
				cc.ID,
				hotline.NewField(hotline.FieldData, otherClient.AutoReply),
				hotline.NewField(hotline.FieldUserName, otherClient.UserName),
				hotline.NewField(hotline.FieldUserID, otherClient.ID[:]),
				hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
			),
		)
	}

	return append(res, cc.NewReply(t))
}

// HandleInviteNewChat invites users to a new chat.
//
// Fields used in the request:
//   - 103 User ID  Optional - User IDs to invite
//
// Fields used in the reply:
//   - 103 User ID       Inviting user's ID
//   - 104 User icon ID  Inviting user's icon
//   - 112 User flags    Inviting user's flags
//   - 102 User name     Inviting user's name
//   - 114 Chat ID       New chat room identifier
func HandleInviteNewChat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenChat) {
		return cc.NewErrReply(t, ErrMsgNotAllowedRequestPrivateChat)
	}

	// Client to Invite
	targetID, ok := hotline.ClientIDFromBytes(t.GetField(hotline.FieldUserID).Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidUserID)
	}

	// Check if target user has "Refuse private chat" flag
	targetClient := cc.Server.ClientMgr.Get(targetID)
	if targetClient == nil {
		return cc.NewErrReply(t, ErrMsgUserNotFound)
	}

	// Create a new chat with self as initial member.
	newChatID := cc.Server.ChatMgr.New(cc)

	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(targetClient.Flags[:])))
	if flagBitmap.Bit(hotline.UserFlagRefusePChat) == 1 {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranServerMsg,
				cc.ID,
				hotline.NewField(hotline.FieldData, []byte(fmt.Sprintf(ErrMsgDoesNotAcceptTemplate, string(targetClient.UserName), "private chats"))),
				hotline.NewField(hotline.FieldUserName, targetClient.UserName),
				hotline.NewField(hotline.FieldUserID, targetClient.ID[:]),
				hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranInviteToChat,
				targetID,
				hotline.NewField(hotline.FieldChatID, newChatID[:]),
				hotline.NewField(hotline.FieldUserName, cc.UserName),
				hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			),
		)
	}

	return append(
		res,
		cc.NewReply(t,
			hotline.NewField(hotline.FieldChatID, newChatID[:]),
			hotline.NewField(hotline.FieldUserName, cc.UserName),
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.Icon),
			hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
		),
	)
}

// HandleInviteToChat invites a user to an existing chat.
//
// Fields used in the request:
//   - 103 User ID  Required - User to invite
//   - 114 Chat ID  Required
//
// Reply is not expected.
func HandleInviteToChat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenChat) {
		return cc.NewErrReply(t, ErrMsgNotAllowedRequestPrivateChat)
	}

	// Client to Invite
	targetID, ok := hotline.ClientIDFromBytes(t.GetField(hotline.FieldUserID).Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidUserID)
	}
	chatID := t.GetField(hotline.FieldChatID).Data

	return []hotline.Transaction{
		hotline.NewTransaction(
			hotline.TranInviteToChat,
			targetID,
			hotline.NewField(hotline.FieldChatID, chatID),
			hotline.NewField(hotline.FieldUserName, cc.UserName),
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
		),
		cc.NewReply(
			t,
			hotline.NewField(hotline.FieldChatID, chatID),
			hotline.NewField(hotline.FieldUserName, cc.UserName),
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.Icon),
			hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
		),
	}
}

// HandleRejectChatInvite rejects an invitation to join a chat.
//
// Fields used in the request:
//   - 114 Chat ID  Required
//
// Reply is not expected.
func HandleRejectChatInvite(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	chatID, ok := hotline.ChatIDFromBytes(t.GetField(hotline.FieldChatID).Data)
	if !ok {
		cc.Logger.Error("reject chat invite: invalid chat ID")
		return res
	}

	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranChatMsg,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID[:]),
				hotline.NewField(hotline.FieldData, append(cc.UserName, []byte(" declined invitation to chat")...)),
			),
		)
	}

	return res
}

// HandleJoinChat joins a chat.
//
// Fields used in the request:
//   - 114 Chat ID  Required
//
// Fields used in the reply:
//   - 115 Chat subject         Current chat room subject
//   - 300 User name with info  Repeated - User information for each chat member
func HandleJoinChat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	chatID, ok := hotline.ChatIDFromBytes(t.GetField(hotline.FieldChatID).Data)
	if !ok {
		return cc.NewErrReply(t, ErrMsgInvalidChatID)
	}

	// Send TranNotifyChatChangeUser to current members of the chat to inform of new user
	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatChangeUser,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID[:]),
				hotline.NewField(hotline.FieldUserName, cc.UserName),
				hotline.NewField(hotline.FieldUserID, cc.ID[:]),
				hotline.NewField(hotline.FieldUserIconID, cc.Icon),
				hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
			),
		)
	}

	cc.Server.ChatMgr.Join(chatID, cc)

	subject := cc.Server.ChatMgr.GetSubject(chatID)

	replyFields := []hotline.Field{hotline.NewField(hotline.FieldChatSubject, []byte(subject))}
	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		b, err := io.ReadAll(&hotline.User{
			ID:    c.ID,
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			cc.Logger.Error("join chat: read member info", "err", err)
			return cc.NewErrReply(t, ErrMsgJoinChat)
		}
		replyFields = append(replyFields, hotline.NewField(hotline.FieldUsernameWithInfo, b))
	}

	return append(res, cc.NewReply(t, replyFields...))
}

// HandleLeaveChat leaves a chat.
//
// Fields used in the request:
//   - 114 Chat ID  Required
//
// Reply is not expected.
func HandleLeaveChat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	chatID, ok := hotline.ChatIDFromBytes(t.GetField(hotline.FieldChatID).Data)
	if !ok {
		cc.Logger.Error("leave chat: invalid chat ID")
		return res
	}

	cc.Server.ChatMgr.Leave(chatID, cc.ID)

	// Notify members of the private chat that the user has left
	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatDeleteUser,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID[:]),
				hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			),
		)
	}

	return res
}

// HandleSetChatSubject sets the chat subject for a chat.
//
// Fields used in the request:
//   - 114 Chat ID       Required
//   - 115 Chat subject  Required - Chat subject string
//
// Reply is not expected.
func HandleSetChatSubject(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	chatID, ok := hotline.ChatIDFromBytes(t.GetField(hotline.FieldChatID).Data)
	if !ok {
		cc.Logger.Error("set chat subject: invalid chat ID")
		return res
	}

	cc.Server.ChatMgr.SetSubject(chatID, string(t.GetField(hotline.FieldChatSubject).Data))

	// Notify chat members of new subject.
	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatSubject,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID[:]),
				hotline.NewField(hotline.FieldChatSubject, t.GetField(hotline.FieldChatSubject).Data),
			),
		)
	}

	return res
}
