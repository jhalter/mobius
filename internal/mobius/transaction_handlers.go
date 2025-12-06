package mobius

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jhalter/mobius/hotline"
	"golang.org/x/text/encoding/charmap"
)

// Public error message constants for reuse by other packages
const (
	// Authorization error messages
	ErrMsgNotAllowedParticipateChat      = "You are not allowed to participate in chat."
	ErrMsgNotAllowedSendPrivateMsg       = "You are not allowed to send private messages."
	ErrMsgNotAllowedReadNews             = "You are not allowed to read news."
	ErrMsgNotAllowedPostNews             = "You are not allowed to post news."
	ErrMsgNotAllowedCreateAccounts       = "You are not allowed to create new accounts."
	ErrMsgNotAllowedViewAccounts         = "You are not allowed to view accounts."
	ErrMsgNotAllowedModifyAccounts       = "You are not allowed to modify accounts."
	ErrMsgNotAllowedDeleteAccounts       = "You are not allowed to delete accounts."
	ErrMsgNotAllowedRequestPrivateChat   = "You are not allowed to request private chat."
	ErrMsgNotAllowedCreateNewsCategories = "You are not allowed to create news categories."
	ErrMsgNotAllowedDeleteNewsArticles   = "You are not allowed to delete news articles."
	ErrMsgNotAllowedSetCommentsFiles     = "You are not allowed to set comments for files."
	ErrMsgNotAllowedSetCommentsFolders   = "You are not allowed to set comments for folders."
	ErrMsgNotAllowedRenameFiles          = "You are not allowed to rename files."
	ErrMsgNotAllowedRenameFolders        = "You are not allowed to rename folders."
	ErrMsgNotAllowedDeleteFiles          = "You are not allowed to delete files."
	ErrMsgNotAllowedDeleteFolders        = "You are not allowed to delete folders."
	ErrMsgNotAllowedMoveFiles            = "You are not allowed to move files."
	ErrMsgNotAllowedMoveFolders          = "You are not allowed to move folders."
	ErrMsgNotAllowedCreateFolders        = "You are not allowed to create folders."
	ErrMsgNotAllowedSendBroadcast        = "You are not allowed to send broadcast messages."
	ErrMsgNotAllowedGetClientInfo        = "You are not allowed to get client info."
	ErrMsgNotAllowedDisconnectUsers      = "You are not allowed to disconnect users."
	ErrMsgNotAllowedCreateNewsfolders    = "You are not allowed to create news folders."
	ErrMsgNotAllowedDeleteNewsCategories = "You are not allowed to delete news categories."
	ErrMsgNotAllowedDeleteNewsFolders    = "You are not allowed to delete news folders."
	ErrMsgNotAllowedPostNewsArticles     = "You are not allowed to post news articles."
	ErrMsgNotAllowedDownloadFiles        = "You are not allowed to download files."
	ErrMsgNotAllowedDownloadFolders      = "You are not allowed to download folders."
	ErrMsgNotAllowedUploadFiles          = "You are not allowed to upload files."
	ErrMsgNotAllowedUploadFolders        = "You are not allowed to upload folders."
	ErrMsgNotAllowedViewDropBoxes        = "You are not allowed to view drop boxes."
	ErrMsgNotAllowedMakeAliases          = "You are not allowed to make aliases."

	// Account error messages
	ErrMsgAccountDeleted    = "You are logged in with an account which was deleted."
	ErrMsgAccountExists     = "Cannot create account because there is already an account with that login."
	ErrMsgAccountMoreAccess = "Cannot create account with more access than yourself."
	ErrMsgAccountNotExist   = "Account does not exist."

	// Account error templates (for dynamic content)
	ErrMsgAccountExistsTemplate = "Cannot create account %s because there is already an account with that login."

	// File operation error templates
	ErrMsgCannotRenameFileNotFound   = "Cannot rename file %s because it does not exist or cannot be found."
	ErrMsgCannotRenameFolderNotFound = "Cannot rename folder %s because it does not exist or cannot be found."
	ErrMsgCannotDeleteFileNotFound   = "Cannot delete file %s because it does not exist or cannot be found."

	// File operation error templates (for dynamic content)
	ErrMsgFolderCreateConflictTemplate = "Cannot create folder \"%s\" because there is already a file or folder with that Name."
	ErrMsgFolderCreateErrorTemplate    = "Cannot create folder \"%s\" because an error occurred."

	// Upload restriction templates (these need dynamic content)
	ErrMsgUploadRestrictedTemplate   = "Cannot accept upload of the %s \"%v\" because you are only allowed to upload to the \"Uploads\" folder."
	ErrMsgFileUploadConflictTemplate = "Cannot accept upload because there is already a file named \"%v\". Try choosing a different Name."

	// Chat/messaging templates (these need dynamic content)
	ErrMsgDoesNotAcceptTemplate = "%s does not accept %s."

	// Ban messages
	ErrMsgTemporaryBan = "You are temporarily banned on this server"
	ErrMsgPermanentBan = "You are permanently banned on this server"

	// General error messages
	ErrMsgAccountNotFound = "Account not found."
	ErrMsgUserNotFound    = "User not found."
	ErrMsgCreateAlias     = "Error creating alias"
)

// Converts bytes from Mac Roman encoding to UTF-8
var txtDecoder = charmap.Macintosh.NewDecoder()

// Converts bytes from UTF-8 to Mac Roman encoding
var txtEncoder = charmap.Macintosh.NewEncoder()

