package hotline

import (
	"encoding/binary"
	"io"
	"slices"
)

type FileHeader struct {
	Size     [2]byte // Total size of FileHeader payload
	Type     [2]byte // 0 for file, 1 for dir
	FilePath []byte  // encoded file path
}

func NewFileHeader(fileName string, isDir bool) FileHeader {
	fh := FileHeader{
		Type:     [2]byte{0x00, 0x00},
		FilePath: EncodeFilePath(fileName),
	}
	if isDir {
		fh.Type = [2]byte{0x00, 0x01}
	}

	encodedPathLen := uint16(len(fh.FilePath) + len(fh.Type))
	binary.BigEndian.PutUint16(fh.Size[:], encodedPathLen)

	return fh
}

func (fh *FileHeader) Read(p []byte) (int, error) {
	return copy(p, slices.Concat(
		fh.Size[:],
		fh.Type[:],
		fh.FilePath,
	),
	), io.EOF
}
