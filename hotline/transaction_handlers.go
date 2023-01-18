package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TransactionType struct {
	Handler        func(*ClientConn, *Transaction) ([]Transaction, error) // function for handling the transaction type
	Name           string                                                 // Name of transaction as it will appear in logging
	RequiredFields []requiredField
}

var TransactionHandlers = map[uint16]TransactionType{
	// Server initiated
	tranChatMsg: {
		Name: "tranChatMsg",
	},
	// Server initiated
	tranNotifyChangeUser: {
		Name: "tranNotifyChangeUser",
	},
	tranError: {
		Name: "tranError",
	},
	tranShowAgreement: {
		Name: "tranShowAgreement",
	},
	tranUserAccess: {
		Name: "tranUserAccess",
	},
	tranNotifyDeleteUser: {
		Name: "tranNotifyDeleteUser",
	},
	tranAgreed: {
		Name:    "tranAgreed",
		Handler: HandleTranAgreed,
	},
	tranChatSend: {
		Name:    "tranChatSend",
		Handler: HandleChatSend,
		RequiredFields: []requiredField{
			{
				ID:     fieldData,
				minLen: 0,
			},
		},
	},
	tranDelNewsArt: {
		Name:    "tranDelNewsArt",
		Handler: HandleDelNewsArt,
	},
	tranDelNewsItem: {
		Name:    "tranDelNewsItem",
		Handler: HandleDelNewsItem,
	},
	tranDeleteFile: {
		Name:    "tranDeleteFile",
		Handler: HandleDeleteFile,
	},
	tranDeleteUser: {
		Name:    "tranDeleteUser",
		Handler: HandleDeleteUser,
	},
	tranDisconnectUser: {
		Name:    "tranDisconnectUser",
		Handler: HandleDisconnectUser,
	},
	tranDownloadFile: {
		Name:    "tranDownloadFile",
		Handler: HandleDownloadFile,
	},
	tranDownloadFldr: {
		Name:    "tranDownloadFldr",
		Handler: HandleDownloadFolder,
	},
	tranGetClientInfoText: {
		Name:    "tranGetClientInfoText",
		Handler: HandleGetClientInfoText,
	},
	tranGetFileInfo: {
		Name:    "tranGetFileInfo",
		Handler: HandleGetFileInfo,
	},
	tranGetFileNameList: {
		Name:    "tranGetFileNameList",
		Handler: HandleGetFileNameList,
	},
	tranGetMsgs: {
		Name:    "tranGetMsgs",
		Handler: HandleGetMsgs,
	},
	tranGetNewsArtData: {
		Name:    "tranGetNewsArtData",
		Handler: HandleGetNewsArtData,
	},
	tranGetNewsArtNameList: {
		Name:    "tranGetNewsArtNameList",
		Handler: HandleGetNewsArtNameList,
	},
	tranGetNewsCatNameList: {
		Name:    "tranGetNewsCatNameList",
		Handler: HandleGetNewsCatNameList,
	},
	tranGetUser: {
		Name:    "tranGetUser",
		Handler: HandleGetUser,
	},
	tranGetUserNameList: {
		Name:    "tranHandleGetUserNameList",
		Handler: HandleGetUserNameList,
	},
	tranInviteNewChat: {
		Name:    "tranInviteNewChat",
		Handler: HandleInviteNewChat,
	},
	tranInviteToChat: {
		Name:    "tranInviteToChat",
		Handler: HandleInviteToChat,
	},
	tranJoinChat: {
		Name:    "tranJoinChat",
		Handler: HandleJoinChat,
	},
	tranKeepAlive: {
		Name:    "tranKeepAlive",
		Handler: HandleKeepAlive,
	},
	tranLeaveChat: {
		Name:    "tranJoinChat",
		Handler: HandleLeaveChat,
	},
	tranListUsers: {
		Name:    "tranListUsers",
		Handler: HandleListUsers,
	},
	tranMoveFile: {
		Name:    "tranMoveFile",
		Handler: HandleMoveFile,
	},
	tranNewFolder: {
		Name:    "tranNewFolder",
		Handler: HandleNewFolder,
	},
	tranNewNewsCat: {
		Name:    "tranNewNewsCat",
		Handler: HandleNewNewsCat,
	},
	tranNewNewsFldr: {
		Name:    "tranNewNewsFldr",
		Handler: HandleNewNewsFldr,
	},
	tranNewUser: {
		Name:    "tranNewUser",
		Handler: HandleNewUser,
	},
	tranUpdateUser: {
		Name:    "tranUpdateUser",
		Handler: HandleUpdateUser,
	},
	tranOldPostNews: {
		Name:    "tranOldPostNews",
		Handler: HandleTranOldPostNews,
	},
	tranPostNewsArt: {
		Name:    "tranPostNewsArt",
		Handler: HandlePostNewsArt,
	},
	tranRejectChatInvite: {
		Name:    "tranRejectChatInvite",
		Handler: HandleRejectChatInvite,
	},
	tranSendInstantMsg: {
		Name:    "tranSendInstantMsg",
		Handler: HandleSendInstantMsg,
		RequiredFields: []requiredField{
			{
				ID:     fieldData,
				minLen: 0,
			},
			{
				ID: fieldUserID,
			},
		},
	},
	tranSetChatSubject: {
		Name:    "tranSetChatSubject",
		Handler: HandleSetChatSubject,
	},
	tranMakeFileAlias: {
		Name:    "tranMakeFileAlias",
		Handler: HandleMakeAlias,
		RequiredFields: []requiredField{
			{ID: fieldFileName, minLen: 1},
			{ID: fieldFilePath, minLen: 1},
			{ID: fieldFileNewPath, minLen: 1},
		},
	},
	tranSetClientUserInfo: {
		Name:    "tranSetClientUserInfo",
		Handler: HandleSetClientUserInfo,
	},
	tranSetFileInfo: {
		Name:    "tranSetFileInfo",
		Handler: HandleSetFileInfo,
	},
	tranSetUser: {
		Name:    "tranSetUser",
		Handler: HandleSetUser,
	},
	tranUploadFile: {
		Name:    "tranUploadFile",
		Handler: HandleUploadFile,
	},
	tranUploadFldr: {
		Name:    "tranUploadFldr",
		Handler: HandleUploadFolder,
	},
	tranUserBroadcast: {
		Name:    "tranUserBroadcast",
		Handler: HandleUserBroadcast,
	},
	tranDownloadBanner: {
		Name:    "tranDownloadBanner",
		Handler: HandleDownloadBanner,
	},
}

