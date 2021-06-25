package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"
)

type TransactionType struct {
	Access         int                                                    // Specifies access privilege required to perform the transaction
	DenyMsg        string                                                 // The error reply message when user does not have access
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
	tranAgreed: {
		Access:  accessAlwaysAllow,
		Name:    "tranAgreed",
		Handler: HandleTranAgreed,
	},
	tranChatSend: {
		Access:  accessSendChat,
		DenyMsg: "You are not allowed to participate in chat.",
		Handler: HandleChatSend,
		Name:    "tranChatSend",
		RequiredFields: []requiredField{
			{
				ID:     fieldData,
				minLen: 0,
			},
		},
	},
	tranDelNewsArt: {
		Access:  accessNewsDeleteArt,
		DenyMsg: "You are not allowed to delete news articles.",
		Name:    "tranDelNewsArt",
		Handler: HandleDelNewsArt,
	},
	tranDelNewsItem: {
		Access: accessAlwaysAllow, // Granular access enforced inside the handler
		// Has multiple access flags: News Delete Folder (37) or News Delete Category (35)
		// TODO: Implement inside the handler
		Name:    "tranDelNewsItem",
		Handler: HandleDelNewsItem,
	},
	tranDeleteFile: {
		Access:  accessAlwaysAllow, // Granular access enforced inside the handler
		Name:    "tranDeleteFile",
		Handler: HandleDeleteFile,
	},
	tranDeleteUser: {
		Access:  accessDeleteUser,
		DenyMsg: "You are not allowed to delete accounts.",
		Name:    "tranDeleteUser",
		Handler: HandleDeleteUser,
	},
	tranDisconnectUser: {
		Access:  accessDisconUser,
		DenyMsg: "You are not allowed to disconnect users.",
		Name:    "tranDisconnectUser",
		Handler: HandleDisconnectUser,
	},
	tranDownloadFile: {
		Access:  accessDownloadFile,
		DenyMsg: "You are not allowed to download files.",
		Name:    "tranDownloadFile",
		Handler: HandleDownloadFile,
	},
	tranDownloadFldr: {
		Access:  accessDownloadFile, // There is no specific access flag for folder vs file download
		DenyMsg: "You are not allowed to download files.",
		Name:    "tranDownloadFldr",
		Handler: HandleDownloadFolder,
	},
	tranGetClientInfoText: {
		Access:  accessGetClientInfo,
		DenyMsg: "You are not allowed to get client info",
		Name:    "tranGetClientInfoText",
		Handler: HandleGetClientConnInfoText,
	},
	tranGetFileInfo: {
		Access:  accessAlwaysAllow,
		Name:    "tranGetFileInfo",
		Handler: HandleGetFileInfo,
	},
	tranGetFileNameList: {
		Access:  accessAlwaysAllow,
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
		Access:  accessOpenUser,
		DenyMsg: "You are not allowed to view accounts.",
		Name:    "tranGetUser",
		Handler: HandleGetUser,
	},
	tranGetUserNameList: {
		Access:  accessAlwaysAllow,
		Name:    "tranHandleGetUserNameList",
		Handler: HandleGetUserNameList,
	},
	tranInviteNewChat: {
		Access:  accessOpenChat,
		DenyMsg: "You are not allowed to request private chat.",
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
	tranNotifyDeleteUser: {
		Name: "tranNotifyDeleteUser",
	},
	tranListUsers: {
		Access:  accessOpenUser,
		DenyMsg: "You are not allowed to view accounts.",
		Name:    "tranListUsers",
		Handler: HandleListUsers,
	},
	tranMoveFile: {
		Access:  accessMoveFile,
		DenyMsg: "You are not allowed to move files.",
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
		Access:  accessCreateUser,
		DenyMsg: "You are not allowed to create new accounts.",
		Name:    "tranNewUser",
		Handler: HandleNewUser,
	},
	tranOldPostNews: {
		Name:    "tranOldPostNews",
		Handler: HandleTranOldPostNews,
	},
	tranPostNewsArt: {
		Access:  accessNewsPostArt,
		DenyMsg: "You are not allowed to post news articles.",
		Name:    "tranPostNewsArt",
		Handler: HandlePostNewsArt,
	},
	tranRejectChatInvite: {
		Name:    "tranRejectChatInvite",
		Handler: HandleRejectChatInvite,
	},
	tranSendInstantMsg: {
		//Access: accessSendPrivMsg,
		//DenyMsg: "You are not allowed to send private messages",
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
	tranSetClientUserInfo: {
		Access:  accessAlwaysAllow,
		Name:    "tranSetClientUserInfo",
		Handler: HandleSetClientUserInfo,
	},
	tranSetFileInfo: {
		Name:    "tranSetFileInfo",
		Handler: HandleSetFileInfo,
	},
	tranSetUser: {
		Access:  accessModifyUser,
		DenyMsg: "You are not allowed to modify accounts.",
		Name:    "tranSetUser",
		Handler: HandleSetUser,
	},
	tranUploadFile: {
		Access:  accessUploadFile,
		DenyMsg: "You are not allowed to upload files.",
		Name:    "tranUploadFile",
		Handler: HandleUploadFile,
	},
	tranUploadFldr: {
		Name:    "tranUploadFldr",
		Handler: HandleUploadFolder,
	},
	tranUserBroadcast: {
		Access:  accessBroadcast,
		DenyMsg: "You are not allowed to send broadcast messages.",
		Name:    "tranUserBroadcast",
		Handler: HandleUserBroadcast,
	},
}

func HandleChatSend(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	var replyTran Transaction

	// Truncate long usernames
	trunc := fmt.Sprintf("%13s", *cc.UserName)
	formattedMsg := fmt.Sprintf("%.13s:  %s\r", trunc, t.GetField(fieldData).Data)

	// By holding the option key, Hotline chat allows users to send /me formatted messages like:
	// *** Halcyon does stuff
	// This is indicated by the presence of the optional field fieldChatOptions in the transaction payload
	if t.GetField(fieldChatOptions).Data != nil {
		formattedMsg = fmt.Sprintf("*** %s %s\r", *cc.UserName, t.GetField(fieldData).Data)
	}

	chatID := t.GetField(fieldChatID).Data
	// a non-nil chatID indicates the message belongs to a private chat
	if chatID != nil {
		chatInt := binary.BigEndian.Uint32(chatID)
		privChat := cc.Server.PrivateChats[chatInt]

		// send the message to all connected clients of the private chat
		for _, c := range privChat.ClientConn {
			res = append(res, *NewNewTransaction(
				tranChatMsg,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldData, []byte(formattedMsg)),
			))
		}
		return res, err
	}

	replyTran = NewTransaction(
		tranChatMsg, 0,
		[]Field{
			NewField(fieldData, []byte(formattedMsg)),
		},
	)

	for _, c := range sortedClients(cc.Server.Clients) {
		// Filter out clients that do not have the read chat permission
		if authorize(c.Account.Access, accessReadChat) {
			replyTran.clientID = c.ID
			res = append(res, replyTran)
		}
	}

	return res, err
}

// HandleSendInstantMsg sends instant message to the user on the current server.
// Fields used in the request:
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
//Fields used in the reply:
// None
func HandleSendInstantMsg(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	msg := t.GetField(fieldData)
	ID := t.GetField(fieldUserID)
	// TODO: Implement reply quoting
	//options := transaction.GetField(hotline.fieldOptions)

	res = append(res,
		*NewNewTransaction(
			tranServerMsg,
			&ID.Data,
			NewField(fieldData, msg.Data),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldOptions, []byte{0, 1}),
		),
	)
	id, _ := byteToInt(ID.Data)

	//keys := make([]uint16, 0, len(cc.Server.Clients))
	//for k := range cc.Server.Clients {
	//	keys = append(keys, k)
	//}

	otherClient := cc.Server.Clients[uint16(id)]
	if otherClient == nil {
		return res, errors.New("ohno")
	}

	// Respond with auto reply if other client has it enabled
	if len(*otherClient.AutoReply) > 0 {
		res = append(res,
			*NewNewTransaction(
				tranServerMsg,
				cc.ID,
				NewField(fieldData, *otherClient.AutoReply),
				NewField(fieldUserName, *otherClient.UserName),
				NewField(fieldUserID, *otherClient.ID),
				NewField(fieldOptions, []byte{0, 1}),
			),
		)
	}

	res = append(res, cc.NewReply(t))

	return res, err
}

func HandleGetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)

	ffo, err := NewFlattenedFileObject(filePath, fileName)
	if err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldFileName, []byte(fileName)),
		NewField(fieldFileTypeString, ffo.FlatFileInformationFork.TypeSignature),
		NewField(fieldFileCreatorString, ffo.FlatFileInformationFork.CreatorSignature),
		NewField(fieldFileComment, ffo.FlatFileInformationFork.Comment),
		NewField(fieldFileType, ffo.FlatFileInformationFork.TypeSignature),
		NewField(fieldFileCreateDate, ffo.FlatFileInformationFork.CreateDate),
		NewField(fieldFileModifyDate, ffo.FlatFileInformationFork.ModifyDate),
		NewField(fieldFileSize, ffo.FlatFileDataForkHeader.DataSize),
	))
	return res, err
}

// HandleSetFileInfo updates a file or folder name and/or comment from the Get Info window
// TODO: Implement support for comments
// Fields used in the request:
// * 201	File name
// * 202	File path	Optional
// * 211	File new name	Optional
// * 210	File comment	Optional
// Fields used in the reply:	None
func HandleSetFileInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)
	//fileComment := t.GetField(fieldFileComment).Data
	fileNewName := t.GetField(fieldFileNewName).Data

	if fileNewName != nil {
		path := filePath + "/" + fileName
		fi, err := os.Stat(path)
		if err != nil {
			return res, err
		}
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if authorize(cc.Account.Access, accessRenameFolder) == false {
				res = append(res, cc.NewErrReply(t, "You are not allowed to rename folders."))
				return res, err
			}
		case mode.IsRegular():
			if authorize(cc.Account.Access, accessRenameFile) == false {
				res = append(res, cc.NewErrReply(t, "You are not allowed to rename files."))
				return res, err
			}
		}

		err = os.Rename(filePath+"/"+fileName, filePath+"/"+string(fileNewName))
		if os.IsNotExist(err) {
			res = append(res, cc.NewErrReply(t, "Cannot rename file "+fileName+" because it does not exist or cannot be found."))
			return res, err
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
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)

	path := "./" + filePath + "/" + fileName

	logger.Debugw("Delete file", "src", filePath+"/"+fileName)

	fi, err := os.Stat(path)
	if err != nil {
		res = append(res, cc.NewErrReply(t, "Cannot delete file "+fileName+" because it does not exist or cannot be found."))
		return res, nil
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if authorize(cc.Account.Access, accessDeleteFolder) == false {
			res = append(res, cc.NewErrReply(t, "You are not allowed to delete folders."))
			return res, err
		}
	case mode.IsRegular():
		if authorize(cc.Account.Access, accessDeleteFile) == false {
			res = append(res, cc.NewErrReply(t, "You are not allowed to delete files."))
			return res, err
		}
	}

	if err := os.RemoveAll(path); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleMoveFile moves files or folders. Note: seemingly not documented
func HandleMoveFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := "./" + cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)
	fileNewPath := "./" + cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFileNewPath).Data)

	logger.Debugw("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	path := filePath + "/" + fileName
	fi, err := os.Stat(path)
	if err != nil {
		return res, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if authorize(cc.Account.Access, accessMoveFolder) == false {
			res = append(res, cc.NewErrReply(t, "You are not allowed to move folders."))
			return res, err
		}
	case mode.IsRegular():
		if authorize(cc.Account.Access, accessMoveFile) == false {
			res = append(res, cc.NewErrReply(t, "You are not allowed to move files."))
			return res, err
		}
	}

	err = os.Rename(filePath+"/"+fileName, fileNewPath+"/"+fileName)
	if os.IsNotExist(err) {
		res = append(res, cc.NewErrReply(t, "Cannot delete file "+fileName+" because it does not exist or cannot be found."))
		return res, err
	}
	if err != nil {
		return []Transaction{}, err
	}
	// TODO: handle other possible errors; e.g. file delete fails due to file permission issue

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleNewFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	newFolderPath := cc.Server.Config.FileRoot

	// fieldFilePath is only present for nested paths
	if t.GetField(fieldFilePath).Data != nil {
		newFp := NewFilePath(t.GetField(fieldFilePath).Data)
		newFolderPath += newFp.String()
	}
	newFolderPath += "/" + string(t.GetField(fieldFileName).Data)

	if err := os.Mkdir(newFolderPath, 0777); err != nil {
		// TODO: Send error response to client
		return []Transaction{}, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleSetUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	userLogin := DecodeUserString(t.GetField(fieldUserLogin).Data)
	userName := string(t.GetField(fieldUserName).Data)

	newAccessLvl := t.GetField(fieldUserAccess).Data

	account := cc.Server.Accounts[userLogin]
	account.Access = &newAccessLvl
	account.Name = userName

	// If the password field is cleared in the Hotline edit user UI, the SetUser transaction does
	// not include fieldUserPassword
	if t.GetField(fieldUserPassword).Data == nil {
		account.Password = hashAndSalt([]byte(""))
	}
	if len(t.GetField(fieldUserPassword).Data) > 1 {
		account.Password = hashAndSalt(t.GetField(fieldUserPassword).Data)
	}

	file := cc.Server.ConfigDir + "Users/" + userLogin + ".yaml"
	out, err := yaml.Marshal(&account)
	if err != nil {
		return res, err
	}
	if err := ioutil.WriteFile(file, out, 0666); err != nil {
		return res, err
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.Clients {
		if c.Account.Login == userLogin {
			// Note: comment out these two lines to test server-side deny messages
			newT := NewNewTransaction(tranUserAccess, c.ID, NewField(fieldUserAccess, newAccessLvl))
			res = append(res, *newT)

			flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*c.Flags)))
			if authorize(c.Account.Access, accessDisconUser) == true {
				flagBitmap.SetBit(flagBitmap, userFlagAdmin, 1)
			} else {
				flagBitmap.SetBit(flagBitmap, userFlagAdmin, 0)
			}
			binary.BigEndian.PutUint16(*c.Flags, uint16(flagBitmap.Int64()))

			c.Account.Access = account.Access

			err := cc.Server.NotifyAll(
				NewTransaction(
					tranNotifyChangeUser,
					0,
					[]Field{
						NewField(fieldUserID, *c.ID),
						NewField(fieldUserFlags, *c.Flags),
						NewField(fieldUserName, *c.UserName),
						NewField(fieldUserIconID, *c.Icon),
					},
				),
			)
			if err != nil {
				return res, err
			}
		}
	}

	// TODO: If we have just promoted a connected user to admin, notify
	// connected clients to turn the user red

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleGetUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	userLogin := string(t.GetField(fieldUserLogin).Data)
	decodedUserLogin := NegatedUserString(t.GetField(fieldUserLogin).Data)
	account := cc.Server.Accounts[userLogin]
	if account == nil {
		errorT := cc.NewErrReply(t, "Account does not exist.")
		res = append(res, errorT)
		return res, err
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldUserName, []byte(account.Name)),
		NewField(fieldUserLogin, []byte(decodedUserLogin)),
		NewField(fieldUserPassword, []byte(account.Password)),
		NewField(fieldUserAccess, *account.Access),
	))
	return res, err
}

