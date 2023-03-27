package hotline

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func fileTypeFromFilename(filename string) fileType {
	fileExt := strings.ToLower(filepath.Ext(filename))
	ft, ok := fileTypes[fileExt]
	if ok {
		return ft
	}
	return defaultFileType
}

func fileTypeFromInfo(info fs.FileInfo) (ft fileType, err error) {
	if info.IsDir() {
		ft.CreatorCode = "n/a "
		ft.TypeCode = "fldr"
	} else {
		ft = fileTypeFromFilename(info.Name())
	}

	return ft, nil
}

func getFileNameList(path string, ignoreList []string) (fields []Field, err error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return fields, nil
	}

	for _, file := range files {
		var fnwi FileNameWithInfo

		if ignoreFile(file.Name(), ignoreList) {
			continue
		}

		fileCreator := make([]byte, 4)

		fileInfo, err := file.Info()
		if err != nil {
			return fields, err
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			resolvedPath, err := os.Readlink(filepath.Join(path, file.Name()))
			if err != nil {
				return fields, err
			}

			rFile, err := os.Stat(filepath.Join(path, resolvedPath))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fields, err
			}

			if rFile.IsDir() {
				dir, err := ioutil.ReadDir(filepath.Join(path, file.Name()))
				if err != nil {
					return fields, err
				}

				var c uint32
				for _, f := range dir {
					if !ignoreFile(f.Name(), ignoreList) {
						c += 1
					}
				}

				binary.BigEndian.PutUint32(fnwi.FileSize[:], c)
				copy(fnwi.Type[:], []byte("fldr")[:])
				copy(fnwi.Creator[:], fileCreator[:])
			} else {
				binary.BigEndian.PutUint32(fnwi.FileSize[:], uint32(rFile.Size()))
				copy(fnwi.Type[:], []byte(fileTypeFromFilename(rFile.Name()).TypeCode)[:])
				copy(fnwi.Creator[:], []byte(fileTypeFromFilename(rFile.Name()).CreatorCode)[:])
			}

		} else if file.IsDir() {
			dir, err := ioutil.ReadDir(filepath.Join(path, file.Name()))
			if err != nil {
				return fields, err
			}

			var c uint32
			for _, f := range dir {
				if !ignoreFile(f.Name(), ignoreList) {
					c += 1
				}
			}

			binary.BigEndian.PutUint32(fnwi.FileSize[:], c)
			copy(fnwi.Type[:], []byte("fldr")[:])
			copy(fnwi.Creator[:], fileCreator[:])
		} else {
			// the Hotline protocol does not support fileWrapper sizes > 4GiB due to the 4 byte field size, so skip them
			if fileInfo.Size() > 4294967296 {
				continue
			}

			hlFile, err := newFileWrapper(&OSFileStore{}, path+"/"+file.Name(), 0)
			if err != nil {
				return nil, err
			}

			copy(fnwi.FileSize[:], hlFile.totalSize()[:])
			copy(fnwi.Type[:], hlFile.ffo.FlatFileInformationFork.TypeSignature[:])
			copy(fnwi.Creator[:], hlFile.ffo.FlatFileInformationFork.CreatorSignature[:])
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
		fields = append(fields, NewField(FieldFileNameWithInfo, b))
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
		if err != nil {
			return err
		}

		if !strings.HasPrefix(info.Name(), ".") {
			itemcount += 1
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

func ignoreFile(fileName string, ignoreList []string) bool {
	// skip files that match any regular expression present in the IgnoreFiles list
	matchIgnoreFilter := 0
	for _, pattern := range ignoreList {
		if match, _ := regexp.MatchString(pattern, fileName); match {
			matchIgnoreFilter += 1
		}
	}
	return matchIgnoreFilter > 0
}