// Assign functions to handle specific Hotline transaction types
func RegisterHandlers(srv *hotline.Server) {
	srv.HandleFunc(hotline.TranAgreed, HandleTranAgreed)
	srv.HandleFunc(hotline.TranChatSend, HandleChatSend)
	srv.HandleFunc(hotline.TranDelNewsArt, HandleDelNewsArt)
	srv.HandleFunc(hotline.TranDelNewsItem, HandleDelNewsItem)
	srv.HandleFunc(hotline.TranDeleteFile, HandleDeleteFile)
	srv.HandleFunc(hotline.TranDeleteUser, HandleDeleteUser)
	srv.HandleFunc(hotline.TranDisconnectUser, HandleDisconnectUser)
	srv.HandleFunc(hotline.TranDownloadFile, HandleDownloadFile)
	srv.HandleFunc(hotline.TranDownloadFldr, HandleDownloadFolder)
	srv.HandleFunc(hotline.TranGetClientInfoText, HandleGetClientInfoText)
	srv.HandleFunc(hotline.TranGetFileInfo, HandleGetFileInfo)
	srv.HandleFunc(hotline.TranGetFileNameList, HandleGetFileNameList)
	srv.HandleFunc(hotline.TranGetMsgs, HandleGetMsgs)
	srv.HandleFunc(hotline.TranGetNewsArtData, HandleGetNewsArtData)
	srv.HandleFunc(hotline.TranGetNewsArtNameList, HandleGetNewsArtNameList)
	srv.HandleFunc(hotline.TranGetNewsCatNameList, HandleGetNewsCatNameList)
	srv.HandleFunc(hotline.TranGetUser, HandleGetUser)
	srv.HandleFunc(hotline.TranGetUserNameList, HandleGetUserNameList)
	srv.HandleFunc(hotline.TranInviteNewChat, HandleInviteNewChat)
	srv.HandleFunc(hotline.TranInviteToChat, HandleInviteToChat)
	srv.HandleFunc(hotline.TranJoinChat, HandleJoinChat)
	srv.HandleFunc(hotline.TranKeepAlive, HandleKeepAlive)
	srv.HandleFunc(hotline.TranLeaveChat, HandleLeaveChat)
	srv.HandleFunc(hotline.TranListUsers, HandleListUsers)
	srv.HandleFunc(hotline.TranMoveFile, HandleMoveFile)
	srv.HandleFunc(hotline.TranNewFolder, HandleNewFolder)
	srv.HandleFunc(hotline.TranNewNewsCat, HandleNewNewsCat)
	srv.HandleFunc(hotline.TranNewNewsFldr, HandleNewNewsFldr)
	srv.HandleFunc(hotline.TranNewUser, HandleNewUser)
	srv.HandleFunc(hotline.TranUpdateUser, HandleUpdateUser)
	srv.HandleFunc(hotline.TranOldPostNews, HandleTranOldPostNews)
	srv.HandleFunc(hotline.TranPostNewsArt, HandlePostNewsArt)
	srv.HandleFunc(hotline.TranRejectChatInvite, HandleRejectChatInvite)
	srv.HandleFunc(hotline.TranSendInstantMsg, HandleSendInstantMsg)
	srv.HandleFunc(hotline.TranSetChatSubject, HandleSetChatSubject)
	srv.HandleFunc(hotline.TranMakeFileAlias, HandleMakeAlias)
	srv.HandleFunc(hotline.TranSetClientUserInfo, HandleSetClientUserInfo)
	srv.HandleFunc(hotline.TranSetFileInfo, HandleSetFileInfo)
	srv.HandleFunc(hotline.TranSetUser, HandleSetUser)
	srv.HandleFunc(hotline.TranUploadFile, HandleUploadFile)
	srv.HandleFunc(hotline.TranUploadFldr, HandleUploadFolder)
	srv.HandleFunc(hotline.TranUserBroadcast, HandleUserBroadcast)
	srv.HandleFunc(hotline.TranDownloadBanner, HandleDownloadBanner)
}

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

		// send the message to all connected clients of the private chat
		for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
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

	reply := hotline.NewTransaction(
		hotline.TranServerMsg,
		[2]byte(userID.Data),
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

	otherClient := cc.Server.ClientMgr.Get([2]byte(userID.Data))
	if otherClient == nil {
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

var fileTypeFLDR = [4]byte{0x66, 0x6c, 0x64, 0x72}

// HandleGetFileInfo requests file information from the server.
//
// Fields used in the request:
//   - 201 File name   Required
//   - 202 File path   Optional
//
// Fields used in the reply:
//   - 201 File name           File name
//   - 205 File type string    Friendly file type description
//   - 206 File creator string Friendly creator description
//   - 210 File comment        Comment string
//   - 213 File type           File type signature
//   - 208 File create date    Creation date
//   - 209 File modify date    Modification date
//   - 207 File size           File size
func HandleGetFileInfo(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	fw, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}

	encodedName, err := txtEncoder.String(fw.Name)
	if err != nil {
		return res
	}

	fields := []hotline.Field{
		hotline.NewField(hotline.FieldFileName, []byte(encodedName)),
		hotline.NewField(hotline.FieldFileTypeString, fw.Ffo.FlatFileInformationFork.FriendlyType()),
		hotline.NewField(hotline.FieldFileCreatorString, fw.Ffo.FlatFileInformationFork.FriendlyCreator()),
		hotline.NewField(hotline.FieldFileType, fw.Ffo.FlatFileInformationFork.TypeSignature[:]),
		hotline.NewField(hotline.FieldFileCreateDate, fw.Ffo.FlatFileInformationFork.CreateDate[:]),
		hotline.NewField(hotline.FieldFileModifyDate, fw.Ffo.FlatFileInformationFork.ModifyDate[:]),
	}
 
	// Include the optional FileComment field if there is a comment.
	if len(fw.Ffo.FlatFileInformationFork.Comment) != 0 {
		fields = append(fields, hotline.NewField(hotline.FieldFileComment, fw.Ffo.FlatFileInformationFork.Comment))
	}

	// Include the FileSize field for files.
	if fw.Ffo.FlatFileInformationFork.TypeSignature != fileTypeFLDR {
		fields = append(fields, hotline.NewField(hotline.FieldFileSize, fw.TotalSize()))
	}

	return append(res, cc.NewReply(t, fields...))
}

// HandleSetFileInfo sets information for the specified file on the server.
//
// Access: Set File Comment (28) or Set Folder Comment (29)
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Optional
//   - 211 File new name  Optional
//   - 210 File comment   Optional
//
// Fields used in the reply: None
func HandleSetFileInfo(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	fi, err := cc.Server.FS.Stat(fullFilePath)
	if err != nil {
		return res
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}
	if t.GetField(hotline.FieldFileComment).Data != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(hotline.AccessSetFolderComment) {
				return cc.NewErrReply(t, ErrMsgNotAllowedSetCommentsFolders)
			}
		case mode.IsRegular():
			if !cc.Authorize(hotline.AccessSetFileComment) {
				return cc.NewErrReply(t, ErrMsgNotAllowedSetCommentsFiles)
			}
		}

		if err := hlFile.Ffo.FlatFileInformationFork.SetComment(t.GetField(hotline.FieldFileComment).Data); err != nil {
			return res
		}
		w, err := hlFile.InfoForkWriter()
		if err != nil {
			return res
		}
		_, err = io.Copy(w, &hlFile.Ffo.FlatFileInformationFork)
		if err != nil {
			return res
		}
	}

	fullNewFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, t.GetField(hotline.FieldFileNewName).Data)
	if err != nil {
		return nil
	}

	fileNewName := t.GetField(hotline.FieldFileNewName).Data

	if fileNewName != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(hotline.AccessRenameFolder) {
				return cc.NewErrReply(t, ErrMsgNotAllowedRenameFolders)
			}
			err = os.Rename(fullFilePath, fullNewFilePath)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotRenameFolderNotFound, string(fileName)))

			}
		case mode.IsRegular():
			if !cc.Authorize(hotline.AccessRenameFile) {
				return cc.NewErrReply(t, ErrMsgNotAllowedRenameFiles)
			}
			fileDir, err := hotline.ReadPath(cc.FileRoot(), filePath, []byte{})
			if err != nil {
				return nil
			}
			hlFile.Name, err = txtDecoder.String(string(fileNewName))
			if err != nil {
				return res
			}

			err = hlFile.Move(fileDir)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotRenameFileNotFound, string(fileName)))
			}
			if err != nil {
				return res
			}
		}
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleDeleteFile deletes the specified file from the server.
//
// Access: Delete File (0) or Delete Folder (6)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Required
//
// Fields used in the reply: None
func HandleDeleteFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		return res
	}

	fi, err := hlFile.DataFile()
	if err != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotDeleteFileNotFound, string(fileName)))
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(hotline.AccessDeleteFolder) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteFolders)
		}
	case mode.IsRegular():
		if !cc.Authorize(hotline.AccessDeleteFile) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteFiles)
		}
	}

	if err := hlFile.Delete(); err != nil {
		return res
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleMoveFile moves a file from one folder to another on the same server.
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Required
//   - 212 File new path  Required - Destination path
//
// Fields used in the reply: None
func HandleMoveFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := string(t.GetField(hotline.FieldFileName).Data)

	filePath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFilePath).Data, t.GetField(hotline.FieldFileName).Data)
	if err != nil {
		return res
	}

	fileNewPath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFileNewPath).Data, nil)
	if err != nil {
		return res
	}

	cc.Logger.Info("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	hlFile, err := hotline.NewFile(cc.Server.FS, filePath, 0)
	if err != nil {
		return res
	}

	fi, err := hlFile.DataFile()
	if err != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotDeleteFileNotFound, fileName))
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(hotline.AccessMoveFolder) {
			return cc.NewErrReply(t, ErrMsgNotAllowedMoveFolders)
		}
	case mode.IsRegular():
		if !cc.Authorize(hotline.AccessMoveFile) {
			return cc.NewErrReply(t, ErrMsgNotAllowedMoveFiles)
		}
	}
	if err := hlFile.Move(fileNewPath); err != nil {
		return res
	}
	// TODO: handle other possible errors; e.g. file delete fails due to permission issue

	res = append(res, cc.NewReply(t))
	return res
}

