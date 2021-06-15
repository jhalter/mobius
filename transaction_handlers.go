package hotline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"
)


func HandleChatSend(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	var replies []Transaction
	var replyTran Transaction

	// Truncate long usernames
	trunc := fmt.Sprintf("%13s", *cc.UserName)
	formattedMsg := fmt.Sprintf("%.13s:  %s\r", trunc, t.GetField(fieldData).Data)

	chatID := t.GetField(fieldChatID).Data
	// a non-nil chatID indicates the message belongs to a private chat
	if chatID != nil {
		chatInt := binary.BigEndian.Uint32(chatID)
		privChat := cc.Server.PrivateChats[chatInt]

		//replyTran = NewTransaction(
		//	tranChatMsg, 0,
		//	[]Field{
		//		NewField(fieldChatID, chatID),
		//		NewField(fieldData, []byte(formattedMsg)),
		//	},
		//)

		// send the message to all connected clients of the private chat
		for _, c := range privChat.ClientConn {
			//replyTran.clientID = c.ID
			replies = append(replies, *NewNewTransaction(
				tranChatMsg,
				c.ID,
				NewField(fieldChatID, chatID),
				NewField(fieldData, []byte(formattedMsg)),
			))
		}
		return replies, nil
	}

	replyTran = NewTransaction(
		tranChatMsg, 0,
		[]Field{
			NewField(fieldData, []byte(formattedMsg)),
		},
	)

	for _, c := range sortedClients(cc.Server.Clients) {
		// Filter out clients that do not have the read chat permission
		if c.Authorize(accessReadChat) {
			replyTran.clientID = c.ID
			replies = append(replies, replyTran)
		}
	}

	return replies, nil
}


func HandleSendInstantMsg(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	msg := t.GetField(fieldData)
	ID := t.GetField(fieldUserID)
	//options := transaction.GetField(hotline.fieldOptions)

	sendChat := NewTransaction(
		tranServerMsg, 0,
		[]Field{
			NewField(fieldData, msg.Data),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
			NewField(fieldOptions, []byte{0, 1}),
		},
	)
	// Send chat message to other cc
	if err := cc.notifyOtherClientConn(ID.Data, sendChat); err != nil {
		return []Transaction{}, err
	}

	// Respond with auto reply if other client has it enabled
	otherClient := cc.Server.Clients[binary.BigEndian.Uint16(ID.Data)]
	if len(*otherClient.AutoReply) > 0 {
		cc.SendTransaction(
			tranServerMsg,
			NewField(fieldData, *otherClient.AutoReply),
			NewField(fieldUserName, *otherClient.UserName),
			NewField(fieldUserID, *otherClient.ID),
			NewField(fieldOptions, []byte{0, 1}),
		)
	}

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleGetFileInfo(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)

	ffo, _ := NewFlattenedFileObject(filePath, fileName)

	err := cc.Reply(t,
		NewField(fieldFileName, []byte(fileName)),
		NewField(fieldFileTypeString, ffo.FlatFileInformationFork.TypeSignature),
		NewField(fieldFileCreatorString, ffo.FlatFileInformationFork.CreatorSignature),
		NewField(fieldFileComment, ffo.FlatFileInformationFork.Comment),
		NewField(fieldFileType, ffo.FlatFileInformationFork.TypeSignature),
		NewField(fieldFileCreateDate, ffo.FlatFileInformationFork.CreateDate),
		NewField(fieldFileModifyDate, ffo.FlatFileInformationFork.ModifyDate),
		NewField(fieldFileSize, ffo.FlatFileDataForkHeader.DataSize),
	)
	return []Transaction{}, err
}

