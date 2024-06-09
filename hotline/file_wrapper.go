package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	incompleteFileSuffix = ".incomplete"
	infoForkNameTemplate = ".info_%s" // template string for info fork filenames
	rsrcForkNameTemplate = ".rsrc_%s" // template string for resource fork filenames
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
	ffo            *flattenedFileObject
}

func newFileWrapper(fs FileStore, path string, dataOffset int64) (*fileWrapper, error) {
	dir := filepath.Dir(path)
	fName := filepath.Base(path)
	f := fileWrapper{
		fs:             fs,
		name:           fName,
		path:           dir,
		dataPath:       path,
		dataOffset:     dataOffset,
		rsrcPath:       filepath.Join(dir, fmt.Sprintf(rsrcForkNameTemplate, fName)),
		infoPath:       filepath.Join(dir, fmt.Sprintf(infoForkNameTemplate, fName)),
		incompletePath: filepath.Join(dir, fName+incompleteFileSuffix),
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
	return fmt.Sprintf(rsrcForkNameTemplate, f.name)
}

func (f *fileWrapper) infoForkName() string {
	return fmt.Sprintf(infoForkNameTemplate, f.name)
}

func (f *fileWrapper) rsrcForkWriter() (io.WriteCloser, error) {
	file, err := os.OpenFile(f.rsrcPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) infoForkWriter() (io.WriteCloser, error) {
	file, err := os.OpenFile(f.infoPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) incFileWriter() (io.WriteCloser, error) {
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

// move a fileWrapper and its associated meta files to newPath.
// Meta files include:
// * Partially uploaded file ending with .incomplete
// * Resource fork starting with .rsrc_
// * Info fork starting with .info
// During move of the meta files, os.ErrNotExist is ignored as these files may legitimately not exist.
func (f *fileWrapper) move(newPath string) error {
	err := f.fs.Rename(f.dataPath, filepath.Join(newPath, f.name))
	if err != nil {
		return err
	}

	err = f.fs.Rename(f.incompletePath, filepath.Join(newPath, f.incompleteDataName()))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = f.fs.Rename(f.rsrcPath, filepath.Join(newPath, f.rsrcForkName()))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = f.fs.Rename(f.infoPath, filepath.Join(newPath, f.infoForkName()))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// delete a fileWrapper and its associated metadata files if they exist
func (f *fileWrapper) delete() error {
	err := f.fs.RemoveAll(f.dataPath)
	if err != nil {
		return err
	}

	err = f.fs.Remove(f.incompletePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = f.fs.Remove(f.rsrcPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = f.fs.Remove(f.infoPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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

		_, err = io.Copy(&f.ffo.FlatFileInformationFork, bytes.NewReader(b))
		if err != nil {
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
