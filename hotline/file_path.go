package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// FilePathItem represents the file or directory portion of a delimited file path (e.g. foo and bar in "/foo/bar")
// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72 // "subfolder"
type FilePathItem struct {
	Len  byte
	Name []byte
}

type FilePath struct {
	ItemCount [2]byte
	Items     []FilePathItem
}

func (fp *FilePath) UnmarshalBinary(b []byte) error {
	reader := bytes.NewReader(b)
	err := binary.Read(reader, binary.BigEndian, &fp.ItemCount)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if errors.Is(err, io.EOF) {
		return nil
	}

	for i := uint16(0); i < fp.Len(); i++ {
		// skip two bytes for the file path delimiter
		_, _ = reader.Seek(2, io.SeekCurrent)

		// read the length of the next pathItem
		segLen, err := reader.ReadByte()
		if err != nil {
			return err
		}

		pBytes := make([]byte, segLen)

		_, err = reader.Read(pBytes)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		fp.Items = append(fp.Items, FilePathItem{Len: segLen, Name: pBytes})
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

	return filepath.Join(out...)
}

func readPath(fileRoot string, filePath, fileName []byte) (fullPath string, err error) {
	var fp FilePath
	if filePath != nil {
		if err = fp.UnmarshalBinary(filePath); err != nil {
			return "", err
		}
	}

	fullPath = filepath.Join(
		"/",
		fileRoot,
		fp.String(),
		filepath.Join("/", string(fileName)),
	)

	return fullPath, nil
}