func HandleSetFileInfo(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	// TODO: figure out how to handle file comments
	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)
	//fileComment := t.GetField(fieldFileComment).Data
	fileNewName := t.GetField(fieldFileNewName).Data

	if fileNewName != nil {
		err := os.Rename(filePath+"/"+fileName, filePath+"/"+string(fileNewName))
		if os.IsNotExist(err) {
			_, err := cc.Connection.Write(t.ReplyError("Cannot delete file " + fileName + " because it does not exist or cannot be found."))
			return []Transaction{}, err
		}
	}

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleDeleteFile(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	if cc.Authorize(accessDeleteFile) == false {
		_, err := cc.Connection.Write(t.ReplyError("You are not allowed to create new accounts."))
		return []Transaction{}, err
	}

	fileName := string(t.GetField(fieldFileName).Data)
	filePath := cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)

	logger.Debugw("Delete file", "src", filePath+"/"+fileName)

	err := os.Remove("./" + filePath + "/" + fileName)
	if os.IsNotExist(err) {
		_, err := cc.Connection.Write(t.ReplyError("Cannot delete file " + fileName + " because it does not exist or cannot be found."))
		return []Transaction{}, err
	}
	// TODO: handle other possible errors; e.g. file delete fails due to file permission issue

	err = cc.Reply(t)
	return []Transaction{}, err
}

func HandleMoveFile(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	//if cc.Authorize(accessDeleteFile) == false {
	//	_, err := cc.Connection.Write(t.ReplyError("You are not allowed to create new accounts."))
	//	return err
	//}

	fileName := string(t.GetField(fieldFileName).Data)
	filePath := "./" + cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFilePath).Data)
	fileNewPath := "./" + cc.Server.Config.FileRoot + ReadFilePath(t.GetField(fieldFileNewPath).Data)

	logger.Debugw("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	err := os.Rename(filePath+"/"+fileName, fileNewPath+"/"+fileName)
	if os.IsNotExist(err) {
		_, err := cc.Connection.Write(t.ReplyError("Cannot delete file " + fileName + " because it does not exist or cannot be found."))
		return []Transaction{}, err
	}
	if err != nil {
		return []Transaction{}, err
	}
	// TODO: handle other possible errors; e.g. file delete fails due to file permission issue

	err = cc.Reply(t)
	return []Transaction{}, err
}

func HandleNewFolder(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleSetUser(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	var response []Transaction
	userLogin := DecodeUserString(t.GetField(fieldUserLogin).Data)
	userName := string(t.GetField(fieldUserName).Data)

	newAccessLvl := t.GetField(fieldUserAccess).Data

	account := cc.Server.Accounts[userLogin]
	account.Access = &newAccessLvl
	account.Name = userName

	if len(t.GetField(fieldUserPassword).Data) > 1 {
		account.Password = hashAndSalt(t.GetField(fieldUserPassword).Data)
	}

	file := cc.Server.ConfigDir + "Users/" + userLogin + ".yaml"
	out, _ := yaml.Marshal(&account)
	if err := ioutil.WriteFile(file, out, 0666); err != nil {
		return []Transaction{}, err
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.Clients {
		if c.Account.Login == userLogin {
			// TODO: Re-enable this
			//newT := NewTransaction(
			//	tranUserAccess, 333, []Field{NewField(fieldUserAccess, newAccessLvl)},
			//)
			//
			//response = append(response, egressTransaction{
			//	ClientID:    c.ID,
			//	Transaction: &newT,
			//})

			flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*c.Flags)))

			if c.Authorize(accessDisconUser) == true {
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
				panic(err)
			}
		}
	}

	// TODO: If we have just promoted a connected user to admin, notify
	// connected clients to turn the user red

	err := cc.Reply(t)
	return response, err
}

func HandleGetUser(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	userLogin := string(t.GetField(fieldUserLogin).Data)
	decodedUserLogin := NegatedUserString(t.GetField(fieldUserLogin).Data)
	account := cc.Server.Accounts[userLogin]

	err := cc.Reply(t,
		NewField(fieldUserName, []byte(account.Name)),
		NewField(fieldUserLogin, []byte(decodedUserLogin)),
		NewField(fieldUserPassword, []byte(account.Password)),
		NewField(fieldUserAccess, *account.Access),
	)
	return []Transaction{}, err
}

func HandleListUsers(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	if cc.Authorize(accessOpenUser) == false {
		_, err := cc.Connection.Write(t.ReplyError("You are not allowed to view accounts."))
		return []Transaction{}, err
	}
	var userFields []Field
	for _, acc := range cc.Server.Accounts {
		userField := acc.Payload()
		userFields = append(userFields, NewField(fieldData, userField))
	}

	err := cc.Reply(t, userFields...)
	return []Transaction{}, err
}

