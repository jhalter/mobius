package hotline

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const (
	incompleteFileSuffix = ".incomplete"
	infoForkNameTemplate = "%s.info_%s" // template string for info fork filenames
	rsrcForkNameTemplate = "%s.rsrc_%s" // template string for resource fork filenames
)

// fileWrapper encapsulates the data, info, and resource forks of a Hotline file and provides methods to manage the files.
type fileWrapper struct {
	fs             FileStore
	name           string // name of the file
	path           string // path to file directory
	dataPath       string // path to the file data fork
	dataOffset     int64
	rsrcPath       string // path to the file resource fork
	infoPath       string // path to the file information fork
	incompletePath string // path to partially transferred temp file
	saveMetaData   bool   // if true, enables saving of info and resource forks in sidecar files
	infoFork       *FlatFileInformationFork
	ffo            *flattenedFileObject
}

func newFileWrapper(fs FileStore, path string, dataOffset int64) (*fileWrapper, error) {
	pathSegs := strings.Split(path, pathSeparator)
	dir := strings.Join(pathSegs[:len(pathSegs)-1], pathSeparator)
	fName := pathSegs[len(pathSegs)-1]
	f := fileWrapper{
		fs:             fs,
		name:           fName,
		path:           dir,
		dataPath:       path,
		dataOffset:     dataOffset,
		rsrcPath:       fmt.Sprintf(rsrcForkNameTemplate, dir+"/", fName),
		infoPath:       fmt.Sprintf(infoForkNameTemplate, dir+"/", fName),
		incompletePath: dir + "/" + fName + incompleteFileSuffix,
		ffo:            &flattenedFileObject{},
	}

	var err error
	f.ffo, err = f.flattenedFileObject()
	if err != nil {
		return nil, err
	}

	return &f, nil
}

func (f *fileWrapper) totalSize() []byte {
	var s int64
	size := make([]byte, 4)

	info, err := f.fs.Stat(f.dataPath)
	if err == nil {
		s += info.Size() - f.dataOffset
	}

	info, err = f.fs.Stat(f.rsrcPath)
	if err == nil {
		s += info.Size()
	}

	binary.BigEndian.PutUint32(size, uint32(s))

	return size
}

func (f *fileWrapper) rsrcForkSize() (s [4]byte) {
	info, err := f.fs.Stat(f.rsrcPath)
	if err != nil {
		return s
	}

	binary.BigEndian.PutUint32(s[:], uint32(info.Size()))
	return s
}

func (f *fileWrapper) rsrcForkHeader() FlatFileForkHeader {
	return FlatFileForkHeader{
		ForkType:        [4]byte{0x4D, 0x41, 0x43, 0x52}, // "MACR"
		CompressionType: [4]byte{},
		RSVD:            [4]byte{},
		DataSize:        f.rsrcForkSize(),
	}
}

func (f *fileWrapper) incompleteDataName() string {
	return f.name + incompleteFileSuffix
}

func (f *fileWrapper) rsrcForkName() string {
	return fmt.Sprintf(rsrcForkNameTemplate, "", f.name)
}

func (f *fileWrapper) infoForkName() string {
	return fmt.Sprintf(infoForkNameTemplate, "", f.name)
}

func (f *fileWrapper) creatorCode() []byte {
	if f.ffo.FlatFileInformationFork.CreatorSignature != nil {
		return f.infoFork.CreatorSignature
	}
	return []byte(fileTypeFromFilename(f.name).CreatorCode)
}

func (f *fileWrapper) typeCode() []byte {
	if f.infoFork != nil {
		return f.infoFork.TypeSignature
	}
	return []byte(fileTypeFromFilename(f.name).TypeCode)
}

