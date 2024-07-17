package hotline

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
)

type FileNameWithInfo struct {
	FileNameWithInfoHeader
	Name []byte // File Name

	readOffset int // Internal offset to track read progress
}

// FileNameWithInfoHeader contains the fixed length fields of FileNameWithInfo
type FileNameWithInfoHeader struct {
	Type       [4]byte // File type code
	Creator    [4]byte // File creator code
	FileSize   [4]byte // File Size in bytes
	RSVD       [4]byte
	NameScript [2]byte // ??
	NameSize   [2]byte // Length of Name field
}

func (f *FileNameWithInfoHeader) nameLen() int {
	return int(binary.BigEndian.Uint16(f.NameSize[:]))
}

// Read implements io.Reader for FileNameWithInfo
func (f *FileNameWithInfo) Read(p []byte) (int, error) {
	buf := slices.Concat(
		f.Type[:],
		f.Creator[:],
		f.FileSize[:],
		f.RSVD[:],
		f.NameScript[:],
		f.NameSize[:],
		f.Name,
	)

	if f.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[f.readOffset:])
	f.readOffset += n

	return n, nil
}

func (f *FileNameWithInfo) Write(p []byte) (int, error) {
	err := binary.Read(bytes.NewReader(p), binary.BigEndian, &f.FileNameWithInfoHeader)
	if err != nil {
		return 0, err
	}
	headerLen := binary.Size(f.FileNameWithInfoHeader)
	f.Name = p[headerLen : headerLen+f.nameLen()]

	return len(p), nil
}