// HandleNewUser creates a new user account
func HandleNewUser(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	if cc.Authorize(accessCreateUser) == false {
		_, err := cc.Connection.Write(t.ReplyError("You are not allowed to create new accounts."))
		return []Transaction{}, err
	}

	login := DecodeUserString(t.GetField(fieldUserLogin).Data)

	// If the account already exists, reply with an error
	if _, ok := cc.Server.Accounts[login]; ok {
		_, err := cc.Connection.Write(t.ReplyError("Cannot create account " + login + " because there is already an account with that login."))
		return []Transaction{}, err
	}

	if err := cc.Server.NewUser(
		login,
		string(t.GetField(fieldUserName).Data),
		string(t.GetField(fieldUserPassword).Data),
		t.GetField(fieldUserAccess).Data,
	); err != nil {
		return []Transaction{}, err
	}

	_, err := cc.Connection.Write(t.ReplyTransaction([]Field{}).Payload())

	return []Transaction{}, err
}

func HandleDeleteUser(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	if cc.Authorize(accessDeleteUser) == false {
		_, err := cc.Connection.Write(t.ReplyError("You are not allowed to delete accounts."))
		return []Transaction{}, err
	}

	// TODO: Handle case where account doesn't exist; e.g. delete race condition

	login := DecodeUserString(t.GetField(fieldUserLogin).Data)

	if err := cc.Server.DeleteUser(login); err != nil {
		return []Transaction{}, err
	}

	err := cc.Reply(t)
	return []Transaction{}, err
}

