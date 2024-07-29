package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/xattr"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	IncompleteFileSuffix = ".incomplete"
	infoForkNameTemplate = ".info_%s" // template string for info fork filenames
	rsrcForkNameTemplate = ".rsrc_%s" // template string for resource fork filenames
)

type xattrWrapper struct {
	data     []byte
	filePath string
}

func (x *xattrWrapper) Write(data []byte) (int, error) {
	//spew.Dump("(x *xattrWrapper) Write", data)
	x.data = append(x.data, data...)
	spew.Dump("w")
	return len(data), nil
}

func (x *xattrWrapper) Close() error {
	spew.Dump("Writing xattr!", x.filePath, len(x.data))
	return xattr.Set(x.filePath, xattrResourceFork, x.data)
}

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

	rsrcSize [4]byte

	rsrcForkReader io.Reader
	rsrcWriter     io.Writer
}

const (
	xattrResourceFork  = "com.apple.ResourceFork"
	xattrFinderInfo    = "com.apple.FinderInfo"
	xattrFinderComment = "com.apple.metadata:kMDItemFinderComment"
)

const (
	forkModeOff = iota
	forkModeSidecar
	forkModeXattr
)

func NewFileWrapper(fs FileStore, path string, dataOffset int64) (*fileWrapper, error) {
	dir := filepath.Dir(path)
	fName := filepath.Base(path)

	if runtime.GOOS == "darwin" {
		spew.Dump("🍎")
	}

	//rsrcPath := filepath.Join(dir, fmt.Sprintf(rsrcForkNameTemplate, fName))

	f := fileWrapper{
		fs:             fs,
		Name:           fName,
		path:           dir,
		dataPath:       path,
		dataOffset:     dataOffset,
		rsrcPath:       filepath.Join(dir, fmt.Sprintf(rsrcForkNameTemplate, fName)),
		infoPath:       filepath.Join(dir, fmt.Sprintf(infoForkNameTemplate, fName)),
		incompletePath: filepath.Join(dir, fName+IncompleteFileSuffix),
		Ffo:            &flattenedFileObject{},
		rsrcWriter:     io.Discard,
		rsrcForkReader: bytes.NewReader([]byte{}),
	}

	fileInfo, err := f.fs.Stat(filepath.Join(dir, fmt.Sprintf(rsrcForkNameTemplate, fName)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat resource fork: %v", err)
	}
	if err == nil {
		binary.BigEndian.PutUint32(f.rsrcSize[:], uint32(fileInfo.Size()))
		f.rsrcForkReader, err = os.Open(f.rsrcPath)
		if err != nil {
			return nil, fmt.Errorf("open resource fork file: %v", err)
		}
	}

	// Check if the file has extended attributes for the info and resource forks.
	list, err := xattr.List(path)
	if err != nil {
		return nil, fmt.Errorf("read extended attributes: %w", err)
	}

	// com.apple.metadata:kMDItemFinderComment: macOS Finder comment
	// com.apple.FinderInfo: type/creator
	// com.apple.ResourceFork: resource fork data
	for _, attr := range list {
		switch attr {
		case "com.apple.ResourceFork":
			data, err := xattr.Get(path, attr)
			if err != nil {
				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
			}

			f.rsrcForkReader = bytes.NewReader(data)
			binary.BigEndian.PutUint32(f.rsrcSize[:], uint32(len(data)))

		case xattrFinderInfo:
			data, err := xattr.Get(path, attr)
			if err != nil {
				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
			}

			typeCode := data[0:4]
			creatorCode := data[4:8]
			spew.Dump(typeCode, creatorCode)
		}
	}

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

	s += int64(binary.BigEndian.Uint32(f.rsrcSize[:]))

	binary.BigEndian.PutUint32(size, uint32(s))

	return size
}

func (f *fileWrapper) rsrcForkHeader() FlatFileForkHeader {
	return FlatFileForkHeader{
		ForkType: forkTypeMACR,
		DataSize: f.rsrcSize,
	}
}

func (f *fileWrapper) incompleteDataName() string {
	return f.Name + IncompleteFileSuffix
}

func (f *fileWrapper) rsrcForkName() string {
	return fmt.Sprintf(rsrcForkNameTemplate, f.Name)
}

func (f *fileWrapper) infoForkName() string {
	return fmt.Sprintf(infoForkNameTemplate, f.Name)
}

// TODO: on macOS, write to xattr
func (f *fileWrapper) rsrcForkWriter() (io.WriteCloser, error) {
	spew.Dump(runtime.GOOS)
	if runtime.GOOS == "darwin" {
		return &xattrWrapper{
			filePath: f.dataPath + ".incomplete",
			data:     make([]byte, 0),
		}, nil
	}

	return os.OpenFile(f.rsrcPath, os.O_CREATE|os.O_WRONLY, 0644)
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

func (f *fileWrapper) dataForkReader() (io.ReadCloser, error) {
	return f.fs.Open(f.dataPath)
}

//
//func (f *fileWrapper) rsrcForkFile() (io.ReadCloser, error) {
//	return f.fs.Open(f.rsrcPath)
//}

//func (f *fileWrapper) rsrcForkReader() (io.ReadCloser, error) {
//	return f.fs.Open(f.rsrcPath)
//}

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

// Delete a file and any associated sidecar files if they exist.
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
	fileInfo, err := f.fs.Stat(f.dataPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat data file: %v", err)
	}

	// If the file doesn't exist, it might be partially transferred.
	if errors.Is(err, fs.ErrNotExist) {
		fileInfo, err = f.fs.Stat(f.incompletePath)
		if err != nil {
			return nil, fmt.Errorf("stat incomplete file: %v", err)
		}
	}

	ft := fileTypeFromInfo(fileInfo)

	list, err := xattr.List(f.dataPath)
	if err != nil {
		return nil, fmt.Errorf("read extended attributes: %w", err)
	}
	for _, attr := range list {
		// Read type and creater codes from com.apple.FinderInfo xattr if present.  This should look like:
		// 00000000  41 50 50 4C C3 2B 47 57 00 00 00 00 00 00 00 00  |APPL.+GW........|
		// 00000010  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  |................|
		// 00000020
		if attr == "com.apple.FinderInfo" {
			data, err := xattr.Get(f.dataPath, attr)
			if err != nil {
				log.Fatal(err)
			}

			if len(data) > 8 {
				ft.TypeCode = string(data[0:4])
				ft.CreatorCode = string(data[4:8])
			}
		}
	}

	f.Ffo.FlatFileHeader = FlatFileHeader{
		Format:    [4]byte{0x46, 0x49, 0x4c, 0x50}, // "FILP"
		Version:   [2]byte{0, 1},
		ForkCount: [2]byte{0, 2},
	}

	// Check to see if we have an info fork sidecar file.
	_, err = f.fs.Stat(f.infoPath)
	if err == nil {
		b, err := f.fs.ReadFile(f.infoPath)
		if err != nil {
			return nil, fmt.Errorf("read info fork: %v", err)
		}

		f.Ffo.FlatFileHeader.ForkCount[1] = 3

		if _, err := io.Copy(&f.Ffo.FlatFileInformationFork, bytes.NewReader(b)); err != nil {
			return nil, fmt.Errorf("copy FlatFileInformationFork: %w", err)
		}

	} else {
		mTime := NewTime(fileInfo.ModTime())

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

		binary.BigEndian.PutUint16(f.Ffo.FlatFileInformationFork.NameSize[:], uint16(len(f.Name)))
	}

	if strings.Contains(fileInfo.Name(), IncompleteFileSuffix) {
		f.Ffo.FlatFileInformationFork.TypeSignature = [4]byte([]byte("HTft"))
		f.Ffo.FlatFileInformationFork.CreatorSignature = [4]byte([]byte("HTLC"))
	}

	f.Ffo.FlatFileInformationForkHeader = FlatFileForkHeader{
		ForkType: forkTypeInfo,
		DataSize: f.Ffo.FlatFileInformationFork.Size(),
	}

	f.Ffo.FlatFileDataForkHeader = FlatFileForkHeader{ForkType: forkTypeData}
	binary.BigEndian.PutUint32(f.Ffo.FlatFileDataForkHeader.DataSize[:], uint32(fileInfo.Size()-f.dataOffset))

	f.Ffo.FlatFileResForkHeader = f.rsrcForkHeader()

	return f.Ffo, nil
}
