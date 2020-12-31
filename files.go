package hotline

import (
	"encoding/binary"
	"fmt"
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

func GetFileNameList(filePath string) []Field {
	var fields []Field

	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		var fileType string
		var fileCreator []byte
		var fileSize uint32
		if file.IsDir() != true {
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
				panic(err)
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

	return fields
}

func ReadFilePath(filePathFieldData []byte) string {
	if len(filePathFieldData) < 5 {
		return ""
	}
	filePathFieldData = filePathFieldData[5:]
	out := ""
	flag := false

	// TODO: oh god this is tortured.  Fix me.
	for _, byte := range filePathFieldData {
		if byte == 0x00 {
			flag = true
		} else {
			if flag == true {
				out = out + "/"
				flag = false
			} else {
				out = out + string(byte)
			}
		}
	}

	return out
}

func CalcTotalSize(filePath string) ([]byte, error) {
	var totalSize uint32
	err := filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
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

func EncodeFilePath(filePath string) []byte {
	pathSections := strings.Split(filePath, "/")
	bytes := []byte{0x00, 0x01, 0x00}

	for _, section := range pathSections {
		pathStr := []byte(section)
		bs := make([]byte, 2)
		binary.BigEndian.PutUint16(bs, uint16(len(pathStr)))

		bytes = append(bytes, bs...)
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

func NewFileHeader(filePath, fileName string) FileHeader {
	fmt.Printf("Creating FH for %v\n", fileName)
	fh := FileHeader{
		Size: make([]byte, 2),
	}

	fh.Type = []byte{0x00, 0x00}
	fh.FilePath = EncodeFilePath(fileName)

	encodedPathLen := uint16(len(fh.FilePath) + len(fh.Type))
	binary.BigEndian.PutUint16(fh.Size, encodedPathLen)

	return fh
}
