package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"slices"
)

// FieldType represents a Hotline protocol field type identifier
type FieldType [2]byte

// List of Hotline protocol field types taken from the official 1.9 protocol document
var (
	FieldError               = FieldType{0x00, 0x64} // 100
	FieldData                = FieldType{0x00, 0x65} // 101
	FieldUserName            = FieldType{0x00, 0x66} // 102
	FieldUserID              = FieldType{0x00, 0x67} // 103
	FieldUserIconID          = FieldType{0x00, 0x68} // 104
	FieldUserLogin           = FieldType{0x00, 0x69} // 105
	FieldUserPassword        = FieldType{0x00, 0x6A} // 106
	FieldRefNum              = FieldType{0x00, 0x6B} // 107
	FieldTransferSize        = FieldType{0x00, 0x6C} // 108
	FieldChatOptions         = FieldType{0x00, 0x6D} // 109
	FieldUserAccess          = FieldType{0x00, 0x6E} // 110
	FieldUserFlags           = FieldType{0x00, 0x70} // 112
	FieldOptions             = FieldType{0x00, 0x71} // 113
	FieldChatID              = FieldType{0x00, 0x72} // 114
	FieldChatSubject         = FieldType{0x00, 0x73} // 115
	FieldWaitingCount        = FieldType{0x00, 0x74} // 116
	FieldBannerType          = FieldType{0x00, 0x98} // 152
	FieldNoServerAgreement   = FieldType{0x00, 0x98} // 152
	FieldVersion             = FieldType{0x00, 0xA0} // 160
	FieldCommunityBannerID   = FieldType{0x00, 0xA1} // 161
	FieldServerName          = FieldType{0x00, 0xA2} // 162
	FieldFileNameWithInfo    = FieldType{0x00, 0xC8} // 200
	FieldFileName            = FieldType{0x00, 0xC9} // 201
	FieldFilePath            = FieldType{0x00, 0xCA} // 202
	FieldFileResumeData      = FieldType{0x00, 0xCB} // 203
	FieldFileTransferOptions = FieldType{0x00, 0xCC} // 204
	FieldFileTypeString      = FieldType{0x00, 0xCD} // 205
	FieldFileCreatorString   = FieldType{0x00, 0xCE} // 206
	FieldFileSize            = FieldType{0x00, 0xCF} // 207
	FieldFileCreateDate      = FieldType{0x00, 0xD0} // 208
	FieldFileModifyDate      = FieldType{0x00, 0xD1} // 209
	FieldFileComment         = FieldType{0x00, 0xD2} // 210
	FieldFileNewName         = FieldType{0x00, 0xD3} // 211
	FieldFileNewPath         = FieldType{0x00, 0xD4} // 212
	FieldFileType            = FieldType{0x00, 0xD5} // 213
	FieldQuotingMsg          = FieldType{0x00, 0xD6} // 214
	FieldAutomaticResponse   = FieldType{0x00, 0xD7} // 215
	FieldFolderItemCount     = FieldType{0x00, 0xDC} // 220
	FieldUsernameWithInfo    = FieldType{0x01, 0x2C} // 300
	FieldNewsArtListData     = FieldType{0x01, 0x41} // 321
	FieldNewsCatName         = FieldType{0x01, 0x42} // 322
	FieldNewsCatListData15   = FieldType{0x01, 0x43} // 323
	FieldNewsPath            = FieldType{0x01, 0x45} // 325
	FieldNewsArtID           = FieldType{0x01, 0x46} // 326
	FieldNewsArtDataFlav     = FieldType{0x01, 0x47} // 327
	FieldNewsArtTitle        = FieldType{0x01, 0x48} // 328
	FieldNewsArtPoster       = FieldType{0x01, 0x49} // 329
	FieldNewsArtDate         = FieldType{0x01, 0x4A} // 330
	FieldNewsArtPrevArt      = FieldType{0x01, 0x4B} // 331
	FieldNewsArtNextArt      = FieldType{0x01, 0x4C} // 332
	FieldNewsArtData         = FieldType{0x01, 0x4D} // 333
	FieldNewsArtParentArt    = FieldType{0x01, 0x4F} // 335
	FieldNewsArt1stChildArt  = FieldType{0x01, 0x50} // 336
	FieldNewsArtRecurseDel   = FieldType{0x01, 0x51} // 337

	// These fields are documented, but seemingly unused.
	// FieldUserAlias           = FieldType{0x00, 0x6F} // 111
	// FieldNewsArtFlags        = FieldType{0x01, 0x4E} // 334
)

