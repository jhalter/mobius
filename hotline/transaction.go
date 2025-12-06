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

type TranType [2]byte

var (
	TranError                = TranType{0x00, 0x00} // 0
	TranGetMsgs              = TranType{0x00, 0x65} // 101
	TranNewMsg               = TranType{0x00, 0x66} // 102
	TranOldPostNews          = TranType{0x00, 0x67} // 103
	TranServerMsg            = TranType{0x00, 0x68} // 104
	TranChatSend             = TranType{0x00, 0x69} // 105
	TranChatMsg              = TranType{0x00, 0x6A} // 106
	TranLogin                = TranType{0x00, 0x6B} // 107
	TranSendInstantMsg       = TranType{0x00, 0x6C} // 108
	TranShowAgreement        = TranType{0x00, 0x6D} // 109
	TranDisconnectUser       = TranType{0x00, 0x6E} // 110
	TranDisconnectMsg        = TranType{0x00, 0x6F} // 111
	TranInviteNewChat        = TranType{0x00, 0x70} // 112
	TranInviteToChat         = TranType{0x00, 0x71} // 113
	TranRejectChatInvite     = TranType{0x00, 0x72} // 114
	TranJoinChat             = TranType{0x00, 0x73} // 115
	TranLeaveChat            = TranType{0x00, 0x74} // 116
	TranNotifyChatChangeUser = TranType{0x00, 0x75} // 117
	TranNotifyChatDeleteUser = TranType{0x00, 0x76} // 118
	TranNotifyChatSubject    = TranType{0x00, 0x77} // 119
	TranSetChatSubject       = TranType{0x00, 0x78} // 120
	TranAgreed               = TranType{0x00, 0x79} // 121
	TranServerBanner         = TranType{0x00, 0x7A} // 122
	TranGetFileNameList      = TranType{0x00, 0xC8} // 200
	TranDownloadFile         = TranType{0x00, 0xCA} // 202
	TranUploadFile           = TranType{0x00, 0xCB} // 203
	TranNewFolder            = TranType{0x00, 0xCD} // 205
	TranDeleteFile           = TranType{0x00, 0xCC} // 204
	TranGetFileInfo          = TranType{0x00, 0xCE} // 206
	TranSetFileInfo          = TranType{0x00, 0xCF} // 207
	TranMoveFile             = TranType{0x00, 0xD0} // 208
	TranMakeFileAlias        = TranType{0x00, 0xD1} // 209
	TranDownloadFldr         = TranType{0x00, 0xD2} // 210
	TranDownloadInfo         = TranType{0x00, 0xD3} // 211
	TranDownloadBanner       = TranType{0x00, 0xD4} // 212
	TranUploadFldr           = TranType{0x00, 0xD5} // 213
	TranGetUserNameList      = TranType{0x01, 0x2C} // 300
	TranNotifyChangeUser     = TranType{0x01, 0x2D} // 301
	TranNotifyDeleteUser     = TranType{0x01, 0x2E} // 302
	TranGetClientInfoText    = TranType{0x01, 0x2F} // 303
	TranSetClientUserInfo    = TranType{0x01, 0x30} // 304
	TranListUsers            = TranType{0x01, 0x5C} // 348
	TranUpdateUser           = TranType{0x01, 0x5D} // 349
	TranNewUser              = TranType{0x01, 0x5E} // 350
	TranDeleteUser           = TranType{0x01, 0x5F} // 351
	TranGetUser              = TranType{0x01, 0x60} // 352
	TranSetUser              = TranType{0x01, 0x61} // 353
	TranUserAccess           = TranType{0x01, 0x62} // 354
	TranUserBroadcast        = TranType{0x01, 0x63} // 355
	TranGetNewsCatNameList   = TranType{0x01, 0x72} // 370
	TranGetNewsArtNameList   = TranType{0x01, 0x73} // 371
	TranDelNewsItem          = TranType{0x01, 0x7C} // 380
	TranNewNewsFldr          = TranType{0x01, 0x7D} // 381
	TranNewNewsCat           = TranType{0x01, 0x7E} // 382
	TranGetNewsArtData       = TranType{0x01, 0x90} // 400
	TranPostNewsArt          = TranType{0x01, 0x9A} // 410
	TranDelNewsArt           = TranType{0x01, 0x9B} // 411
	TranKeepAlive            = TranType{0x01, 0xF4} // 500
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

	ClientID   ClientID // Internal identifier for target client
	readOffset int      // Internal offset to track read progress
}

var tranTypeNames = map[TranType]string{
	TranServerMsg:          "Server Message",
	TranChatMsg:            "Receive chat",
	TranNotifyChangeUser:   "User change",
	TranError:              "Error",
	TranShowAgreement:      "Show agreement",
	TranUserAccess:         "User access",
	TranNotifyDeleteUser:   "User left",
	TranAgreed:             "Accept agreement",
	TranLogin:              "Log In",
	TranChatSend:           "Send chat",
	TranDelNewsArt:         "Delete news article",
	TranDelNewsItem:        "Delete news item",
	TranDeleteFile:         "Delete file",
	TranDeleteUser:         "Delete user",
	TranDisconnectUser:     "Disconnect user",
	TranDownloadFile:       "Download file",
	TranDownloadFldr:       "Download folder",
	TranGetClientInfoText:  "Get client info",
	TranGetFileInfo:        "Get file info",
	TranGetFileNameList:    "Get file list",
	TranGetMsgs:            "Get messages",
	TranGetNewsArtData:     "Get news article",
	TranGetNewsArtNameList: "Get news article list",
	TranGetNewsCatNameList: "Get news categories",
	TranGetUser:            "Get user",
	TranGetUserNameList:    "Get user list",
	TranInviteNewChat:      "Invite to new chat",
	TranInviteToChat:       "Invite to chat",
	TranJoinChat:           "Join chat",
	TranKeepAlive:          "Keepalive",
	TranLeaveChat:          "Leave chat",
	TranListUsers:          "List user accounts",
	TranMoveFile:           "Move file",
	TranNewFolder:          "Create folder",
	TranNewNewsCat:         "Create news category",
	TranNewNewsFldr:        "Create news bundle",
	TranNewUser:            "Create user account",
	TranUpdateUser:         "Update user account",
	TranOldPostNews:        "Post to message board",
	TranPostNewsArt:        "Create news article",
	TranRejectChatInvite:   "Decline chat invite",
	TranSendInstantMsg:     "Send message",
	TranSetChatSubject:     "Set chat subject",
	TranMakeFileAlias:      "Make file alias",
	TranSetClientUserInfo:  "Set client user info",
	TranSetFileInfo:        "Set file info",
	TranSetUser:            "Set user",
	TranUploadFile:         "Upload file",
	TranUploadFldr:         "Upload folder",
	TranUserBroadcast:      "Send broadcast",
	TranDownloadBanner:     "Download banner",
}

// NewTransaction creates a new Transaction with the specified type, client, and optional fields.
func NewTransaction(t TranType, clientID ClientID, fields ...Field) Transaction {
	transaction := Transaction{
		Type:     t,
		ClientID: clientID,
		Fields:   fields,
	}

	// Give the transaction a random ID.
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
	scanner.Split(FieldScanner)

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

func (t *Transaction) GetField(id FieldType) *Field {
	for _, field := range t.Fields {
		if id == field.Type {
			return &field
		}
	}

	return &Field{}
}
