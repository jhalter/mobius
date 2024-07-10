package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// HandlerFunc is the signature of a func to handle a Hotline transaction.
type HandlerFunc func(*ClientConn, *Transaction) []Transaction

// TransactionHandlers maps a transaction type to a handler function.
var TransactionHandlers = map[TranType]HandlerFunc{
	TranAgreed:             HandleTranAgreed,
	TranChatSend:           HandleChatSend,
	TranDelNewsArt:         HandleDelNewsArt,
	TranDelNewsItem:        HandleDelNewsItem,
	TranDeleteFile:         HandleDeleteFile,
	TranDeleteUser:         HandleDeleteUser,
	TranDisconnectUser:     HandleDisconnectUser,
	TranDownloadFile:       HandleDownloadFile,
	TranDownloadFldr:       HandleDownloadFolder,
	TranGetClientInfoText:  HandleGetClientInfoText,
	TranGetFileInfo:        HandleGetFileInfo,
	TranGetFileNameList:    HandleGetFileNameList,
	TranGetMsgs:            HandleGetMsgs,
	TranGetNewsArtData:     HandleGetNewsArtData,
	TranGetNewsArtNameList: HandleGetNewsArtNameList,
	TranGetNewsCatNameList: HandleGetNewsCatNameList,
	TranGetUser:            HandleGetUser,
	TranGetUserNameList:    HandleGetUserNameList,
	TranInviteNewChat:      HandleInviteNewChat,
	TranInviteToChat:       HandleInviteToChat,
	TranJoinChat:           HandleJoinChat,
	TranKeepAlive:          HandleKeepAlive,
	TranLeaveChat:          HandleLeaveChat,
	TranListUsers:          HandleListUsers,
	TranMoveFile:           HandleMoveFile,
	TranNewFolder:          HandleNewFolder,
	TranNewNewsCat:         HandleNewNewsCat,
	TranNewNewsFldr:        HandleNewNewsFldr,
	TranNewUser:            HandleNewUser,
	TranUpdateUser:         HandleUpdateUser,
	TranOldPostNews:        HandleTranOldPostNews,
	TranPostNewsArt:        HandlePostNewsArt,
	TranRejectChatInvite:   HandleRejectChatInvite,
	TranSendInstantMsg:     HandleSendInstantMsg,
	TranSetChatSubject:     HandleSetChatSubject,
	TranMakeFileAlias:      HandleMakeAlias,
	TranSetClientUserInfo:  HandleSetClientUserInfo,
	TranSetFileInfo:        HandleSetFileInfo,
	TranSetUser:            HandleSetUser,
	TranUploadFile:         HandleUploadFile,
	TranUploadFldr:         HandleUploadFolder,
	TranUserBroadcast:      HandleUserBroadcast,
	TranDownloadBanner:     HandleDownloadBanner,
}

// The total size of a chat message data field is 8192 bytes.
const chatMsgLimit = 8192

func HandleChatSend(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessSendChat) {
		return cc.NewErrReply(t, "You are not allowed to participate in chat.")
	}

	// Truncate long usernames
	// %13.13s: This means a string that is right-aligned in a field of 13 characters.
	// If the string is longer than 13 characters, it will be truncated to 13 characters.
	formattedMsg := fmt.Sprintf("\r%13.13s:  %s", cc.UserName, t.GetField(FieldData).Data)

	// By holding the option key, Hotline chat allows users to send /me formatted messages like:
	// *** Halcyon does stuff
	// This is indicated by the presence of the optional field FieldChatOptions set to a value of 1.
	// Most clients do not send this option for normal chat messages.
	if t.GetField(FieldChatOptions).Data != nil && bytes.Equal(t.GetField(FieldChatOptions).Data, []byte{0, 1}) {
		formattedMsg = fmt.Sprintf("\r*** %s %s", cc.UserName, t.GetField(FieldData).Data)
	}

	// Truncate the message to the limit.  This does not handle the edge case of a string ending on multibyte character.
	formattedMsg = formattedMsg[:min(len(formattedMsg), chatMsgLimit)]

	// The ChatID field is used to identify messages as belonging to a private chat.
	// All clients *except* Frogblast omit this field for public chat, but Frogblast sends a value of 00 00 00 00.
	chatID := t.GetField(FieldChatID).Data
	if chatID != nil && !bytes.Equal([]byte{0, 0, 0, 0}, chatID) {

		// send the message to all connected clients of the private chat
		for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
			res = append(res, NewTransaction(
				TranChatMsg,
				c.ID,
				NewField(FieldChatID, chatID),
				NewField(FieldData, []byte(formattedMsg)),
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
		if c.Authorize(AccessReadChat) {
			res = append(res, NewTransaction(TranChatMsg, c.ID, NewField(FieldData, []byte(formattedMsg))))
		}
	}
	//cc.Server.mux.Unlock()

	return res
}

// HandleSendInstantMsg sends instant message to the user on the current server.
// Fields used in the request:
//
//	103	User Type
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
func HandleSendInstantMsg(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessSendPrivMsg) {
		return cc.NewErrReply(t, "You are not allowed to send private messages.")
	}

	msg := t.GetField(FieldData)
	userID := t.GetField(FieldUserID)

	reply := NewTransaction(
		TranServerMsg,
		[2]byte(userID.Data),
		NewField(FieldData, msg.Data),
		NewField(FieldUserName, cc.UserName),
		NewField(FieldUserID, cc.ID[:]),
		NewField(FieldOptions, []byte{0, 1}),
	)

	// Later versions of Hotline include the original message in the FieldQuotingMsg field so
	//  the receiving client can display both the received message and what it is in reply to
	if t.GetField(FieldQuotingMsg).Data != nil {
		reply.Fields = append(reply.Fields, NewField(FieldQuotingMsg, t.GetField(FieldQuotingMsg).Data))
	}

	otherClient := cc.Server.ClientMgr.Get([2]byte(userID.Data))
	if otherClient == nil {
		return res
	}

	// Check if target user has "Refuse private messages" flag
	if otherClient.Flags.IsSet(UserFlagRefusePM) {
		res = append(res,
			NewTransaction(
				TranServerMsg,
				cc.ID,
				NewField(FieldData, []byte(string(otherClient.UserName)+" does not accept private messages.")),
				NewField(FieldUserName, otherClient.UserName),
				NewField(FieldUserID, otherClient.ID[:]),
				NewField(FieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res, reply)
	}

	// Respond with auto reply if other client has it enabled
	if len(otherClient.AutoReply) > 0 {
		res = append(res,
			NewTransaction(
				TranServerMsg,
				cc.ID,
				NewField(FieldData, otherClient.AutoReply),
				NewField(FieldUserName, otherClient.UserName),
				NewField(FieldUserID, otherClient.ID[:]),
				NewField(FieldOptions, []byte{0, 1}),
			),
		)
	}

	return append(res, cc.NewReply(t))
}

var fileTypeFLDR = [4]byte{0x66, 0x6c, 0x64, 0x72}

func HandleGetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction) {
	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	fw, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}

	encodedName, err := txtEncoder.String(fw.name)
	if err != nil {
		return res
	}

	fields := []Field{
		NewField(FieldFileName, []byte(encodedName)),
		NewField(FieldFileTypeString, fw.ffo.FlatFileInformationFork.friendlyType()),
		NewField(FieldFileCreatorString, fw.ffo.FlatFileInformationFork.friendlyCreator()),
		NewField(FieldFileType, fw.ffo.FlatFileInformationFork.TypeSignature[:]),
		NewField(FieldFileCreateDate, fw.ffo.FlatFileInformationFork.CreateDate[:]),
		NewField(FieldFileModifyDate, fw.ffo.FlatFileInformationFork.ModifyDate[:]),
	}

	// Include the optional FileComment field if there is a comment.
	if len(fw.ffo.FlatFileInformationFork.Comment) != 0 {
		fields = append(fields, NewField(FieldFileComment, fw.ffo.FlatFileInformationFork.Comment))
	}

	// Include the FileSize field for files.
	if fw.ffo.FlatFileInformationFork.TypeSignature != fileTypeFLDR {
		fields = append(fields, NewField(FieldFileSize, fw.totalSize()))
	}

	res = append(res, cc.NewReply(t, fields...))
	return res
}

