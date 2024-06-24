package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"slices"
)

var (
	TranError                = [2]byte{0x00, 0x00} // 0
	TranGetMsgs              = [2]byte{0x00, 0x65} // 101
	TranNewMsg               = [2]byte{0x00, 0x66} // 102
	TranOldPostNews          = [2]byte{0x00, 0x67} // 103
	TranServerMsg            = [2]byte{0x00, 0x68} // 104
	TranChatSend             = [2]byte{0x00, 0x69} // 105
	TranChatMsg              = [2]byte{0x00, 0x6A} // 106
	TranLogin                = [2]byte{0x00, 0x6B} // 107
	TranSendInstantMsg       = [2]byte{0x00, 0x6C} // 108
	TranShowAgreement        = [2]byte{0x00, 0x6D} // 109
	TranDisconnectUser       = [2]byte{0x00, 0x6E} // 110
	TranDisconnectMsg        = [2]byte{0x00, 0x6F} // 111
	TranInviteNewChat        = [2]byte{0x00, 0x70} // 112
	TranInviteToChat         = [2]byte{0x00, 0x71} // 113
	TranRejectChatInvite     = [2]byte{0x00, 0x72} // 114
	TranJoinChat             = [2]byte{0x00, 0x73} // 115
	TranLeaveChat            = [2]byte{0x00, 0x74} // 116
	TranNotifyChatChangeUser = [2]byte{0x00, 0x75} // 117
	TranNotifyChatDeleteUser = [2]byte{0x00, 0x76} // 118
	TranNotifyChatSubject    = [2]byte{0x00, 0x77} // 119
	TranSetChatSubject       = [2]byte{0x00, 0x78} // 120
	TranAgreed               = [2]byte{0x00, 0x79} // 121
	TranServerBanner         = [2]byte{0x00, 0x7A} // 122
	TranGetFileNameList      = [2]byte{0x00, 0xC8} // 200
	TranDownloadFile         = [2]byte{0x00, 0xCA} // 202
	TranUploadFile           = [2]byte{0x00, 0xCB} // 203
	TranNewFolder            = [2]byte{0x00, 0xCD} // 205
	TranDeleteFile           = [2]byte{0x00, 0xCC} // 204
	TranGetFileInfo          = [2]byte{0x00, 0xCE} // 206
	TranSetFileInfo          = [2]byte{0x00, 0xCF} // 207
	TranMoveFile             = [2]byte{0x00, 0xD0} // 208
	TranMakeFileAlias        = [2]byte{0x00, 0xD1} // 209
	TranDownloadFldr         = [2]byte{0x00, 0xD2} // 210
	TranDownloadInfo         = [2]byte{0x00, 0xD3} // 211
	TranDownloadBanner       = [2]byte{0x00, 0xD4} // 212
	TranUploadFldr           = [2]byte{0x00, 0xD5} // 213
	TranGetUserNameList      = [2]byte{0x01, 0x2C} // 300
	TranNotifyChangeUser     = [2]byte{0x01, 0x2D} // 301
	TranNotifyDeleteUser     = [2]byte{0x01, 0x2E} // 302
	TranGetClientInfoText    = [2]byte{0x01, 0x2F} // 303
	TranSetClientUserInfo    = [2]byte{0x01, 0x30} // 304
	TranListUsers            = [2]byte{0x01, 0x5C} // 348
	TranUpdateUser           = [2]byte{0x01, 0x5D} // 349
	TranNewUser              = [2]byte{0x01, 0x5E} // 350
	TranDeleteUser           = [2]byte{0x01, 0x5F} // 351
	TranGetUser              = [2]byte{0x01, 0x60} // 352
	TranSetUser              = [2]byte{0x01, 0x61} // 353
	TranUserAccess           = [2]byte{0x01, 0x62} // 354
	TranUserBroadcast        = [2]byte{0x01, 0x63} // 355
	TranGetNewsCatNameList   = [2]byte{0x01, 0x72} // 370
	TranGetNewsArtNameList   = [2]byte{0x01, 0x73} // 371
	TranDelNewsItem          = [2]byte{0x01, 0x7C} // 380
	TranNewNewsFldr          = [2]byte{0x01, 0x7D} // 381
	TranNewNewsCat           = [2]byte{0x01, 0x7E} // 382
	TranGetNewsArtData       = [2]byte{0x01, 0x90} // 400
	TranPostNewsArt          = [2]byte{0x01, 0x9A} // 410
	TranDelNewsArt           = [2]byte{0x01, 0x9B} // 411
	TranKeepAlive            = [2]byte{0x01, 0xF4} // 500
)

type Transaction struct {
	Flags      byte     // Reserved (should be 0)
	IsReply    byte     // Request (0) or reply (1)
	Type       TranType // Requested operation (user defined)
	ID         [4]byte  // Unique transaction ID (must be != 0)
	ErrorCode  [4]byte  // Used in the reply (user defined, 0 = no error)
	TotalSize  [4]byte  // Total data size for the fields in this transaction.
	DataSize   [4]byte  // Size of data in this transaction part. This allows splitting large transactions into smaller parts.
	ParamCount [2]byte  // Number of the parameters for this transaction
	Fields     []Field

	clientID   [2]byte // Internal identifier for target client
	readOffset int     // Internal offset to track read progress
}

type TranType [2]byte

