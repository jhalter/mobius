package hotline

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/jhalter/mobius/concat"
	"math/rand"
)

const (
	tranError          = 0
	tranGetMsgs        = 101
	tranNewMsg         = 102
	tranOldPostNews    = 103
	tranServerMsg      = 104
	tranChatSend       = 105
	tranChatMsg        = 106
	tranLogin          = 107
	tranSendInstantMsg = 108
	tranShowAgreement  = 109
	tranDisconnectUser = 110
	// tranDisconnectMsg        = 111 TODO: implement friendly disconnect
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
	// tranDownloadInfo         = 211 TODO: implement file transfer queue
	// tranDownloadBanner     = 212 TODO: figure out what this is used for
	tranUploadFldr         = 213
	tranGetUserNameList    = 300
	tranNotifyChangeUser   = 301
	tranNotifyDeleteUser   = 302
	tranGetClientInfoText  = 303
	tranSetClientUserInfo  = 304
	tranListUsers          = 348
	tranUpdateUser         = 349
	tranNewUser            = 350
	tranDeleteUser         = 351
	tranGetUser            = 352
	tranSetUser            = 353
	tranUserAccess         = 354
	tranUserBroadcast      = 355
	tranGetNewsCatNameList = 370
	tranGetNewsArtNameList = 371
	tranDelNewsItem        = 380
	tranNewNewsFldr        = 381
	tranNewNewsCat         = 382
	tranGetNewsArtData     = 400
	tranPostNewsArt        = 410
	tranDelNewsArt         = 411
	tranKeepAlive          = 500
)

type Transaction struct {
	clientID *[]byte

	Flags      byte   // Reserved (should be 0)
	IsReply    byte   // Request (0) or reply (1)
	Type       []byte // Requested operation (user defined)
	ID         []byte // Unique transaction ID (must be != 0)
	ErrorCode  []byte // Used in the reply (user defined, 0 = no error)
	TotalSize  []byte // Total data size for the transaction (all parts)
	DataSize   []byte // Size of data in this transaction part. This allows splitting large transactions into smaller parts.
	ParamCount []byte // Number of the parameters for this transaction
	Fields     []Field
}

func NewTransaction(t int, clientID *[]byte, fields ...Field) *Transaction {
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

// ReadTransaction parses a byte slice into a struct.  The input slice may be shorter or longer
// that the transaction size depending on what was read from the network connection.
func ReadTransaction(buf []byte) (*Transaction, int, error) {
	totalSize := binary.BigEndian.Uint32(buf[12:16])

	// the buf may include extra bytes that are not part of the transaction
	// tranLen represents the length of bytes that are part of the transaction
	tranLen := int(20 + totalSize)

	if tranLen > len(buf) {
		return nil, 0, errors.New("buflen too small for tranLen")
	}
	fields, err := ReadFields(buf[20:22], buf[22:tranLen])
	if err != nil {
		return nil, 0, err
	}

	return &Transaction{
		Flags:      buf[0],
		IsReply:    buf[1],
		Type:       buf[2:4],
		ID:         buf[4:8],
		ErrorCode:  buf[8:12],
		TotalSize:  buf[12:16],
		DataSize:   buf[16:20],
		ParamCount: buf[20:22],
		Fields:     fields,
	}, tranLen, nil
}

func readTransactions(buf []byte) ([]Transaction, int, error) {
	var transactions []Transaction

	bufLen := len(buf)

	var bytesRead = 0
	for bytesRead < bufLen {
		t, tReadLen, err := ReadTransaction(buf[bytesRead:])
		if err != nil {
			return transactions, bytesRead, err
		}
		bytesRead += tReadLen

		transactions = append(transactions, *t)
	}

	return transactions, bytesRead, nil
}

const minFieldLen = 4

func ReadFields(paramCount []byte, buf []byte) ([]Field, error) {
	paramCountInt := int(binary.BigEndian.Uint16(paramCount))
	if paramCountInt > 0 && len(buf) < minFieldLen {
		return []Field{}, fmt.Errorf("invalid field length %v", len(buf))
	}

	// A Field consists of:
	// ID: 2 bytes
	// Size: 2 bytes
	// Data: FieldSize number of bytes
	var fields []Field
	for i := 0; i < paramCountInt; i++ {
		if len(buf) < minFieldLen {
			return []Field{}, fmt.Errorf("invalid field length %v", len(buf))
		}
		fieldID := buf[0:2]
		fieldSize := buf[2:4]
		fieldSizeInt := int(binary.BigEndian.Uint16(buf[2:4]))
		expectedLen := minFieldLen + fieldSizeInt
		if len(buf) < expectedLen {
			return []Field{}, fmt.Errorf("field length too short")
		}

		fields = append(fields, Field{
			ID:        fieldID,
			FieldSize: fieldSize,
			Data:      buf[4 : 4+fieldSizeInt],
		})

		buf = buf[fieldSizeInt+4:]
	}

	if len(buf) != 0 {
		return []Field{}, fmt.Errorf("extra field bytes")
	}

	return fields, nil
}

func (t *Transaction) MarshalBinary() (data []byte, err error) {
	payloadSize := t.Size()

	fieldCount := make([]byte, 2)
	binary.BigEndian.PutUint16(fieldCount, uint16(len(t.Fields)))

	var fieldPayload []byte
	for _, field := range t.Fields {
		fieldPayload = append(fieldPayload, field.Payload()...)
	}

	return concat.Slices(
		[]byte{t.Flags, t.IsReply},
		t.Type,
		t.ID,
		t.ErrorCode,
		payloadSize,
		payloadSize, // this is the dataSize field, but seeming the same as totalSize
		fieldCount,
		fieldPayload,
	), err
}

// Size returns the total size of the transaction payload
func (t *Transaction) Size() []byte {
	bs := make([]byte, 4)

	fieldSize := 0
	for _, field := range t.Fields {
		fieldSize += len(field.Data) + 4
	}

	binary.BigEndian.PutUint32(bs, uint32(fieldSize+2))

	return bs
}

func (t *Transaction) GetField(id int) Field {
	for _, field := range t.Fields {
		if id == int(binary.BigEndian.Uint16(field.ID)) {
			return field
		}
	}

	return Field{}
}