// HandleSetFileInfo updates a file or folder Name and/or comment from the Get Info window
// Fields used in the request:
// * 201	File Name
// * 202	File path	Optional
// * 211	File new Name	Optional
// * 210	File comment	Optional
// Fields used in the reply:	None
func HandleSetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction) {
	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	fi, err := cc.Server.FS.Stat(fullFilePath)
	if err != nil {
		return res
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}
	if t.GetField(FieldFileComment).Data != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(AccessSetFolderComment) {
				return cc.NewErrReply(t, "You are not allowed to set comments for folders.")
			}
		case mode.IsRegular():
			if !cc.Authorize(AccessSetFileComment) {
				return cc.NewErrReply(t, "You are not allowed to set comments for files.")
			}
		}

		if err := hlFile.ffo.FlatFileInformationFork.setComment(t.GetField(FieldFileComment).Data); err != nil {
			return res
		}
		w, err := hlFile.infoForkWriter()
		if err != nil {
			return res
		}
		_, err = io.Copy(w, &hlFile.ffo.FlatFileInformationFork)
		if err != nil {
			return res
		}
	}

	fullNewFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, t.GetField(FieldFileNewName).Data)
	if err != nil {
		return nil
	}

	fileNewName := t.GetField(FieldFileNewName).Data

	if fileNewName != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(AccessRenameFolder) {
				return cc.NewErrReply(t, "You are not allowed to rename folders.")
			}
			err = os.Rename(fullFilePath, fullNewFilePath)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, "Cannot rename folder "+string(fileName)+" because it does not exist or cannot be found.")

			}
		case mode.IsRegular():
			if !cc.Authorize(AccessRenameFile) {
				return cc.NewErrReply(t, "You are not allowed to rename files.")
			}
			fileDir, err := readPath(cc.Server.Config.FileRoot, filePath, []byte{})
			if err != nil {
				return nil
			}
			hlFile.name, err = txtDecoder.String(string(fileNewName))
			if err != nil {
				return res
			}

			err = hlFile.move(fileDir)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, "Cannot rename file "+string(fileName)+" because it does not exist or cannot be found.")
			}
			if err != nil {
				return res
			}
		}
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleDeleteFile deletes a file or folder
// Fields used in the request:
// * 201	File Name
// * 202	File path
// Fields used in the reply: none
func HandleDeleteFile(cc *ClientConn, t *Transaction) (res []Transaction) {
	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}

	fi, err := hlFile.dataFile()
	if err != nil {
		return cc.NewErrReply(t, "Cannot delete file "+string(fileName)+" because it does not exist or cannot be found.")
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(AccessDeleteFolder) {
			return cc.NewErrReply(t, "You are not allowed to delete folders.")
		}
	case mode.IsRegular():
		if !cc.Authorize(AccessDeleteFile) {
			return cc.NewErrReply(t, "You are not allowed to delete files.")
		}
	}

	if err := hlFile.delete(); err != nil {
		return res
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleMoveFile moves files or folders. Note: seemingly not documented
func HandleMoveFile(cc *ClientConn, t *Transaction) (res []Transaction) {
	fileName := string(t.GetField(FieldFileName).Data)

	filePath, err := readPath(cc.Server.Config.FileRoot, t.GetField(FieldFilePath).Data, t.GetField(FieldFileName).Data)
	if err != nil {
		return res
	}

	fileNewPath, err := readPath(cc.Server.Config.FileRoot, t.GetField(FieldFileNewPath).Data, nil)
	if err != nil {
		return res
	}

	cc.logger.Info("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	hlFile, err := newFileWrapper(cc.Server.FS, filePath, 0)
	if err != nil {
		return res
	}

	fi, err := hlFile.dataFile()
	if err != nil {
		return cc.NewErrReply(t, "Cannot delete file "+fileName+" because it does not exist or cannot be found.")
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(AccessMoveFolder) {
			return cc.NewErrReply(t, "You are not allowed to move folders.")
		}
	case mode.IsRegular():
		if !cc.Authorize(AccessMoveFile) {
			return cc.NewErrReply(t, "You are not allowed to move files.")
		}
	}
	if err := hlFile.move(fileNewPath); err != nil {
		return res
	}
	// TODO: handle other possible errors; e.g. fileWrapper delete fails due to fileWrapper permission issue

	res = append(res, cc.NewReply(t))
	return res
}

func HandleNewFolder(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessCreateFolder) {
		return cc.NewErrReply(t, "You are not allowed to create folders.")
	}
	folderName := string(t.GetField(FieldFileName).Data)

	folderName = path.Join("/", folderName)

	var subPath string

	// FieldFilePath is only present for nested paths
	if t.GetField(FieldFilePath).Data != nil {
		var newFp FilePath
		_, err := newFp.Write(t.GetField(FieldFilePath).Data)
		if err != nil {
			return res
		}

		for _, pathItem := range newFp.Items {
			subPath = filepath.Join("/", subPath, string(pathItem.Name))
		}
	}
	newFolderPath := path.Join(cc.Server.Config.FileRoot, subPath, folderName)
	newFolderPath, err := txtDecoder.String(newFolderPath)
	if err != nil {
		return res
	}

	// TODO: check path and folder Name lengths

	if _, err := cc.Server.FS.Stat(newFolderPath); !os.IsNotExist(err) {
		msg := fmt.Sprintf("Cannot create folder \"%s\" because there is already a file or folder with that Name.", folderName)
		return cc.NewErrReply(t, msg)
	}

	if err := cc.Server.FS.Mkdir(newFolderPath, 0777); err != nil {
		msg := fmt.Sprintf("Cannot create folder \"%s\" because an error occurred.", folderName)
		return cc.NewErrReply(t, msg)
	}

	return append(res, cc.NewReply(t))
}

func HandleSetUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessModifyUser) {
		return cc.NewErrReply(t, "You are not allowed to modify accounts.")
	}

	login := t.GetField(FieldUserLogin).DecodeObfuscatedString()
	userName := string(t.GetField(FieldUserName).Data)

	newAccessLvl := t.GetField(FieldUserAccess).Data

	account := cc.Server.AccountManager.Get(login)
	if account == nil {
		return cc.NewErrReply(t, "Account not found.")
	}
	account.Name = userName
	copy(account.Access[:], newAccessLvl)

	// If the password field is cleared in the Hotline edit user UI, the SetUser transaction does
	// not include FieldUserPassword
	if t.GetField(FieldUserPassword).Data == nil {
		account.Password = hashAndSalt([]byte(""))
	}

	if !bytes.Equal([]byte{0}, t.GetField(FieldUserPassword).Data) {
		account.Password = hashAndSalt(t.GetField(FieldUserPassword).Data)
	}

	err := cc.Server.AccountManager.Update(*account, account.Login)
	if err != nil {
		cc.logger.Error("Error updating account", "Err", err)
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.ClientMgr.List() {
		if c.Account.Login == login {
			newT := NewTransaction(TranUserAccess, c.ID, NewField(FieldUserAccess, newAccessLvl))
			res = append(res, newT)

			if c.Authorize(AccessDisconUser) {
				c.Flags.Set(UserFlagAdmin, 1)
			} else {
				c.Flags.Set(UserFlagAdmin, 0)
			}

			c.Account.Access = account.Access

			cc.SendAll(
				TranNotifyChangeUser,
				NewField(FieldUserID, c.ID[:]),
				NewField(FieldUserFlags, c.Flags[:]),
				NewField(FieldUserName, c.UserName),
				NewField(FieldUserIconID, c.Icon),
			)
		}
	}

	return append(res, cc.NewReply(t))
}

func HandleGetUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessOpenUser) {
		return cc.NewErrReply(t, "You are not allowed to view accounts.")
	}

	account := cc.Server.AccountManager.Get(string(t.GetField(FieldUserLogin).Data))
	if account == nil {
		return cc.NewErrReply(t, "Account does not exist.")
	}

	return append(res, cc.NewReply(t,
		NewField(FieldUserName, []byte(account.Name)),
		NewField(FieldUserLogin, encodeString(t.GetField(FieldUserLogin).Data)),
		NewField(FieldUserPassword, []byte(account.Password)),
		NewField(FieldUserAccess, account.Access[:]),
	))
}

func HandleListUsers(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessOpenUser) {
		return cc.NewErrReply(t, "You are not allowed to view accounts.")
	}

	var userFields []Field
	for _, acc := range cc.Server.AccountManager.List() {
		b, err := io.ReadAll(&acc)
		if err != nil {
			cc.logger.Error("Error reading account", "Account", acc.Login, "Err", err)
			continue
		}

		userFields = append(userFields, NewField(FieldData, b))
	}

	return append(res, cc.NewReply(t, userFields...))
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
func HandleUpdateUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	for _, field := range t.Fields {
		var subFields []Field

		// Create a new scanner for parsing incoming bytes into transaction tokens
		scanner := bufio.NewScanner(bytes.NewReader(field.Data[2:]))
		scanner.Split(fieldScanner)

		for i := 0; i < int(binary.BigEndian.Uint16(field.Data[0:2])); i++ {
			scanner.Scan()

			var field Field
			if _, err := field.Write(scanner.Bytes()); err != nil {
				return res
			}
			subFields = append(subFields, field)
		}

		// If there's only one subfield, that indicates this is a delete operation for the login in FieldData
		if len(subFields) == 1 {
			if !cc.Authorize(AccessDeleteUser) {
				return cc.NewErrReply(t, "You are not allowed to delete accounts.")
			}

			login := string(encodeString(getField(FieldData, &subFields).Data))

			cc.logger.Info("DeleteUser", "login", login)

			if err := cc.Server.AccountManager.Delete(login); err != nil {
				cc.logger.Error("Error deleting account", "Err", err)
				return res
			}

			for _, client := range cc.Server.ClientMgr.List() {
				if client.Account.Login == login {
					//					"You are logged in with an account which was deleted."

					res = append(res,
						NewTransaction(TranServerMsg, [2]byte{},
							NewField(FieldData, []byte("You are logged in with an account which was deleted.")),
							NewField(FieldChatOptions, []byte{0}),
						),
					)

					go func(c *ClientConn) {
						time.Sleep(3 * time.Second)
						c.Disconnect()
					}(client)
				}
			}

			continue
		}

		// login of the account to update
		var accountToUpdate, loginToRename string

		// If FieldData is included, this is a rename operation where FieldData contains the login of the existing
		// account and FieldUserLogin contains the new login.
		if getField(FieldData, &subFields) != nil {
			loginToRename = string(encodeString(getField(FieldData, &subFields).Data))
		}
		userLogin := string(encodeString(getField(FieldUserLogin, &subFields).Data))
		if loginToRename != "" {
			accountToUpdate = loginToRename
		} else {
			accountToUpdate = userLogin
		}

		// Check if accountToUpdate has an existing account.  If so, we know we are updating an existing user.
		if acc := cc.Server.AccountManager.Get(accountToUpdate); acc != nil {
			if loginToRename != "" {
				cc.logger.Info("RenameUser", "prevLogin", accountToUpdate, "newLogin", userLogin)
			} else {
				cc.logger.Info("UpdateUser", "login", accountToUpdate)
			}

			// Account exists, so this is an update action.
			if !cc.Authorize(AccessModifyUser) {
				return cc.NewErrReply(t, "You are not allowed to modify accounts.")
			}

			// This part is a bit tricky. There are three possibilities:
			// 1) The transaction is intended to update the password.
			//	  In this case, FieldUserPassword is sent with the new password.
			// 2) The transaction is intended to remove the password.
			//    In this case, FieldUserPassword is not sent.
			// 3) The transaction updates the users access bits, but not the password.
			//    In this case, FieldUserPassword is sent with zero as the only byte.
			if getField(FieldUserPassword, &subFields) != nil {
				newPass := getField(FieldUserPassword, &subFields).Data
				if !bytes.Equal([]byte{0}, newPass) {
					acc.Password = hashAndSalt(newPass)
				}
			} else {
				acc.Password = hashAndSalt([]byte(""))
			}

			if getField(FieldUserAccess, &subFields) != nil {
				copy(acc.Access[:], getField(FieldUserAccess, &subFields).Data)
			}

			acc.Name = string(getField(FieldUserName, &subFields).Data)

			err := cc.Server.AccountManager.Update(*acc, string(encodeString(getField(FieldUserLogin, &subFields).Data)))

			if err != nil {
				return res
			}
		} else {
			if !cc.Authorize(AccessCreateUser) {
				return cc.NewErrReply(t, "You are not allowed to create new accounts.")
			}

			cc.logger.Info("CreateUser", "login", userLogin)

			newAccess := accessBitmap{}
			copy(newAccess[:], getField(FieldUserAccess, &subFields).Data)

			// Prevent account from creating new account with greater permission
			for i := 0; i < 64; i++ {
				if newAccess.IsSet(i) {
					if !cc.Authorize(i) {
						return cc.NewErrReply(t, "Cannot create account with more access than yourself.")
					}
				}
			}

			account := NewAccount(userLogin, string(getField(FieldUserName, &subFields).Data), string(getField(FieldUserPassword, &subFields).Data), newAccess)

			err := cc.Server.AccountManager.Create(*account)
			if err != nil {
				return cc.NewErrReply(t, "Cannot create account because there is already an account with that login.")
			}
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleNewUser creates a new user account
func HandleNewUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessCreateUser) {
		return cc.NewErrReply(t, "You are not allowed to create new accounts.")
	}

	login := t.GetField(FieldUserLogin).DecodeObfuscatedString()

	// If the account already exists, reply with an error.
	if account := cc.Server.AccountManager.Get(login); account != nil {
		return cc.NewErrReply(t, "Cannot create account "+login+" because there is already an account with that login.")
	}

	var newAccess accessBitmap
	copy(newAccess[:], t.GetField(FieldUserAccess).Data)

	// Prevent account from creating new account with greater permission
	for i := 0; i < 64; i++ {
		if newAccess.IsSet(i) {
			if !cc.Authorize(i) {
				return cc.NewErrReply(t, "Cannot create account with more access than yourself.")
			}
		}
	}

	account := NewAccount(login, string(t.GetField(FieldUserName).Data), string(t.GetField(FieldUserPassword).Data), newAccess)

	err := cc.Server.AccountManager.Create(*account)
	if err != nil {
		return cc.NewErrReply(t, "Cannot create account because there is already an account with that login.")
	}

	return append(res, cc.NewReply(t))
}

func HandleDeleteUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessDeleteUser) {
		return cc.NewErrReply(t, "You are not allowed to delete accounts.")
	}

	login := t.GetField(FieldUserLogin).DecodeObfuscatedString()

	if err := cc.Server.AccountManager.Delete(login); err != nil {
		cc.logger.Error("Error deleting account", "Err", err)
		return res
	}

	for _, client := range cc.Server.ClientMgr.List() {
		if client.Account.Login == login {
			res = append(res,
				NewTransaction(TranServerMsg, client.ID,
					NewField(FieldData, []byte("You are logged in with an account which was deleted.")),
					NewField(FieldChatOptions, []byte{2}),
				),
			)

			go func(c *ClientConn) {
				time.Sleep(2 * time.Second)
				c.Disconnect()
			}(client)
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleUserBroadcast sends an Administrator Message to all connected clients of the server
func HandleUserBroadcast(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessBroadcast) {
		return cc.NewErrReply(t, "You are not allowed to send broadcast messages.")
	}

	cc.SendAll(
		TranServerMsg,
		NewField(FieldData, t.GetField(FieldData).Data),
		NewField(FieldChatOptions, []byte{0}),
	)

	return append(res, cc.NewReply(t))
}

// HandleGetClientInfoText returns user information for the specific user.
//
// Fields used in the request:
// 103	User Type
//
// Fields used in the reply:
// 102	User Name
// 101	Data		User info text string
func HandleGetClientInfoText(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessGetClientInfo) {
		return cc.NewErrReply(t, "You are not allowed to get client info.")
	}

	clientID := t.GetField(FieldUserID).Data

	clientConn := cc.Server.ClientMgr.Get(ClientID(clientID))
	if clientConn == nil {
		return cc.NewErrReply(t, "User not found.")
	}

	return append(res, cc.NewReply(t,
		NewField(FieldData, []byte(clientConn.String())),
		NewField(FieldUserName, clientConn.UserName),
	))
}

func HandleGetUserNameList(cc *ClientConn, t *Transaction) (res []Transaction) {
	var fields []Field
	for _, c := range cc.Server.ClientMgr.List() {
		b, err := io.ReadAll(&User{
			ID:    c.ID,
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			return nil
		}

		fields = append(fields, NewField(FieldUsernameWithInfo, b))
	}

	return []Transaction{cc.NewReply(t, fields...)}
}

func HandleTranAgreed(cc *ClientConn, t *Transaction) (res []Transaction) {
	if t.GetField(FieldUserName).Data != nil {
		if cc.Authorize(AccessAnyName) {
			cc.UserName = t.GetField(FieldUserName).Data
		} else {
			cc.UserName = []byte(cc.Account.Name)
		}
	}

	cc.Icon = t.GetField(FieldUserIconID).Data

	cc.logger = cc.logger.With("Name", string(cc.UserName))
	cc.logger.Info("Login successful")

	options := t.GetField(FieldOptions).Data
	optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

	// Check refuse private PM option

	cc.flagsMU.Lock()
	defer cc.flagsMU.Unlock()
	cc.Flags.Set(UserFlagRefusePM, optBitmap.Bit(UserOptRefusePM))

	// Check refuse private chat option
	cc.Flags.Set(UserFlagRefusePChat, optBitmap.Bit(UserOptRefuseChat))

	// Check auto response
	if optBitmap.Bit(UserOptAutoResponse) == 1 {
		cc.AutoReply = t.GetField(FieldAutomaticResponse).Data
	}

	trans := cc.NotifyOthers(
		NewTransaction(
			TranNotifyChangeUser, [2]byte{0, 0},
			NewField(FieldUserName, cc.UserName),
			NewField(FieldUserID, cc.ID[:]),
			NewField(FieldUserIconID, cc.Icon),
			NewField(FieldUserFlags, cc.Flags[:]),
		),
	)
	res = append(res, trans...)

	if cc.Server.Config.BannerFile != "" {
		res = append(res, NewTransaction(TranServerBanner, cc.ID, NewField(FieldBannerType, []byte("JPEG"))))
	}

	res = append(res, cc.NewReply(t))

	return res
}

// HandleTranOldPostNews updates the flat news
// Fields used in this request:
// 101	Data
func HandleTranOldPostNews(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsPostArt) {
		return cc.NewErrReply(t, "You are not allowed to post news.")
	}

	newsDateTemplate := defaultNewsDateFormat
	if cc.Server.Config.NewsDateFormat != "" {
		newsDateTemplate = cc.Server.Config.NewsDateFormat
	}

	newsTemplate := defaultNewsTemplate
	if cc.Server.Config.NewsDelimiter != "" {
		newsTemplate = cc.Server.Config.NewsDelimiter
	}

	newsPost := fmt.Sprintf(newsTemplate+"\r", cc.UserName, time.Now().Format(newsDateTemplate), t.GetField(FieldData).Data)
	newsPost = strings.ReplaceAll(newsPost, "\n", "\r")

	_, err := cc.Server.MessageBoard.Write([]byte(newsPost))
	if err != nil {
		cc.logger.Error("error writing news post", "err", err)
		return nil
	}

	// Notify all clients of updated news
	cc.SendAll(
		TranNewMsg,
		NewField(FieldData, []byte(newsPost)),
	)

	return append(res, cc.NewReply(t))
}

