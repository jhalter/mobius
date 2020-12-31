package hotline

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const tranError = 0
const tranGetMsgs = 101
const tranNewMsg = 102
const tranOldPostNews = 103
const tranServerMsg = 104
const tranChatSend = 105
const tranChatMsg = 106
const tranLogin = 107
const tranSendInstantMsg = 108
const tranShowAgreement = 109
const tranDisconnectUser = 110
const tranDisconnectMsg = 111
const tranInviteNewChat = 112
const tranInviteToChat = 113
const tranRejectChatInvite = 114
const tranJoinChat = 115
const tranLeaveChat = 116
const tranNotifyChatChangeUser = 117
const tranNotifyChatDeleteUser = 118
const tranNotifyChatSubject = 119
const tranSetChatSubject = 120
const tranAgreed = 121
const tranGetFileNameList = 200
const tranDownloadFile = 202
const tranUploadFile = 203
const tranNewFolder = 205
const tranDeleteFile = 204
const tranGetFileInfo = 206
const tranSetFileInfo = 207
const tranMoveFile = 208
const tranMakeFileAlias = 209
const tranDownloadFldr = 210
const tranDownloadInfo = 211
const tranDownloadBanner = 212
const tranUploadFldr = 213
const tranGetUserNameList = 300
const tranNotifyChangeUser = 301
const tranNotifyDeleteUser = 302
const tranGetClientInfoText = 303
const tranSetClientUserInfo = 304
const tranListUsers = 348  // Undocumented?
const tranUpdateUser = 349 // Undocumented?
const tranNewUser = 350
const tranDeleteUser = 351
const tranGetUser = 352
const tranSetUser = 353
const tranUserAccess = 354
const tranUserBroadcast = 355
const tranGetNewsCatNameList = 370
const tranGetNewsArtNameList = 371
const tranDelNewsItem = 380
const tranNewNewsFldr = 381
const tranNewNewsCat = 382
const tranGetNewsArtData = 400
const tranPostNewsArt = 410
const tranDelNewsArt = 411
const tranKeepAlive = 500

type TransactionType struct {
	Handler func(cc *ClientConn, transaction *Transaction) error
	Name    string
}

var TransactionHandlers = map[uint16]TransactionType{
	tranAgreed:             {Name: "tranAgreed", Handler: HandleTranAgreed},
	tranGetFileInfo:        {Name: "tranGetFileInfo", Handler: HandleGetFileInfo},
	tranDownloadFile:       {Name: "tranDownloadFile", Handler: HandleDownloadFile},
	tranUploadFile:         {Name: "tranUploadFile", Handler: HandleUploadFile},
	tranSetClientUserInfo:  {Name: "tranSetClientUserInfo", Handler: HandleSetClientUserInfo},
	tranNewNewsFldr:        {Name: "tranNewNewsFldr", Handler: HandleNewNewsFldr},
	tranPostNewsArt:        {Name: "tranPostNewsArt", Handler: HandlePostNewsArt},
	tranKeepAlive:          {Name: "tranKeepAlive", Handler: HandleKeepAlive},
	tranGetFileNameList:    {Name: "tranGetFileNameList", Handler: HandleGetFileNameList},
	tranDisconnectUser:     {Name: "tranDisconnectUser", Handler: HandleDisconnectUser},
	tranOldPostNews:        {Name: "tranOldPostNews", Handler: HandleTranOldPostNews},
	tranGetUser:            {Name: "tranGetUser", Handler: HandleGetUser},
	tranSetUser:            {Name: "tranSetUser", Handler: HandleSetUser},
	tranNewUser:            {Name: "tranNewUser", Handler: HandleNewUser},
	tranDeleteUser:         {Name: "tranDeleteUser", Handler: HandleDeleteUser},
	tranNewFolder:          {Name: "tranNewFolder", Handler: HandleNewFolder},
	tranDeleteFile:         {Name: "tranDeleteFile", Handler: HandleDeleteFile},
	tranSendInstantMsg:     {Name: "tranSendInstantMsg", Handler: HandleSendInstantMsg},
	tranChatSend:           {Name: "tranChatSend", Handler: HandleChatSend},
	tranGetMsgs:            {Name: "tranGetMsgs", Handler: HandleGetMsgs},
	tranUserBroadcast:      {Name: "tranUserBroadcast", Handler: HandleUserBroadcast},
	tranGetClientInfoText:  {Name: "tranGetClientInfoText", Handler: HandleGetClientConnInfoText},
	tranGetUserNameList:    {Name: "tranHandleGetUserNameList", Handler: HandleGetUserNameList},
	tranGetNewsCatNameList: {Name: "tranGetNewsCatNameList", Handler: HandleGetNewsCatNameList},
	tranGetNewsArtNameList: {Name: "tranGetNewsArtNameList", Handler: HandleGetNewsArtNameList},
	tranGetNewsArtData:     {Name: "tranGetNewsArtData", Handler: HandleGetNewsArtData},
	tranDelNewsItem:        {Name: "tranDelNewsItem", Handler: HandleDelNewsItem},
	tranDelNewsArt:         {Name: "tranDelNewsArt", Handler: HandleDelNewsArt},
	tranNewNewsCat:         {Name: "tranNewNewsCat", Handler: HandleNewNewsCat},
	tranListUsers:          {Name: "tranListUsers", Handler: HandleListUsers},
	tranInviteNewChat:      {Name: "tranInviteNewChat", Handler: HandleInviteNewChat},
	tranRejectChatInvite:   {Name: "tranRejectChatInvite", Handler: HandleRejectChatInvite},
	tranJoinChat:           {Name: "tranJoinChat", Handler: HandleJoinChat},
	tranLeaveChat:          {Name: "tranJoinChat", Handler: HandleLeaveChat},
	tranSetChatSubject:     {Name: "tranSetChatSubject", Handler: HandleSetChatSubject},
	tranDownloadFldr:       {Name: "tranDownloadFldr", Handler: HandleDownloadFolder},
	tranUploadFldr:         {Name: "tranUploadFldr", Handler: HandleUploadFolder},
}

