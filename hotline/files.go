package hotline

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func downcaseFileExtension(filename string) string {
	splitStr := strings.Split(filename, ".")
	ext := strings.ToLower(
		splitStr[len(splitStr)-1],
	)

	return ext
}

func fileTypeFromFilename(fn string) fileType {
	ft, ok := fileTypes[downcaseFileExtension(fn)]
	if ok {
		return ft
	}
	return defaultFileType
}

func getFileNameList(filePath string) (fields []Field, err error) {
	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		return fields, nil
	}

	for _, file := range files {
		var fileType []byte
		var fnwi FileNameWithInfo
		fileCreator := make([]byte, 4)
		if !file.IsDir() {
			fileType = []byte(fileTypeFromFilename(file.Name()).TypeCode)
			fileCreator = []byte(fileTypeFromFilename(file.Name()).CreatorCode)

			binary.BigEndian.PutUint32(fnwi.FileSize[:], uint32(file.Size()))
			copy(fnwi.Type[:], fileType[:])
			copy(fnwi.Creator[:], fileCreator[:])
		} else {
			fileType = []byte("fldr")

			dir, err := ioutil.ReadDir(filePath + "/" + file.Name())
			if err != nil {
				return fields, err
			}
			binary.BigEndian.PutUint32(fnwi.FileSize[:], uint32(len(dir)))
			copy(fnwi.Type[:], fileType[:])
			copy(fnwi.Creator[:], fileCreator[:])
		}

		strippedName := strings.Replace(file.Name(), ".incomplete", "", -1)

		nameSize := make([]byte, 2)
		binary.BigEndian.PutUint16(nameSize, uint16(len(strippedName)))
		copy(fnwi.NameSize[:], nameSize[:])

		fnwi.name = []byte(strippedName)

		b, err := fnwi.MarshalBinary()
		if err != nil {
			return nil, err
		}
		fields = append(fields, NewField(fieldFileNameWithInfo, b))
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

const incompleteFileSuffix = ".incomplete"

// effectiveFile wraps os.Open to check for the presence of a partial file transfer as a fallback
func effectiveFile(filePath string) (*os.File, error) {
	file, err := os.Open(filePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	if errors.Is(err, fs.ErrNotExist) {
		file, err = os.OpenFile(filePath+incompleteFileSuffix, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
	}
	return file, nil
}
