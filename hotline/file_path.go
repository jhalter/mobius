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
	PathItemCount []byte
	PathItems     []FilePathItem
}

func NewFilePath(b []byte) FilePath {
	if b == nil {
		return FilePath{}
	}

	fp := FilePath{PathItemCount: b[0:2]}

	// number of items in the path
	pathItemLen := binary.BigEndian.Uint16(b[0:2])
	pathData := b[2:]
	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		fp.PathItems = append(fp.PathItems, NewFilePathItem(pathData[:segLen+3]))
		pathData = pathData[3+segLen:]
	}

	return fp
}

func (fp *FilePath) String() string {
	var out []string
	for _, i := range fp.PathItems {
		out = append(out, string(i.Name))
	}
	return strings.Join(out, pathSeparator)
}