func HandleDisconnectUser(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessDisconUser) {
		return cc.NewErrReply(t, "You are not allowed to disconnect users.")
	}

	clientID := [2]byte(t.GetField(FieldUserID).Data)
	clientConn := cc.Server.ClientMgr.Get(clientID)

	if clientConn.Authorize(AccessCannotBeDiscon) {
		return cc.NewErrReply(t, clientConn.Account.Login+" is not allowed to be disconnected.")
	}

	// If FieldOptions is set, then the client IP is banned in addition to disconnected.
	// 00 01 = temporary ban
	// 00 02 = permanent ban
	if t.GetField(FieldOptions).Data != nil {
		switch t.GetField(FieldOptions).Data[1] {
		case 1:
			// send message: "You are temporarily banned on this server"
			cc.logger.Info("Disconnect & temporarily ban " + string(clientConn.UserName))

			res = append(res, NewTransaction(
				TranServerMsg,
				clientConn.ID,
				NewField(FieldData, []byte("You are temporarily banned on this server")),
				NewField(FieldChatOptions, []byte{0, 0}),
			))

			banUntil := time.Now().Add(tempBanDuration)
			ip := strings.Split(clientConn.RemoteAddr, ":")[0]

			err := cc.Server.BanList.Add(ip, &banUntil)
			if err != nil {
				cc.logger.Error("Error saving ban", "err", err)
				// TODO
			}
		case 2:
			// send message: "You are permanently banned on this server"
			cc.logger.Info("Disconnect & ban " + string(clientConn.UserName))

			res = append(res, NewTransaction(
				TranServerMsg,
				clientConn.ID,
				NewField(FieldData, []byte("You are permanently banned on this server")),
				NewField(FieldChatOptions, []byte{0, 0}),
			))

			ip := strings.Split(clientConn.RemoteAddr, ":")[0]

			err := cc.Server.BanList.Add(ip, nil)
			if err != nil {
				// TODO
			}
		}
	}

	// TODO: remove this awful hack
	go func() {
		time.Sleep(1 * time.Second)
		clientConn.Disconnect()
	}()

	return append(res, cc.NewReply(t))
}

// HandleGetNewsCatNameList returns a list of news categories for a path
// Fields used in the request:
// 325	News path	(Optional)
func HandleGetNewsCatNameList(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsReadArt) {
		return cc.NewErrReply(t, "You are not allowed to read news.")
	}

	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {

	}

	var fields []Field
	for _, cat := range cc.Server.ThreadedNewsMgr.GetCategories(pathStrs) {
		b, err := io.ReadAll(&cat)
		if err != nil {
			// TODO
		}

		fields = append(fields, NewField(FieldNewsCatListData15, b))
	}

	return append(res, cc.NewReply(t, fields...))
}

func HandleNewNewsCat(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsCreateCat) {
		return cc.NewErrReply(t, "You are not allowed to create news categories.")
	}

	name := string(t.GetField(FieldNewsCatName).Data)
	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, NewsCategory)
	if err != nil {
		cc.logger.Error("error creating news category", "err", err)
	}

	return []Transaction{cc.NewReply(t)}
}

// Fields used in the request:
// 322	News category Name
// 325	News path
func HandleNewNewsFldr(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsCreateFldr) {
		return cc.NewErrReply(t, "You are not allowed to create news folders.")
	}

	name := string(t.GetField(FieldFileName).Data)
	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, NewsBundle)
	if err != nil {
		cc.logger.Error("error creating news bundle", "err", err)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetNewsArtData gets the list of article names at the specified news path.

// Fields used in the request:
// 325	News path	Optional

// Fields used in the reply:
// 321	News article list data	Optional
func HandleGetNewsArtNameList(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsReadArt) {
		return cc.NewErrReply(t, "You are not allowed to read news.")
	}

	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	nald := cc.Server.ThreadedNewsMgr.ListArticles(pathStrs)

	b, err := io.ReadAll(&nald)
	if err != nil {
		return res
	}

	return append(res, cc.NewReply(t, NewField(FieldNewsArtListData, b)))
}

// HandleGetNewsArtData requests information about the specific news article.
// Fields used in the request:
//
// Request fields
// 325	News path
// 326	News article Type
// 327	News article data flavor
//
// Fields used in the reply:
// 328	News article title
// 329	News article poster
// 330	News article date
// 331	Previous article Type
// 332	Next article Type
// 335	Parent article Type
// 336	First child article Type
// 327	News article data flavor	"Should be “text/plain”
// 333	News article data	Optional (if data flavor is “text/plain”)
func HandleGetNewsArtData(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsReadArt) {
		return cc.NewErrReply(t, "You are not allowed to read news.")
	}

	newsPath, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	convertedID, err := t.GetField(FieldNewsArtID).DecodeInt()
	if err != nil {
		return res
	}

	art := cc.Server.ThreadedNewsMgr.GetArticle(newsPath, uint32(convertedID))
	if art == nil {
		return append(res, cc.NewReply(t))
	}

	res = append(res, cc.NewReply(t,
		NewField(FieldNewsArtTitle, []byte(art.Title)),
		NewField(FieldNewsArtPoster, []byte(art.Poster)),
		NewField(FieldNewsArtDate, art.Date[:]),
		NewField(FieldNewsArtPrevArt, art.PrevArt[:]),
		NewField(FieldNewsArtNextArt, art.NextArt[:]),
		NewField(FieldNewsArtParentArt, art.ParentArt[:]),
		NewField(FieldNewsArt1stChildArt, art.FirstChildArt[:]),
		NewField(FieldNewsArtDataFlav, []byte("text/plain")),
		NewField(FieldNewsArtData, []byte(art.Data)),
	))
	return res
}

