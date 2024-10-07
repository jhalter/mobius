package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/xattr"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	IncompleteFileSuffix = ".incomplete"
	InfoForkNameTemplate = ".info_%s" // template string for info fork filenames
	RsrcForkNameTemplate = ".rsrc_%s" // template string for resource fork filenames
)

// File encapsulates the data, info, and resource forks of a Hotline file and provides methods to manage the files.
type File struct {
	Name string

	dataOffset int64
	path       string // path to file directory

	IsDir      bool
	Incomplete bool

	DataFork fork
	RsrcFork fork
	InfoFork fork
}

// Q: What is a hotline.File?
// A: A logical abstraction.
//    A collection of three forks: data, rsrc, info.
//    A collection of methods to perform operations on the set of forks:
//		Move
// 		Delete
//
// Q: What is a fork?
// A: A slice of bytes that we need to read, write, seek, close.
//
// Q: What do we need to do with a fork?
// A: Get its size, optionally subtracting an offset.
// 	  Read/Seek/Write/Close
//
// What do we need to do with a logical file?
// * Check if it exists
// * Check if it isDir
// * Check if it is regular file
// * Get its create/modify time
// * Delete
// * Move
// *

// A hotline.File has forks

type fork interface {
	Size() int64
	Delete() error
	Move(newPath string) error
	Stat() (os.FileInfo, error)
	io.Seeker
	io.ReadWriteCloser
}

//type noopFork struct{}
//
//func (f *noopFork) Size() int64 {
//	return 0
//}
//
//func (f *noopFork) Stat() (os.FileInfo, error) {
//	return nil, nil
//}
//
//func (f *noopFork) Seek(offset int64, whence int) (int64, error) {
//	return 0, nil
//}
//
//func (f *noopFork) Read(p []byte) (n int, err error) {
//	return 0, io.EOF
//}
//
//func (f *noopFork) Write(p []byte) (n int, err error) {
//	return 0, nil
//}
//
//func (f *noopFork) Close() error {
//	return nil
//}
//
//func (f *noopFork) Delete() error {
//	return nil
//}
//
//func (f *noopFork) Move(newPath string) error {
//	return nil
//}

const (
	xattrResourceFork  = "com.apple.ResourceFork"
	xattrFinderInfo    = "com.apple.FinderInfo"
	xattrFinderComment = "com.apple.metadata:kMDItemFinderComment"
)

type xattrWrapper struct {
	attr       string
	data       []byte
	filePath   string
	readOffset int // Internal offset to track read progress

}

func newXattrFork(filepath, attr string, data []byte) fork {
	return &xattrWrapper{
		data:     data,
		filePath: filepath,
		attr:     attr,
	}
}

func (x *xattrWrapper) Read(p []byte) (n int, err error) {
	if x.readOffset >= len(x.data) {
		return 0, io.EOF // All bytes have been read
	}

	n = copy(p, x.data[x.readOffset:])
	x.readOffset += n

	return n, nil
}

func (x *xattrWrapper) Write(data []byte) (int, error) {
	x.data = append(x.data, data...)

	return len(data), nil
}

func (x *xattrWrapper) Close() error {
	spew.Dump("Writing xattr!", x.filePath, x.attr, len(x.data))

	switch x.attr {
	case xattrFinderInfo:
		var ffif FlatFileInformationFork
		r := bytes.NewReader(x.data)
		io.Copy(&ffif, r)
		spew.Dump(ffif)

		finderInfo := slices.Concat(
			ffif.TypeSignature[:],
			ffif.CreatorSignature[:],
			[]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0, 0, 0, 0, 0, 0, 0},
		)

		if err := xattr.Set(x.filePath, "com.trtphotl.Mobius", x.data); err != nil {
			spew.Dump(err)
		}

		spew.Dump(finderInfo)
		return xattr.Set(x.filePath, xattrFinderInfo, finderInfo)
	}

	return xattr.Set(x.filePath, x.attr, x.data)
}

func (x *xattrWrapper) Size() int64 {
	return int64(len(x.data))
}

func (x *xattrWrapper) Delete() error {
	return nil
}

