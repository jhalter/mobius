package hotline

import (
	"encoding/binary"
	"github.com/jhalter/mobius/concat"
)

type FileHeader struct {
	Size     []byte // Total size of FileHeader payload
	Type     []byte // 0 for file, 1 for dir
	FilePath []byte // encoded file path
}

func NewFileHeader(fileName string, isDir bool) FileHeader {
	fh := FileHeader{
		Size:     make([]byte, 2),
		Type:     []byte{0x00, 0x00},
		FilePath: EncodeFilePath(fileName),
	}
	if isDir {
		fh.Type = []byte{0x00, 0x01}
	}

	encodedPathLen := uint16(len(fh.FilePath) + len(fh.Type))
	binary.BigEndian.PutUint16(fh.Size, encodedPathLen)

	return fh
}

func (fh *FileHeader) Payload() []byte {
	return concat.Slices(
		fh.Size,
		fh.Type,
		fh.FilePath,
	)
}