type Transaction struct {
	Flags      byte
	IsReply    byte
	Type       []byte // Size 2
	ID         []byte // Size 4
	ErrorCode  []byte // Size 4
	TotalSize  []byte // Total size of transaction in bytes
	DataSize   []byte // Size of the data section of transaction in bytes
	ParamCount []byte // Number of fields in transaction data
	Fields     []Field
}

func NewTransaction(t, id int, f []Field) Transaction {
	typeSlice := make([]byte, 2)
	binary.BigEndian.PutUint16(typeSlice, uint16(t))

	idSlice := make([]byte, 4)
	binary.BigEndian.PutUint32(idSlice, uint32(id))

	return Transaction{
		Flags:     0x00,
		IsReply:   0x00,
		Type:      typeSlice,
		ID:        idSlice,
		ErrorCode: []byte{0, 0, 0, 0},
		Fields:    f,
	}
}

// ReadTransaction parses a byte slice into a struct
func ReadTransaction(buf []byte) *Transaction {
	return &Transaction{
		Flags:      buf[0],
		IsReply:    buf[1],
		Type:       buf[2:4],
		ID:         buf[4:8],
		ErrorCode:  buf[8:12],
		TotalSize:  buf[12:16],
		DataSize:   buf[16:20],
		ParamCount: buf[20:22],
		Fields:     ReadFields(buf[20:22], buf[22:]),
	}
}

func ReadTransactions(buf []byte) []Transaction {
	var transactions []Transaction

	bufLen := len(buf)

	var bytesRead = 0
	for bytesRead < bufLen {
		t := ReadTransaction(buf[bytesRead:])
		bytesRead += len(t.Payload())

		transactions = append(transactions, *t)
	}

	return transactions
}

func FindTransactions(id uint16, transactions []Transaction) (Transaction, error) {
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, id)

	for _, t := range transactions {
		fmt.Printf("got: %#v, want: %#v\n", t.Type, bs)
		if bytes.Compare(t.Type, bs) == 0 {
			return t, nil
		}
	}

	return Transaction{}, fmt.Errorf("transaction type %v not found", id)
}

func ReadFields(paramCount []byte, buf []byte) []Field {
	paramCountInt := int(binary.BigEndian.Uint16(paramCount))

	// A Field consists of:
	// ID: 2 bytes
	// Size: 2 bytes
	// Data: FieldSize number of bytes
	var fields []Field
	for i := 0; i < paramCountInt; i++ {
		fieldID := buf[0:2]
		fieldSize := buf[2:4]
		fieldSizeInt := int(binary.BigEndian.Uint16(buf[2:4]))
		fieldData := buf[4 : 4+fieldSizeInt]

		fields = append(fields, Field{
			ID:        fieldID,
			FieldSize: fieldSize,
			Data:      fieldData,
		})

		buf = buf[fieldSizeInt+4:]
	}

	return fields
}

func (t Transaction) Payload() []byte {
	ts := t.CalcTotalSize()
	ds := t.CalcTotalSize()

	paramCount := make([]byte, 2)
	binary.BigEndian.PutUint16(paramCount, uint16(len(t.Fields)))

	payload := []byte{
		t.Flags,
		t.IsReply,
		t.Type[0], t.Type[1],
		t.ID[0], t.ID[1], t.ID[2], t.ID[3],
		t.ErrorCode[0], t.ErrorCode[1], t.ErrorCode[2], t.ErrorCode[3],
		ts[0], ts[1], ts[2], ts[3],
		ds[0], ds[1], ds[2], ds[3],
		paramCount[0], paramCount[1],
	}

	for _, field := range t.Fields {
		payload = append(payload, field.Payload()...)
	}

	return payload
}

// total size
func (t Transaction) CalcTotalSize() []byte {
	bs := make([]byte, 4)

	fieldSize := 0
	for _, field := range t.Fields {
		fieldSize += len(field.Data) + 4
	}

	binary.BigEndian.PutUint32(bs, uint32(fieldSize+2))

	return bs
}

func (t Transaction) GetField(id int) Field {
	for _, field := range t.Fields {
		if id == int(binary.BigEndian.Uint16(field.ID)) {
			return field
		}
	}

	// TODO: return an err if no fields found
	return Field{}
}

func (t Transaction) GetFields(id int) []Field {
	var fields []Field
	for _, field := range t.Fields {
		if id == int(binary.BigEndian.Uint16(field.ID)) {
			fields = append(fields, field)
		}
	}

	return fields
}

func (t Transaction) ReplyTransaction(f []Field) Transaction {
	return Transaction{
		Flags:     0x00,
		IsReply:   0x01,
		Type:      t.Type,
		ID:        t.ID,
		ErrorCode: []byte{0, 0, 0, 0},
		Fields:    f,
	}
}

func (t Transaction) ReplyError(errMsg string) []byte {
	return Transaction{
		Flags:     0x00,
		IsReply:   0x01,
		Type:      []byte{0, 0},
		ID:        t.ID,
		ErrorCode: []byte{0, 0, 0, 1},
		Fields: []Field{
			NewField(fieldError, []byte(errMsg)),
		},
	}.Payload()
}
