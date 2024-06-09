package hotline

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
)

type FileNameWithInfo struct {
	fileNameWithInfoHeader
	name []byte // File name
}

// fileNameWithInfoHeader contains the fixed length fields of FileNameWithInfo
type fileNameWithInfoHeader struct {
	Type       [4]byte // File type code
	Creator    [4]byte // File creator code
	FileSize   [4]byte // File Size in bytes
	RSVD       [4]byte
	NameScript [2]byte // ??
	NameSize   [2]byte // Length of name field
}

func (f *fileNameWithInfoHeader) nameLen() int {
	return int(binary.BigEndian.Uint16(f.NameSize[:]))
}

// Read implements io.Reader for FileNameWithInfo
func (f *FileNameWithInfo) Read(b []byte) (int, error) {
	return copy(b,
		slices.Concat(
			f.Type[:],
			f.Creator[:],
			f.FileSize[:],
			f.RSVD[:],
			f.NameScript[:],
			f.NameSize[:],
			f.name,
		),
	), io.EOF
}

func (f *FileNameWithInfo) Write(p []byte) (int, error) {
	err := binary.Read(bytes.NewReader(p), binary.BigEndian, &f.fileNameWithInfoHeader)
	if err != nil {
		return 0, err
	}
	headerLen := binary.Size(f.fileNameWithInfoHeader)
	f.name = p[headerLen : headerLen+f.nameLen()]

	return len(p), nil
}