// HandleUserBroadcast sends an Administrator Message to all connected clients of the server
func HandleUserBroadcast(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	cc.NotifyOthers(
		NewTransaction(
			tranServerMsg, 0,
			[]Field{
				NewField(fieldData, t.GetField(tranGetMsgs).Data),
				NewField(fieldChatOptions, []byte{0}),
			},
		),
	)

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleGetClientConnInfoText(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	clientConn := cc.Server.Clients[binary.BigEndian.Uint16(t.GetField(fieldUserID).Data)]

	// TODO: Implement non-hardcoded values
	template := `Nickname:   %s
Name:       %v
Account:    guest
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
	template = fmt.Sprintf(template, *clientConn.UserName, *clientConn.ID, clientConn.Connection.RemoteAddr().String())
	template = strings.Replace(template, "\n", "\r", -1)

	err := cc.Reply(
		t,
		NewField(fieldData, []byte(template)),
		NewField(fieldUserName, *clientConn.UserName),
	)
	return []Transaction{}, err
}

func HandleGetUserNameList(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	reply := t.ReplyTransaction(cc.Server.connectedUsers())
	reply.clientID = cc.ID

	return []Transaction{reply}, nil
}

func (cc *ClientConn) notifyNewUserHasJoined() ([]Transaction, error) {
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

	return []Transaction{}, nil
}

func HandleTranAgreed(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, *cc.Server.NextGuestID)

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
	if err := cc.Reply(t); err != nil {
		return []Transaction{}, err
	}

	return []Transaction{}, nil
}

func HandleTranOldPostNews(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	current := time.Now()
	formattedDate := fmt.Sprintf("%s%02d %d:%d", current.Month().String()[:3], current.Day(), current.Hour(), current.Minute())
	// TODO: format news post
	newsPost := fmt.Sprintf(newsTemplate, *cc.UserName, formattedDate, t.GetField(fieldData).Data)
	newsPost = strings.Replace(newsPost, "\n", "\r", -1)

	cc.Server.FlatNews = append([]byte(newsPost), cc.Server.FlatNews...)

	if _, err := cc.Connection.Write(t.ReplyTransaction([]Field{}).Payload()); err != nil {
		return []Transaction{}, err
	}

	err := ioutil.WriteFile(cc.Server.ConfigDir+"MessageBoard.txt", cc.Server.FlatNews, 0644)
	if err != nil {
		return []Transaction{}, err
	}

	_ = cc.Server.NotifyAll(
		NewTransaction(
			tranNewMsg, 0,
			[]Field{
				NewField(fieldData, []byte(newsPost)),
			},
		),
	)
	// Notify all clients of updated news
	return []Transaction{}, nil
}

func HandleDisconnectUser(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	if cc.Authorize(accessDisconUser) == false {
		// TODO: Reply with server message:
		// msg := "You are not allowed to disconnect users."
		return []Transaction{}, nil
	}

	clientConn := cc.Server.Clients[binary.BigEndian.Uint16(t.GetField(fieldUserID).Data)]

	if err := clientConn.Connection.Close(); err != nil {
		return []Transaction{}, err
	}

	_, err := cc.Connection.Write(
		t.ReplyTransaction([]Field{}).Payload(),
	)

	return []Transaction{}, err
}

func HandleGetNewsCatNameList(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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

	err := cc.Reply(t, fieldData...)
	return []Transaction{}, err
}

func HandleNewNewsCat(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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

	_ = cc.Server.WriteThreadedNews()
	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleNewNewsFldr(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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
	_ = cc.Server.WriteThreadedNews()

	err := cc.Reply(t)
	return []Transaction{}, err
}

// Fields used in the request:
// 325	News path	Optional
//
// Reply fields:
// 321	News article list data	Optional
func HandleGetNewsArtNameList(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	var cat NewsCategoryListData15
	cats := cc.Server.ThreadedNews.Categories

	for _, path := range pathStrs {
		cat = cats[path]
		cats = cats[path].SubCats
	}

	nald := cat.GetNewsArtListData()

	err := cc.Reply(t, NewField(fieldNewsArtListData, nald.Payload()))
	return []Transaction{}, err
}

func HandleGetNewsArtData(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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
		cc.Reply(t)
		return []Transaction{}, nil
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

	fields := []Field{
		NewField(fieldNewsArtTitle, []byte(art.Title)),
		NewField(fieldNewsArtPoster, []byte(art.Poster)),
		NewField(fieldNewsArtDate, art.Date),

		NewField(fieldNewsArtPrevArt, art.PrevArt),
		NewField(fieldNewsArtNextArt, art.NextArt),
		NewField(fieldNewsArtParentArt, art.ParentArt),
		NewField(fieldNewsArt1stChildArt, art.FirstChildArt),
		NewField(fieldNewsArtDataFlav, []byte("text/plain")),
		NewField(fieldNewsArtData, []byte(art.Data)),
	}

	err := cc.Reply(t, fields...)
	return []Transaction{}, err
}

func HandleDelNewsItem(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	// Access:		News Delete Folder (37) or News Delete Category (35)

	pathStrs := ReadNewsPath(t.GetField(fieldNewsPath).Data)

	cc.Server.Logger.Infof("DelNewsItem %v", pathStrs)

	cats := cc.Server.ThreadedNews.Categories

	delName := pathStrs[len(pathStrs)-1]
	if len(pathStrs) > 1 {
		for _, path := range pathStrs[0 : len(pathStrs)-1] {
			cats = cats[path].SubCats
		}
	}

	delete(cats, delName)

	cc.Server.WriteThreadedNews()

	// Reply params: none
	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleDelNewsArt(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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
	if err := cc.Server.WriteThreadedNews(); err != nil {
		return []Transaction{}, err
	}

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandlePostNewsArt(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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
	cc.Server.WriteThreadedNews()

	err := cc.Reply(t)
	return []Transaction{}, err
}

func HandleGetMsgs(cc *ClientConn, transaction *Transaction) ([]Transaction, error) {
	_, err := cc.Connection.Write(
		transaction.ReplyTransaction(
			[]Field{NewField(fieldData, cc.Server.FlatNews)},
		).Payload(),
	)

	return []Transaction{}, err
}

func HandleDownloadFile(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	fileName := t.GetField(fieldFileName).Data
	filePath := ReadFilePath(t.GetField(fieldFilePath).Data)

	ffo, _ := NewFlattenedFileObject(
		cc.Server.Config.FileRoot+filePath,
		string(fileName),
	)

	transactionRef := cc.Server.NewTransactionRef()
	data := binary.BigEndian.Uint32(transactionRef)

	fileTransfer := &FileTransfer{
		FileName:        fileName,
		FilePath:        []byte(filePath),
		ReferenceNumber: transactionRef,
		Type:            FileDownload,
	}

	cc.Server.FileTransfers[data] = fileTransfer

	_, err := cc.Connection.Write(
		t.ReplyTransaction(
			[]Field{
				NewField(fieldRefNum, transactionRef),
				NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
				NewField(fieldTransferSize, ffo.TransferSize()),
				NewField(fieldFileSize, ffo.FlatFileDataForkHeader.DataSize),
			},
		).Payload(),
	)
	if err != nil {
		return []Transaction{}, err
	}

	return []Transaction{}, err
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
func HandleDownloadFolder(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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
	transferSize, _ := CalcTotalSize(fullFilePath)
	itemCount, _ := CalcItemCount(fullFilePath)

	err := cc.Reply(t,
		NewField(fieldRefNum, transactionRef),
		NewField(fieldTransferSize, transferSize),
		NewField(fieldFolderItemCount, itemCount),       // TODO: Remove hardcode
		NewField(fieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	)
	return []Transaction{}, err
}

// Upload all files from the local folder and its subfolders to the specified path on the server
// Fields used in the request
// 201	File name
// 202	File path
// 108	Transfer size	Total size of all items in the folder
// 220	Folder item count
// 204	File transfer options	"Optional Currently set to 1" (TODO: ??)
func HandleUploadFolder(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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

	err := cc.Reply(t, NewField(fieldRefNum, transactionRef))
	return []Transaction{}, err
}

func HandleUploadFile(cc *ClientConn, t *Transaction) ([]Transaction, error) {
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

	err := cc.Reply(t, NewField(fieldRefNum, transactionRef))
	return []Transaction{}, err
}

const refusePM = 0
const refuseChat = 1
const autoResponse = 2

func HandleSetClientUserInfo(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	*cc.Icon = t.GetField(fieldUserIconID).Data
	*cc.UserName = t.GetField(fieldUserName).Data

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

	return []Transaction{}, nil
}

func HandleKeepAlive(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	// HL 1.9.2 Client sends keepalive msg every 3 minutes
	// HL 1.2.3 Client doesn't send keepalives
	err := cc.Reply(t)

	return []Transaction{}, err
}

func HandleGetFileNameList(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	filePath := cc.Server.Config.FileRoot

	fieldFilePath := t.GetField(fieldFilePath).Data
	if len(fieldFilePath) > 0 {
		filePath = cc.Server.Config.FileRoot + ReadFilePath(fieldFilePath)
	}

	_, err := cc.Connection.Write(
		t.ReplyTransaction(
			GetFileNameList(filePath),
		).Payload(),
	)

	return []Transaction{}, err
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
func HandleInviteNewChat(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	// Client to Invite
	targetID := t.GetField(fieldUserID).Data

	newChatID := cc.Server.NewPrivateChat(cc)

	inviteOther := NewTransaction(
		tranInviteToChat, 0,
		[]Field{
			NewField(fieldChatID, newChatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
		},
	)
	if err := cc.notifyOtherClientConn(targetID, inviteOther); err != nil {
		return []Transaction{}, err
	}

	err := cc.Reply(t,
		NewField(fieldChatID, newChatID),
		NewField(fieldUserName, *cc.UserName),
		NewField(fieldUserID, *cc.ID),
		NewField(fieldUserIconID, *cc.Icon),
		NewField(fieldUserFlags, *cc.Flags),
	)
	return []Transaction{}, err
}

func HandleInviteToChat(cc *ClientConn, t *Transaction) ([]Transaction, error) {
	// Client to Invite
	targetID := t.GetField(fieldUserID).Data
	chatID := t.GetField(fieldChatID).Data

	inviteOther := NewTransaction(
		tranInviteToChat, 0,
		[]Field{
			NewField(fieldChatID, chatID),
			NewField(fieldUserName, *cc.UserName),
			NewField(fieldUserID, *cc.ID),
		},
	)
	if err := cc.notifyOtherClientConn(targetID, inviteOther); err != nil {
		return []Transaction{}, err
	}

	_, err := cc.Connection.Write(
		t.ReplyTransaction(
			[]Field{
				NewField(fieldChatID, chatID),
				NewField(fieldUserName, *cc.UserName),
				NewField(fieldUserID, *cc.ID),
				NewField(fieldUserIconID, *cc.Icon),
				NewField(fieldUserFlags, *cc.Flags),
			},
		).Payload(),
	)

	return []Transaction{}, err
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