// HandleNewFolder creates a new folder on the server.
//
// Access: Create Folder (5)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Optional
//
// Fields used in the reply: None
func HandleNewFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessCreateFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateFolders)
	}
	folderName := string(t.GetField(hotline.FieldFileName).Data)

	folderName = path.Join("/", folderName)

	var subPath string

	// FieldFilePath is only present for nested paths
	if t.GetField(hotline.FieldFilePath).Data != nil {
		var newFp hotline.FilePath
		_, err := newFp.Write(t.GetField(hotline.FieldFilePath).Data)
		if err != nil {
			return res
		}

		for _, pathItem := range newFp.Items {
			subPath = path.Join("/", subPath, string(pathItem.Name))
		}
	}
	newFolderPath := path.Join(cc.FileRoot(), subPath, folderName)
	newFolderPath, err := txtDecoder.String(newFolderPath)
	if err != nil {
		return res
	}

	// TODO: check path and folder Name lengths

	if _, err := cc.Server.FS.Stat(newFolderPath); !os.IsNotExist(err) {
		msg := fmt.Sprintf(ErrMsgFolderCreateConflictTemplate, folderName)
		return cc.NewErrReply(t, msg)
	}

	if err := cc.Server.FS.Mkdir(newFolderPath, 0777); err != nil {
		msg := fmt.Sprintf(ErrMsgFolderCreateErrorTemplate, folderName)
		return cc.NewErrReply(t, msg)
	}

	return append(res, cc.NewReply(t))
}

// HandleSetUser sets the information for a specific user in the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login     Required
//   - 106 User password  Optional
//   - 102 User name      Required
//   - 110 User access    Required - User access privileges bitmap
//
// Fields used in the reply: None
func HandleSetUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessModifyUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedModifyAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()
	userName := string(t.GetField(hotline.FieldUserName).Data)

	newAccessLvl := t.GetField(hotline.FieldUserAccess).Data

	account := cc.Server.AccountManager.Get(login)
	if account == nil {
		return cc.NewErrReply(t, ErrMsgAccountNotFound)
	}
	account.Name = userName
	copy(account.Access[:], newAccessLvl)

	// If the password field is cleared in the Hotline edit user UI, the SetUser transaction does
	// not include FieldUserPassword
	if t.GetField(hotline.FieldUserPassword).Data == nil {
		account.Password = hotline.HashAndSalt([]byte(""))
	}

	if !bytes.Equal([]byte{0}, t.GetField(hotline.FieldUserPassword).Data) {
		account.Password = hotline.HashAndSalt(t.GetField(hotline.FieldUserPassword).Data)
	}

	err := cc.Server.AccountManager.Update(*account, account.Login)
	if err != nil {
		cc.Logger.Error("Error updating account", "Err", err)
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.ClientMgr.List() {
		if c.Account.Login == login {
			newT := hotline.NewTransaction(hotline.TranUserAccess, c.ID, hotline.NewField(hotline.FieldUserAccess, newAccessLvl))
			res = append(res, newT)

			if c.Authorize(hotline.AccessDisconUser) {
				c.Flags.Set(hotline.UserFlagAdmin, 1)
			} else {
				c.Flags.Set(hotline.UserFlagAdmin, 0)
			}

			c.Account.Access = account.Access

			cc.SendAll(
				hotline.TranNotifyChangeUser,
				hotline.NewField(hotline.FieldUserID, c.ID[:]),
				hotline.NewField(hotline.FieldUserFlags, c.Flags[:]),
				hotline.NewField(hotline.FieldUserName, c.UserName),
				hotline.NewField(hotline.FieldUserIconID, c.Icon),
			)
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleGetUser requests the information for a specific user from the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login  Required
//
// Fields used in the reply:
//   - 102 User name      Account display name
//   - 105 User login     Account login (encoded, each character negated)
//   - 106 User password  Account password
//   - 110 User access    User access privileges bitmap
func HandleGetUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewAccounts)
	}

	account := cc.Server.AccountManager.Get(string(t.GetField(hotline.FieldUserLogin).Data))
	if account == nil {
		return cc.NewErrReply(t, ErrMsgAccountNotExist)
	}

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldUserName, []byte(account.Name)),
		hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString(t.GetField(hotline.FieldUserLogin).Data)),
		hotline.NewField(hotline.FieldUserPassword, []byte(account.Password)),
		hotline.NewField(hotline.FieldUserAccess, account.Access[:]),
	))
}