// HandleDelNewsItem deletes a threaded news folder or category.
// Fields used in the request:
// 325	News path
// Fields used in the reply:
// None
func HandleDelNewsItem(cc *ClientConn, t *Transaction) (res []Transaction) {
	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	item := cc.Server.ThreadedNewsMgr.NewsItem(pathStrs)

	if item.Type == [2]byte{0, 3} {
		if !cc.Authorize(AccessNewsDeleteCat) {
			return cc.NewErrReply(t, "You are not allowed to delete news categories.")
		}
	} else {
		if !cc.Authorize(AccessNewsDeleteFldr) {
			return cc.NewErrReply(t, "You are not allowed to delete news folders.")
		}
	}

	err = cc.Server.ThreadedNewsMgr.DeleteNewsItem(pathStrs)
	if err != nil {
		return res
	}

	return append(res, cc.NewReply(t))
}

// HandleDelNewsArt deletes a threaded news article.
// Request Fields
// 325	News path
// 326	News article Type
// 337	News article recursive delete	- Delete child articles (1) or not (0)
func HandleDelNewsArt(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsDeleteArt) {
		return cc.NewErrReply(t, "You are not allowed to delete news articles.")

	}

	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	articleID, err := t.GetField(FieldNewsArtID).DecodeInt()
	if err != nil {
		cc.logger.Error("error reading article Type", "err", err)
		return
	}

	deleteRecursive := bytes.Equal([]byte{0, 1}, t.GetField(FieldNewsArtRecurseDel).Data)

	err = cc.Server.ThreadedNewsMgr.DeleteArticle(pathStrs, uint32(articleID), deleteRecursive)
	if err != nil {
		cc.logger.Error("error deleting news article", "err", err)
	}

	return []Transaction{cc.NewReply(t)}
}

// Request fields
// 325	News path
// 326	News article Type	 						Type of the parent article?
// 328	News article title
// 334	News article flags
// 327	News article data flavor		Currently “text/plain”
// 333	News article data
func HandlePostNewsArt(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsPostArt) {
		return cc.NewErrReply(t, "You are not allowed to post news articles.")
	}

	pathStrs, err := t.GetField(FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	parentArticleID, err := t.GetField(FieldNewsArtID).DecodeInt()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.PostArticle(
		pathStrs,
		uint32(parentArticleID),
		NewsArtData{
			Title:    string(t.GetField(FieldNewsArtTitle).Data),
			Poster:   string(cc.UserName),
			Date:     toHotlineTime(time.Now()),
			DataFlav: NewsFlavor,
			Data:     string(t.GetField(FieldNewsArtData).Data),
		},
	)
	if err != nil {
		cc.logger.Error("error posting news article", "err", err)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetMsgs returns the flat news data
func HandleGetMsgs(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessNewsReadArt) {
		return cc.NewErrReply(t, "You are not allowed to read news.")
	}

	_, _ = cc.Server.MessageBoard.Seek(0, 0)

	newsData, err := io.ReadAll(cc.Server.MessageBoard)
	if err != nil {
		// TODO
	}

	return append(res, cc.NewReply(t, NewField(FieldData, newsData)))
}

func HandleDownloadFile(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessDownloadFile) {
		return cc.NewErrReply(t, "You are not allowed to download files.")
	}

	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data
	resumeData := t.GetField(FieldFileResumeData).Data

	var dataOffset int64
	var frd FileResumeData
	if resumeData != nil {
		if err := frd.UnmarshalBinary(t.GetField(FieldFileResumeData).Data); err != nil {
			return res
		}
		// TODO: handle rsrc fork offset
		dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
	}

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	hlFile, err := newFileWrapper(cc.Server.FS, fullFilePath, dataOffset)
	if err != nil {
		return res
	}

	xferSize := hlFile.ffo.TransferSize(0)

	ft := cc.newFileTransfer(FileDownload, fileName, filePath, xferSize)

	// TODO: refactor to remove this
	if resumeData != nil {
		var frd FileResumeData
		if err := frd.UnmarshalBinary(t.GetField(FieldFileResumeData).Data); err != nil {
			return res
		}
		ft.fileResumeData = &frd
	}

	// Optional field for when a client requests file preview
	// Used only for TEXT, JPEG, GIFF, BMP or PICT files
	// The value will always be 2
	if t.GetField(FieldFileTransferOptions).Data != nil {
		ft.options = t.GetField(FieldFileTransferOptions).Data
		xferSize = hlFile.ffo.FlatFileDataForkHeader.DataSize[:]
	}

	res = append(res, cc.NewReply(t,
		NewField(FieldRefNum, ft.refNum[:]),
		NewField(FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
		NewField(FieldTransferSize, xferSize),
		NewField(FieldFileSize, hlFile.ffo.FlatFileDataForkHeader.DataSize[:]),
	))

	return res
}

// Download all files from the specified folder and sub-folders
func HandleDownloadFolder(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessDownloadFile) {
		return cc.NewErrReply(t, "You are not allowed to download folders.")
	}

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, t.GetField(FieldFilePath).Data, t.GetField(FieldFileName).Data)
	if err != nil {
		return res
	}

	transferSize, err := CalcTotalSize(fullFilePath)
	if err != nil {
		return res
	}
	itemCount, err := CalcItemCount(fullFilePath)
	if err != nil {
		return res
	}

	fileTransfer := cc.newFileTransfer(FolderDownload, t.GetField(FieldFileName).Data, t.GetField(FieldFilePath).Data, transferSize)

	var fp FilePath
	_, err = fp.Write(t.GetField(FieldFilePath).Data)
	if err != nil {
		return res
	}

	res = append(res, cc.NewReply(t,
		NewField(FieldRefNum, fileTransfer.refNum[:]),
		NewField(FieldTransferSize, transferSize),
		NewField(FieldFolderItemCount, itemCount),
		NewField(FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	))
	return res
}

// Upload all files from the local folder and its subfolders to the specified path on the server
// Fields used in the request
// 201	File Name
// 202	File path
// 108	transfer size	Total size of all items in the folder
// 220	Folder item count
// 204	File transfer options	"Optional Currently set to 1" (TODO: ??)
func HandleUploadFolder(cc *ClientConn, t *Transaction) (res []Transaction) {
	var fp FilePath
	if t.GetField(FieldFilePath).Data != nil {
		if _, err := fp.Write(t.GetField(FieldFilePath).Data); err != nil {
			return res
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload of the folder \"%v\" because you are only allowed to upload to the \"Uploads\" folder.", string(t.GetField(FieldFileName).Data)))
		}
	}

	fileTransfer := cc.newFileTransfer(FolderUpload,
		t.GetField(FieldFileName).Data,
		t.GetField(FieldFilePath).Data,
		t.GetField(FieldTransferSize).Data,
	)

	fileTransfer.FolderItemCount = t.GetField(FieldFolderItemCount).Data

	return append(res, cc.NewReply(t, NewField(FieldRefNum, fileTransfer.refNum[:])))
}