func HandleChatSend(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessSendChat) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to participate in chat."))
		return res, err
	}

	// Truncate long usernames
	trunc := fmt.Sprintf("%13s", cc.UserName)
	formattedMsg := fmt.Sprintf("\r%.14s:  %s", trunc, t.GetField(fieldData).Data)

	// By holding the option key, Hotline chat allows users to send /me formatted messages like:
	// *** Halcyon does stuff
	// This is indicated by the presence of the optional field fieldChatOptions set to a value of 1.
	// Most clients do not send this option for normal chat messages.
	if t.GetField(fieldChatOptions).Data != nil && bytes.Equal(t.GetField(fieldChatOptions).Data, []byte{0, 1}) {
		formattedMsg = fmt.Sprintf("\r*** %s %s", cc.UserName, t.GetField(fieldData).Data)
	}

	// The ChatID field is used to identify messages as belonging to a private chat.
	// All clients *except* Frogblast omit this field for public chat, but Frogblast sends a value of 00 00 00 00.
	chatID := t.GetField(fieldChatID).Data
	if chatID != nil && !bytes.Equal([]byte{0, 0, 0, 0}, chatID) {
		chatInt := binary.BigEndian.Uint32(chatID)
		privChat := cc.Server.PrivateChats[chatInt]

		clients := sortedClients(privChat.ClientConn)

		// send the message to all connected clients of the private chat
		for _, c := range clients {
			res = append(res, *NewTransaction(
				tranChatMsg,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldData, []byte(formattedMsg)),
			))
		}
		return res, err
	}

	for _, c := range sortedClients(cc.Server.Clients) {
		// Filter out clients that do not have the read chat permission
		if c.Authorize(accessReadChat) {
			res = append(res, *NewTransaction(tranChatMsg, c.ID, NewField(fieldData, []byte(formattedMsg))))
		}
	}

	return res, err
}

// HandleSendInstantMsg sends instant message to the user on the current server.
// Fields used in the request:
//
//	103	User ID
//	113	Options
//		One of the following values:
//		- User message (myOpt_UserMessage = 1)
//		- Refuse message (myOpt_RefuseMessage = 2)
//		- Refuse chat (myOpt_RefuseChat  = 3)
//		- Automatic response (myOpt_AutomaticResponse = 4)"
//	101	Data	Optional
//	214	Quoting message	Optional
//
// Fields used in the reply:
// None
func HandleSendInstantMsg(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessSendPrivMsg) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to send private messages."))
		return res, err
	}

	msg := t.GetField(fieldData)
	ID := t.GetField(fieldUserID)

	reply := NewTransaction(
		tranServerMsg,
		&ID.Data,
		NewField(fieldData, msg.Data),
		NewField(fieldUserName, cc.UserName),
		NewField(fieldUserID, *cc.ID),
		NewField(fieldOptions, []byte{0, 1}),
	)

	// Later versions of Hotline include the original message in the fieldQuotingMsg field so
	//  the receiving client can display both the received message and what it is in reply to
	if t.GetField(fieldQuotingMsg).Data != nil {
		reply.Fields = append(reply.Fields, NewField(fieldQuotingMsg, t.GetField(fieldQuotingMsg).Data))
	}

	id, _ := byteToInt(ID.Data)
	otherClient, ok := cc.Server.Clients[uint16(id)]
	if !ok {
		return res, errors.New("invalid client ID")
	}

	// Check if target user has "Refuse private messages" flag
	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(otherClient.Flags)))
	if flagBitmap.Bit(userFLagRefusePChat) == 1 {
		res = append(res,
			*NewTransaction(
				tranServerMsg,
				cc.ID,
				NewField(fieldData, []byte(string(otherClient.UserName)+" does not accept private messages.")),
				NewField(fieldUserName, otherClient.UserName),
				NewField(fieldUserID, *otherClient.ID),
				NewField(fieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res, *reply)
	}

	// Respond with auto reply if other client has it enabled
	if len(otherClient.AutoReply) > 0 {
		res = append(res,
			*NewTransaction(
				tranServerMsg,
				cc.ID,
				NewField(fieldData, otherClient.AutoReply),
				NewField(fieldUserName, otherClient.UserName),
				NewField(fieldUserID, *otherClient.ID),
				NewField(fieldOptions, []byte{0, 1}),
			),
		)
	}

	res = append(res, cc.NewReply(t))

	return res, err
}

func HandleGetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	fw, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldFileName, []byte(fw.name)),
		NewField(fieldFileTypeString, fw.ffo.FlatFileInformationFork.friendlyType()),
		NewField(fieldFileCreatorString, fw.ffo.FlatFileInformationFork.friendlyCreator()),
		NewField(fieldFileComment, fw.ffo.FlatFileInformationFork.Comment),
		NewField(fieldFileType, fw.ffo.FlatFileInformationFork.TypeSignature),
		NewField(fieldFileCreateDate, fw.ffo.FlatFileInformationFork.CreateDate),
		NewField(fieldFileModifyDate, fw.ffo.FlatFileInformationFork.ModifyDate),
		NewField(fieldFileSize, fw.totalSize()),
	))
	return res, err
}

// HandleSetFileInfo updates a file or folder name and/or comment from the Get Info window
// Fields used in the request:
// * 201	File name
// * 202	File path	Optional
// * 211	File new name	Optional
// * 210	File comment	Optional
// Fields used in the reply:	None
func HandleSetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	fi, err := cc.Server.FS.Stat(fullFilePath)
	if err != nil {
		return res, err
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res, err
	}
	if t.GetField(fieldFileComment).Data != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(accessSetFolderComment) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to set comments for folders."))
				return res, err
			}
		case mode.IsRegular():
			if !cc.Authorize(accessSetFileComment) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to set comments for files."))
				return res, err
			}
		}

		if err := hlFile.ffo.FlatFileInformationFork.setComment(t.GetField(fieldFileComment).Data); err != nil {
			return res, err
		}
		w, err := hlFile.infoForkWriter()
		if err != nil {
			return res, err
		}
		_, err = w.Write(hlFile.ffo.FlatFileInformationFork.MarshalBinary())
		if err != nil {
			return res, err
		}
	}

	fullNewFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, t.GetField(fieldFileNewName).Data)
	if err != nil {
		return nil, err
	}

	fileNewName := t.GetField(fieldFileNewName).Data

	if fileNewName != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(accessRenameFolder) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to rename folders."))
				return res, err
			}
			err = os.Rename(fullFilePath, fullNewFilePath)
			if os.IsNotExist(err) {
				res = append(res, cc.NewErrReply(t, "Cannot rename folder "+string(fileName)+" because it does not exist or cannot be found."))
				return res, err
			}
		case mode.IsRegular():
			if !cc.Authorize(accessRenameFile) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to rename files."))
				return res, err
			}
			fileDir, err := readPath(cc.Server.Config.FileRoot, filePath, []byte{})
			if err != nil {
				return nil, err
			}
			hlFile.name = string(fileNewName)
			err = hlFile.move(fileDir)
			if os.IsNotExist(err) {
				res = append(res, cc.NewErrReply(t, "Cannot rename file "+string(fileName)+" because it does not exist or cannot be found."))
				return res, err
			}
			if err != nil {
				return res, err
			}
		}
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleDeleteFile deletes a file or folder
// Fields used in the request:
// * 201	File name
// * 202	File path
// Fields used in the reply: none
func HandleDeleteFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res, err
	}

	fi, err := hlFile.dataFile()
	if err != nil {
		res = append(res, cc.NewErrReply(t, "Cannot delete file "+string(fileName)+" because it does not exist or cannot be found."))
		return res, nil
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(accessDeleteFolder) {
			res = append(res, cc.NewErrReply(t, "You are not allowed to delete folders."))
			return res, err
		}
	case mode.IsRegular():
		if !cc.Authorize(accessDeleteFile) {
			res = append(res, cc.NewErrReply(t, "You are not allowed to delete files."))
			return res, err
		}
	}

	if err := hlFile.delete(); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleMoveFile moves files or folders. Note: seemingly not documented
func HandleMoveFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := string(t.GetField(fieldFileName).Data)

	filePath, err := readPath(cc.Server.Config.FileRoot, t.GetField(fieldFilePath).Data, t.GetField(fieldFileName).Data)
	if err != nil {
		return res, err
	}

	fileNewPath, err := readPath(cc.Server.Config.FileRoot, t.GetField(fieldFileNewPath).Data, nil)
	if err != nil {
		return res, err
	}

	cc.logger.Infow("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	hlFile, err := newFileWrapper(cc.Server.FS, filePath, 0)
	if err != nil {
		return res, err
	}

	fi, err := hlFile.dataFile()
	if err != nil {
		res = append(res, cc.NewErrReply(t, "Cannot delete file "+fileName+" because it does not exist or cannot be found."))
		return res, err
	}
	if err != nil {
		return res, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(accessMoveFolder) {
			res = append(res, cc.NewErrReply(t, "You are not allowed to move folders."))
			return res, err
		}
	case mode.IsRegular():
		if !cc.Authorize(accessMoveFile) {
			res = append(res, cc.NewErrReply(t, "You are not allowed to move files."))
			return res, err
		}
	}
	if err := hlFile.move(fileNewPath); err != nil {
		return res, err
	}
	// TODO: handle other possible errors; e.g. fileWrapper delete fails due to fileWrapper permission issue

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleNewFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessCreateFolder) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to create folders."))
		return res, err
	}
	folderName := string(t.GetField(fieldFileName).Data)

	folderName = path.Join("/", folderName)

	var subPath string

	// fieldFilePath is only present for nested paths
	if t.GetField(fieldFilePath).Data != nil {
		var newFp FilePath
		_, err := newFp.Write(t.GetField(fieldFilePath).Data)
		if err != nil {
			return nil, err
		}

		for _, pathItem := range newFp.Items {
			subPath = filepath.Join("/", subPath, string(pathItem.Name))
		}
	}
	newFolderPath := path.Join(cc.Server.Config.FileRoot, subPath, folderName)

	// TODO: check path and folder name lengths

	if _, err := cc.Server.FS.Stat(newFolderPath); !os.IsNotExist(err) {
		msg := fmt.Sprintf("Cannot create folder \"%s\" because there is already a file or folder with that name.", folderName)
		return []Transaction{cc.NewErrReply(t, msg)}, nil
	}

	// TODO: check for disallowed characters to maintain compatibility for original client

	if err := cc.Server.FS.Mkdir(newFolderPath, 0777); err != nil {
		msg := fmt.Sprintf("Cannot create folder \"%s\" because an error occurred.", folderName)
		return []Transaction{cc.NewErrReply(t, msg)}, nil
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleSetUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessModifyUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to modify accounts."))
		return res, err
	}

	login := DecodeUserString(t.GetField(fieldUserLogin).Data)
	userName := string(t.GetField(fieldUserName).Data)

	newAccessLvl := t.GetField(fieldUserAccess).Data

	account := cc.Server.Accounts[login]
	account.Name = userName
	copy(account.Access[:], newAccessLvl)

	// If the password field is cleared in the Hotline edit user UI, the SetUser transaction does
	// not include fieldUserPassword
	if t.GetField(fieldUserPassword).Data == nil {
		account.Password = hashAndSalt([]byte(""))
	}
	if len(t.GetField(fieldUserPassword).Data) > 1 {
		account.Password = hashAndSalt(t.GetField(fieldUserPassword).Data)
	}

	out, err := yaml.Marshal(&account)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(cc.Server.ConfigDir, "Users", login+".yaml"), out, 0666); err != nil {
		return res, err
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.Clients {
		if c.Account.Login == login {
			// Note: comment out these two lines to test server-side deny messages
			newT := NewTransaction(tranUserAccess, c.ID, NewField(fieldUserAccess, newAccessLvl))
			res = append(res, *newT)

			flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(c.Flags)))
			if c.Authorize(accessDisconUser) {
				flagBitmap.SetBit(flagBitmap, userFlagAdmin, 1)
			} else {
				flagBitmap.SetBit(flagBitmap, userFlagAdmin, 0)
			}
			binary.BigEndian.PutUint16(c.Flags, uint16(flagBitmap.Int64()))

			c.Account.Access = account.Access

			cc.sendAll(
				tranNotifyChangeUser,
				NewField(fieldUserID, *c.ID),
				NewField(fieldUserFlags, c.Flags),
				NewField(fieldUserName, c.UserName),
				NewField(fieldUserIconID, c.Icon),
			)
		}
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleGetUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessOpenUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to view accounts."))
		return res, err
	}

	account := cc.Server.Accounts[string(t.GetField(fieldUserLogin).Data)]
	if account == nil {
		res = append(res, cc.NewErrReply(t, "Account does not exist."))
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldUserName, []byte(account.Name)),
		NewField(fieldUserLogin, negateString(t.GetField(fieldUserLogin).Data)),
		NewField(fieldUserPassword, []byte(account.Password)),
		NewField(fieldUserAccess, account.Access[:]),
	))
	return res, err
}

func HandleListUsers(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessOpenUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to view accounts."))
		return res, err
	}

	var userFields []Field
	for _, acc := range cc.Server.Accounts {
		b := make([]byte, 0, 100)
		n, err := acc.Read(b)
		if err != nil {
			return res, err
		}

		userFields = append(userFields, NewField(fieldData, b[:n]))
	}

	res = append(res, cc.NewReply(t, userFields...))
	return res, err
}

