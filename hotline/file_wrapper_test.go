package hotline

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFile_DataFile(t *testing.T) {
	t.Run("returns file info when data file exists", func(t *testing.T) {
		mfs := &MockFileStore{}
		mfi := &MockFileInfo{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
		}

		mfs.On("Stat", "/files/testfile.txt").Return(mfi, nil)

		fi, err := f.DataFile()

		assert.NoError(t, err)
		assert.Equal(t, mfi, fi)
		mfs.AssertExpectations(t)
	})

	t.Run("returns file info from incomplete file when data file not found", func(t *testing.T) {
		mfs := &MockFileStore{}
		mfi := &MockFileInfo{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
		}

		mfs.On("Stat", "/files/testfile.txt").Return(nil, os.ErrNotExist)
		mfs.On("Stat", "/files/testfile.txt.incomplete").Return(mfi, nil)

		fi, err := f.DataFile()

		assert.NoError(t, err)
		assert.Equal(t, mfi, fi)
		mfs.AssertExpectations(t)
	})

	t.Run("returns error when neither data file nor incomplete file exists", func(t *testing.T) {
		mfs := &MockFileStore{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
		}

		mfs.On("Stat", "/files/testfile.txt").Return(nil, os.ErrNotExist)
		mfs.On("Stat", "/files/testfile.txt.incomplete").Return(nil, os.ErrNotExist)

		fi, err := f.DataFile()

		assert.Nil(t, fi)
		assert.EqualError(t, err, "file or directory not found")
		mfs.AssertExpectations(t)
	})
}

func TestFile_Move(t *testing.T) {
	t.Run("succeeds when data rename works and meta files do not exist", func(t *testing.T) {
		mfs := &MockFileStore{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
			rsrcPath:       "/files/.rsrc_testfile.txt",
			infoPath:       "/files/.info_testfile.txt",
		}

		newPath := "/dest"

		mfs.On("Rename", "/files/testfile.txt", "/dest/testfile.txt").Return(nil)
		mfs.On("Rename", "/files/testfile.txt.incomplete", "/dest/testfile.txt.incomplete").Return(os.ErrNotExist)
		mfs.On("Rename", "/files/.rsrc_testfile.txt", "/dest/.rsrc_testfile.txt").Return(os.ErrNotExist)
		mfs.On("Rename", "/files/.info_testfile.txt", "/dest/.info_testfile.txt").Return(os.ErrNotExist)

		err := f.Move(newPath)

		assert.NoError(t, err)
		mfs.AssertExpectations(t)
	})

	t.Run("returns error when data file rename fails", func(t *testing.T) {
		mfs := &MockFileStore{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
			rsrcPath:       "/files/.rsrc_testfile.txt",
			infoPath:       "/files/.info_testfile.txt",
		}

		renameErr := errors.New("permission denied")
		mfs.On("Rename", "/files/testfile.txt", "/dest/testfile.txt").Return(renameErr)

		err := f.Move("/dest")

		assert.ErrorIs(t, err, renameErr)
		// Meta file renames should not have been called.
		mfs.AssertNotCalled(t, "Rename", mock.Anything, "/dest/testfile.txt.incomplete")
		mfs.AssertExpectations(t)
	})
}

func TestFile_Delete(t *testing.T) {
	t.Run("succeeds when RemoveAll works and meta files do not exist", func(t *testing.T) {
		mfs := &MockFileStore{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
			rsrcPath:       "/files/.rsrc_testfile.txt",
			infoPath:       "/files/.info_testfile.txt",
		}

		mfs.On("RemoveAll", "/files/testfile.txt").Return(nil)
		mfs.On("Remove", "/files/testfile.txt.incomplete").Return(os.ErrNotExist)
		mfs.On("Remove", "/files/.rsrc_testfile.txt").Return(os.ErrNotExist)
		mfs.On("Remove", "/files/.info_testfile.txt").Return(os.ErrNotExist)

		err := f.Delete()

		assert.NoError(t, err)
		mfs.AssertExpectations(t)
	})

	t.Run("returns error when RemoveAll fails", func(t *testing.T) {
		mfs := &MockFileStore{}

		f := &File{
			fs:             mfs,
			Name:           "testfile.txt",
			dataPath:       "/files/testfile.txt",
			incompletePath: "/files/testfile.txt.incomplete",
			rsrcPath:       "/files/.rsrc_testfile.txt",
			infoPath:       "/files/.info_testfile.txt",
		}

		removeErr := errors.New("permission denied")
		mfs.On("RemoveAll", "/files/testfile.txt").Return(removeErr)

		err := f.Delete()

		assert.ErrorIs(t, err, removeErr)
		// Meta file removes should not have been called.
		mfs.AssertNotCalled(t, "Remove", mock.Anything)
		mfs.AssertExpectations(t)
	})
}
