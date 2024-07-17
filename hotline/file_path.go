package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// FilePathItem represents the file or directory portion of a delimited file path (e.g. foo and bar in "/foo/bar")
// Example bytes:
// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72  "subfolder"
type FilePathItem struct {
	Len  byte
	Name []byte
}

const fileItemMinLen = 3

// fileItemScanner implements bufio.SplitFunc for parsing incoming byte slices into complete tokens
func fileItemScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	if len(data) < fileItemMinLen {
		return 0, nil, nil
	}

	advance = fileItemMinLen + int(data[2])
	return advance, data[0:advance], nil
}

// Write implements the io.Writer interface for FilePathItem
func (fpi *FilePathItem) Write(b []byte) (n int, err error) {
	if len(b) < 3 {
		return n, errors.New("buflen too small")
	}
	fpi.Len = b[2]
	fpi.Name = b[fileItemMinLen : fpi.Len+fileItemMinLen]

	return int(fpi.Len) + fileItemMinLen, nil
}

type FilePath struct {
	ItemCount [2]byte
	Items     []FilePathItem
}

// Write implements io.Writer interface for FilePath
func (fp *FilePath) Write(b []byte) (n int, err error) {
	reader := bytes.NewReader(b)
	err = binary.Read(reader, binary.BigEndian, &fp.ItemCount)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, err
	}
	if errors.Is(err, io.EOF) {
		return n, nil
	}

	scanner := bufio.NewScanner(reader)
	scanner.Split(fileItemScanner)

	for i := 0; i < int(binary.BigEndian.Uint16(fp.ItemCount[:])); i++ {
		var fpi FilePathItem
		scanner.Scan()

		// Make a new []byte slice and copy the scanner bytes to it.  This is critical to avoid a data race as the
		// scanner re-uses the buffer for subsequent scans.
		buf := make([]byte, len(scanner.Bytes()))
		copy(buf, scanner.Bytes())

		if _, err := fpi.Write(buf); err != nil {
			return n, err
		}
		fp.Items = append(fp.Items, fpi)
	}

	return n, nil
}

// IsDropbox checks if a FilePath matches the special drop box folder type
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

func ReadPath(fileRoot string, filePath, fileName []byte) (fullPath string, err error) {
	var fp FilePath
	if filePath != nil {
		if _, err = fp.Write(filePath); err != nil {
			return "", err
		}
	}

	var subPath string
	for _, pathItem := range fp.Items {
		subPath = filepath.Join("/", subPath, string(pathItem.Name))
	}

	fullPath = filepath.Join(
		fileRoot,
		subPath,
		filepath.Join("/", string(fileName)),
	)
	fullPath, err = txtDecoder.String(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid filepath encoding: %w", err)
	}
	return fullPath, nil
}