func HandleListUsers(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	var userFields []Field
	// TODO: make order deterministic
	for _, acc := range cc.Server.Accounts {
		userField := acc.Payload()
		userFields = append(userFields, NewField(fieldData, userField))
	}

	res = append(res, cc.NewReply(t, userFields...))
	return res, err
}

// HandleNewUser creates a new user account
func HandleNewUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	login := DecodeUserString(t.GetField(fieldUserLogin).Data)

	// If the account already exists, reply with an error
	// TODO: make order deterministic
	if _, ok := cc.Server.Accounts[login]; ok {
		res = append(res, cc.NewErrReply(t, "Cannot create account "+login+" because there is already an account with that login."))
		return res, err
	}

	if err := cc.Server.NewUser(
		login,
		string(t.GetField(fieldUserName).Data),
		string(t.GetField(fieldUserPassword).Data),
		t.GetField(fieldUserAccess).Data,
	); err != nil {
		return []Transaction{}, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleDeleteUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
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
	cc.NotifyOthers(
		NewTransaction(
			tranServerMsg, 0,
			[]Field{
				NewField(fieldData, t.GetField(tranGetMsgs).Data),
				NewField(fieldChatOptions, []byte{0}),
			},
		),
	)

	res = append(res, cc.NewReply(t))
	return res, err
}

func byteToInt(bytes []byte) (int, error) {
	switch len(bytes) {
	case 2:
		return int(binary.BigEndian.Uint16(bytes)), nil
	case 4:
		return int(binary.BigEndian.Uint32(bytes)), nil
	}

	return 0, errors.New("unknown byte length")
}

func HandleGetClientConnInfoText(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	spew.Dump(t.GetField(fieldUserID).Data)

	clientID, _ := byteToInt(t.GetField(fieldUserID).Data)

	clientConn := cc.Server.Clients[uint16(clientID)]
	if clientConn == nil {
		return res, errors.New("invalid client")
	}

	// TODO: Implement non-hardcoded values
	template := `Nickname:   %s
Name:       %s
Account:    %s
Address:    %s

-------- File Downloads ---------

None.

------- Folder Downloads --------

None.

--------- File Uploads ----------

None.

-------- Folder Uploads ---------

None.

------- Waiting Downloads -------

None.

	`
	template = fmt.Sprintf(template, *clientConn.UserName, clientConn.Account.Name, clientConn.Account.Login, clientConn.Connection.RemoteAddr().String())
	template = strings.Replace(template, "\n", "\r", -1)

	res = append(res, cc.NewReply(t,
		NewField(fieldData, []byte(template)),
		NewField(fieldUserName, *clientConn.UserName),
	))
	return res, err
}

func HandleGetUserNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	res = append(res, cc.NewReply(t, cc.Server.connectedUsers()...))

	return res, err
}

