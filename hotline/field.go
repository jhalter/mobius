package hotline

import (
	"encoding/binary"
	"io"
	"slices"
)

// List of Hotline protocol field types taken from the official 1.9 protocol document
const (
	FieldError               = 100
	FieldData                = 101
	FieldUserName            = 102
	FieldUserID              = 103
	FieldUserIconID          = 104
	FieldUserLogin           = 105
	FieldUserPassword        = 106
	FieldRefNum              = 107
	FieldTransferSize        = 108
	FieldChatOptions         = 109
	FieldUserAccess          = 110
	FieldUserAlias           = 111 // TODO: implement
	FieldUserFlags           = 112
	FieldOptions             = 113
	FieldChatID              = 114
	FieldChatSubject         = 115
	FieldWaitingCount        = 116
	FieldBannerType          = 152
	FieldNoServerAgreement   = 152
	FieldVersion             = 160
	FieldCommunityBannerID   = 161
	FieldServerName          = 162
	FieldFileNameWithInfo    = 200
	FieldFileName            = 201
	FieldFilePath            = 202
	FieldFileResumeData      = 203
	FieldFileTransferOptions = 204
	FieldFileTypeString      = 205
	FieldFileCreatorString   = 206
	FieldFileSize            = 207
	FieldFileCreateDate      = 208
	FieldFileModifyDate      = 209
	FieldFileComment         = 210
	FieldFileNewName         = 211
	FieldFileNewPath         = 212
	FieldFileType            = 213
	FieldQuotingMsg          = 214
	FieldAutomaticResponse   = 215
	FieldFolderItemCount     = 220
	FieldUsernameWithInfo    = 300
	FieldNewsArtListData     = 321
	FieldNewsCatName         = 322
	FieldNewsCatListData15   = 323
	FieldNewsPath            = 325
	FieldNewsArtID           = 326
	FieldNewsArtDataFlav     = 327
	FieldNewsArtTitle        = 328
	FieldNewsArtPoster       = 329
	FieldNewsArtDate         = 330
	FieldNewsArtPrevArt      = 331
	FieldNewsArtNextArt      = 332
	FieldNewsArtData         = 333
	FieldNewsArtFlags        = 334 // TODO: what is this used for?
	FieldNewsArtParentArt    = 335
	FieldNewsArt1stChildArt  = 336
	FieldNewsArtRecurseDel   = 337 // TODO: implement news article recusive deletion
)

type Field struct {
	ID        [2]byte // Type of field
	FieldSize [2]byte // Size of the data part
	Data      []byte  // Actual field content

	readOffset int // Internal offset to track read progress
}

type requiredField struct {
	ID     int
	minLen int
}

func NewField(id uint16, data []byte) Field {
	f := Field{Data: data}
	binary.BigEndian.PutUint16(f.ID[:], id)
	binary.BigEndian.PutUint16(f.FieldSize[:], uint16(len(data)))

	return f
}

// fieldScanner implements bufio.SplitFunc for parsing byte slices into complete tokens
func fieldScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	if len(data) < minFieldLen {
		return 0, nil, nil
	}

	// tranLen represents the length of bytes that are part of the transaction
	neededSize := minFieldLen + int(binary.BigEndian.Uint16(data[2:4]))
	if neededSize > len(data) {
		return 0, nil, nil
	}

	return neededSize, data[0:neededSize], nil
}

// Read implements io.Reader for Field
func (f *Field) Read(p []byte) (int, error) {
	buf := slices.Concat(f.ID[:], f.FieldSize[:], f.Data)

	if f.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[f.readOffset:])
	f.readOffset += n

	return n, nil
}

// Write implements io.Writer for Field
func (f *Field) Write(p []byte) (int, error) {
	f.ID = [2]byte(p[0:2])
	f.FieldSize = [2]byte(p[2:4])

	i := int(binary.BigEndian.Uint16(f.FieldSize[:]))
	f.Data = p[4 : 4+i]

	return minFieldLen + i, nil
}

func getField(id int, fields *[]Field) *Field {
	for _, field := range *fields {
		if id == int(binary.BigEndian.Uint16(field.ID[:])) {
			return &field
		}
	}
	return nil
}
