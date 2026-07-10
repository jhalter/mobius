package hotline

// In-package copies of the mocks these tests need. They cannot use hotline/hltest — importing it
// from an in-package test would create an import cycle through hotline — so the mocks used by
// in-package tests live here. Cross-package consumers use hotline/hltest instead; keep the two in
// sync (the conformance assertions in both files catch signature drift).

import (
	"io"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/mock"
)

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

var _ fs.FileInfo = (*MockFileInfo)(nil)

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

type MockClientMgr struct {
	mock.Mock
}

var _ ClientManager = (*MockClientMgr)(nil)

func (m *MockClientMgr) List() []*ClientConn {
	args := m.Called()

	return args.Get(0).([]*ClientConn)
}

func (m *MockClientMgr) Get(id ClientID) *ClientConn {
	args := m.Called(id)

	return args.Get(0).(*ClientConn)
}

func (m *MockClientMgr) Add(cc *ClientConn) {
	m.Called(cc)
}

func (m *MockClientMgr) Delete(id ClientID) {
	m.Called(id)
}