// HandleUpdateUser is used by the v1.5+ multi-user editor to perform account editing for multiple users at a time.
// An update can be a mix of these actions:
// * Create user
// * Delete user
// * Modify user (including renaming the account login)
//
// The Transaction sent by the client includes one data field per user that was modified.  This data field in turn
// contains another data field encoded in its payload with a varying number of sub fields depending on which action is
// performed.  This seems to be the only place in the Hotline protocol where a data field contains another data field.
func HandleUpdateUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	for _, field := range t.Fields {
		subFields, err := ReadFields(field.Data[0:2], field.Data[2:])
		if err != nil {
			return res, err
		}

		if len(subFields) == 1 {
			login := DecodeUserString(getField(fieldData, &subFields).Data)
			cc.logger.Infow("DeleteUser", "login", login)

			if !cc.Authorize(accessDeleteUser) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to delete accounts."))
				return res, err
			}

			if err := cc.Server.DeleteUser(login); err != nil {
				return res, err
			}
			continue
		}

		login := DecodeUserString(getField(fieldUserLogin, &subFields).Data)

		// check if the login dataFile; if so, we know we are updating an existing user
		if acc, ok := cc.Server.Accounts[login]; ok {
			cc.logger.Infow("UpdateUser", "login", login)

			// account dataFile, so this is an update action
			if !cc.Authorize(accessModifyUser) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to modify accounts."))
				return res, err
			}

			if getField(fieldUserPassword, &subFields) != nil {
				newPass := getField(fieldUserPassword, &subFields).Data
				acc.Password = hashAndSalt(newPass)
			} else {
				acc.Password = hashAndSalt([]byte(""))
			}

			if getField(fieldUserAccess, &subFields) != nil {
				copy(acc.Access[:], getField(fieldUserAccess, &subFields).Data)
			}

			err = cc.Server.UpdateUser(
				DecodeUserString(getField(fieldData, &subFields).Data),
				DecodeUserString(getField(fieldUserLogin, &subFields).Data),
				string(getField(fieldUserName, &subFields).Data),
				acc.Password,
				acc.Access,
			)
			if err != nil {
				return res, err
			}
		} else {
			cc.logger.Infow("CreateUser", "login", login)

			if !cc.Authorize(accessCreateUser) {
				res = append(res, cc.NewErrReply(t, "You are not allowed to create new accounts."))
				return res, err
			}

			newAccess := accessBitmap{}
			copy(newAccess[:], getField(fieldUserAccess, &subFields).Data[:])

			// Prevent account from creating new account with greater permission
			for i := 0; i < 64; i++ {
				if newAccess.IsSet(i) {
					if !cc.Authorize(i) {
						return append(res, cc.NewErrReply(t, "Cannot create account with more access than yourself.")), err
					}
				}
			}

			err := cc.Server.NewUser(login, string(getField(fieldUserName, &subFields).Data), string(getField(fieldUserPassword, &subFields).Data), newAccess)
			if err != nil {
				return []Transaction{}, err
			}
		}
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleNewUser creates a new user account
func HandleNewUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessCreateUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to create new accounts."))
		return res, err
	}

	login := DecodeUserString(t.GetField(fieldUserLogin).Data)

	// If the account already dataFile, reply with an error
	if _, ok := cc.Server.Accounts[login]; ok {
		res = append(res, cc.NewErrReply(t, "Cannot create account "+login+" because there is already an account with that login."))
		return res, err
	}

	newAccess := accessBitmap{}
	copy(newAccess[:], t.GetField(fieldUserAccess).Data[:])

	// Prevent account from creating new account with greater permission
	for i := 0; i < 64; i++ {
		if newAccess.IsSet(i) {
			if !cc.Authorize(i) {
				res = append(res, cc.NewErrReply(t, "Cannot create account with more access than yourself."))
				return res, err
			}
		}
	}

	if err := cc.Server.NewUser(login, string(t.GetField(fieldUserName).Data), string(t.GetField(fieldUserPassword).Data), newAccess); err != nil {
		return []Transaction{}, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleDeleteUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessDeleteUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to delete accounts."))
		return res, err
	}

	// TODO: Handle case where account doesn't exist; e.g. delete race condition
	login := DecodeUserString(t.GetField(fieldUserLogin).Data)

	if err := cc.Server.DeleteUser(login); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleUserBroadcast sends an Administrator Message to all connected clients of the server
func HandleUserBroadcast(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessBroadcast) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to send broadcast messages."))
		return res, err
	}

	cc.sendAll(
		tranServerMsg,
		NewField(fieldData, t.GetField(tranGetMsgs).Data),
		NewField(fieldChatOptions, []byte{0}),
	)

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleGetClientInfoText returns user information for the specific user.
//
// Fields used in the request:
// 103	User ID
//
// Fields used in the reply:
// 102	User name
// 101	Data		User info text string
func HandleGetClientInfoText(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessGetClientInfo) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to get client info."))
		return res, err
	}

	clientID, _ := byteToInt(t.GetField(fieldUserID).Data)

	clientConn := cc.Server.Clients[uint16(clientID)]
	if clientConn == nil {
		return append(res, cc.NewErrReply(t, "User not found.")), err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldData, []byte(clientConn.String())),
		NewField(fieldUserName, clientConn.UserName),
	))
	return res, err
}

func HandleGetUserNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	res = append(res, cc.NewReply(t, cc.Server.connectedUsers()...))

	return res, err
}

func HandleTranAgreed(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	cc.Agreed = true

	if t.GetField(fieldUserName).Data != nil {
		if cc.Authorize(accessAnyName) {
			cc.UserName = t.GetField(fieldUserName).Data
		} else {
			cc.UserName = []byte(cc.Account.Name)
		}
	}

	cc.Icon = t.GetField(fieldUserIconID).Data

	cc.logger = cc.logger.With("name", string(cc.UserName))
	cc.logger.Infow("Login successful", "clientVersion", fmt.Sprintf("%v", func() int { i, _ := byteToInt(cc.Version); return i }()))

	options := t.GetField(fieldOptions).Data
	optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(cc.Flags)))

	// Check refuse private PM option
	if optBitmap.Bit(refusePM) == 1 {
		flagBitmap.SetBit(flagBitmap, userFlagRefusePM, 1)
		binary.BigEndian.PutUint16(cc.Flags, uint16(flagBitmap.Int64()))
	}

	// Check refuse private chat option
	if optBitmap.Bit(refuseChat) == 1 {
		flagBitmap.SetBit(flagBitmap, userFLagRefusePChat, 1)
		binary.BigEndian.PutUint16(cc.Flags, uint16(flagBitmap.Int64()))
	}

	// Check auto response
	if optBitmap.Bit(autoResponse) == 1 {
		cc.AutoReply = t.GetField(fieldAutomaticResponse).Data
	} else {
		cc.AutoReply = []byte{}
	}

	trans := cc.notifyOthers(
		*NewTransaction(
			tranNotifyChangeUser, nil,
			NewField(fieldUserName, cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, cc.Icon),
			NewField(fieldUserFlags, cc.Flags),
		),
	)
	res = append(res, trans...)

	if cc.Server.Config.BannerFile != "" {
		res = append(res, *NewTransaction(tranServerBanner, cc.ID, NewField(fieldBannerType, []byte("JPEG"))))
	}

	res = append(res, cc.NewReply(t))

	return res, err
}

