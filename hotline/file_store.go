package hotline

import (
	"github.com/stretchr/testify/mock"
	"io/fs"
	"os"
	"time"
)

type FileStore interface {
	Create(name string) (*os.File, error)
	Mkdir(name string, perm os.FileMode) error
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	Remove(name string) error
	RemoveAll(path string) error
	Rename(oldpath string, newpath string) error
	Stat(name string) (fs.FileInfo, error)
	Symlink(oldname, newname string) error
	WriteFile(name string, data []byte, perm fs.FileMode) error
	ReadFile(name string) ([]byte, error)
}

type OSFileStore struct{}

func (fs *OSFileStore) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (fs *OSFileStore) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (fs *OSFileStore) Open(name string) (*os.File, error) {
	return os.Open(name)
}

func (fs *OSFileStore) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (fs *OSFileStore) RemoveAll(name string) error {
	return os.RemoveAll(name)
}

func (fs *OSFileStore) Remove(name string) error {
	return os.Remove(name)
}

func (fs *OSFileStore) Create(name string) (*os.File, error) {
	return os.Create(name)
}

func (fs *OSFileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (fs *OSFileStore) Rename(oldpath string, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (fs *OSFileStore) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fs *OSFileStore) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

type MockFileStore struct {
	mock.Mock
}

func (mfs *MockFileStore) Mkdir(name string, perm os.FileMode) error {
	args := mfs.Called(name, perm)
	return args.Error(0)
}

func (mfs *MockFileStore) Stat(name string) (os.FileInfo, error) {
	args := mfs.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)

	}
	return args.Get(0).(os.FileInfo), args.Error(1)
}

func (mfs *MockFileStore) Open(name string) (*os.File, error) {
	args := mfs.Called(name)
	return args.Get(0).(*os.File), args.Error(1)
}

func (mfs *MockFileStore) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	args := mfs.Called(name, flag, perm)
	return args.Get(0).(*os.File), args.Error(1)
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

func (mfs *MockFileStore) Create(name string) (*os.File, error) {
	args := mfs.Called(name)
	return args.Get(0).(*os.File), args.Error(1)
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