type Field struct {
	Type      FieldType // Type of field
	FieldSize [2]byte   // Size of the data field
	Data      []byte    // Field data

	readOffset int // Internal offset to track read progress
}

func NewField(fieldType FieldType, data []byte) Field {
	f := Field{
		Type: fieldType,
		Data: make([]byte, len(data)),
	}

	// Copy instead of assigning to avoid data race when the field is read in another go routine.
	copy(f.Data, data)

	binary.BigEndian.PutUint16(f.FieldSize[:], uint16(len(data)))
	return f
}

// FieldScanner implements bufio.SplitFunc for parsing byte slices into complete tokens
func FieldScanner(data []byte, _ bool) (advance int, token []byte, err error) {
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

// DecodeInt decodes the field bytes to an int.
// The official Hotline clients will send uint32s as 2 bytes if possible, but
// some third party clients such as Frogblast and Heildrun will always send 4 bytes
func (f *Field) DecodeInt() (int, error) {
	switch len(f.Data) {
	case 2:
		return int(binary.BigEndian.Uint16(f.Data)), nil
	case 4:
		return int(binary.BigEndian.Uint32(f.Data)), nil
	}

	return 0, errors.New("unknown byte length")
}

func (f *Field) DecodeObfuscatedString() string {
	return string(EncodeString(f.Data))
}

// DecodeNewsPath decodes the field data to a news path.
// Example News Path data for a Category nested under two Bundles:
// 00000000  00 03 00 00 10 54 6f 70  20 4c 65 76 65 6c 20 42  |.....Top Level B|
// 00000010  75 6e 64 6c 65 00 00 13  53 65 63 6f 6e 64 20 4c  |undle...Second L|
// 00000020  65 76 65 6c 20 42 75 6e  64 6c 65 00 00 0f 4e 65  |evel Bundle...Ne|
// 00000030  73 74 65 64 20 43 61 74  65 67 6f 72 79           |sted Category|
func (f *Field) DecodeNewsPath() ([]string, error) {
	if len(f.Data) == 0 {
		return []string{}, nil
	}

	pathCount := binary.BigEndian.Uint16(f.Data[0:2])

	scanner := bufio.NewScanner(bytes.NewReader(f.Data[2:]))
	scanner.Split(newsPathScanner)

	var paths []string

	for i := uint16(0); i < pathCount; i++ {
		scanner.Scan()
		paths = append(paths, scanner.Text())
	}

	return paths, nil
}

// Read implements io.Reader for Field
func (f *Field) Read(p []byte) (int, error) {
	buf := slices.Concat(f.Type[:], f.FieldSize[:], f.Data)

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

	copy(f.Type[:], p[0:2])
	copy(f.FieldSize[:], p[2:4])

	dataSize := int(binary.BigEndian.Uint16(f.FieldSize[:]))
	if len(p) < minFieldLen+dataSize {
		return 0, errors.New("input slice too short for data size")
	}

	f.Data = make([]byte, dataSize)
	copy(f.Data, p[4:4+dataSize])

	return minFieldLen + dataSize, nil
}

func GetField(id FieldType, fields *[]Field) *Field {
	for _, field := range *fields {
		if id == field.Type {
			return &field
		}
	}
	return nil
}
