package hotline

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

const maxFileSize = 4294967296

func GetFileNameList(path string, ignoreList []string) (fields []Field, err error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return fields, fmt.Errorf("error reading path: %s: %w", path, err)
	}

	for _, file := range files {
		var fnwi FileNameWithInfo

		if ignoreFile(file.Name(), ignoreList) {
			continue
		}

		fileCreator := make([]byte, 4)

		fileInfo, err := file.Info()
		if err != nil {
			return fields, fmt.Errorf("error getting file info: %s: %w", file.Name(), err)
		}

		// Check if path is a symlink.  If so, follow it.
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			resolvedPath, err := os.Readlink(filepath.Join(path, file.Name()))
			if err != nil {
				return fields, fmt.Errorf("error following symlink: %s: %w", resolvedPath, err)
			}

			rFile, err := os.Stat(resolvedPath)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fields, err
			}

			if rFile.IsDir() {
				dir, err := os.ReadDir(filepath.Join(path, file.Name()))
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
				copy(fnwi.Type[:], "fldr")
				copy(fnwi.Creator[:], fileCreator)
			} else {
				binary.BigEndian.PutUint32(fnwi.FileSize[:], uint32(rFile.Size()))
				copy(fnwi.Type[:], fileTypeFromFilename(rFile.Name()).TypeCode)
				copy(fnwi.Creator[:], fileTypeFromFilename(rFile.Name()).CreatorCode)
			}
		} else if file.IsDir() {
			dir, err := os.ReadDir(filepath.Join(path, file.Name()))
			if err != nil {
				return fields, fmt.Errorf("readDir: %w", err)
			}

			var c uint32
			for _, f := range dir {
				if !ignoreFile(f.Name(), ignoreList) {
					c += 1
				}
			}

			binary.BigEndian.PutUint32(fnwi.FileSize[:], c)
			copy(fnwi.Type[:], "fldr")
			copy(fnwi.Creator[:], fileCreator)
		} else {
			// the Hotline protocol does not support file sizes > 4GiB due to the 4 byte field size, so skip them
			if fileInfo.Size() > maxFileSize {
				continue
			}

			hlFile, err := NewFileWrapper(&OSFileStore{}, path+"/"+file.Name(), 0)
			if err != nil {
				return nil, fmt.Errorf("NewFileWrapper: %w", err)
			}

			copy(fnwi.FileSize[:], hlFile.TotalSize())
			copy(fnwi.Type[:], hlFile.Ffo.FlatFileInformationFork.TypeSignature[:])
			copy(fnwi.Creator[:], hlFile.Ffo.FlatFileInformationFork.CreatorSignature[:])
		}

		strippedName := strings.ReplaceAll(file.Name(), ".incomplete", "")
		strippedName, err = txtEncoder.String(strippedName)
		if err != nil {
			continue
		}

		nameSize := make([]byte, 2)
		binary.BigEndian.PutUint16(nameSize, uint16(len(strippedName)))
		copy(fnwi.NameSize[:], nameSize)

		fnwi.Name = []byte(strippedName)

		b, err := io.ReadAll(&fnwi)
		if err != nil {
			return nil, fmt.Errorf("error io.ReadAll: %w", err)
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

// CalcItemCount recurses through a file path and counts the number of non-hidden files.
func CalcItemCount(filePath string) ([]byte, error) {
	var itemCount uint16

	// Walk the directory and count items
	err := filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files
		if !strings.HasPrefix(info.Name(), ".") {
			itemCount++
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, itemCount-1)

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
