package hotline

import (
	"encoding/binary"
	"github.com/jhalter/mobius/concat"
)

// FileNameWithInfo field content is presented in this structure:
// Type	4		Folder (‘fldr’) or other
// Creator	4
// File size	4
// 4		Reserved?
// Name script	2
// Name size	2
// Name data	size
type FileNameWithInfo struct {
	Type       []byte // file type code
	Creator    []byte // File creator code
	FileSize   []byte // File Size in bytes
	RSVD       []byte
	NameScript []byte // TODO: What is this?
	NameSize   []byte // Length of name field
	Name       []byte // File name
}

func (f FileNameWithInfo) Payload() []byte {
	name := f.Name
	nameSize := make([]byte, 2)
	binary.BigEndian.PutUint16(nameSize, uint16(len(name)))

	return concat.Slices(
		f.Type,
		f.Creator,
		f.FileSize,
		[]byte{0, 0, 0, 0},
		f.NameScript,
		nameSize,
		f.Name,
	)
}

func (f *FileNameWithInfo) Read(p []byte) (n int, err error) {
	// TODO: check p for expected len
	f.Type = p[0:4]
	f.Creator = p[4:8]
	f.FileSize = p[8:12]
	f.RSVD = p[12:16]
	f.NameScript = p[16:18]
	f.NameSize = p[18:20]
	f.Name = p[20:]

	return len(p), err
}
