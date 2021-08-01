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
	"pdf": "CARO",
}

var fileTypeCodes = map[string]string{
	"sit": "SIT!",
	"jpg": "JPEG",
	"pdf": "PDF ",
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
		var fileType []byte
		fileCreator := make([]byte, 4)
		fileSize := make([]byte, 4)
		if !file.IsDir() {
			fileType = []byte(fileTypeFromFilename(file.Name()))
			fileCreator = []byte(fileCreatorFromFilename(file.Name()))

			binary.BigEndian.PutUint32(fileSize, uint32(file.Size()))
		} else {
			fileType = []byte("fldr")

			dir, err := ioutil.ReadDir(filePath + "/" + file.Name())
			if err != nil {
				return fields, err
			}
			binary.BigEndian.PutUint32(fileSize, uint32(len(dir)))
		}

		fields = append(fields, NewField(
			fieldFileNameWithInfo,
			FileNameWithInfo{
				Type:       fileType,
				Creator:    fileCreator,
				FileSize:   fileSize,
				NameScript: []byte{0, 0},
				Name:       []byte(file.Name()),
			}.Payload(),
		))
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

func ReadFilePath(filePathFieldData []byte) string {
	fp := NewFilePath(filePathFieldData)
	return fp.String()
}