func (f *fileWrapper) rsrcForkWriter() (io.Writer, error) {
	file, err := os.OpenFile(f.rsrcPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) infoForkWriter() (io.Writer, error) {
	file, err := os.OpenFile(f.infoPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) incFileWriter() (io.Writer, error) {
	file, err := os.OpenFile(f.incompletePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) dataForkReader() (io.Reader, error) {
	return f.fs.Open(f.dataPath)
}

func (f *fileWrapper) rsrcForkFile() (*os.File, error) {
	return f.fs.Open(f.rsrcPath)
}

func (f *fileWrapper) dataFile() (os.FileInfo, error) {
	if fi, err := f.fs.Stat(f.dataPath); err == nil {
		return fi, nil
	}
	if fi, err := f.fs.Stat(f.incompletePath); err == nil {
		return fi, nil
	}

	return nil, errors.New("file or directory not found")
}

// move a fileWrapper and its associated metadata files to newPath
func (f *fileWrapper) move(newPath string) error {
	err := f.fs.Rename(f.dataPath, path.Join(newPath, f.name))
	if err != nil {
		// TODO
	}

	err = f.fs.Rename(f.incompletePath, path.Join(newPath, f.incompleteDataName()))
	if err != nil {
		// TODO
	}

	err = f.fs.Rename(f.rsrcPath, path.Join(newPath, f.rsrcForkName()))
	if err != nil {
		// TODO
	}

	err = f.fs.Rename(f.infoPath, path.Join(newPath, f.infoForkName()))
	if err != nil {
		// TODO
	}

	return nil
}

// delete a fileWrapper and its associated metadata files if they exist
func (f *fileWrapper) delete() error {
	err := f.fs.RemoveAll(f.dataPath)
	if err != nil {
		// TODO
	}

	err = f.fs.Remove(f.incompletePath)
	if err != nil {
		// TODO
	}

	err = f.fs.Remove(f.rsrcPath)
	if err != nil {
		// TODO
	}

	err = f.fs.Remove(f.infoPath)
	if err != nil {
		// TODO
	}

	return nil
}

func (f *fileWrapper) flattenedFileObject() (*flattenedFileObject, error) {
	dataSize := make([]byte, 4)
	mTime := make([]byte, 8)

	ft := defaultFileType

	fileInfo, err := f.fs.Stat(f.dataPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, fs.ErrNotExist) {
		fileInfo, err = f.fs.Stat(f.incompletePath)
		if err == nil {
			mTime = toHotlineTime(fileInfo.ModTime())
			binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()-f.dataOffset))
			ft, _ = fileTypeFromInfo(fileInfo)
		}
	} else {
		mTime = toHotlineTime(fileInfo.ModTime())
		binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()-f.dataOffset))
		ft, _ = fileTypeFromInfo(fileInfo)
	}

	f.ffo.FlatFileHeader = FlatFileHeader{
		Format:    [4]byte{0x46, 0x49, 0x4c, 0x50}, // "FILP"
		Version:   [2]byte{0, 1},
		RSVD:      [16]byte{},
		ForkCount: [2]byte{0, 2},
	}

	_, err = f.fs.Stat(f.infoPath)
	if err == nil {
		b, err := f.fs.ReadFile(f.infoPath)
		if err != nil {
			return nil, err
		}

		f.ffo.FlatFileHeader.ForkCount[1] = 3

		if err := f.ffo.FlatFileInformationFork.UnmarshalBinary(b); err != nil {
			return nil, err
		}
	} else {
		f.ffo.FlatFileInformationFork = FlatFileInformationFork{
			Platform:         []byte("AMAC"), // TODO: Remove hardcode to support "AWIN" Platform (maybe?)
			TypeSignature:    []byte(ft.TypeCode),
			CreatorSignature: []byte(ft.CreatorCode),
			Flags:            []byte{0, 0, 0, 0},
			PlatformFlags:    []byte{0, 0, 1, 0}, // TODO: What is this?
			RSVD:             make([]byte, 32),
			CreateDate:       mTime, // some filesystems don't support createTime
			ModifyDate:       mTime,
			NameScript:       []byte{0, 0},
			Name:             []byte(f.name),
			NameSize:         []byte{0, 0},
			CommentSize:      []byte{0, 0},
			Comment:          []byte{},
		}
		binary.BigEndian.PutUint16(f.ffo.FlatFileInformationFork.NameSize, uint16(len(f.name)))
	}

	f.ffo.FlatFileInformationForkHeader = FlatFileForkHeader{
		ForkType:        [4]byte{0x49, 0x4E, 0x46, 0x4F}, // "INFO"
		CompressionType: [4]byte{},
		RSVD:            [4]byte{},
		DataSize:        f.ffo.FlatFileInformationFork.Size(),
	}

	f.ffo.FlatFileDataForkHeader = FlatFileForkHeader{
		ForkType:        [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
		CompressionType: [4]byte{},
		RSVD:            [4]byte{},
		DataSize:        [4]byte{dataSize[0], dataSize[1], dataSize[2], dataSize[3]},
	}
	f.ffo.FlatFileResForkHeader = f.rsrcForkHeader()

	return f.ffo, nil
}
