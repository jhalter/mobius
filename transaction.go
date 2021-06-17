package hotline

import (
	"encoding/binary"
	"math/rand"
)

const (
	tranError                = 0
	tranGetMsgs              = 101
	tranNewMsg               = 102
	tranOldPostNews          = 103
	tranServerMsg            = 104
	tranChatSend             = 105
	tranChatMsg              = 106
	tranLogin                = 107
	tranSendInstantMsg       = 108
	tranShowAgreement        = 109
	tranDisconnectUser       = 110
	tranDisconnectMsg        = 111
	tranInviteNewChat        = 112
	tranInviteToChat         = 113
	tranRejectChatInvite     = 114
	tranJoinChat             = 115
	tranLeaveChat            = 116
	tranNotifyChatChangeUser = 117
	tranNotifyChatDeleteUser = 118
	tranNotifyChatSubject    = 119
	tranSetChatSubject       = 120
	tranAgreed               = 121
	tranGetFileNameList      = 200
	tranDownloadFile         = 202
	tranUploadFile           = 203
	tranNewFolder            = 205
	tranDeleteFile           = 204
	tranGetFileInfo          = 206
	tranSetFileInfo          = 207
	tranMoveFile             = 208
	tranMakeFileAlias        = 209
	tranDownloadFldr         = 210
	tranDownloadInfo         = 211
	tranDownloadBanner       = 212
	tranUploadFldr           = 213
	tranGetUserNameList      = 300
	tranNotifyChangeUser     = 301
	tranNotifyDeleteUser     = 302
	tranGetClientInfoText    = 303
	tranSetClientUserInfo    = 304
	tranListUsers            = 348 // Undocumented?
	tranUpdateUser           = 349 // Undocumented?
	tranNewUser              = 350
	tranDeleteUser           = 351
	tranGetUser              = 352
	tranSetUser              = 353
	tranUserAccess           = 354
	tranUserBroadcast        = 355
	tranGetNewsCatNameList   = 370
	tranGetNewsArtNameList   = 371
	tranDelNewsItem          = 380
	tranNewNewsFldr          = 381
	tranNewNewsCat           = 382
	tranGetNewsArtData       = 400
	tranPostNewsArt          = 410
	tranDelNewsArt           = 411
	tranKeepAlive            = 500
)

type Transaction struct {
	clientID *[]byte

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

func NewNewTransaction(t int, clientID *[]byte, fields ...Field) *Transaction {
	typeSlice := make([]byte, 2)
	binary.BigEndian.PutUint16(typeSlice, uint16(t))

	idSlice := make([]byte, 4)
	binary.BigEndian.PutUint32(idSlice, rand.Uint32())

	return &Transaction{
		clientID:  clientID,
		Flags:     0x00,
		IsReply:   0x00,
		Type:      typeSlice,
		ID:        idSlice,
		ErrorCode: []byte{0, 0, 0, 0},
		Fields:    fields,
	}
}

func NewTransaction(t, _ int, f []Field) Transaction {
	typeSlice := make([]byte, 2)
	binary.BigEndian.PutUint16(typeSlice, uint16(t))

	idSlice := make([]byte, 4)
	binary.BigEndian.PutUint32(idSlice, rand.Uint32())

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

func (t *Transaction) uint32ID() uint32 {
	return binary.BigEndian.Uint32(t.ID)
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

//func FindTransactions(id uint16, transactions []Transaction) (Transaction, error) {
//	bs := make([]byte, 2)
//	binary.BigEndian.PutUint16(bs, id)
//
//	for _, t := range transactions {
//		fmt.Printf("got: %#v, want: %#v\n", t.Type, bs)
//		if bytes.Compare(t.Type, bs) == 0 {
//			return t, nil
//		}
//	}
//
//	return Transaction{}, fmt.Errorf("transaction type %v not found", id)
//}

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

//func (t Transaction) GetFields(id int) []Field {
//	var fields []Field
//	for _, field := range t.Fields {
//		if id == int(binary.BigEndian.Uint16(field.ID)) {
//			fields = append(fields, field)
//		}
//	}
//
//	return fields
//}

//func (t Transaction) reply(f ...Field) *Transaction {
//	return &Transaction{
//		Flags:     0x00,
//		IsReply:   0x01,
//		Type:      t.Type,
//		ID:        t.ID,
//		ErrorCode: []byte{0, 0, 0, 0},
//		Fields:    f,
//	}
//}

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

// TODO: remove deprecated func
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

func (t Transaction) NewErrorReply(clientID *[]byte, errMsg string) *Transaction {
	return &Transaction{
		clientID:  clientID,
		Flags:     0x00,
		IsReply:   0x01,
		Type:      []byte{0, 0},
		ID:        t.ID,
		ErrorCode: []byte{0, 0, 0, 1},
		Fields: []Field{
			NewField(fieldError, []byte(errMsg)),
		},
	}
}