// HandleUploadFile
// Fields used in the request:
// 201	File Name
// 202	File path
// 204	File transfer options	"Optional
// Used only to resume download, currently has value 2"
// 108	File transfer size	"Optional used if download is not resumed"
func HandleUploadFile(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessUploadFile) {
		return cc.NewErrReply(t, "You are not allowed to upload files.")
	}

	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data
	transferOptions := t.GetField(FieldFileTransferOptions).Data
	transferSize := t.GetField(FieldTransferSize).Data // not sent for resume

	var fp FilePath
	if filePath != nil {
		if _, err := fp.Write(filePath); err != nil {
			return res
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload of the file \"%v\" because you are only allowed to upload to the \"Uploads\" folder.", string(fileName)))
		}
	}
	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	if _, err := cc.Server.FS.Stat(fullFilePath); err == nil {
		return cc.NewErrReply(t, fmt.Sprintf("Cannot accept upload because there is already a file named \"%v\".  Try choosing a different Name.", string(fileName)))
	}

	ft := cc.newFileTransfer(FileUpload, fileName, filePath, transferSize)

	replyT := cc.NewReply(t, NewField(FieldRefNum, ft.refNum[:]))

	// client has requested to resume a partially transferred file
	if transferOptions != nil {
		fileInfo, err := cc.Server.FS.Stat(fullFilePath + incompleteFileSuffix)
		if err != nil {
			return res
		}

		offset := make([]byte, 4)
		binary.BigEndian.PutUint32(offset, uint32(fileInfo.Size()))

		fileResumeData := NewFileResumeData([]ForkInfoList{
			*NewForkInfoList(offset),
		})

		b, _ := fileResumeData.BinaryMarshal()

		ft.TransferSize = offset

		replyT.Fields = append(replyT.Fields, NewField(FieldFileResumeData, b))
	}

	res = append(res, replyT)
	return res
}

func HandleSetClientUserInfo(cc *ClientConn, t *Transaction) (res []Transaction) {
	if len(t.GetField(FieldUserIconID).Data) == 4 {
		cc.Icon = t.GetField(FieldUserIconID).Data[2:]
	} else {
		cc.Icon = t.GetField(FieldUserIconID).Data
	}
	if cc.Authorize(AccessAnyName) {
		cc.UserName = t.GetField(FieldUserName).Data
	}

	// the options field is only passed by the client versions > 1.2.3.
	options := t.GetField(FieldOptions).Data
	if options != nil {
		optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

		//flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(cc.Flags[:])))
		//flagBitmap.SetBit(flagBitmap, UserFlagRefusePM, optBitmap.Bit(UserOptRefusePM))
		//binary.BigEndian.PutUint16(cc.Flags[:], uint16(flagBitmap.Int64()))

		cc.Flags.Set(UserFlagRefusePM, optBitmap.Bit(UserOptRefusePM))
		cc.Flags.Set(UserFlagRefusePChat, optBitmap.Bit(UserOptRefuseChat))
		//
		//flagBitmap.SetBit(flagBitmap, UserFlagRefusePChat, optBitmap.Bit(UserOptRefuseChat))
		//binary.BigEndian.PutUint16(cc.Flags[:], uint16(flagBitmap.Int64()))

		// Check auto response
		if optBitmap.Bit(UserOptAutoResponse) == 1 {
			cc.AutoReply = t.GetField(FieldAutomaticResponse).Data
		} else {
			cc.AutoReply = []byte{}
		}
	}

	for _, c := range cc.Server.ClientMgr.List() {
		res = append(res, NewTransaction(
			TranNotifyChangeUser,
			c.ID,
			NewField(FieldUserID, cc.ID[:]),
			NewField(FieldUserIconID, cc.Icon),
			NewField(FieldUserFlags, cc.Flags[:]),
			NewField(FieldUserName, cc.UserName),
		))
	}

	return res
}

// HandleKeepAlive responds to keepalive transactions with an empty reply
// * HL 1.9.2 Client sends keepalive msg every 3 minutes
// * HL 1.2.3 Client doesn't send keepalives
func HandleKeepAlive(cc *ClientConn, t *Transaction) (res []Transaction) {
	res = append(res, cc.NewReply(t))

	return res
}

func HandleGetFileNameList(cc *ClientConn, t *Transaction) (res []Transaction) {
	fullPath, err := readPath(
		cc.Server.Config.FileRoot,
		t.GetField(FieldFilePath).Data,
		nil,
	)
	if err != nil {
		return res
	}

	var fp FilePath
	if t.GetField(FieldFilePath).Data != nil {
		if _, err = fp.Write(t.GetField(FieldFilePath).Data); err != nil {
			return res
		}
	}

	// Handle special case for drop box folders
	if fp.IsDropbox() && !cc.Authorize(AccessViewDropBoxes) {
		return cc.NewErrReply(t, "You are not allowed to view drop boxes.")
	}

	fileNames, err := getFileNameList(fullPath, cc.Server.Config.IgnoreFiles)
	if err != nil {
		return res
	}

	res = append(res, cc.NewReply(t, fileNames...))

	return res
}

// =================================
//     Hotline private chat flow
// =================================
// 1. ClientA sends TranInviteNewChat to server with user Type to invite
// 2. Server creates new ChatID
// 3. Server sends TranInviteToChat to invitee
// 4. Server replies to ClientA with new Chat Type
//
// A dialog box pops up in the invitee client with options to accept or decline the invitation.
// If Accepted is clicked:
// 1. ClientB sends TranJoinChat with FieldChatID

// HandleInviteNewChat invites users to new private chat
func HandleInviteNewChat(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessOpenChat) {
		return cc.NewErrReply(t, "You are not allowed to request private chat.")
	}

	// Client to Invite
	targetID := t.GetField(FieldUserID).Data

	// Create a new chat with self as initial member.
	newChatID := cc.Server.ChatMgr.New(cc)

	// Check if target user has "Refuse private chat" flag
	targetClient := cc.Server.ClientMgr.Get([2]byte(targetID))
	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(targetClient.Flags[:])))
	if flagBitmap.Bit(UserFlagRefusePChat) == 1 {
		res = append(res,
			NewTransaction(
				TranServerMsg,
				cc.ID,
				NewField(FieldData, []byte(string(targetClient.UserName)+" does not accept private chats.")),
				NewField(FieldUserName, targetClient.UserName),
				NewField(FieldUserID, targetClient.ID[:]),
				NewField(FieldOptions, []byte{0, 2}),
			),
		)
	} else {
		res = append(res,
			NewTransaction(
				TranInviteToChat,
				[2]byte(targetID),
				NewField(FieldChatID, newChatID[:]),
				NewField(FieldUserName, cc.UserName),
				NewField(FieldUserID, cc.ID[:]),
			),
		)
	}

	return append(
		res,
		cc.NewReply(t,
			NewField(FieldChatID, newChatID[:]),
			NewField(FieldUserName, cc.UserName),
			NewField(FieldUserID, cc.ID[:]),
			NewField(FieldUserIconID, cc.Icon),
			NewField(FieldUserFlags, cc.Flags[:]),
		),
	)
}

