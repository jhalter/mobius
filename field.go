package hotline

import (
	"encoding/binary"
)

const fieldError = 100
const fieldData = 101
const fieldUserName = 102
const fieldUserID = 103
const fieldUserIconID = 104
const fieldUserLogin = 105
const fieldUserPassword = 106
const fieldRefNum = 107
const fieldTransferSize = 108
const fieldChatOptions = 109
const fieldUserAccess = 110
const fieldUserAlias = 111
const fieldUserFlags = 112
const fieldOptions = 113
const fieldChatID = 114
const fieldChatSubject = 115
const fieldWaitingCount = 116
const fieldVersion = 160
const fieldCommunityBannerID = 161
const fieldServerName = 162
const fieldFileNameWithInfo = 200
const fieldFileName = 201
const fieldFilePath = 202
const fieldFileTypeString = 205
const fieldFileCreatorString = 206
const fieldFileSize = 207
const fieldFileCreateDate = 208
const fieldFileModifyDate = 209
const fieldFileComment = 210
const fieldFileNewName = 211
const fieldFileNewPath = 212
const fieldFileType = 213
const fieldQuotingMsg = 214 // Defined but unused in the Hotline Protocol spec
const fieldAutomaticResponse = 215
const fieldFolderItemCount = 220
const fieldUsernameWithInfo = 300
const fieldNewsArtListData = 321
const fieldNewsCatName = 322
const fieldNewsCatListData15 = 323
const fieldNewsPath = 325
const fieldNewsArtID = 326
const fieldNewsArtDataFlav = 327
const fieldNewsArtTitle = 328
const fieldNewsArtPoster = 329
const fieldNewsArtDate = 330
const fieldNewsArtPrevArt = 331
const fieldNewsArtNextArt = 332
const fieldNewsArtData = 333
const fieldNewsArtFlags = 334
const fieldNewsArtParentArt = 335
const fieldNewsArt1stChildArt = 336
const fieldNewsArtRecurseDel = 337

type Field struct {
	ID        []byte // Size 2
	FieldSize []byte
	Data      []byte
}

func NewField(id uint16, data []byte) Field {
	idBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(idBytes, id)

	return Field{
		ID:   idBytes,
		Data: data,
	}
}

func (f Field) Uint16ID() int {
	fieldID := binary.BigEndian.Uint16(f.ID)
	return int(fieldID)
}

// Size of the data part: maxlen 2
func (f Field) CalcFieldSize() []byte {
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, uint16(len(f.Data)))

	return bs
}

func (f Field) Payload() []byte {
	out := append(f.ID, f.CalcFieldSize()...)
	out = append(out, f.Data...)
	return out
}

type FileNameWithInfo struct {
	Type       string
	Creator    []byte
	FileSize   uint32 //File Size in bytes
	NameScript []byte
	NameSize   []byte
	Name       string
}

func (f FileNameWithInfo) Payload() []byte {
	name := []byte(f.Name)
	nameSize := make([]byte, 2)
	binary.BigEndian.PutUint16(nameSize, uint16(len(name)))

	kb := f.FileSize

	fSize := make([]byte, 4)
	binary.BigEndian.PutUint32(fSize, kb)

	out := []byte(f.Type)
	out = append(out, f.Creator...)
	out = append(out, fSize...)
	out = append(out, []byte{0, 0, 0, 0}...) // Reserved
	out = append(out, f.NameScript...)
	out = append(out, nameSize...)
	out = append(out, []byte(f.Name)...)

	return out
}

func ReadFileNameWithInfo(b []byte) FileNameWithInfo {
	return FileNameWithInfo{
		Type:       string(b[0:4]),
		Creator:    b[4:8],
		FileSize:   1,
		NameScript: b[12:16],
		NameSize:   b[16:20],
		Name:       string(b[20:]),
	}
}
