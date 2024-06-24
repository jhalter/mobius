package hotline

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"
)

// List of Hotline protocol field types taken from the official 1.9 protocol document
var (
	FieldError               = [2]byte{0x00, 0x64} // 100
	FieldData                = [2]byte{0x00, 0x65} // 101
	FieldUserName            = [2]byte{0x00, 0x66} // 102
	FieldUserID              = [2]byte{0x00, 0x67} // 103
	FieldUserIconID          = [2]byte{0x00, 0x68} // 104
	FieldUserLogin           = [2]byte{0x00, 0x69} // 105
	FieldUserPassword        = [2]byte{0x00, 0x6A} // 106
	FieldRefNum              = [2]byte{0x00, 0x6B} // 107
	FieldTransferSize        = [2]byte{0x00, 0x6C} // 108
	FieldChatOptions         = [2]byte{0x00, 0x6D} // 109
	FieldUserAccess          = [2]byte{0x00, 0x6E} // 110
	FieldUserFlags           = [2]byte{0x00, 0x70} // 112
	FieldOptions             = [2]byte{0x00, 0x71} // 113
	FieldChatID              = [2]byte{0x00, 0x72} // 114
	FieldChatSubject         = [2]byte{0x00, 0x73} // 115
	FieldWaitingCount        = [2]byte{0x00, 0x74} // 116
	FieldBannerType          = [2]byte{0x00, 0x98} // 152
	FieldNoServerAgreement   = [2]byte{0x00, 0x98} // 152
	FieldVersion             = [2]byte{0x00, 0xA0} // 160
	FieldCommunityBannerID   = [2]byte{0x00, 0xA1} // 161
	FieldServerName          = [2]byte{0x00, 0xA2} // 162
	FieldFileNameWithInfo    = [2]byte{0x00, 0xC8} // 200
	FieldFileName            = [2]byte{0x00, 0xC9} // 201
	FieldFilePath            = [2]byte{0x00, 0xCA} // 202
	FieldFileResumeData      = [2]byte{0x00, 0xCB} // 203
	FieldFileTransferOptions = [2]byte{0x00, 0xCC} // 204
	FieldFileTypeString      = [2]byte{0x00, 0xCD} // 205
	FieldFileCreatorString   = [2]byte{0x00, 0xCE} // 206
	FieldFileSize            = [2]byte{0x00, 0xCF} // 207
	FieldFileCreateDate      = [2]byte{0x00, 0xD0} // 208
	FieldFileModifyDate      = [2]byte{0x00, 0xD1} // 209
	FieldFileComment         = [2]byte{0x00, 0xD2} // 210
	FieldFileNewName         = [2]byte{0x00, 0xD3} // 211
	FieldFileNewPath         = [2]byte{0x00, 0xD4} // 212
	FieldFileType            = [2]byte{0x00, 0xD5} // 213
	FieldQuotingMsg          = [2]byte{0x00, 0xD6} // 214
	FieldAutomaticResponse   = [2]byte{0x00, 0xD7} // 215
	FieldFolderItemCount     = [2]byte{0x00, 0xDC} // 220
	FieldUsernameWithInfo    = [2]byte{0x01, 0x2C} // 300
	FieldNewsArtListData     = [2]byte{0x01, 0x41} // 321
	FieldNewsCatName         = [2]byte{0x01, 0x42} // 322
	FieldNewsCatListData15   = [2]byte{0x01, 0x43} // 323
	FieldNewsPath            = [2]byte{0x01, 0x45} // 325
	FieldNewsArtID           = [2]byte{0x01, 0x46} // 326
	FieldNewsArtDataFlav     = [2]byte{0x01, 0x47} // 327
	FieldNewsArtTitle        = [2]byte{0x01, 0x48} // 328
	FieldNewsArtPoster       = [2]byte{0x01, 0x49} // 329
	FieldNewsArtDate         = [2]byte{0x01, 0x4A} // 330
	FieldNewsArtPrevArt      = [2]byte{0x01, 0x4B} // 331
	FieldNewsArtNextArt      = [2]byte{0x01, 0x4C} // 332
	FieldNewsArtData         = [2]byte{0x01, 0x4D} // 333
	FieldNewsArtParentArt    = [2]byte{0x01, 0x4F} // 335
	FieldNewsArt1stChildArt  = [2]byte{0x01, 0x50} // 336

	// These fields are documented, but seemingly unused.
	// FieldUserAlias           = [2]byte{0x00, 0x6F} // 111
	// FieldNewsArtFlags        = [2]byte{0x01, 0x4E} // 334
	// FieldNewsArtRecurseDel   = [2]byte{0x01, 0x51} // 337
)

type Field struct {
	ID        [2]byte // Type of field
	FieldSize [2]byte // Size of the data part
	Data      []byte  // Actual field content

	readOffset int // Internal offset to track read progress
}

func NewField(id [2]byte, data []byte) Field {
	f := Field{
		ID:   id,
		Data: make([]byte, len(data)),
	}

	// Copy instead of assigning to avoid data race when the field is read in another go routine.
	copy(f.Data, data)

	binary.BigEndian.PutUint16(f.FieldSize[:], uint16(len(data)))
	return f
}

// fieldScanner implements bufio.SplitFunc for parsing byte slices into complete tokens
func fieldScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	if len(data) < minFieldLen {
		return 0, nil, nil
	}

	// neededSize represents the length of bytes that are part of the field token.
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
	if len(p) < minFieldLen {
		return 0, errors.New("input slice too short")
	}

	copy(f.ID[:], p[0:2])
	copy(f.FieldSize[:], p[2:4])

	dataSize := int(binary.BigEndian.Uint16(f.FieldSize[:]))
	if len(p) < minFieldLen+dataSize {
		return 0, errors.New("input slice too short for data size")
	}

	f.Data = make([]byte, dataSize)
	copy(f.Data, p[4:4+dataSize])

	return minFieldLen + dataSize, nil
}

func getField(id [2]byte, fields *[]Field) *Field {
	for _, field := range *fields {
		if id == field.ID {
			return &field
		}
	}
	return nil
}
