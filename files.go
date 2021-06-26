package hotline

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const defaultCreator = "TTXT"
const defaultType = "TEXT"

var fileCreatorCodes = map[string]string{
	"sit": "SIT!",
}

var fileTypeCodes = map[string]string{
	"sit": "SIT!",
	"jpg": "JPEG",
}

func fileTypeFromFilename(fn string) string {
	ext := strings.Split(fn, ".")
	code := fileTypeCodes[ext[len(ext)-1]]

	if code == "" {
		code = defaultType
	}

	return code
}

func fileCreatorFromFilename(fn string) string {
	ext := strings.Split(fn, ".")
	code := fileCreatorCodes[ext[len(ext)-1]]

	if code == "" {
		code = defaultCreator
	}

	return code
}

func getFileNameList(filePath string) ([]Field, error) {
	var fields []Field

	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		return fields, nil
	}

	for _, file := range files {
		var fileType string
		var fileCreator []byte
		var fileSize uint32
		if !file.IsDir()  {
			fileType = fileTypeFromFilename(file.Name())
			fileCreator = []byte(
				fileCreatorFromFilename(file.Name()),
			)
			fileSize = uint32(file.Size())
		} else {
			fileType = "fldr"
			fileCreator = make([]byte, 4)

			dir, err := ioutil.ReadDir(filePath + "/" + file.Name())
			if err != nil {
				return fields, err
			}
			fileSize = uint32(len(dir))
		}

		field := Field{
			ID: []byte{0, 0xc8},
			Data: FileNameWithInfo{
				Type:       fileType,
				Creator:    fileCreator,
				FileSize:   fileSize,
				NameScript: []byte{0, 0},
				Name:       file.Name(),
			}.Payload(),
		}
		fields = append(fields, field)
	}

	return fields, nil
}

func CalcTotalSize(filePath string) ([]byte, error) {
	var totalSize uint32
	err := filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		totalSize += uint32(info.Size())

		return nil
	})
	if err != nil {
		return nil, err
	}

	bs := make([]byte, 4)
	binary.BigEndian.PutUint32(bs, totalSize)

	return bs, nil
}

func CalcItemCount(filePath string) ([]byte, error) {
	var itemcount uint16
	err := filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
		itemcount += 1

		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, itemcount-1)

	return bs, nil
}

func EncodeFilePath(filePath string) []byte {
	pathSections := strings.Split(filePath, "/")
	pathItemCount := make([]byte, 2)
	binary.BigEndian.PutUint16(pathItemCount, uint16(len(pathSections)))

	bytes := pathItemCount

	for _, section := range pathSections {
		bytes = append(bytes, []byte{0, 0}...)

		pathStr := []byte(section)
		bytes = append(bytes, byte(len(pathStr)))
		bytes = append(bytes, pathStr...)
	}

	return bytes
}

type FileHeader struct {
	Size     []byte
	Type     []byte
	FilePath []byte
}

func (fh *FileHeader) Payload() []byte {
	var out []byte
	out = append(out, fh.Size...)
	out = append(out, fh.Type...)
	out = append(out, fh.FilePath...)

	return out
}

func NewFileHeader(filePath, fileName string, isDir bool) FileHeader {
	fh := FileHeader{
		Size: make([]byte, 2),
	}

	fh.Type = []byte{0x00, 0x00}
	if isDir {
		fh.Type = []byte{0x00, 0x01}
	}
	fh.FilePath = EncodeFilePath(fileName)

	encodedPathLen := uint16(len(fh.FilePath) + len(fh.Type))
	binary.BigEndian.PutUint16(fh.Size, encodedPathLen)

	return fh
}

func ReadFilePath(filePathFieldData []byte) string {
	fp := NewFilePath(filePathFieldData)
	return fp.String()
}
