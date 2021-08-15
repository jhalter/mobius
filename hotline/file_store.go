package hotline

import (
	"github.com/stretchr/testify/mock"
	"os"
)

var FS FileStore

type FileStore interface {
	Mkdir(name string, perm os.FileMode) error

	Stat(name string) (os.FileInfo, error)
	// TODO: implement

	//Rename(oldpath string, newpath string) error
	//RemoveAll(path string) error
}

type OSFileStore struct{}

func (fs OSFileStore) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (fs OSFileStore) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

type MockFileStore struct {
	mock.Mock
}

func (mfs MockFileStore) Mkdir(name string, perm os.FileMode) error {
	args := mfs.Called(name, perm)
	return args.Error(0)
}

func (mfs MockFileStore) Stat(name string) (os.FileInfo, error) {
	args := mfs.Called(name)
	if  args.Get(0) == nil {
		return nil, args.Error(1)

	}
	return args.Get(0).(os.FileInfo), args.Error(1)
}