func (cc *ClientConn) notifyNewUserHasJoined() (res []Transaction, err error) {
	// Notify other ccs that a new user has connected
	cc.NotifyOthers(
		NewTransaction(
			tranNotifyChangeUser, 1,
			[]Field{
				NewField(fieldUserName, *cc.UserName),
				NewField(fieldUserID, *cc.ID),
				NewField(fieldUserIconID, *cc.Icon),
				NewField(fieldUserFlags, *cc.Flags),
			},
		),
	)

	// When a client connects to a server, other clients print a message into their
	// local chat window announcing the new user has joined:
	//  <<<   Foo has joined   >>>
	// <<<   5/31/20 7:19:20 AM    >>>
	// The second line with the timestamp doesn't end with a carriage return and
	// causes the next chat message to start on the same line.  To work around this,
	// we send a CR to all clients.  Strange that this is necessary.
	cc.NotifyOthers(
		NewTransaction(
			tranChatMsg, 3,
			[]Field{
				NewField(fieldData, []byte("\r")),
			},
		),
	)

	return res, nil
}

func HandleTranAgreed(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, *cc.Server.NextGuestID)

	spew.Dump(t)
	*cc.UserName = t.GetField(fieldUserName).Data
	*cc.ID = bs
	*cc.Icon = t.GetField(fieldUserIconID).Data

	options := t.GetField(fieldOptions).Data
	optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))

	flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*cc.Flags)))

	// Check refuse private PM option
	if optBitmap.Bit(refusePM) == 1 {
		flagBitmap.SetBit(flagBitmap, userFlagRefusePM, 1)
		binary.BigEndian.PutUint16(*cc.Flags, uint16(flagBitmap.Int64()))
	}

	// Check refuse private chat option
	if optBitmap.Bit(refuseChat) == 1 {
		flagBitmap.SetBit(flagBitmap, userFLagRefusePChat, 1)
		binary.BigEndian.PutUint16(*cc.Flags, uint16(flagBitmap.Int64()))
	}

	// Check auto response
	if optBitmap.Bit(autoResponse) == 1 {
		*cc.AutoReply = t.GetField(fieldAutomaticResponse).Data
	} else {
		*cc.AutoReply = []byte{}
	}

	_, _ = cc.notifyNewUserHasJoined()

	res = append(res, cc.NewReply(t))

	return res, err
}

// HandleTranOldPostNews updates the flat news
// Fields used in this request:
// 101	Data
func HandleTranOldPostNews(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	cc.Server.flatNewsMux.Lock()
	defer cc.Server.flatNewsMux.Unlock()

	current := time.Now()
	formattedDate := fmt.Sprintf("%s%02d %d:%d", current.Month().String()[:3], current.Day(), current.Hour(), current.Minute())
	// TODO: format news post
	newsPost := fmt.Sprintf(newsTemplate, *cc.UserName, formattedDate, t.GetField(fieldData).Data)
	newsPost = strings.Replace(newsPost, "\n", "\r", -1)

	// update news in memory
	cc.Server.FlatNews = append([]byte(newsPost), cc.Server.FlatNews...)

	// update news on disk
	err = ioutil.WriteFile(cc.Server.ConfigDir+"MessageBoard.txt", cc.Server.FlatNews, 0644)
	if err != nil {
		return res, err
	}

	// Notify all clients of updated news
	_ = cc.Server.NotifyAll(
		NewTransaction(
			tranNewMsg, 0,
			[]Field{
				NewField(fieldData, []byte(newsPost)),
			},
		),
	)

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleDisconnectUser(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	clientConn := cc.Server.Clients[binary.BigEndian.Uint16(t.GetField(fieldUserID).Data)]

	if authorize(clientConn.Account.Access, accessCannotBeDiscon) {
		res = append(res, cc.NewErrReply(t, clientConn.Account.Login+" is not allowed to be disconnected."))
		return res, err
	}

	if err := clientConn.Connection.Close(); err != nil {
		return res, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleGetNewsCatNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Fields used in the request:
	// 325	News path	(Optional)

	newsPath := t.GetField(fieldNewsPath).Data
	cc.Server.Logger.Infow("NewsPath: ", "np", string(newsPath))

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
		fieldData = append(fieldData, NewField(
			fieldNewsCatListData15,
			cat.Payload(),
		))
	}

	res = append(res, cc.NewReply(t, fieldData...))
	return res, err
}

func HandleNewNewsCat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	name := string(t.GetField(fieldNewsCatName).Data)
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cc.Server.Logger.Infof("Creating new news cat %s in %v", name, pathStrs)

	cats := cc.Server.GetNewsCatByPath(pathStrs)

	cats[name] = NewsCategoryListData15{
		Name:     name,
		Type:     []byte{0, 3},
		Articles: map[uint32]*NewsArtData{},
		SubCats:  make(map[string]NewsCategoryListData15),
	}

	_ = cc.Server.writeThreadedNews()
	res = append(res, cc.NewReply(t))
	return res, err
}