var tranTypeNames = map[TranType]string{
	TranChatMsg:            "Receive Chat",
	TranNotifyChangeUser:   "TranNotifyChangeUser",
	TranError:              "TranError",
	TranShowAgreement:      "TranShowAgreement",
	TranUserAccess:         "TranUserAccess",
	TranNotifyDeleteUser:   "TranNotifyDeleteUser",
	TranAgreed:             "TranAgreed",
	TranChatSend:           "Send Chat",
	TranDelNewsArt:         "TranDelNewsArt",
	TranDelNewsItem:        "TranDelNewsItem",
	TranDeleteFile:         "TranDeleteFile",
	TranDeleteUser:         "TranDeleteUser",
	TranDisconnectUser:     "TranDisconnectUser",
	TranDownloadFile:       "TranDownloadFile",
	TranDownloadFldr:       "TranDownloadFldr",
	TranGetClientInfoText:  "TranGetClientInfoText",
	TranGetFileInfo:        "TranGetFileInfo",
	TranGetFileNameList:    "TranGetFileNameList",
	TranGetMsgs:            "TranGetMsgs",
	TranGetNewsArtData:     "TranGetNewsArtData",
	TranGetNewsArtNameList: "TranGetNewsArtNameList",
	TranGetNewsCatNameList: "TranGetNewsCatNameList",
	TranGetUser:            "TranGetUser",
	TranGetUserNameList:    "tranHandleGetUserNameList",
	TranInviteNewChat:      "TranInviteNewChat",
	TranInviteToChat:       "TranInviteToChat",
	TranJoinChat:           "TranJoinChat",
	TranKeepAlive:          "TranKeepAlive",
	TranLeaveChat:          "TranJoinChat",
	TranListUsers:          "TranListUsers",
	TranMoveFile:           "TranMoveFile",
	TranNewFolder:          "TranNewFolder",
	TranNewNewsCat:         "TranNewNewsCat",
	TranNewNewsFldr:        "TranNewNewsFldr",
	TranNewUser:            "TranNewUser",
	TranUpdateUser:         "TranUpdateUser",
	TranOldPostNews:        "TranOldPostNews",
	TranPostNewsArt:        "TranPostNewsArt",
	TranRejectChatInvite:   "TranRejectChatInvite",
	TranSendInstantMsg:     "TranSendInstantMsg",
	TranSetChatSubject:     "TranSetChatSubject",
	TranMakeFileAlias:      "TranMakeFileAlias",
	TranSetClientUserInfo:  "TranSetClientUserInfo",
	TranSetFileInfo:        "TranSetFileInfo",
	TranSetUser:            "TranSetUser",
	TranUploadFile:         "TranUploadFile",
	TranUploadFldr:         "TranUploadFldr",
	TranUserBroadcast:      "TranUserBroadcast",
	TranDownloadBanner:     "TranDownloadBanner",
}

func (t TranType) LogValue() slog.Value {
	return slog.StringValue(tranTypeNames[t])
}

// NewTransaction creates a new Transaction with the specified type, client ID, and optional fields.
func NewTransaction(t, clientID [2]byte, fields ...Field) Transaction {
	transaction := Transaction{
		Type:     t,
		clientID: clientID,
		Fields:   fields,
	}

	binary.BigEndian.PutUint32(transaction.ID[:], rand.Uint32())

	return transaction
}

// Write implements io.Writer interface for Transaction.
// Transactions read from the network are read as complete tokens with a bufio.Scanner, so
// the arg p is guaranteed to have the full byte payload of a complete transaction.
func (t *Transaction) Write(p []byte) (n int, err error) {
	// Make sure we have the minimum number of bytes for a transaction.
	if len(p) < 22 {
		return 0, errors.New("buffer too small")
	}

	// Read the total size field.
	totalSize := binary.BigEndian.Uint32(p[12:16])
	tranLen := int(20 + totalSize)

	paramCount := binary.BigEndian.Uint16(p[20:22])

	t.Flags = p[0]
	t.IsReply = p[1]
	copy(t.Type[:], p[2:4])
	copy(t.ID[:], p[4:8])
	copy(t.ErrorCode[:], p[8:12])
	copy(t.TotalSize[:], p[12:16])
	copy(t.DataSize[:], p[16:20])
	copy(t.ParamCount[:], p[20:22])

	scanner := bufio.NewScanner(bytes.NewReader(p[22:tranLen]))
	scanner.Split(fieldScanner)

	for i := 0; i < int(paramCount); i++ {
		if !scanner.Scan() {
			return 0, fmt.Errorf("error scanning field: %w", scanner.Err())
		}

		var field Field
		if _, err := field.Write(scanner.Bytes()); err != nil {
			return 0, fmt.Errorf("error reading field: %w", err)
		}
		t.Fields = append(t.Fields, field)
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scanner error: %w", err)
	}

	return len(p), nil
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
		f := field
		_, err := bbuf.ReadFrom(&f)
		if err != nil {
			return 0, fmt.Errorf("error reading field: %w", err)
		}
	}

	buf := slices.Concat(
		[]byte{t.Flags, t.IsReply},
		t.Type[:],
		t.ID[:],
		t.ErrorCode[:],
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

func (t *Transaction) GetField(id [2]byte) Field {
	for _, field := range t.Fields {
		if id == field.ID {
			return field
		}
	}

	return Field{}
}
