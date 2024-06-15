package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"slices"
)

const (
	TranError                = 0
	TranGetMsgs              = 101
	TranNewMsg               = 102
	TranOldPostNews          = 103
	TranServerMsg            = 104
	TranChatSend             = 105
	TranChatMsg              = 106
	TranLogin                = 107
	TranSendInstantMsg       = 108
	TranShowAgreement        = 109
	TranDisconnectUser       = 110
	TranDisconnectMsg        = 111 // TODO: implement server initiated friendly disconnect
	TranInviteNewChat        = 112
	TranInviteToChat         = 113
	TranRejectChatInvite     = 114
	TranJoinChat             = 115
	TranLeaveChat            = 116
	TranNotifyChatChangeUser = 117
	TranNotifyChatDeleteUser = 118
	TranNotifyChatSubject    = 119
	TranSetChatSubject       = 120
	TranAgreed               = 121
	TranServerBanner         = 122
	TranGetFileNameList      = 200
	TranDownloadFile         = 202
	TranUploadFile           = 203
	TranNewFolder            = 205
	TranDeleteFile           = 204
	TranGetFileInfo          = 206
	TranSetFileInfo          = 207
	TranMoveFile             = 208
	TranMakeFileAlias        = 209
	TranDownloadFldr         = 210
	TranDownloadInfo         = 211 // TODO: implement file transfer queue
	TranDownloadBanner       = 212
	TranUploadFldr           = 213
	TranGetUserNameList      = 300
	TranNotifyChangeUser     = 301
	TranNotifyDeleteUser     = 302
	TranGetClientInfoText    = 303
	TranSetClientUserInfo    = 304
	TranListUsers            = 348
	TranUpdateUser           = 349
	TranNewUser              = 350
	TranDeleteUser           = 351
	TranGetUser              = 352
	TranSetUser              = 353
	TranUserAccess           = 354
	TranUserBroadcast        = 355
	TranGetNewsCatNameList   = 370
	TranGetNewsArtNameList   = 371
	TranDelNewsItem          = 380
	TranNewNewsFldr          = 381
	TranNewNewsCat           = 382
	TranGetNewsArtData       = 400
	TranPostNewsArt          = 410
	TranDelNewsArt           = 411
	TranKeepAlive            = 500
)

type Transaction struct {
	Flags      byte   // Reserved (should be 0)
	IsReply    byte   // Request (0) or reply (1)
	Type       []byte // Requested operation (user defined)
	ID         []byte // Unique transaction ID (must be != 0)
	ErrorCode  []byte // Used in the reply (user defined, 0 = no error)
	TotalSize  []byte // Total data size for the transaction (all parts)
	DataSize   []byte // Size of data in this transaction part. This allows splitting large transactions into smaller parts.
	ParamCount []byte // Number of the parameters for this transaction
	Fields     []Field

	clientID   *[]byte // Internal identifier for target client
	readOffset int     // Internal offset to track read progress
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

// Write implements io.Writer interface for Transaction
func (t *Transaction) Write(p []byte) (n int, err error) {
	totalSize := binary.BigEndian.Uint32(p[12:16])

	// the buf may include extra bytes that are not part of the transaction
	// tranLen represents the length of bytes that are part of the transaction
	tranLen := int(20 + totalSize)

	if tranLen > len(p) {
		return n, errors.New("buflen too small for tranLen")
	}

	// Create a new scanner for parsing incoming bytes into transaction tokens
	scanner := bufio.NewScanner(bytes.NewReader(p[22:tranLen]))
	scanner.Split(fieldScanner)

	for i := 0; i < int(binary.BigEndian.Uint16(p[20:22])); i++ {
		scanner.Scan()

		var field Field
		if _, err := field.Write(scanner.Bytes()); err != nil {
			return 0, fmt.Errorf("error reading field: %w", err)
		}
		t.Fields = append(t.Fields, field)
	}

	t.Flags = p[0]
	t.IsReply = p[1]
	t.Type = p[2:4]
	t.ID = p[4:8]
	t.ErrorCode = p[8:12]
	t.TotalSize = p[12:16]
	t.DataSize = p[16:20]
	t.ParamCount = p[20:22]

	return len(p), err
}

const tranHeaderLen = 20 // fixed length of transaction fields before the variable length fields

// transactionScanner implements bufio.SplitFunc for parsing incoming byte slices into complete tokens
func transactionScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	// The bytes that contain the size of a transaction are from 12:16, so we need at least 16 bytes
	if len(data) < 16 {
		return 0, nil, nil
	}

	totalSize := binary.BigEndian.Uint32(data[12:16])

	// tranLen represents the length of bytes that are part of the transaction
	tranLen := int(tranHeaderLen + totalSize)
	if tranLen > len(data) {
		return 0, nil, nil
	}

	return tranLen, data[0:tranLen], nil
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
			ID:        [2]byte(fieldID),
			FieldSize: [2]byte(fieldSize),
			Data:      buf[4 : 4+fieldSizeInt],
		})

		buf = buf[fieldSizeInt+4:]
	}

	if len(buf) != 0 {
		return []Field{}, fmt.Errorf("extra field bytes")
	}

	return fields, nil
}

// Read implements the io.Reader interface for Transaction
func (t *Transaction) Read(p []byte) (int, error) {
	payloadSize := t.Size()

	fieldCount := make([]byte, 2)
	binary.BigEndian.PutUint16(fieldCount, uint16(len(t.Fields)))

	bbuf := new(bytes.Buffer)

	for _, field := range t.Fields {
		_, err := bbuf.ReadFrom(&field)
		if err != nil {
			return 0, fmt.Errorf("error reading field: %w", err)
		}
	}

	buf := slices.Concat(
		[]byte{t.Flags, t.IsReply},
		t.Type,
		t.ID,
		t.ErrorCode,
		payloadSize,
		payloadSize, // this is the dataSize field, but seeming the same as totalSize
		fieldCount,
		bbuf.Bytes(),
	)

	if t.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[t.readOffset:])
	t.readOffset += n

	return n, nil
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
		if id == int(binary.BigEndian.Uint16(field.ID[:])) {
			return field
		}
	}

	return Field{}
}

func (t *Transaction) IsError() bool {
	return bytes.Equal(t.ErrorCode, []byte{0, 0, 0, 1})
}