func HandleNewNewsFldr(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Fields used in the request:
	// 322	News category name
	// 325	News path
	name := string(t.GetField(fieldFileName).Data)
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cc.Server.Logger.Infof("Creating new news folder %s", name)

	cats := cc.Server.GetNewsCatByPath(pathStrs)
	cats[name] = NewsCategoryListData15{
		Name:     name,
		Type:     []byte{0, 2},
		Articles: map[uint32]*NewsArtData{},
		SubCats:  make(map[string]NewsCategoryListData15),
	}
	_ = cc.Server.writeThreadedNews()

	res = append(res, cc.NewReply(t))
	return res, err
}

// Fields used in the request:
// 325	News path	Optional
//
// Reply fields:
// 321	News article list data	Optional
func HandleGetNewsArtNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	var cat NewsCategoryListData15
	cats := cc.Server.ThreadedNews.Categories

	for _, path := range pathStrs {
		cat = cats[path]
		cats = cats[path].SubCats
	}

	nald := cat.GetNewsArtListData()

	res = append(res, cc.NewReply(t, NewField(fieldNewsArtListData, nald.Payload())))
	return res, err
}

func HandleGetNewsArtData(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Request fields
	// 325	News path
	// 326	News article ID
	// 327	News article data flavor

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	var cat NewsCategoryListData15
	cats := cc.Server.ThreadedNews.Categories

	for _, path := range pathStrs {
		cat = cats[path]
		cats = cats[path].SubCats
	}
	newsArtID := t.GetField(fieldNewsArtID).Data

	convertedArtID := binary.BigEndian.Uint16(newsArtID)

	art := cat.Articles[uint32(convertedArtID)]
	if art == nil {
		res = append(res, cc.NewReply(t))
		return res, err
	}

	// Reply fields
	// 328	News article title
	// 329	News article poster
	// 330	News article date
	// 331	Previous article ID
	// 332	Next article ID
	// 335	Parent article ID
	// 336	First child article ID
	// 327	News article data flavor	"Should be “text/plain”
	// 333	News article data	Optional (if data flavor is “text/plain”)

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

func HandleDelNewsItem(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Access:		News Delete Folder (37) or News Delete Category (35)

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)
	spew.Dump(pathStrs)

	// TODO: determine if path is a Folder (Bundle) or Category and check for permission

	cc.Server.Logger.Infof("DelNewsItem %v", pathStrs)

	cats := cc.Server.ThreadedNews.Categories

	delName := pathStrs[len(pathStrs)-1]
	if len(pathStrs) > 1 {
		for _, path := range pathStrs[0 : len(pathStrs)-1] {
			cats = cats[path].SubCats
		}
	}

	delete(cats, delName)

	err = cc.Server.writeThreadedNews()
	if err != nil {
		return res, err
	}

	// Reply params: none
	res = append(res, cc.NewReply(t))

	return res, err
}

func HandleDelNewsArt(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
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
	cc.Server.Logger.Infof("Deleting news article ID %s from %v", ID, catName)
	if err := cc.Server.writeThreadedNews(); err != nil {
		return []Transaction{}, err
	}

	res = append(res, cc.NewReply(t))
	return res, err
}

func HandlePostNewsArt(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Request fields
	// 325	News path
	// 326	News article ID	 						ID of the parent article?
	// 328	News article title
	// 334	News article flags
	// 327	News article data flavor		Currently “text/plain”
	// 333	News article data

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)
	cats := cc.Server.GetNewsCatByPath(pathStrs[:len(pathStrs)-1])

	catName := pathStrs[len(pathStrs)-1]
	cat := cats[catName]

	newArt := NewsArtData{
		Title:         string(t.GetField(fieldNewsArtTitle).Data),
		Poster:        string(*cc.UserName),
		Date:          NewsDate(),
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

		if bytes.Compare(parentArt.FirstChildArt, []byte{0, 0, 0, 0}) == 0 {
			binary.BigEndian.PutUint32(parentArt.FirstChildArt, nextID)
		}
	}

	cat.Articles[nextID] = &newArt

	cats[catName] = cat
	cc.Server.Logger.Infof("Posting news article to %s", pathStrs)
	cc.Server.writeThreadedNews()

	res = append(res, cc.NewReply(t))
	return res, err
}

// HandleGetMsgs returns the flat news data
func HandleGetMsgs(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	res = append(res, cc.NewReply(t, NewField(fieldData, cc.Server.FlatNews)))

	return res, err
}

func HandleDownloadFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := ReadFilePath(t.GetField(fieldFilePath).Data)

	ffo, err := NewFlattenedFileObject(
		cc.Server.Config.FileRoot+filePath,
		string(fileName),
	)
	if err != nil {
		return res, err
	}

	transactionRef := cc.Server.NewTransactionRef()
	data := binary.BigEndian.Uint32(transactionRef)

	cc.Server.Logger.Infow("File download", "path", filePath)

	cc.Server.FileTransfers[data] = &FileTransfer{
		FileName:        fileName,
		FilePath:        []byte(filePath),
		ReferenceNumber: transactionRef,
		Type:            FileDownload,
	}

	res = append(res, cc.NewReply(t,
		NewField(fieldRefNum, transactionRef),
		NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
		NewField(fieldTransferSize, ffo.TransferSize()),
		NewField(fieldFileSize, ffo.FlatFileDataForkHeader.DataSize),
	))

	return res, err
}

// Download all files from the specified folder and sub-folders
// response example
//
//	00
//	01
//	00 00
//	00 00 00 11
//	00 00 00 00
//	00 00 00 18
//	00 00 00 18
//
//	00 03
//
//	00 6c // transfer size
//	00 04 // len
//	00 0f d5 ae
//
//	00 dc // field Folder item count
//	00 02 // len
//	00 02
//
//	00 6b // ref number
//	00 04 // len
//	00 03 64 b1
func HandleDownloadFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	transactionRef := cc.Server.NewTransactionRef()
	data := binary.BigEndian.Uint32(transactionRef)

	fileTransfer := &FileTransfer{
		FileName:        t.GetField(fieldFileName).Data,
		FilePath:        t.GetField(fieldFilePath).Data,
		ReferenceNumber: transactionRef,
		Type:            FolderDownload,
	}
	cc.Server.FileTransfers[data] = fileTransfer

	fp := NewFilePath(t.GetField(fieldFilePath).Data)

	fullFilePath := fmt.Sprintf("./%v/%v", cc.Server.Config.FileRoot+fp.String(), string(fileTransfer.FileName))
	transferSize, err := CalcTotalSize(fullFilePath)
	if err != nil {
		return res, err
	}
	itemCount, err := CalcItemCount(fullFilePath)
	if err != nil {
		return res, err
	}
	res = append(res, cc.NewReply(t,
		NewField(fieldRefNum, transactionRef),
		NewField(fieldTransferSize, transferSize),
		NewField(fieldFolderItemCount, itemCount),       // TODO: Remove hardcode
		NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	))
	return res, err
}

// Upload all files from the local folder and its subfolders to the specified path on the server
// Fields used in the request
// 201	File name
// 202	File path
// 108	Transfer size	Total size of all items in the folder
// 220	Folder item count
// 204	File transfer options	"Optional Currently set to 1" (TODO: ??)
func HandleUploadFolder(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	transactionRef := cc.Server.NewTransactionRef()
	data := binary.BigEndian.Uint32(transactionRef)

	fileTransfer := &FileTransfer{
		FileName:        t.GetField(fieldFileName).Data,
		FilePath:        t.GetField(fieldFilePath).Data,
		ReferenceNumber: transactionRef,
		Type:            FolderUpload,
		FolderItemCount: t.GetField(fieldFolderItemCount).Data,
		TransferSize:    t.GetField(fieldTransferSize).Data,
	}
	cc.Server.FileTransfers[data] = fileTransfer

	res = append(res, cc.NewReply(t, NewField(fieldRefNum, transactionRef)))
	return res, err
}

func HandleUploadFile(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := t.GetField(fieldFilePath).Data

	transactionRef := cc.Server.NewTransactionRef()
	data := binary.BigEndian.Uint32(transactionRef)

	fileTransfer := &FileTransfer{
		FileName:        fileName,
		FilePath:        filePath,
		ReferenceNumber: transactionRef,
		Type:            FileUpload,
	}

	cc.Server.FileTransfers[data] = fileTransfer

	res = append(res, cc.NewReply(t, NewField(fieldRefNum, transactionRef)))
	return res, err
}

const refusePM = 0
const refuseChat = 1
const autoResponse = 2