// HandleListUsers returns a list of all user accounts on the server.
// This is a server-specific transaction not in the original Hotline protocol.
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 101 Data  Repeated - Serialized account data for each user
func HandleListUsers(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewAccounts)
	}

	var userFields []hotline.Field
	for _, acc := range cc.Server.AccountManager.List() {
		b, err := io.ReadAll(&acc)
		if err != nil {
			cc.Logger.Error("Error reading account", "Account", acc.Login, "Err", err)
			continue
		}

		userFields = append(userFields, hotline.NewField(hotline.FieldData, b))
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
// HandleUpdateUser processes batch user account operations from the v1.5+ multi-user editor.
// This handler supports creating, deleting, and modifying multiple user accounts in a single transaction.
//
// Fields used in the request:
// * 101	Data				Repeated - Each contains encoded sub-fields for one user operation
//
// Sub-fields for user operations:
// * 101	Data				Optional - Original login name (for rename operations)
// * 105	User Login			Required - Login name (new name for renames)
// * 102	User Name			Optional - Display name (for create/modify)
// * 106	User Password		Optional - Password (for create/modify)
// * 110	User Access			Optional - Access permissions (for create/modify)
//
// Fields used in the reply:
// None
func HandleUpdateUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	for _, field := range t.Fields {
		var subFields []hotline.Field

		// Create a new scanner for parsing incoming bytes into transaction tokens
		scanner := bufio.NewScanner(bytes.NewReader(field.Data[2:]))
		scanner.Split(hotline.FieldScanner)

		for i := 0; i < int(binary.BigEndian.Uint16(field.Data[0:2])); i++ {
			scanner.Scan()

			var field hotline.Field
			if _, err := field.Write(scanner.Bytes()); err != nil {
				return res
			}
			subFields = append(subFields, field)
		}

		// If there's only one subfield, that indicates this is a delete operation for the login in FieldData
		if len(subFields) == 1 {
			if !cc.Authorize(hotline.AccessDeleteUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedDeleteAccounts)
			}

			login := string(hotline.EncodeString(hotline.GetField(hotline.FieldData, &subFields).Data))

			cc.Logger.Info("DeleteUser", "login", login)

			if err := cc.Server.AccountManager.Delete(login); err != nil {
				cc.Logger.Error("Error deleting account", "Err", err)
				return res
			}

			for _, client := range cc.Server.ClientMgr.List() {
				if client.Account.Login == login {
					//					"You are logged in with an account which was deleted."

					res = append(res,
						hotline.NewTransaction(hotline.TranServerMsg, [2]byte{},
							hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
							hotline.NewField(hotline.FieldChatOptions, []byte{0}),
						),
					)

					go func(c *hotline.ClientConn) {
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
		if hotline.GetField(hotline.FieldData, &subFields) != nil {
			loginToRename = string(hotline.EncodeString(hotline.GetField(hotline.FieldData, &subFields).Data))
		}
		userLogin := string(hotline.EncodeString(hotline.GetField(hotline.FieldUserLogin, &subFields).Data))
		if loginToRename != "" {
			accountToUpdate = loginToRename
		} else {
			accountToUpdate = userLogin
		}

		// Check if accountToUpdate has an existing account.  If so, we know we are updating an existing user.
		if acc := cc.Server.AccountManager.Get(accountToUpdate); acc != nil {
			if loginToRename != "" {
				cc.Logger.Info("RenameUser", "prevLogin", accountToUpdate, "newLogin", userLogin)
			} else {
				cc.Logger.Info("UpdateUser", "login", accountToUpdate)
			}

			// Account exists, so this is an update action.
			if !cc.Authorize(hotline.AccessModifyUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedModifyAccounts)
			}

			// This part is a bit tricky. There are three possibilities:
			// 1) The transaction is intended to update the password.
			//	  In this case, FieldUserPassword is sent with the new password.
			// 2) The transaction is intended to remove the password.
			//    In this case, FieldUserPassword is not sent.
			// 3) The transaction updates the users access bits, but not the password.
			//    In this case, FieldUserPassword is sent with zero as the only byte.
			if hotline.GetField(hotline.FieldUserPassword, &subFields) != nil {
				newPass := hotline.GetField(hotline.FieldUserPassword, &subFields).Data
				if !bytes.Equal([]byte{0}, newPass) {
					acc.Password = hotline.HashAndSalt(newPass)
				}
			} else {
				acc.Password = hotline.HashAndSalt([]byte(""))
			}

			if hotline.GetField(hotline.FieldUserAccess, &subFields) != nil {
				copy(acc.Access[:], hotline.GetField(hotline.FieldUserAccess, &subFields).Data)
			}

			acc.Name = string(hotline.GetField(hotline.FieldUserName, &subFields).Data)

			err := cc.Server.AccountManager.Update(*acc, string(hotline.EncodeString(hotline.GetField(hotline.FieldUserLogin, &subFields).Data)))

			if err != nil {
				return res
			}
		} else {
			if !cc.Authorize(hotline.AccessCreateUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedCreateAccounts)
			}

			cc.Logger.Info("CreateUser", "login", userLogin)

			var newAccess hotline.AccessBitmap
			copy(newAccess[:], hotline.GetField(hotline.FieldUserAccess, &subFields).Data)

			// Prevent account from creating new account with greater permission
			for i := 0; i < 64; i++ {
				if newAccess.IsSet(i) {
					if !cc.Authorize(i) {
						return cc.NewErrReply(t, "Cannot create account with more access than yourself.")
					}
				}
			}

			account := hotline.NewAccount(
				userLogin,
				string(hotline.GetField(hotline.FieldUserName, &subFields).Data),
				string(hotline.GetField(hotline.FieldUserPassword, &subFields).Data),
				newAccess,
			)

			err := cc.Server.AccountManager.Create(*account)
			if err != nil {
				return cc.NewErrReply(t, ErrMsgAccountExists)
			}
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleNewUser adds a new user to the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login     Required
//   - 106 User password  Required
//   - 102 User name      Required
//   - 110 User access    Required - User access privileges bitmap
//
// Fields used in the reply: None
func HandleNewUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessCreateUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()

	// If the account already exists, reply with an error.
	if account := cc.Server.AccountManager.Get(login); account != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgAccountExistsTemplate, login))
	}

	var newAccess hotline.AccessBitmap
	copy(newAccess[:], t.GetField(hotline.FieldUserAccess).Data)

	// Prevent account from creating new account with greater permission
	for i := 0; i < 64; i++ {
		if newAccess.IsSet(i) {
			if !cc.Authorize(i) {
				return cc.NewErrReply(t, ErrMsgAccountMoreAccess)
			}
		}
	}

	account := hotline.NewAccount(login, string(t.GetField(hotline.FieldUserName).Data), string(t.GetField(hotline.FieldUserPassword).Data), newAccess)

	err := cc.Server.AccountManager.Create(*account)
	if err != nil {
		return cc.NewErrReply(t, ErrMsgAccountExists)
	}

	return append(res, cc.NewReply(t))
}