func (x *xattrWrapper) Move(_ string) error {
	return nil
}

func (x *xattrWrapper) Stat() (os.FileInfo, error) {
	return nil, nil
}
func (x *xattrWrapper) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

type rsrcForkSidecar struct {
	path         string
	nameTemplate string
	file         *os.File
}

func (f *rsrcForkSidecar) Size() int64 {
	fileInfo, err := f.file.Stat()
	if err != nil {
		panic(err)
	}

	return fileInfo.Size()
}

func (f *rsrcForkSidecar) Stat() (os.FileInfo, error) {
	return f.file.Stat()
}

func (f *rsrcForkSidecar) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *rsrcForkSidecar) Read(p []byte) (n int, err error) {
	return f.file.Read(p)
}

func (f *rsrcForkSidecar) Write(p []byte) (n int, err error) {
	return f.file.Write(p)
}

func (f *rsrcForkSidecar) Close() error {
	return f.file.Close()
}

func (f *rsrcForkSidecar) Delete() error {
	err := os.RemoveAll(f.path)
	if err != nil {
		return fmt.Errorf("remove file: %s", err)
	}

	return nil
}

func (f *rsrcForkSidecar) Move(newPath string) error {
	dir := filepath.Dir(newPath)
	fName := fmt.Sprintf(f.nameTemplate, filepath.Base(newPath))
	fullPath := filepath.Join(dir, fName)

	fmt.Printf("MOVE FORK: %s->%s\n", f.path, fullPath)

	err := os.Rename(f.path, fullPath)
	if err != nil {
		fmt.Printf("Rename failed: %s\n", err)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("move fork: %v", err)
	}

	return nil
}

func newFileFork(path, nameTemplate string, mode int) fork {
	//dir := filepath.Dir(path)
	//fName := filepath.Base(path)
	//fullPath := filepath.Join(dir, fmt.Sprintf(RsrcForkNameTemplate, fName))

	file, err := os.OpenFile(path, mode, 0644)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		spew.Dump(err)
		panic(err)
	}

	return &rsrcForkSidecar{
		path:         path,
		nameTemplate: nameTemplate,
		file:         file,
	}
}

type FileModes struct {
	DataMode int
	InfoMode int
	RsrcMode int
}