func HandleInviteToChat(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessOpenChat) {
		return cc.NewErrReply(t, "You are not allowed to request private chat.")
	}

	// Client to Invite
	targetID := t.GetField(FieldUserID).Data
	chatID := t.GetField(FieldChatID).Data

	return []Transaction{
		NewTransaction(
			TranInviteToChat,
			[2]byte(targetID),
			NewField(FieldChatID, chatID),
			NewField(FieldUserName, cc.UserName),
			NewField(FieldUserID, cc.ID[:]),
		),
		cc.NewReply(
			t,
			NewField(FieldChatID, chatID),
			NewField(FieldUserName, cc.UserName),
			NewField(FieldUserID, cc.ID[:]),
			NewField(FieldUserIconID, cc.Icon),
			NewField(FieldUserFlags, cc.Flags[:]),
		),
	}
}

func HandleRejectChatInvite(cc *ClientConn, t *Transaction) (res []Transaction) {
	chatID := [4]byte(t.GetField(FieldChatID).Data)

	for _, c := range cc.Server.ChatMgr.Members(chatID) {
		res = append(res,
			NewTransaction(
				TranChatMsg,
				c.ID,
				NewField(FieldChatID, chatID[:]),
				NewField(FieldData, append(cc.UserName, []byte(" declined invitation to chat")...)),
			),
		)
	}

	return res
}

// HandleJoinChat is sent from a v1.8+ Hotline client when the joins a private chat
// Fields used in the reply:
// * 115	Chat subject
// * 300	User Name with info (Optional)
// * 300 	(more user names with info)
func HandleJoinChat(cc *ClientConn, t *Transaction) (res []Transaction) {
	chatID := t.GetField(FieldChatID).Data

	// Send TranNotifyChatChangeUser to current members of the chat to inform of new user
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		res = append(res,
			NewTransaction(
				TranNotifyChatChangeUser,
				c.ID,
				NewField(FieldChatID, chatID),
				NewField(FieldUserName, cc.UserName),
				NewField(FieldUserID, cc.ID[:]),
				NewField(FieldUserIconID, cc.Icon),
				NewField(FieldUserFlags, cc.Flags[:]),
			),
		)
	}

	cc.Server.ChatMgr.Join(ChatID(chatID), cc)

	subject := cc.Server.ChatMgr.GetSubject(ChatID(chatID))

	replyFields := []Field{NewField(FieldChatSubject, []byte(subject))}
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		b, err := io.ReadAll(&User{
			ID:    c.ID,
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			return res
		}
		replyFields = append(replyFields, NewField(FieldUsernameWithInfo, b))
	}

	return append(res, cc.NewReply(t, replyFields...))
}

// HandleLeaveChat is sent from a v1.8+ Hotline client when the user exits a private chat
// Fields used in the request:
//   - 114	FieldChatID
//
// Reply is not expected.
func HandleLeaveChat(cc *ClientConn, t *Transaction) (res []Transaction) {
	chatID := t.GetField(FieldChatID).Data

	cc.Server.ChatMgr.Leave([4]byte(chatID), cc.ID)

	// Notify members of the private chat that the user has left
	for _, c := range cc.Server.ChatMgr.Members(ChatID(chatID)) {
		res = append(res,
			NewTransaction(
				TranNotifyChatDeleteUser,
				c.ID,
				NewField(FieldChatID, chatID),
				NewField(FieldUserID, cc.ID[:]),
			),
		)
	}

	return res
}

// HandleSetChatSubject is sent from a v1.8+ Hotline client when the user sets a private chat subject
// Fields used in the request:
// * 114	Chat Type
// * 115	Chat subject
// Reply is not expected.
func HandleSetChatSubject(cc *ClientConn, t *Transaction) (res []Transaction) {
	chatID := t.GetField(FieldChatID).Data

	cc.Server.ChatMgr.SetSubject([4]byte(chatID), string(t.GetField(FieldChatSubject).Data))

	// Notify chat members of new subject.
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		res = append(res,
			NewTransaction(
				TranNotifyChatSubject,
				c.ID,
				NewField(FieldChatID, chatID),
				NewField(FieldChatSubject, t.GetField(FieldChatSubject).Data),
			),
		)
	}

	return res
}

// HandleMakeAlias makes a file alias using the specified path.
// Fields used in the request:
// 201	File Name
// 202	File path
// 212	File new path	Destination path
//
// Fields used in the reply:
// None
func HandleMakeAlias(cc *ClientConn, t *Transaction) (res []Transaction) {
	if !cc.Authorize(AccessMakeAlias) {
		return cc.NewErrReply(t, "You are not allowed to make aliases.")
	}
	fileName := t.GetField(FieldFileName).Data
	filePath := t.GetField(FieldFilePath).Data
	fileNewPath := t.GetField(FieldFileNewPath).Data

	fullFilePath, err := readPath(cc.Server.Config.FileRoot, filePath, fileName)
	if err != nil {
		return res
	}

	fullNewFilePath, err := readPath(cc.Server.Config.FileRoot, fileNewPath, fileName)
	if err != nil {
		return res
	}

	cc.logger.Debug("Make alias", "src", fullFilePath, "dst", fullNewFilePath)

	if err := cc.Server.FS.Symlink(fullFilePath, fullNewFilePath); err != nil {
		return cc.NewErrReply(t, "Error creating alias")
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleDownloadBanner handles requests for a new banner from the server
// Fields used in the request:
// None
// Fields used in the reply:
// 107	FieldRefNum			Used later for transfer
// 108	FieldTransferSize	Size of data to be downloaded
func HandleDownloadBanner(cc *ClientConn, t *Transaction) (res []Transaction) {
	ft := cc.newFileTransfer(BannerDownload, []byte{}, []byte{}, make([]byte, 4))
	binary.BigEndian.PutUint32(ft.TransferSize, uint32(len(cc.Server.banner)))

	return append(res, cc.NewReply(t,
		NewField(FieldRefNum, ft.refNum[:]),
		NewField(FieldTransferSize, ft.TransferSize),
	))
}