// HandleDeleteUser deletes the specified user from the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login  Required
//
// Fields used in the reply: None
func HandleDeleteUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDeleteUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDeleteAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()

	if err := cc.Server.AccountManager.Delete(login); err != nil {
		cc.Logger.Error("Error deleting account", "Err", err)
		return res
	}

	for _, client := range cc.Server.ClientMgr.List() {
		if client.Account.Login == login {
			res = append(res,
				hotline.NewTransaction(hotline.TranServerMsg, client.ID,
					hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
					hotline.NewField(hotline.FieldChatOptions, []byte{2}),
				),
			)

			go func(c *hotline.ClientConn) {
				time.Sleep(2 * time.Second)
				c.Disconnect()
			}(client)
		}
	}

	return append(res, cc.NewReply(t))
}

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

	clientID := t.GetField(hotline.FieldUserID).Data

	clientConn := cc.Server.ClientMgr.Get(hotline.ClientID(clientID))
	if clientConn == nil {
		return cc.NewErrReply(t, ErrMsgUserNotFound)
	}

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldData, []byte(clientConn.String())),
		hotline.NewField(hotline.FieldUserName, clientConn.UserName),
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
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			return nil
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
			cc.UserName = t.GetField(hotline.FieldUserName).Data
		} else {
			cc.UserName = []byte(cc.Account.Name)
		}
	}

	if cc.Server.Redis != nil {
		login := cc.Account.Login
		ip, _, _ := net.SplitHostPort(cc.RemoteAddr)
		// Remove old entry (login::ip)
		cc.Server.Redis.SRem(context.Background(), "mobius:online", login+"::"+ip)
		// Add new entry with login, nickname, ip
		cc.Server.Redis.SAdd(context.Background(), "mobius:online", login+":"+string(cc.UserName)+":"+ip)
		// Ban check for nickname
		bannedNick, _ := cc.Server.Redis.SIsMember(context.Background(), "mobius:banned:nicknames", string(cc.UserName)).Result()
		if bannedNick {
			// Remove all possible online entries for this login and IP
			cc.Server.Redis.SRem(context.Background(), "mobius:online", login+"::"+ip)
			cc.Server.Redis.SRem(context.Background(), "mobius:online", login+":"+string(cc.UserName)+":"+ip)
			// If we track the previous nickname, remove that too:
			// cc.Server.Redis.SRem(context.Background(), "mobius:online", login+":"+oldNickname+":"+ip)
			cc.Server.Redis.SAdd(context.Background(), "mobius:banned:ips", ip)
			cc.Disconnect()
			return res
		}
	}

	cc.Icon = t.GetField(hotline.FieldUserIconID).Data

	cc.Logger = cc.Logger.With("Name", string(cc.UserName))
	cc.Logger.Info("Login successful")

	options := t.GetField(hotline.FieldOptions).Data
	optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

	// Check refuse private PM option

	cc.FlagsMU.Lock()
	defer cc.FlagsMU.Unlock()
	cc.Flags.Set(hotline.UserFlagRefusePM, optBitmap.Bit(hotline.UserOptRefusePM))

	// Check refuse private chat option
	cc.Flags.Set(hotline.UserFlagRefusePChat, optBitmap.Bit(hotline.UserOptRefuseChat))

	// Check auto response
	if optBitmap.Bit(hotline.UserOptAutoResponse) == 1 {
		cc.AutoReply = t.GetField(hotline.FieldAutomaticResponse).Data
	}

	trans := cc.NotifyOthers(
		hotline.NewTransaction(
			hotline.TranNotifyChangeUser, [2]byte{0, 0},
			hotline.NewField(hotline.FieldUserName, cc.UserName),
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.Icon),
			hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
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

// HandleTranOldPostNews posts to the flat news (message board).
//
// Access: News Post Article (21)
//
// Fields used in the request:
//   - 101 Data  Required - News post content
//
// Fields used in the reply: None
func HandleTranOldPostNews(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsPostArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedPostNews)
	}

	newsDateTemplate := hotline.NewsDateFormat
	if cc.Server.Config.NewsDateFormat != "" {
		newsDateTemplate = cc.Server.Config.NewsDateFormat
	}

	newsTemplate := hotline.NewsTemplate
	if cc.Server.Config.NewsDelimiter != "" {
		newsTemplate = cc.Server.Config.NewsDelimiter
	}

	newsPost := fmt.Sprintf(newsTemplate+"\r", cc.UserName, time.Now().Format(newsDateTemplate), t.GetField(hotline.FieldData).Data)
	newsPost = strings.ReplaceAll(newsPost, "\n", "\r")

	_, err := cc.Server.MessageBoard.Write([]byte(newsPost))
	if err != nil {
		cc.Logger.Error("error writing news post", "err", err)
		return nil
	}

	// Notify all clients of updated news
	cc.SendAll(
		hotline.TranNewMsg,
		hotline.NewField(hotline.FieldData, []byte(newsPost)),
	)

	return append(res, cc.NewReply(t))
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

	clientID := [2]byte(t.GetField(hotline.FieldUserID).Data)
	clientConn := cc.Server.ClientMgr.Get(clientID)

	if clientConn.Authorize(hotline.AccessCannotBeDiscon) {
		return cc.NewErrReply(t, clientConn.Account.Login+" is not allowed to be disconnected.")
	}

	// If FieldOptions is set, then the client IP is banned in addition to disconnected.
	// 00 01 = temporary ban
	// 00 02 = permanent ban
	if t.GetField(hotline.FieldOptions).Data != nil {
		switch t.GetField(hotline.FieldOptions).Data[1] {
		case 1:
			// send message: "You are temporarily banned on this server"
			cc.Logger.Info("Disconnect & temporarily ban " + string(clientConn.UserName))

			res = append(res, hotline.NewTransaction(
				hotline.TranServerMsg,
				clientConn.ID,
				hotline.NewField(hotline.FieldData, []byte(ErrMsgTemporaryBan)),
				hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
			))

			banUntil := time.Now().Add(hotline.BanDuration)
			ip, _, _ := net.SplitHostPort(clientConn.RemoteAddr)

			err := cc.Server.BanList.Add(ip, &banUntil)
			if err != nil {
				cc.Logger.Error("Error saving ban", "err", err)
				// TODO
			}
		case 2:
			// send message: "You are permanently banned on this server"
			cc.Logger.Info("Disconnect & ban " + string(clientConn.UserName))

			res = append(res, hotline.NewTransaction(
				hotline.TranServerMsg,
				clientConn.ID,
				hotline.NewField(hotline.FieldData, []byte(ErrMsgPermanentBan)),
				hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
			))

			ip, _, _ := net.SplitHostPort(clientConn.RemoteAddr)

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

// HandleGetNewsCatNameList gets the list of category names at the specified news path.
//
// Fields used in the request:
//   - 325 News path  Optional
//
// Fields used in the reply:
//   - 323 News category list data  Repeated - Category information
func HandleGetNewsCatNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("get news path", "err", err)
		return nil
	}

	var fields []hotline.Field
	for _, cat := range cc.Server.ThreadedNewsMgr.GetCategories(pathStrs) {
		b, err := io.ReadAll(&cat)
		if err != nil {
			cc.Logger.Error("get news categories", "err", err)
		}

		fields = append(fields, hotline.NewField(hotline.FieldNewsCatListData15, b))
	}

	return append(res, cc.NewReply(t, fields...))
}

