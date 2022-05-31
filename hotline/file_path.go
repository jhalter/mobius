package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path"
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
	ItemCount [2]byte
	Items     []FilePathItem
}

const minFilePathLen = 2

func (fp *FilePath) UnmarshalBinary(b []byte) error {
	if b == nil {
		return nil
	}
	if len(b) < minFilePathLen {
		return errors.New("insufficient bytes")
	}
	err := binary.Read(bytes.NewReader(b[0:2]), binary.BigEndian, &fp.ItemCount)
	if err != nil {
		return err
	}

	pathData := b[2:]
	for i := uint16(0); i < fp.Len(); i++ {
		segLen := pathData[2]
		fp.Items = append(fp.Items, NewFilePathItem(pathData[:segLen+3]))
		pathData = pathData[3+segLen:]
	}

	return nil
}

func (fp *FilePath) IsDropbox() bool {
	if fp.Len() == 0 {
		return false
	}

	return strings.Contains(strings.ToLower(string(fp.Items[fp.Len()-1].Name)), "drop box")
}

func (fp *FilePath) IsUploadDir() bool {
	if fp.Len() == 0 {
		return false
	}

	return strings.Contains(strings.ToLower(string(fp.Items[fp.Len()-1].Name)), "upload")
}

func (fp *FilePath) Len() uint16 {
	return binary.BigEndian.Uint16(fp.ItemCount[:])
}

func (fp *FilePath) String() string {
	out := []string{"/"}
	for _, i := range fp.Items {
		out = append(out, string(i.Name))
	}

	return path.Join(out...)
}

func ReadFilePath(filePathFieldData []byte) string {
	var fp FilePath
	err := fp.UnmarshalBinary(filePathFieldData)
	if err != nil {
		// TODO
	}
	return fp.String()
}

func readPath(fileRoot string, filePath, fileName []byte) (fullPath string, err error) {
	var fp FilePath
	if filePath != nil {
		if err = fp.UnmarshalBinary(filePath); err != nil {
			return "", err
		}
	}

	fullPath = path.Join(
		"/",
		fileRoot,
		fp.String(),
		path.Join("/", string(fileName)),
	)

	return fullPath, nil
}