const defaultNewsDateFormat = "Jan02 15:04" // Jun23 20:49
//  "Mon, 02 Jan 2006 15:04:05 MST"

const defaultNewsTemplate = `From %s (%s):

%s

__________________________________________________________`

// HandleTranOldPostNews updates the flat news
// Fields used in this request:
// 101	Data
func HandleTranOldPostNews(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsPostArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to post news."))
		return res, err
	}

	cc.Server.flatNewsMux.Lock()
	defer cc.Server.flatNewsMux.Unlock()

	newsDateTemplate := defaultNewsDateFormat
	if cc.Server.Config.NewsDateFormat != "" {
		newsDateTemplate = cc.Server.Config.NewsDateFormat
	}

	newsTemplate := defaultNewsTemplate
	if cc.Server.Config.NewsDelimiter != "" {
		newsTemplate = cc.Server.Config.NewsDelimiter
	}

	newsPost := fmt.Sprintf(newsTemplate+"\r", cc.UserName, time.Now().Format(newsDateTemplate), t.GetField(fieldData).Data)
	newsPost = strings.Replace(newsPost, "\n", "\r", -1)

	// update news in memory
	cc.Server.FlatNews = append([]byte(newsPost), cc.Server.FlatNews...)

	// update news on disk
	if err := cc.Server.FS.WriteFile(filepath.Join(cc.Server.ConfigDir, "MessageBoard.txt"), cc.Server.FlatNews, 0644); err != nil {
		return res, err
	}

	// Notify all clients of updated news
	cc.sendAll(
		tranNewMsg,
		NewField(fieldData, []byte(newsPost)),
	)

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleDisconnectUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessDisconUser) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to disconnect users."))
		return res, err
	}

	clientConn := cc.Server.Clients[binary.BigEndian.Uint16(t.GetField(fieldUserID).Data)]

	if clientConn.Authorize(accessCannotBeDiscon) {
		res = append(res, cc.NewErrReply(t, clientConn.Account.Login+" is not allowed to be disconnected."))
		return res, err
	}

	// If fieldOptions is set, then the client IP is banned in addition to disconnected.
	// 00 01 = temporary ban
	// 00 02 = permanent ban
	if t.GetField(fieldOptions).Data != nil {
		switch t.GetField(fieldOptions).Data[1] {
		case 1:
			// send message: "You are temporarily banned on this server"
			cc.logger.Infow("Disconnect & temporarily ban " + string(clientConn.UserName))

			res = append(res, *NewTransaction(
				tranServerMsg,
				clientConn.ID,
				NewField(fieldData, []byte("You are temporarily banned on this server")),
				NewField(fieldChatOptions, []byte{0, 0}),
			))

			banUntil := time.Now().Add(tempBanDuration)
			cc.Server.banList[strings.Split(clientConn.RemoteAddr, ":")[0]] = &banUntil
			cc.Server.writeBanList()
		case 2:
			// send message: "You are permanently banned on this server"
			cc.logger.Infow("Disconnect & ban " + string(clientConn.UserName))

			res = append(res, *NewTransaction(
				tranServerMsg,
				clientConn.ID,
				NewField(fieldData, []byte("You are permanently banned on this server")),
				NewField(fieldChatOptions, []byte{0, 0}),
			))

			cc.Server.banList[strings.Split(clientConn.RemoteAddr, ":")[0]] = nil
			cc.Server.writeBanList()
		}
	}

	// TODO: remove this awful hack
	go func() {
		time.Sleep(1 * time.Second)
		clientConn.Disconnect()
	}()

	return append(res, cc.NewReply(t)), err
}

// HandleGetNewsCatNameList returns a list of news categories for a path
// Fields used in the request:
// 325	News path	(Optional)
func HandleGetNewsCatNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsReadArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to read news."))
		return res, err
	}

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)
	cats := cc.Server.GetNewsCatByPath(pathStrs)

	// To store the keys in slice in sorted order
	keys := make([]string, len(cats))
	i := 0
	for k := range cats {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	var fieldData []Field
	for _, k := range keys {
		cat := cats[k]
		b, _ := cat.MarshalBinary()
		fieldData = append(fieldData, NewField(
			fieldNewsCatListData15,
			b,
		))
	}

	res = append(res, cc.NewReply(t, fieldData...))
	return res, err
}

func HandleNewNewsCat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsCreateCat) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to create news categories."))
		return res, err
	}

	name := string(t.GetField(fieldNewsCatName).Data)
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cats := cc.Server.GetNewsCatByPath(pathStrs)
	cats[name] = NewsCategoryListData15{
		Name:     name,
		Type:     []byte{0, 3},
		Articles: map[uint32]*NewsArtData{},
		SubCats:  make(map[string]NewsCategoryListData15),
	}

	if err := cc.Server.writeThreadedNews(); err != nil {
		return res, err
	}
	res = append(res, cc.NewReply(t))
	return res, err
}

// Fields used in the request:
// 322	News category name
// 325	News path
func HandleNewNewsFldr(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsCreateFldr) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to create news folders."))
		return res, err
	}

	name := string(t.GetField(fieldFileName).Data)
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cc.logger.Infof("Creating new news folder %s", name)

	cats := cc.Server.GetNewsCatByPath(pathStrs)
	cats[name] = NewsCategoryListData15{
		Name:     name,
		Type:     []byte{0, 2},
		Articles: map[uint32]*NewsArtData{},
		SubCats:  make(map[string]NewsCategoryListData15),
	}
	if err := cc.Server.writeThreadedNews(); err != nil {
		return res, err
	}
	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleGetNewsArtData gets the list of article names at the specified news path.

// Fields used in the request:
// 325	News path	Optional

// Fields used in the reply:
// 321	News article list data	Optional
func HandleGetNewsArtNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsReadArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to read news."))
		return res, err
	}
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	var cat NewsCategoryListData15
	cats := cc.Server.ThreadedNews.Categories

	for _, fp := range pathStrs {
		cat = cats[fp]
		cats = cats[fp].SubCats
	}

	nald := cat.GetNewsArtListData()

	res = append(res, cc.NewReply(t, NewField(fieldNewsArtListData, nald.Payload())))
	return res, err
}