func HandleSetClientUserInfo(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	spew.Dump(t)
	var icon []byte
	if len(t.GetField(fieldUserIconID).Data) == 4{
		icon = t.GetField(fieldUserIconID).Data[2:]
	} else {
		icon = t.GetField(fieldUserIconID).Data
	}
	*cc.Icon = icon
	*cc.UserName = t.GetField(fieldUserName).Data

	// the options field is only passed by the client versions > 1.2.3.
	options := t.GetField(fieldOptions).Data

	if options != nil {
		optBitmap := big.NewInt(int64(binary.BigEndian.Uint16(options)))
		flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*cc.Flags)))

		// Check refuse private PM option
		if optBitmap.Bit(refusePM) == 1 {
			flagBitmap.SetBit(flagBitmap, userFlagRefusePM, 1)
			binary.BigEndian.PutUint16(*cc.Flags, uint16(flagBitmap.Int64()))
		}

		// Check refuse private chat option
		if optBitmap.Bit(refuseChat) == 1 {
			flagBitmap.SetBit(flagBitmap, userFLagRefusePChat, 1)
			binary.BigEndian.PutUint16(*cc.Flags, uint16(flagBitmap.Int64()))
		}

		// Check auto response
		if optBitmap.Bit(autoResponse) == 1 {
			*cc.AutoReply = t.GetField(fieldAutomaticResponse).Data
		} else {
			*cc.AutoReply = []byte{}
		}
	}

	// Notify all clients of updated user info
	cc.Server.NotifyAll(
		NewTransaction(
			tranNotifyChangeUser, 0,
			[]Field{
				NewField(fieldUserID, *cc.ID),
				NewField(fieldUserIconID, *cc.Icon),
				NewField(fieldUserFlags, *cc.Flags),
				NewField(fieldUserName, *cc.UserName),
			},
		),
	)

	return res, err
}

// HandleKeepAlive response to keepalive transactions with an empty reply
// HL 1.9.2 Client sends keepalive msg every 3 minutes
// HL 1.2.3 Client doesn't send keepalives
func HandleKeepAlive(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	res = append(res, cc.NewReply(t))

	return res, err
}

func HandleGetFileNameList(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	filePath := cc.Server.Config.FileRoot

	fieldFilePath := t.GetField(fieldFilePath).Data
	if len(fieldFilePath) > 0 {
		filePath = cc.Server.Config.FileRoot + ReadFilePath(fieldFilePath)
	}

	fileNames, err := getFileNameList(filePath)
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
	// Client to Invite
	targetID := t.GetField(fieldUserID).Data
	newChatID := cc.Server.NewPrivateChat(cc)

	res = append(res,
		*NewNewTransaction(
			tranInviteToChat,
			&targetID,
			NewField(fieldChatID, newChatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
		),
	)

	res = append(res,
		cc.NewReply(t,
			NewField(fieldChatID, newChatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, *cc.Icon),
			NewField(fieldUserFlags, *cc.Flags),
		),
	)

	return res, err
}

func HandleInviteToChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	// Client to Invite
	targetID := t.GetField(fieldUserID).Data
	chatID := t.GetField(fieldChatID).Data

	res = append(res,
		*NewNewTransaction(
			tranInviteToChat,
			&targetID,
			NewField(fieldChatID, chatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
		),
	)
	res = append(res,
		cc.NewReply(
			t,
			NewField(fieldChatID, chatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldUserIconID, *cc.Icon),
			NewField(fieldUserFlags, *cc.Flags),
		),
	)

	return res, err
}

func HandleRejectChatInvite(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat := cc.Server.PrivateChats[chatInt]

	resMsg := append(*cc.UserName, []byte(" declined invitation to chat")...)

	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewNewTransaction(
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
			*NewNewTransaction(
				tranNotifyChatChangeUser,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldUserName, *cc.UserName),
				NewField(fieldUserID, *cc.ID),
				NewField(fieldUserIconID, *cc.Icon),
				NewField(fieldUserFlags, *cc.Flags),
			),
		)
	}

	privChat.ClientConn[cc.uint16ID()] = cc

	replyFields := []Field{NewField(fieldChatSubject, []byte(privChat.Subject))}
	for _, c := range sortedClients(privChat.ClientConn) {
		user := User{
			ID:    *c.ID,
			Icon:  *c.Icon,
			Flags: *c.Flags,
			Name:  string(*c.UserName),
		}

		replyFields = append(replyFields, NewField(fieldUsernameWithInfo, user.Payload()))
	}

	res = append(res, cc.NewReply(t, replyFields...))
	return res, err
}

// HandleLeaveChat is sent from a v1.8+ Hotline client when the user exits a private chat
// Fields used in the request:
//	* 114	fieldChatID
// Reply is not expected.
func HandleLeaveChat(cc *ClientConn, t *Transaction) (res []Transaction, err error) {
	chatID := t.GetField(fieldChatID).Data
	spew.Dump(chatID)
	chatInt := binary.BigEndian.Uint32(chatID)

	privChat := cc.Server.PrivateChats[chatInt]

	delete(privChat.ClientConn, cc.uint16ID())

	// Notify members of the private chat that the user has left
	for _, c := range sortedClients(privChat.ClientConn) {
		res = append(res,
			*NewNewTransaction(
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
			*NewNewTransaction(
				tranNotifyChatSubject,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldChatSubject, t.GetField(fieldChatSubject).Data),
			),
		)
	}

	return res, err
}