func NewFile(fs FileStore, path string, mode, infoMode, rsrcMode int, dataOffset int64) (*File, error) {
	dir := filepath.Dir(path)
	fName := filepath.Base(path)

	// Check for partial file transfer.
	if _, err := fs.Stat(path + IncompleteFileSuffix); err == nil {
		mode |= os.O_APPEND
		path += IncompleteFileSuffix
	}

	// Check missing data file.
	fileInfo, err := fs.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		path += IncompleteFileSuffix
	}

	f := File{
		Name:       fName,
		path:       dir,
		dataOffset: dataOffset, // TODO: move this out of File
		IsDir:      fileInfo.IsDir(),

		InfoFork: newFileFork(filepath.Join(dir, fmt.Sprintf(InfoForkNameTemplate, fName)), InfoForkNameTemplate, infoMode),
		RsrcFork: newFileFork(filepath.Join(dir, fmt.Sprintf(RsrcForkNameTemplate, fName)), RsrcForkNameTemplate, rsrcMode),
	}

	if !fileInfo.IsDir() {
		f.DataFork = newFileFork(path, "%s", mode)

	}

	//if runtime.GOOS != "darwin" {
	//	f.RsrcFork = newFileFork(filepath.Join(dir, fmt.Sprintf(RsrcForkNameTemplate, fName)), RsrcForkNameTemplate, rsrcMode)
	//	f.InfoFork = newFileFork(filepath.Join(dir, fmt.Sprintf(InfoForkNameTemplate, fName)), InfoForkNameTemplate, infoMode)
	//}

	// Check if the file has extended attributes for the info and resource forks.
	//list, err := xattr.List(path)
	//if err != nil {
	//	return nil, fmt.Errorf("read extended attributes: %w", err)
	//}

	//if runtime.GOOS == "darwin" {
	//	if infoMode&os.O_CREATE != 0 {
	//		f.InfoFork = newXattrFork(path, xattrFinderInfo, []byte{})
	//	}
	//
	//	if rsrcMode&os.O_CREATE != 0 {
	//		f.RsrcFork = newXattrFork(path, xattrResourceFork, []byte{})
	//	}
	//
	//	for _, attr := range list {
	//		switch attr {
	//		case "com.apple.metadata:kMDItemComment":
	//			data, err := xattr.Get(path, attr)
	//			if err != nil {
	//				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
	//			}
	//			spew.Dump(attr, data)
	//
	//		case xattrFinderComment:
	//			data, err := xattr.Get(path, attr)
	//			if err != nil {
	//				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
	//			}
	//
	//			// ([]uint8) (len=53 cap=1024) {
	//			// 00000000  62 70 6c 69 73 74 30 30  5b 54 65 73 74 43 6f 6d  |bplist00[TestCom|
	//			// 00000010  6d 65 6e 74 08 00 00 00  00 00 00 01 01 00 00 00  |ment............|
	//			// 00000020  00 00 00 00 01 00 00 00  00 00 00 00 00 00 00 00  |................|
	//			// 00000030  00 00 00 00 14                                    |.....|
	//			//}
	//			spew.Dump(attr, data)
	//		case xattrResourceFork:
	//			data, err := xattr.Get(path, attr)
	//			if err != nil {
	//				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
	//			}
	//
	//			f.RsrcFork = newXattrFork(path, xattrResourceFork, data)
	//
	//		case xattrFinderInfo:
	//			data, err := xattr.Get(path, attr)
	//			if err != nil {
	//				return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
	//			}
	//
	//			ffif := FlatFileInformationFork{
	//				Platform:         PlatformAMAC,
	//				TypeSignature:    [4]byte(data[0:4]),
	//				CreatorSignature: [4]byte(data[4:8]),
	//				Name:             []byte(f.Name),
	//			}
	//			binary.BigEndian.PutUint16(ffif.NameSize[:], uint16(len(f.Name)))
	//
	//			var b []byte
	//			buf := bytes.NewBuffer(b)
	//			n, err := io.Copy(buf, &ffif)
	//
	//			f.InfoFork = newXattrFork(path, attr, buf.Bytes()[:n])
	//
	//		case "com.trtphotl.Mobius":
	//			//data, err := xattr.Get(path, attr)
	//			//if err != nil {
	//			//	return nil, fmt.Errorf("get xattr: %s: %v", attr, err)
	//			//}
	//
	//			//f.InfoFork = bytes.NewReader(data)
	//		}
	//	}
	//
	//}

	return &f, nil
}

func (f *File) Size() []byte {
	size := f.DataFork.Size()
	if f.RsrcFork != nil {
		size += f.RsrcFork.Size()
	}

	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(size))

	return b
}

func (f *File) Stat() (os.FileInfo, error) {
	spew.Dump("f.IsDir", f.IsDir)
	if f.IsDir {
		return os.Stat(f.path)
	}
	return f.DataFork.Stat()
	//if fi, err := f.fs.Stat(f.dataPath); err == nil {
	//	return fi, nil
	//}
	//if fi, err := f.fs.Stat(f.incompletePath); err == nil {
	//	return fi, nil
	//}
	//
	//return nil, os.ErrNotExist
}

// Move a file and its associated meta files to newPath.
// Meta files include:
// * Partially uploaded file ending with .incomplete
// * Resource fork starting with .rsrc_
// * Info fork starting with .info
// During Move of the meta files, os.ErrNotExist is ignored as these files may legitimately not exist.
func (f *File) Move(newPath string) error {
	if err := f.DataFork.Move(newPath); err != nil {
		return fmt.Errorf("move data fork: %v", err)
	}

	if err := f.RsrcFork.Move(newPath); err != nil {
		return fmt.Errorf("move rsrc fork: %v", err)
	}

	if err := f.InfoFork.Move(newPath); err != nil {
		return fmt.Errorf("move info fork: %v", err)
	}

	return nil
}

