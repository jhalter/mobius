package hotline

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/mock"
)

// FileStore is the storage backend for the file library (the FileRoot that clients browse,
// upload, and download). It is deliberately expressed in terms of io/fs interface types rather
// than *os.File so that non-filesystem backends (e.g. an object store such as S3/R2) can
// implement it. OSFileStore is the default, filesystem-backed implementation.
//
// Symlink/ReadLink exist only to support Hotline aliases and have no analog on object stores;
// such backends may return errors.ErrUnsupported, and callers degrade gracefully.
type FileStore interface {
	// Reads
	Open(name string) (io.ReadCloser, error)
	Stat(name string) (fs.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadLink(name string) (string, error)
	Walk(root string, fn filepath.WalkFunc) error

	// Writes
	Create(name string) (io.WriteCloser, error)
	OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Mkdir(name string, perm fs.FileMode) error

	// Mutations
	Rename(oldpath string, newpath string) error
	Remove(name string) error
	RemoveAll(path string) error
	Symlink(oldname, newname string) error
}

// OSFileStore is a FileStore backed by the local filesystem via the os and filepath packages.
type OSFileStore struct{}

var _ FileStore = (*OSFileStore)(nil)

func (*OSFileStore) Mkdir(name string, perm fs.FileMode) error {
	return os.Mkdir(name, perm)
}

func (*OSFileStore) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (*OSFileStore) Open(name string) (io.ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (*OSFileStore) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (*OSFileStore) ReadLink(name string) (string, error) {
	return os.Readlink(name)
}

func (*OSFileStore) Walk(root string, fn filepath.WalkFunc) error {
	return filepath.Walk(root, fn)
}

func (*OSFileStore) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (*OSFileStore) RemoveAll(name string) error {
	return os.RemoveAll(name)
}

func (*OSFileStore) Remove(name string) error {
	return os.Remove(name)
}

func (*OSFileStore) Create(name string) (io.WriteCloser, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (*OSFileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (*OSFileStore) Rename(oldpath string, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (*OSFileStore) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (*OSFileStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	f, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type MockFileStore struct {
	mock.Mock
}

var _ FileStore = (*MockFileStore)(nil)

func (mfs *MockFileStore) Mkdir(name string, perm fs.FileMode) error {
	args := mfs.Called(name, perm)
	return args.Error(0)
}

func (mfs *MockFileStore) Stat(name string) (fs.FileInfo, error) {
	args := mfs.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(fs.FileInfo), args.Error(1)
}

func (mfs *MockFileStore) Open(name string) (io.ReadCloser, error) {
	args := mfs.Called(name)
	f, _ := args.Get(0).(io.ReadCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) ReadDir(name string) ([]fs.DirEntry, error) {
	args := mfs.Called(name)
	entries, _ := args.Get(0).([]fs.DirEntry)
	return entries, args.Error(1)
}

func (mfs *MockFileStore) ReadLink(name string) (string, error) {
	args := mfs.Called(name)
	return args.String(0), args.Error(1)
}

func (mfs *MockFileStore) Walk(root string, fn filepath.WalkFunc) error {
	args := mfs.Called(root, fn)
	return args.Error(0)
}

func (mfs *MockFileStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	args := mfs.Called(name, flag, perm)
	f, _ := args.Get(0).(io.WriteCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) Symlink(oldname, newname string) error {
	args := mfs.Called(oldname, newname)
	return args.Error(0)
}

func (mfs *MockFileStore) RemoveAll(name string) error {
	args := mfs.Called(name)
	return args.Error(0)
}

func (mfs *MockFileStore) Remove(name string) error {
	args := mfs.Called(name)
	return args.Error(0)
}

func (mfs *MockFileStore) Create(name string) (io.WriteCloser, error) {
	args := mfs.Called(name)
	f, _ := args.Get(0).(io.WriteCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	args := mfs.Called(name, data, perm)
	return args.Error(0)
}

func (mfs *MockFileStore) Rename(oldpath, newpath string) error {
	args := mfs.Called(oldpath, newpath)
	return args.Error(0)
}

func (mfs *MockFileStore) ReadFile(name string) ([]byte, error) {
	args := mfs.Called(name)
	return args.Get(0).([]byte), args.Error(1)
}

type MockFileInfo struct {
	mock.Mock
}

func (mfi *MockFileInfo) Name() string {
	args := mfi.Called()
	return args.String(0)
}

func (mfi *MockFileInfo) Size() int64 {
	args := mfi.Called()
	return args.Get(0).(int64)
}

func (mfi *MockFileInfo) Mode() fs.FileMode {
	args := mfi.Called()
	return args.Get(0).(fs.FileMode)
}

func (mfi *MockFileInfo) ModTime() time.Time {
	_ = mfi.Called()
	return time.Now()
}

func (mfi *MockFileInfo) IsDir() bool {
	args := mfi.Called()
	return args.Bool(0)
}

func (mfi *MockFileInfo) Sys() interface{} {
	_ = mfi.Called()
	return nil
}