// HandleNewNewsCat creates a new news category on the server.
//
// Access: News Create Category (34)
//
// Fields used in the request:
//   - 322 News category name  Required
//   - 325 News path           Required
//
// Fields used in the reply: None
func HandleNewNewsCat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsCreateCat) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateNewsCategories)
	}

	name := string(t.GetField(hotline.FieldNewsCatName).Data)
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, hotline.NewsCategory)
	if err != nil {
		cc.Logger.Error("error creating news category", "err", err)
	}

	return []hotline.Transaction{cc.NewReply(t)}
}

// HandleNewNewsFldr creates a new news folder on the server.
//
// Access: News Create Folder (36)
//
// Fields used in the request:
//   - 201 File name   Required
//   - 325 News path   Required
//
// Fields used in the reply: None
func HandleNewNewsFldr(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsCreateFldr) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateNewsfolders)
	}

	name := string(t.GetField(hotline.FieldFileName).Data)
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, hotline.NewsBundle)
	if err != nil {
		cc.Logger.Error("error creating news bundle", "err", err)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetNewsArtNameList gets the list of article names at the specified news path.
//
// Fields used in the request:
//   - 325 News path  Optional
//
// Fields used in the reply:
//   - 321 News article list data  Optional
func HandleGetNewsArtNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	nald, err := cc.Server.ThreadedNewsMgr.ListArticles(pathStrs)
	if err != nil {
		return res
	}

	b, err := io.ReadAll(&nald)
	if err != nil {
		return res
	}

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldNewsArtListData, b)))
}

// HandleGetNewsArtData requests information about a specific news article.
//
// Access: News Read Article (20)
//
// Fields used in the request:
//   - 325 News path                 Required
//   - 326 News article ID           Required
//   - 327 News article data flavor  Required
//
// Fields used in the reply:
//   - 328 News article title        Article title
//   - 329 News article poster       Author
//   - 330 News article date         Publication date
//   - 331 Previous article ID       ID of previous article
//   - 332 Next article ID           ID of next article
//   - 335 Parent article ID         ID of parent article
//   - 336 First child article ID    ID of first reply
//   - 327 News article data flavor  Should be "text/plain"
//   - 333 News article data         Optional - Article content
func HandleGetNewsArtData(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	newsPath, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	convertedID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		return res
	}

	art := cc.Server.ThreadedNewsMgr.GetArticle(newsPath, uint32(convertedID))
	if art == nil {
		return append(res, cc.NewReply(t))
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldNewsArtTitle, []byte(art.Title)),
		hotline.NewField(hotline.FieldNewsArtPoster, []byte(art.Poster)),
		hotline.NewField(hotline.FieldNewsArtDate, art.Date[:]),
		hotline.NewField(hotline.FieldNewsArtPrevArt, art.PrevArt[:]),
		hotline.NewField(hotline.FieldNewsArtNextArt, art.NextArt[:]),
		hotline.NewField(hotline.FieldNewsArtParentArt, art.ParentArt[:]),
		hotline.NewField(hotline.FieldNewsArt1stChildArt, art.FirstChildArt[:]),
		hotline.NewField(hotline.FieldNewsArtDataFlav, []byte("text/plain")),
		hotline.NewField(hotline.FieldNewsArtData, []byte(art.Data)),
	))
	return res
}

// HandleDelNewsItem deletes an existing news item from the server.
//
// Access: News Delete Folder (37) or News Delete Category (35)
//
// Fields used in the request:
//   - 325 News path  Required
//
// Fields used in the reply: None
func HandleDelNewsItem(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil || len(pathStrs) == 0 {
		cc.Logger.Error("invalid news path")
		return nil
	}

	item := cc.Server.ThreadedNewsMgr.NewsItem(pathStrs)

	if item.Type == [2]byte{0, 3} {
		if !cc.Authorize(hotline.AccessNewsDeleteCat) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsCategories)
		}
	} else {
		if !cc.Authorize(hotline.AccessNewsDeleteFldr) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsFolders)
		}
	}

	err = cc.Server.ThreadedNewsMgr.DeleteNewsItem(pathStrs)
	if err != nil {
		return res
	}

	return append(res, cc.NewReply(t))
}

// HandleDelNewsArt deletes a specific news article.
//
// Access: News Delete Article (33)
//
// Fields used in the request:
//   - 325 News path                       Required
//   - 326 News article ID                 Required
//   - 337 News article – recursive delete Optional - Delete child articles (1) or not (0)
//
// Fields used in the reply: None
func HandleDelNewsArt(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsDeleteArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsArticles)

	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		return res
	}

	articleID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		cc.Logger.Error("error reading article Type", "err", err)
		return
	}

	deleteRecursive := bytes.Equal([]byte{0, 1}, t.GetField(hotline.FieldNewsArtRecurseDel).Data)

	err = cc.Server.ThreadedNewsMgr.DeleteArticle(pathStrs, uint32(articleID), deleteRecursive)
	if err != nil {
		cc.Logger.Error("error deleting news article", "err", err)
	}

	return []hotline.Transaction{cc.NewReply(t)}
}

// HandlePostNewsArt posts a new news article on the server.
//
// Access: News Post Article (21)
//
// Fields used in the request:
//   - 325 News path                 Required
//   - 326 News article ID           ID of the parent article
//   - 328 News article title        Required
//   - 334 News article flags        Optional
//   - 327 News article data flavor  Currently "text/plain"
//   - 333 News article data         Required
//
// Fields used in the reply: None
func HandlePostNewsArt(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsPostArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedPostNewsArticles)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil || len(pathStrs) == 0 {
		cc.Logger.Error("invalid news path")
		return res
	}

	parentArticleID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		return res
	}

	err = cc.Server.ThreadedNewsMgr.PostArticle(
		pathStrs,
		uint32(parentArticleID),
		hotline.NewsArtData{
			Title:    string(t.GetField(hotline.FieldNewsArtTitle).Data),
			Poster:   string(cc.UserName),
			Date:     hotline.NewTime(time.Now()),
			DataFlav: hotline.NewsFlavor,
			Data:     string(t.GetField(hotline.FieldNewsArtData).Data),
		},
	)
	if err != nil {
		cc.Logger.Error("error posting news article", "err", err)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetMsgs returns the flat news data (message board content).
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 101 Data  Message text
func HandleGetMsgs(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	_, _ = cc.Server.MessageBoard.Seek(0, 0)

	newsData, err := io.ReadAll(cc.Server.MessageBoard)
	if err != nil {
		cc.Logger.Error("Error reading messageboard", "err", err)
	}

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldData, newsData)))
}