func (f *File) Close() error {
	if f.DataFork != nil {
		if err := f.DataFork.Close(); err != nil {
			return fmt.Errorf("close data fork: %v", err)
		}
	}

	if f.InfoFork != nil {
		if err := f.InfoFork.Close(); err != nil {
			return fmt.Errorf("close info fork: %v", err)
		}
	}

	if f.RsrcFork != nil {
		if err := f.RsrcFork.Close(); err != nil {
			return fmt.Errorf("close rsrc fork: %v", err)
		}
	}

	return nil
}

// Delete a file and its associated metadata files if they exist
func (f *File) Delete() error {
	if err := f.DataFork.Delete(); err != nil {
		return fmt.Errorf("delete data fork: %v", err)
	}

	if err := f.InfoFork.Delete(); err != nil {
		return fmt.Errorf("delete info fork: %v", err)
	}

	if f.RsrcFork != nil {
		if err := f.RsrcFork.Delete(); err != nil {
			return fmt.Errorf("delete rsrc fork: %v", err)
		}
	}

	return nil
}

func (f *File) FlattenedFileObject() (*FlattenedFileObject, error) {
	ffo := FlattenedFileObject{
		FlatFileHeader: FlatFileHeader{
			Format:    FlatFileFormat,
			Version:   [2]byte{0, 1},
			ForkCount: [2]byte{0, 2},
		},
		FlatFileDataForkHeader: FlatFileForkHeader{
			ForkType: [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
		},
		FlatFileInformationForkHeader: FlatFileForkHeader{
			ForkType: [4]byte{0x49, 0x4E, 0x46, 0x4F}, // "INFO"
		},
		FlatFileResForkHeader: FlatFileForkHeader{
			ForkType: [4]byte{0x4D, 0x41, 0x43, 0x52}, // "MACR"
		},
		FlatFileInformationFork: FlatFileInformationFork{
			Platform:      PlatformAMAC,
			PlatformFlags: [4]byte{0, 0, 1, 0}, // TODO: What is this?
			Name:          []byte(f.Name),
			Comment:       []byte{},
		},
	}

	fileInfo, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if f.InfoFork != nil {
		if _, err := io.Copy(&ffo.FlatFileInformationFork, f.InfoFork); err != nil {
			return nil, fmt.Errorf("read info fork sidecar file: %s", err)
		}
	} else {

		ft := fileTypeFromInfo(fileInfo)
		ffo.FlatFileInformationFork.TypeSignature = ft.TypeCode
		ffo.FlatFileInformationFork.CreatorSignature = ft.CreatorCode

		ffo.FlatFileInformationFork.CreateDate = NewTime(fileInfo.ModTime())
		ffo.FlatFileInformationFork.ModifyDate = NewTime(fileInfo.ModTime())

		binary.BigEndian.PutUint16(ffo.FlatFileInformationFork.NameSize[:], uint16(len(f.Name)))
	}

	if strings.HasSuffix(fileInfo.Name(), IncompleteFileSuffix) {
		ffo.FlatFileInformationFork.TypeSignature = fileTypes[IncompleteFileSuffix].TypeCode
		ffo.FlatFileInformationFork.CreatorSignature = fileTypes[IncompleteFileSuffix].CreatorCode
	}

	var size int64
	if !f.IsDir {
		size = f.DataFork.Size() - f.dataOffset
	}
	// Populate data fork header.
	binary.BigEndian.PutUint32(ffo.FlatFileDataForkHeader.DataSize[:], uint32(size))

	// Populate info fork header.
	binary.BigEndian.PutUint32(
		ffo.FlatFileInformationForkHeader.DataSize[:],
		uint32(len(f.Name)+len(ffo.FlatFileInformationFork.Comment)+74), // 74 = len of fixed size headers
	)

	if f.RsrcFork != nil {
		ffo.FlatFileHeader.ForkCount = [2]byte{0, 3}

		// Populate resource fork header.
		binary.BigEndian.PutUint32(ffo.FlatFileResForkHeader.DataSize[:], uint32(f.RsrcFork.Size()))
	}

	return &ffo, nil
}