// HandleGetNewsArtData requests information about the specific news article.
// Fields used in the request:
//
// Request fields
// 325	News path
// 326	News article ID
// 327	News article data flavor
//
// Fields used in the reply:
// 328	News article title
// 329	News article poster
// 330	News article date
// 331	Previous article ID
// 332	Next article ID
// 335	Parent article ID
// 336	First child article ID
// 327	News article data flavor	"Should be “text/plain”
// 333	News article data	Optional (if data flavor is “text/plain”)
func HandleGetNewsArtData(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsReadArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to read news."))
		return res, err
	}

	var cat NewsCategoryListData15
	cats := cc.Server.ThreadedNews.Categories

	for _, fp := range ReadNewsPath(t.GetField(fieldNewsPath).Data) {
		cat = cats[fp]
		cats = cats[fp].SubCats
	}

	// The official Hotline clients will send the article ID as 2 bytes if possible, but
	// some third party clients such as Frogblast and Heildrun will always send 4 bytes
	convertedID, err := byteToInt(t.GetField(fieldNewsArtID).Data)
	if err != nil {
		return res, err
	}

	art := cat.Articles[uint32(convertedID)]
	if art == nil {
		res = append(res, cc.NewReply(t))
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldNewsArtTitle, []byte(art.Title)),
		NewField(fieldNewsArtPoster, []byte(art.Poster)),
		NewField(fieldNewsArtDate, art.Date),
		NewField(fieldNewsArtPrevArt, art.PrevArt),
		NewField(fieldNewsArtNextArt, art.NextArt),
		NewField(fieldNewsArtParentArt, art.ParentArt),
		NewField(fieldNewsArt1stChildArt, art.FirstChildArt),
		NewField(fieldNewsArtDataFlav, []byte("text/plain")),
		NewField(fieldNewsArtData, []byte(art.Data)),
	))
	return res, err
}

// HandleDelNewsItem deletes an existing threaded news folder or category from the server.
// Fields used in the request:
// 325	News path
// Fields used in the reply:
// None
func HandleDelNewsItem(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cats := cc.Server.ThreadedNews.Categories
	delName := pathStrs[len(pathStrs)-1]
	if len(pathStrs) > 1 {
		for _, fp := range pathStrs[0 : len(pathStrs)-1] {
			cats = cats[fp].SubCats
		}
	}

	if bytes.Equal(cats[delName].Type, []byte{0, 3}) {
		if !cc.Authorize(accessNewsDeleteCat) {
			return append(res, cc.NewErrReply(t, "You are not allowed to delete news categories.")), nil
		}
	} else {
		if !cc.Authorize(accessNewsDeleteFldr) {
			return append(res, cc.NewErrReply(t, "You are not allowed to delete news folders.")), nil
		}
	}

	delete(cats, delName)

	if err := cc.Server.writeThreadedNews(); err != nil {
		return res, err
	}

	return append(res, cc.NewReply(t)), nil
}

func HandleDelNewsArt(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsDeleteArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to delete news articles."))
		return res, err
	}

	// Request Fields
	// 325	News path
	// 326	News article ID
	// 337	News article – recursive delete	Delete child articles (1) or not (0)
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)
	ID := binary.BigEndian.Uint16(t.GetField(fieldNewsArtID).Data)

	// TODO: Delete recursive
	cats := cc.Server.GetNewsCatByPath(pathStrs[:len(pathStrs)-1])

	catName := pathStrs[len(pathStrs)-1]
	cat := cats[catName]

	delete(cat.Articles, uint32(ID))

	cats[catName] = cat
	if err := cc.Server.writeThreadedNews(); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// Request fields
// 325	News path
// 326	News article ID	 						ID of the parent article?
// 328	News article title
// 334	News article flags
// 327	News article data flavor		Currently “text/plain”
// 333	News article data
func HandlePostNewsArt(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsPostArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to post news articles."))
		return res, err
	}

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)
	cats := cc.Server.GetNewsCatByPath(pathStrs[:len(pathStrs)-1])

	catName := pathStrs[len(pathStrs)-1]
	cat := cats[catName]

	newArt := NewsArtData{
		Title:         string(t.GetField(fieldNewsArtTitle).Data),
		Poster:        string(cc.UserName),
		Date:          toHotlineTime(time.Now()),
		PrevArt:       []byte{0, 0, 0, 0},
		NextArt:       []byte{0, 0, 0, 0},
		ParentArt:     append([]byte{0, 0}, t.GetField(fieldNewsArtID).Data...),
		FirstChildArt: []byte{0, 0, 0, 0},
		DataFlav:      []byte("text/plain"),
		Data:          string(t.GetField(fieldNewsArtData).Data),
	}

	var keys []int
	for k := range cat.Articles {
		keys = append(keys, int(k))
	}

	nextID := uint32(1)
	if len(keys) > 0 {
		sort.Ints(keys)
		prevID := uint32(keys[len(keys)-1])
		nextID = prevID + 1

		binary.BigEndian.PutUint32(newArt.PrevArt, prevID)

		// Set next article ID
		binary.BigEndian.PutUint32(cat.Articles[prevID].NextArt, nextID)
	}

	// Update parent article with first child reply
	parentID := binary.BigEndian.Uint16(t.GetField(fieldNewsArtID).Data)
	if parentID != 0 {
		parentArt := cat.Articles[uint32(parentID)]

		if bytes.Equal(parentArt.FirstChildArt, []byte{0, 0, 0, 0}) {
			binary.BigEndian.PutUint32(parentArt.FirstChildArt, nextID)
		}
	}

	cat.Articles[nextID] = &newArt

	cats[catName] = cat
	if err := cc.Server.writeThreadedNews(); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleGetMsgs returns the flat news data
func HandleGetMsgs(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessNewsReadArt) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to read news."))
		return res, err
	}

	res = append(res, cc.NewReply(t, NewField(fieldData, cc.Server.FlatNews)))

	return res, err
}

func HandleDownloadFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessDownloadFile) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to download files."))
		return res, err
	}

	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data
	resumeData := t.GetField(fieldFileResumeData).Data

	var dataOffset int64
	var frd FileResumeData
	if resumeData != nil {
		if err := frd.UnmarshalBinary(t.GetField(fieldFileResumeData).Data); err != nil {
			return res, err
		}
		// TODO: handle rsrc fork offset
		dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
	}

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, dataOffset)
	if err != nil {
		return res, err
	}

	xferSize := hlFile.ffo.TransferSize(0)

	ft := cc.newFileTransfer(FileDownload, fileName, filePath, xferSize)

	// TODO: refactor to remove this
	if resumeData != nil {
		var frd FileResumeData
		if err := frd.UnmarshalBinary(t.GetField(fieldFileResumeData).Data); err != nil {
			return res, err
		}
		ft.fileResumeData = &frd
	}

	// Optional field for when a HL v1.5+ client requests file preview
	// Used only for TEXT, JPEG, GIFF, BMP or PICT files
	// The value will always be 2
	if t.GetField(fieldFileTransferOptions).Data != nil {
		ft.options = t.GetField(fieldFileTransferOptions).Data
		xferSize = hlFile.ffo.FlatFileDataForkHeader.DataSize[:]
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldRefNum, ft.refNum[:]),
		NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
		NewField(fieldTransferSize, xferSize),
		NewField(fieldFileSize, hlFile.ffo.FlatFileDataForkHeader.DataSize[:]),
	))

	return res, err
}

