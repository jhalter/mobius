package hotline

import (
	"encoding/binary"
	"strings"
)

const pathSeparator = "/" // File path separator TODO: make configurable to support Windows

// FilePathItem represents the file or directory portion of a delimited file path (e.g. foo and bar in "/foo/bar")
// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72 // "subfolder"
type FilePathItem struct {
	Len  byte
	Name []byte
}

func NewFilePathItem(b []byte) FilePathItem {
	return FilePathItem{
		Len:  b[2],
		Name: b[3:],
	}
}

type FilePath struct {
	ItemCount []byte
	Items     []FilePathItem
}

func (fp *FilePath) UnmarshalBinary(b []byte) error {
	fp.ItemCount = b[0:2]

	pathData := b[2:]
	for i := uint16(0); i < fp.Len(); i++ {
		segLen := pathData[2]
		fp.Items = append(fp.Items, NewFilePathItem(pathData[:segLen+3]))
		pathData = pathData[3+segLen:]
	}

	return nil
}

func (fp *FilePath) Len() uint16 {
	return binary.BigEndian.Uint16(fp.ItemCount)
}

func (fp *FilePath) String() string {
	var out []string
	for _, i := range fp.Items {
		out = append(out, string(i.Name))
	}
	return strings.Join(out, pathSeparator)
}
