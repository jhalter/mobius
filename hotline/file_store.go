package hotline

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