// Download all files from the specified folder and sub-folders
func HandleDownloadFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessDownloadFile) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to download folders."))
		return res, err
	}

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, t.GetField(fieldFilePath).Data, t.GetField(fieldFileName).Data)
	if err != nil {
		return res, err
	}

	transferSize, err := CalcTotalSize(fullFilePath)
	if err != nil {
		return res, err
	}
	itemCount, err := CalcItemCount(fullFilePath)
	if err != nil {
		return res, err
	}

	fileTransfer := cc.newFileTransfer(FolderDownload, t.GetField(fieldFileName).Data, t.GetField(fieldFilePath).Data, transferSize)

	var fp FilePath
	_, err = fp.Write(t.GetField(fieldFilePath).Data)
	if err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldRefNum, fileTransfer.ReferenceNumber),
		NewField(fieldTransferSize, transferSize),
		NewField(fieldFolderItemCount, itemCount),
		NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	))
	return res, err
}

// Upload all files from the local folder and its subfolders to the specified path on the server
// Fields used in the request
// 201	File name
// 202	File path
// 108	transfer size	Total size of all items in the folder
// 220	Folder item count
// 204	File transfer options	"Optional Currently set to 1" (TODO: ??)
func HandleUploadFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	var fp FilePath
	if t.GetField(fieldFilePath).Data != nil {
		if _, err = fp.Write(t.GetField(fieldFilePath).Data); err != nil {
			return res, err
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(accessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			res = append(res, cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload of the folder \"%v\" because you are only allowed to upload to the \"Uploads\" folder.", string(t.GetField(fieldFileName).Data))))
			return res, err
		}
	}

	fileTransfer := cc.newFileTransfer(FolderUpload,
		t.GetField(fieldFileName).Data,
		t.GetField(fieldFilePath).Data,
		t.GetField(fieldTransferSize).Data,
	)

	fileTransfer.FolderItemCount = t.GetField(fieldFolderItemCount).Data

	res = append(res, cc.NewReply(t, NewField(fieldRefNum, fileTransfer.ReferenceNumber)))
	return res, err
}

// HandleUploadFile
// Fields used in the request:
// 201	File name
// 202	File path
// 204	File transfer options	"Optional
// Used only to resume download, currently has value 2"
// 108	File transfer size	"Optional used if download is not resumed"
func HandleUploadFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessUploadFile) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to upload files."))
		return res, err
	}

	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data
	transferOptions := t.GetField(fieldFileTransferOptions).Data
	transferSize := t.GetField(fieldTransferSize).Data // not sent for resume

	var fp FilePath
	if filePath != nil {
		if _, err = fp.Write(filePath); err != nil {
			return res, err
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(accessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			res = append(res, cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload of the file \"%v\" because you are only allowed to upload to the \"Uploads\" folder.", string(fileName))))
			return res, err
		}
	}
	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	if _, err := cc.Server.FS.Stat(fullFilePath); err == nil {
		res = append(res, cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload because there is already a file named \"%v\".  Try choosing a different name.", string(fileName))))
		return res, err
	}

	ft := cc.newFileTransfer(FileUpload, fileName, filePath, transferSize)

	replyT := cc.NewReply(t, NewField(fieldRefNum, ft.ReferenceNumber))

	// client has requested to resume a partially transferred file
	if transferOptions != nil {

		fileInfo, err := cc.Server.FS.Stat(fullFilePath + incompleteFileSuffix)
		if err != nil {
			return res, err
		}

		offset := make([]byte, 4)
		binary.BigEndian.PutUint32(offset, uint32(fileInfo.Size()))

		fileResumeData := NewFileResumeData([]ForkInfoList{
			*NewForkInfoList(offset),
		})

		b, _ := fileResumeData.BinaryMarshal()

		ft.TransferSize = offset

		replyT.Fields = append(replyT.Fields, NewField(fieldFileResumeData, b))
	}

	res = append(res, replyT)
	return res, err
}

func HandleSetClientUserInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if len(t.GetField(fieldUserIconID).Data) == 4 {
		cc.Icon = t.GetField(fieldUserIconID).Data[2:]
	} else {
		cc.Icon = t.GetField(fieldUserIconID).Data
	}
	if cc.Authorize(accessAnyName) {
		cc.UserName = t.GetField(fieldUserName).Data
	}

	// the options field is only passed by the client versions > 1.2.3.
	options := t.GetField(fieldOptions).Data
	if options != nil {
		optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))
		flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(cc.Flags)))

		flagBitmap.SetBit(flagBitmap, userFlagRefusePM, optBitmap.Bit(refusePM))
		binary.BigEndian.PutUint16(cc.Flags, uint16(flagBitmap.Int64()))

		flagBitmap.SetBit(flagBitmap, userFLagRefusePChat, optBitmap.Bit(refuseChat))
		binary.BigEndian.PutUint16(cc.Flags, uint16(flagBitmap.Int64()))

		// Check auto response
		if optBitmap.Bit(autoResponse) == 1 {
			cc.AutoReply = t.GetField(fieldAutomaticResponse).Data
		} else {
			cc.AutoReply = []byte{}
		}
	}

	for _, c := range sortedClients(cc.Server.Clients) {
		res = append(res, *NewTransaction(
			tranNotifyChangeUser,
			c.ID,
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, cc.Icon),
			NewField(fieldUserFlags, cc.Flags),
			NewField(fieldUserName, cc.UserName),
		))
	}

	return res, err
}

// HandleKeepAlive responds to keepalive transactions with an empty reply
// * HL 1.9.2 Client sends keepalive msg every 3 minutes
// * HL 1.2.3 Client doesn't send keepalives
func HandleKeepAlive(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	res = append(res, cc.NewReply(t))

	return res, err
}

func HandleGetFileNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fullPath, err := readPath(
		cc.Server.Config.FileRoot,
		t.GetField(fieldFilePath).Data,
		nil,
	)
	if err != nil {
		return res, err
	}

	var fp FilePath
	if t.GetField(fieldFilePath).Data != nil {
		if _, err = fp.Write(t.GetField(fieldFilePath).Data); err != nil {
			return res, err
		}
	}

	// Handle special case for drop box folders
	if fp.IsDropbox() && !cc.Authorize(accessViewDropBoxes) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to view drop boxes."))
		return res, err
	}

	fileNames, err := getFileNameList(fullPath, cc.Server.Config.IgnoreFiles)
	if err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t, fileNames...))

	return res, err
}

