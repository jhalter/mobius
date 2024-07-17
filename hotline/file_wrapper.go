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
	IncompleteFileSuffix = ".incomplete"
	InfoForkNameTemplate = ".info_%s" // template string for info fork filenames
	RsrcForkNameTemplate = ".rsrc_%s" // template string for resource fork filenames
)

// fileWrapper encapsulates the data, info, and resource forks of a Hotline file and provides methods to manage the files.
type fileWrapper struct {
	fs             FileStore
	Name           string // Name of the file
	path           string // path to file directory
	dataPath       string // path to the file data fork
	dataOffset     int64
	rsrcPath       string // path to the file resource fork
	infoPath       string // path to the file information fork
	incompletePath string // path to partially transferred temp file
	Ffo            *flattenedFileObject
}

func NewFileWrapper(fs FileStore, path string, dataOffset int64) (*fileWrapper, error) {
	dir := filepath.Dir(path)
	fName := filepath.Base(path)
	f := fileWrapper{
		fs:             fs,
		Name:           fName,
		path:           dir,
		dataPath:       path,
		dataOffset:     dataOffset,
		rsrcPath:       filepath.Join(dir, fmt.Sprintf(RsrcForkNameTemplate, fName)),
		infoPath:       filepath.Join(dir, fmt.Sprintf(InfoForkNameTemplate, fName)),
		incompletePath: filepath.Join(dir, fName+IncompleteFileSuffix),
		Ffo:            &flattenedFileObject{},
	}

	var err error
	f.Ffo, err = f.flattenedFileObject()
	if err != nil {
		return nil, err
	}

	return &f, nil
}

func (f *fileWrapper) TotalSize() []byte {
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
		ForkType: [4]byte{0x4D, 0x41, 0x43, 0x52}, // "MACR"
		DataSize: f.rsrcForkSize(),
	}
}

func (f *fileWrapper) incompleteDataName() string {
	return f.Name + IncompleteFileSuffix
}

func (f *fileWrapper) rsrcForkName() string {
	return fmt.Sprintf(RsrcForkNameTemplate, f.Name)
}

func (f *fileWrapper) infoForkName() string {
	return fmt.Sprintf(InfoForkNameTemplate, f.Name)
}

func (f *fileWrapper) rsrcForkWriter() (io.WriteCloser, error) {
	file, err := os.OpenFile(f.rsrcPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fileWrapper) InfoForkWriter() (io.WriteCloser, error) {
	file, err := os.OpenFile(f.infoPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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

func (f *fileWrapper) DataFile() (os.FileInfo, error) {
	if fi, err := f.fs.Stat(f.dataPath); err == nil {
		return fi, nil
	}
	if fi, err := f.fs.Stat(f.incompletePath); err == nil {
		return fi, nil
	}

	return nil, errors.New("file or directory not found")
}

// Move a file and its associated meta files to newPath.
// Meta files include:
// * Partially uploaded file ending with .incomplete
// * Resource fork starting with .rsrc_
// * Info fork starting with .info
// During Move of the meta files, os.ErrNotExist is ignored as these files may legitimately not exist.
func (f *fileWrapper) Move(newPath string) error {
	err := f.fs.Rename(f.dataPath, filepath.Join(newPath, f.Name))
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

// Delete a fileWrapper and its associated metadata files if they exist
func (f *fileWrapper) Delete() error {
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
	mTime := [8]byte{}

	ft := defaultFileType

	fileInfo, err := f.fs.Stat(f.dataPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, fs.ErrNotExist) {
		fileInfo, err = f.fs.Stat(f.incompletePath)
		if err == nil {
			mTime = NewTime(fileInfo.ModTime())
			binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()-f.dataOffset))
			ft, _ = fileTypeFromInfo(fileInfo)
		}
	} else {
		mTime = NewTime(fileInfo.ModTime())
		binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()-f.dataOffset))
		ft, _ = fileTypeFromInfo(fileInfo)
	}

	f.Ffo.FlatFileHeader = FlatFileHeader{
		Format:    [4]byte{0x46, 0x49, 0x4c, 0x50}, // "FILP"
		Version:   [2]byte{0, 1},
		ForkCount: [2]byte{0, 2},
	}

	_, err = f.fs.Stat(f.infoPath)
	if err == nil {
		b, err := f.fs.ReadFile(f.infoPath)
		if err != nil {
			return nil, err
		}

		f.Ffo.FlatFileHeader.ForkCount[1] = 3

		_, err = io.Copy(&f.Ffo.FlatFileInformationFork, bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("error copying FlatFileInformationFork: %w", err)
		}
	} else {
		f.Ffo.FlatFileInformationFork = FlatFileInformationFork{
			Platform:         [4]byte{0x41, 0x4D, 0x41, 0x43}, // "AMAC" TODO: Remove hardcode to support "AWIN" Platform (maybe?)
			TypeSignature:    [4]byte([]byte(ft.TypeCode)),
			CreatorSignature: [4]byte([]byte(ft.CreatorCode)),
			PlatformFlags:    [4]byte{0, 0, 1, 0}, // TODO: What is this?
			CreateDate:       mTime,               // some filesystems don't support createTime
			ModifyDate:       mTime,
			Name:             []byte(f.Name),
			Comment:          []byte{},
		}

		ns := make([]byte, 2)
		binary.BigEndian.PutUint16(ns, uint16(len(f.Name)))
		f.Ffo.FlatFileInformationFork.NameSize = [2]byte(ns[:])
	}

	f.Ffo.FlatFileInformationForkHeader = FlatFileForkHeader{
		ForkType: [4]byte{0x49, 0x4E, 0x46, 0x4F}, // "INFO"
		DataSize: f.Ffo.FlatFileInformationFork.Size(),
	}

	f.Ffo.FlatFileDataForkHeader = FlatFileForkHeader{
		ForkType: [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
		DataSize: [4]byte{dataSize[0], dataSize[1], dataSize[2], dataSize[3]},
	}
	f.Ffo.FlatFileResForkHeader = f.rsrcForkHeader()

	return f.Ffo, nil
}