// HandleDownloadFile downloads a file from the specified path on the server.
//
// Access: Download File (2)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 203 File resume data       Optional
//   - 204 File transfer options  Optional - Set to 2 for TEXT, JPEG, GIFF, BMP or PICT files
//
// Fields used in the reply:
//   - 108 Transfer size     Size of data to be downloaded
//   - 207 File size         Actual file size
//   - 107 Reference number  Used later for transfer
//   - 116 Waiting count     Queue position
func HandleDownloadFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDownloadFile) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDownloadFiles)
	}

	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	resumeData := t.GetField(hotline.FieldFileResumeData).Data

	var dataOffset int64
	var frd hotline.FileResumeData
	if resumeData != nil {
		if err := frd.UnmarshalBinary(t.GetField(hotline.FieldFileResumeData).Data); err != nil {
			return res
		}
		// TODO: handle rsrc fork offset
		dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
	}

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, dataOffset)
	if err != nil {
		return res
	}

	xferSize := hlFile.Ffo.TransferSize(0)

	ft := cc.NewFileTransfer(
		hotline.FileDownload,
		cc.FileRoot(),
		fileName,
		filePath,
		xferSize,
	)

	if resumeData != nil {
		var frd hotline.FileResumeData
		if err := frd.UnmarshalBinary(t.GetField(hotline.FieldFileResumeData).Data); err != nil {
			return res
		}
		ft.FileResumeData = &frd
	}

	// Optional field for when a client requests file preview
	// Used only for TEXT, JPEG, GIFF, BMP or PICT files
	// The value will always be 2
	if t.GetField(hotline.FieldFileTransferOptions).Data != nil {
		ft.Options = t.GetField(hotline.FieldFileTransferOptions).Data
		xferSize = hlFile.Ffo.FlatFileDataForkHeader.DataSize[:]
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]),
		hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
		hotline.NewField(hotline.FieldTransferSize, xferSize),
		hotline.NewField(hotline.FieldFileSize, hlFile.Ffo.FlatFileDataForkHeader.DataSize[:]),
	))

	return res
}

// HandleDownloadFolder downloads all files from the specified folder and its subfolders.
//
// Access: Download File (2)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Optional
//
// Fields used in the reply:
//   - 220 Folder item count  Number of items in the folder
//   - 107 Reference number   Used later for transfer
//   - 108 Transfer size      Size of data to be downloaded
//   - 116 Waiting count      Queue position
func HandleDownloadFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDownloadFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDownloadFolders)
	}

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFilePath).Data, t.GetField(hotline.FieldFileName).Data)
	if err != nil {
		return nil
	}

	transferSize, err := hotline.CalcTotalSize(fullFilePath)
	if err != nil {
		return nil
	}
	itemCount, err := hotline.CalcItemCount(fullFilePath)
	if err != nil {
		return nil
	}

	fileTransfer := cc.NewFileTransfer(hotline.FolderDownload, cc.FileRoot(), t.GetField(hotline.FieldFileName).Data, t.GetField(hotline.FieldFilePath).Data, transferSize)

	var fp hotline.FilePath
	_, err = fp.Write(t.GetField(hotline.FieldFilePath).Data)
	if err != nil {
		return nil
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, fileTransfer.RefNum[:]),
		hotline.NewField(hotline.FieldTransferSize, transferSize),
		hotline.NewField(hotline.FieldFolderItemCount, itemCount),
		hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	))
	return res
}

// HandleUploadFolder uploads all files from a local folder and its subfolders to the server.
//
// Access: Upload File (1)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 108 Transfer size          Total size of all items in the folder
//   - 220 Folder item count      Number of items in the folder
//   - 204 File transfer options  Optional - Currently set to 1
//
// Fields used in the reply:
//   - 107 Reference number  Used later for transfer
func HandleUploadFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessUploadFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedUploadFolders)
	}

	var fp hotline.FilePath
	if t.GetField(hotline.FieldFilePath).Data != nil {
		if _, err := fp.Write(t.GetField(hotline.FieldFilePath).Data); err != nil {
			return res
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(hotline.AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf(ErrMsgUploadRestrictedTemplate, "folder", string(t.GetField(hotline.FieldFileName).Data)))
		}
	}

	fileTransfer := cc.NewFileTransfer(hotline.FolderUpload,
		cc.FileRoot(),
		t.GetField(hotline.FieldFileName).Data,
		t.GetField(hotline.FieldFilePath).Data,
		t.GetField(hotline.FieldTransferSize).Data,
	)

	fileTransfer.FolderItemCount = t.GetField(hotline.FieldFolderItemCount).Data

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldRefNum, fileTransfer.RefNum[:])))
}