// =================================
//     Hotline private chat flow
// =================================
// 1. ClientA sends tranInviteNewChat to server with user ID to invite
// 2. Server creates new ChatID
// 3. Server sends tranInviteToChat to invitee
// 4. Server replies to ClientA with new Chat ID
//
// A dialog box pops up in the invitee client with options to accept or decline the invitation.
// If Accepted is clicked:
// 1. ClientB sends tranJoinChat with fieldChatID

// HandleInviteNewChat invites users to new private chat
func HandleInviteNewChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessOpenChat) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to request private chat."))
		return res, err
	}

	// Client to Invite
	targetID := t.GetField(fieldUserID).Data
	newChatID := cc.Server.NewPrivateChat(cc)

	// Check if target user has "Refuse private chat" flag
	binary.BigEndian.Uint16(targetID)
	targetClient := cc.Server.Clients[binary.BigEndian.Uint16(targetID)]

	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(targetClient.Flags)))
	if flagBitmap.Bit(userFLagRefusePChat) == 1 {
		res = append(res,
			*NewTransaction(
				tranServerMsg,
				cc.ID,
				NewField(fieldData, []byte(string(targetClient.UserName)+" does not accept private chats.")),
				NewField(fieldUserName, targetClient.UserName),
				NewField(fieldUserID, *targetClient.ID),
				NewField(fieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res,
			*NewTransaction(
				tranInviteToChat,
				&targetID,
				NewField(fieldChatID, newChatID),
				NewField(fieldUserName, cc.UserName),
				NewField(fieldUserID, *cc.ID),
			),
		)
	}

	res = append(res,
		cc.NewReply(t,
			NewField(fieldChatID, newChatID),
			NewField(fieldUserName, cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, cc.Icon),
			NewField(fieldUserFlags, cc.Flags),
		),
	)

	return res, err
}

func HandleInviteToChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessOpenChat) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to request private chat."))
		return res, err
	}

	// Client to Invite
	targetID := t.GetField(fieldUserID).Data
	chatID := t.GetField(fieldChatID).Data

	res = append(res,
		*NewTransaction(
			tranInviteToChat,
			&targetID,
			NewField(fieldChatID, chatID),
			NewField(fieldUserName, cc.UserName),
			NewField(fieldUserID, *cc.ID),
		),
	)
	res = append(res,
		cc.NewReply(
			t,
			NewField(fieldChatID, chatID),
			NewField(fieldUserName, cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, cc.Icon),
			NewField(fieldUserFlags, cc.Flags),
		),
	)

	return res, err
}

func HandleRejectChatInvite(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat := cc.Server.PrivateChats[chatInt]

	resMsg := append(cc.UserName, []byte(" declined invitation to chat")...)

	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewTransaction(
				tranChatMsg,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldData, resMsg),
			),
		)
	}

	return res, err
}

// HandleJoinChat is sent from a v1.8+ Hotline client when the joins a private chat
// Fields used in the reply:
// * 115	Chat subject
// * 300	User name with info (Optional)
// * 300 	(more user names with info)
func HandleJoinChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat := cc.Server.PrivateChats[chatInt]

	// Send tranNotifyChatChangeUser to current members of the chat to inform of new user
	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewTransaction(
				tranNotifyChatChangeUser,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldUserName, cc.UserName),
				NewField(fieldUserID, *cc.ID),
				NewField(fieldUserIconID, cc.Icon),
				NewField(fieldUserFlags, cc.Flags),
			),
		)
	}

	privChat.ClientConn[cc.uint16ID()] = cc

	replyFields := []Field{NewField(fieldChatSubject, []byte(privChat.Subject))}
	for _, c := range sortedClients(privChat.ClientConn) {
		user := User{
			ID:    *c.ID,
			Icon:  c.Icon,
			Flags: c.Flags,
			Name:  string(c.UserName),
		}

		replyFields = append(replyFields, NewField(fieldUsernameWithInfo, user.Payload()))
	}

	res = append(res, cc.NewReply(t, replyFields...))
	return res, err
}

// HandleLeaveChat is sent from a v1.8+ Hotline client when the user exits a private chat
// Fields used in the request:
//   - 114	fieldChatID
//
// Reply is not expected.
func HandleLeaveChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat, ok := cc.Server.PrivateChats[chatInt]
	if !ok {
		return res, nil
	}

	delete(privChat.ClientConn, cc.uint16ID())

	// Notify members of the private chat that the user has left
	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewTransaction(
				tranNotifyChatDeleteUser,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldUserID, *cc.ID),
			),
		)
	}

	return res, err
}

// HandleSetChatSubject is sent from a v1.8+ Hotline client when the user sets a private chat subject
// Fields used in the request:
// * 114	Chat ID
// * 115	Chat subject	Chat subject string
// Reply is not expected.
func HandleSetChatSubject(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat := cc.Server.PrivateChats[chatInt]
	privChat.Subject = string(t.GetField(fieldChatSubject).Data)

	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewTransaction(
				tranNotifyChatSubject,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldChatSubject, t.GetField(fieldChatSubject).Data),
			),
		)
	}

	return res, err
}

// HandleMakeAlias makes a filer alias using the specified path.
// Fields used in the request:
// 201	File name
// 202	File path
// 212	File new path	Destination path
//
// Fields used in the reply:
// None
func HandleMakeAlias(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	if !cc.Authorize(accessMakeAlias) {
		res = append(res, cc.NewErrReply(t, "You are not allowed to make aliases."))
		return res, err
	}
	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data
	fileNewPath := t.GetField(fieldFileNewPath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res, err
	}

	fullNewFilePath, err := readPath(cc.Server.Config.FileRoot, fileNewPath, fileName)
	if err != nil {
		return res, err
	}

	cc.logger.Debugw("Make alias", "src", fullFilePath, "dst", fullNewFilePath)

	if err := cc.Server.FS.Symlink(fullFilePath, fullNewFilePath); err != nil {
		res = append(res, cc.NewErrReply(t, "Error creating alias"))
		return res, nil
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleDownloadBanner handles requests for a new banner from the server
// Fields used in the request:
// None
// Fields used in the reply:
// 107	fieldRefNum			Used later for transfer
// 108	fieldTransferSize	Size of data to be downloaded
func HandleDownloadBanner(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fi, err := cc.Server.FS.Stat(filepath.Join(cc.Server.ConfigDir, cc.Server.Config.BannerFile))
	if err != nil {
		return res, err
	}

	ft := cc.newFileTransfer(bannerDownload, []byte{}, []byte{}, make([]byte, 4))

	binary.BigEndian.PutUint32(ft.TransferSize, uint32(fi.Size()))

	res = append(res, cc.NewReply(t,
		NewField(fieldRefNum, ft.refNum[:]),
		NewField(fieldTransferSize, ft.TransferSize),
	))

	return res, err
}