// HandleUploadFile uploads a file to the specified path on the server.
//
// Access: Upload File (1)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 204 File transfer options  Optional - Used to resume download, value 2
//   - 108 File transfer size     Optional - Used if download is not resumed
//
// Fields used in the reply:
//   - 203 File resume data  Optional - Used only to resume download
//   - 107 Reference number  Transfer reference
func HandleUploadFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessUploadFile) {
		return cc.NewErrReply(t, ErrMsgNotAllowedUploadFiles)
	}

	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	transferOptions := t.GetField(hotline.FieldFileTransferOptions).Data
	transferSize := t.GetField(hotline.FieldTransferSize).Data // not sent for resume

	var fp hotline.FilePath
	if filePath != nil {
		if _, err := fp.Write(filePath); err != nil {
			return res
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(hotline.AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf(ErrMsgUploadRestrictedTemplate, "file", string(fileName)))
		}
	}
	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	if _, err := cc.Server.FS.Stat(fullFilePath); err == nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgFileUploadConflictTemplate, string(fileName)))
	}

	ft := cc.NewFileTransfer(hotline.FileUpload, cc.FileRoot(), fileName, filePath, transferSize)

	replyT := cc.NewReply(t, hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]))

	// client has requested to resume a partially transferred file
	if transferOptions != nil {
		fileInfo, err := cc.Server.FS.Stat(fullFilePath + hotline.IncompleteFileSuffix)
		if err != nil {
			return res
		}

		offset := make([]byte, 4)
		binary.BigEndian.PutUint32(offset, uint32(fileInfo.Size()))

		fileResumeData := hotline.NewFileResumeData([]hotline.ForkInfoList{
			*hotline.NewForkInfoList(offset),
		})

		b, _ := fileResumeData.BinaryMarshal()

		ft.TransferSize = offset

		replyT.Fields = append(replyT.Fields, hotline.NewField(hotline.FieldFileResumeData, b))
	}

	res = append(res, replyT)
	return res
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
		cc.Icon = t.GetField(hotline.FieldUserIconID).Data[2:]
	} else {
		cc.Icon = t.GetField(hotline.FieldUserIconID).Data
	}
	if cc.Authorize(hotline.AccessAnyName) {
		oldNickname := string(cc.UserName)
		newNickname := string(t.GetField(hotline.FieldUserName).Data)
		cc.UserName = t.GetField(hotline.FieldUserName).Data
		if cc.Server.Redis != nil {
			login := cc.Account.Login
			ip, _, _ := net.SplitHostPort(cc.RemoteAddr)
			// Remove old entry (login:oldnickname:ip) and (login::ip)
			cc.Server.Redis.SRem(context.Background(), "mobius:online", login+"::"+ip)
			if oldNickname != "" {
				cc.Server.Redis.SRem(context.Background(), "mobius:online", login+":"+oldNickname+":"+ip)
			}
			// Add new entry
			cc.Server.Redis.SAdd(context.Background(), "mobius:online", login+":"+newNickname+":"+ip)
			// Ban check for nickname
			bannedNick, _ := cc.Server.Redis.SIsMember(context.Background(), "mobius:banned:nicknames", newNickname).Result()
			if bannedNick {
				// Remove all possible online entries for this login and IP
				cc.Server.Redis.SRem(context.Background(), "mobius:online", login+"::"+ip)
				cc.Server.Redis.SRem(context.Background(), "mobius:online", login+":"+newNickname+":"+ip)
				if oldNickname != "" {
					cc.Server.Redis.SRem(context.Background(), "mobius:online", login+":"+oldNickname+":"+ip)
				}
				cc.Server.Redis.SAdd(context.Background(), "mobius:banned:ips", ip)
				cc.Disconnect()
				return res
			}
		}
	}

	// the options field is only passed by the client versions > 1.2.3.
	options := t.GetField(hotline.FieldOptions).Data
	if options != nil {
		optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

		cc.Flags.Set(hotline.UserFlagRefusePM, optBitmap.Bit(hotline.UserOptRefusePM))
		cc.Flags.Set(hotline.UserFlagRefusePChat, optBitmap.Bit(hotline.UserOptRefuseChat))

		// Check auto response
		if optBitmap.Bit(hotline.UserOptAutoResponse) == 1 {
			cc.AutoReply = t.GetField(hotline.FieldAutomaticResponse).Data
		} else {
			cc.AutoReply = []byte{}
		}
	}

	for _, c := range cc.Server.ClientMgr.List() {
		res = append(res, hotline.NewTransaction(
			hotline.TranNotifyChangeUser,
			c.ID,
			hotline.NewField(hotline.FieldUserID, cc.ID[:]),
			hotline.NewField(hotline.FieldUserIconID, cc.Icon),
			hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
			hotline.NewField(hotline.FieldUserName, cc.UserName),
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

// HandleGetFileNameList gets the list of file names from the specified folder.
//
// Fields used in the request:
//   - 202 File path  Optional - If not specified, root folder assumed
//
// Fields used in the reply:
//   - 200 File name with info  Repeated - File information for each item
func HandleGetFileNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fullPath, err := hotline.ReadPath(
		cc.FileRoot(),
		t.GetField(hotline.FieldFilePath).Data,
		nil,
	)
	if err != nil {
		return res
	}

	var fp hotline.FilePath
	if t.GetField(hotline.FieldFilePath).Data != nil {
		if _, err = fp.Write(t.GetField(hotline.FieldFilePath).Data); err != nil {
			return res
		}
	}

	// Handle special case for drop box folders
	if fp.IsDropbox() && !cc.Authorize(hotline.AccessViewDropBoxes) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewDropBoxes)
	}

	fileNames, err := hotline.GetFileNameList(fullPath, cc.Server.Config.IgnoreFiles)
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
	targetID := t.GetField(hotline.FieldUserID).Data

	// Create a new chat with self as initial member.
	newChatID := cc.Server.ChatMgr.New(cc)

	// Check if target user has "Refuse private chat" flag
	targetClient := cc.Server.ClientMgr.Get([2]byte(targetID))
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
				[2]byte(targetID),
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
	targetID := t.GetField(hotline.FieldUserID).Data
	chatID := t.GetField(hotline.FieldChatID).Data

	return []hotline.Transaction{
		hotline.NewTransaction(
			hotline.TranInviteToChat,
			[2]byte(targetID),
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
	chatID := [4]byte(t.GetField(hotline.FieldChatID).Data)

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
	chatID := t.GetField(hotline.FieldChatID).Data

	// Send TranNotifyChatChangeUser to current members of the chat to inform of new user
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatChangeUser,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID),
				hotline.NewField(hotline.FieldUserName, cc.UserName),
				hotline.NewField(hotline.FieldUserID, cc.ID[:]),
				hotline.NewField(hotline.FieldUserIconID, cc.Icon),
				hotline.NewField(hotline.FieldUserFlags, cc.Flags[:]),
			),
		)
	}

	cc.Server.ChatMgr.Join(hotline.ChatID(chatID), cc)

	subject := cc.Server.ChatMgr.GetSubject(hotline.ChatID(chatID))

	replyFields := []hotline.Field{hotline.NewField(hotline.FieldChatSubject, []byte(subject))}
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		b, err := io.ReadAll(&hotline.User{
			ID:    c.ID,
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			return res
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
	chatID := t.GetField(hotline.FieldChatID).Data

	cc.Server.ChatMgr.Leave([4]byte(chatID), cc.ID)

	// Notify members of the private chat that the user has left
	for _, c := range cc.Server.ChatMgr.Members(hotline.ChatID(chatID)) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatDeleteUser,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID),
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
	chatID := t.GetField(hotline.FieldChatID).Data

	cc.Server.ChatMgr.SetSubject([4]byte(chatID), string(t.GetField(hotline.FieldChatSubject).Data))

	// Notify chat members of new subject.
	for _, c := range cc.Server.ChatMgr.Members([4]byte(chatID)) {
		res = append(res,
			hotline.NewTransaction(
				hotline.TranNotifyChatSubject,
				c.ID,
				hotline.NewField(hotline.FieldChatID, chatID),
				hotline.NewField(hotline.FieldChatSubject, t.GetField(hotline.FieldChatSubject).Data),
			),
		)
	}

	return res
}

// HandleMakeAlias makes a file alias using the specified path.
//
// Access: Make Alias (31)
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Required
//   - 212 File new path  Required - Destination path
//
// Fields used in the reply: None
func HandleMakeAlias(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessMakeAlias) {
		return cc.NewErrReply(t, ErrMsgNotAllowedMakeAliases)
	}
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	fileNewPath := t.GetField(hotline.FieldFileNewPath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName)
	if err != nil {
		return res
	}

	fullNewFilePath, err := hotline.ReadPath(cc.FileRoot(), fileNewPath, fileName)
	if err != nil {
		return res
	}

	if err := cc.Server.FS.Symlink(fullFilePath, fullNewFilePath); err != nil {
		return cc.NewErrReply(t, ErrMsgCreateAlias)
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleDownloadBanner requests a new banner from the server.
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 107 Reference number  Used later for transfer
//   - 108 Transfer size     Size of data to be downloaded
func HandleDownloadBanner(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	ft := cc.NewFileTransfer(hotline.BannerDownload, "", []byte{}, []byte{}, make([]byte, 4))
	binary.BigEndian.PutUint32(ft.TransferSize, uint32(len(cc.Server.Banner)))

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]),
		hotline.NewField(hotline.FieldTransferSize, ft.TransferSize),
	))
}
