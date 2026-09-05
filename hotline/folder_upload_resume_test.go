package hotline

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resumeFolderUpload(t *testing.T, store FileStore, root string, payload []byte) error {
	t.Helper()
	count, encodedPath := encodeFolderUploadPath("resume.txt")
	size := len(encodedPath) + 4
	wire := []byte{byte(size >> 8), byte(size), 0, 0, byte(count >> 8), byte(count)}
	wire = append(wire, encodedPath...)
	transferSize := u32b(len(payload))
	wire = append(wire, transferSize[:]...)
	wire = append(wire, payload...)
	transfer := &FileTransfer{FolderItemCount: []byte{0, 1}, bytesSentCounter: &WriteCounter{}}
	return UploadFolderHandler(readWriter{r: bytes.NewReader(wire)}, root, transfer, store,
		slog.New(slog.NewTextHandler(io.Discard, nil)), false)
}

func TestUploadFolderResumePreservesInterruptedFile(t *testing.T) {
	for _, backend := range []string{"os", "memory"} {
		t.Run(backend, func(t *testing.T) {
			var store FileStore = &OSFileStore{}
			if backend == "memory" {
				store = NewMemFileStore()
			}
			root := t.TempDir()
			partial := filepath.Join(root, "resume.txt"+IncompleteFileSuffix)
			complete := filepath.Join(root, "resume.txt")
			require.NoError(t, store.WriteFile(partial, []byte("012"), 0644))
			payload := flatFileBytes("resume.txt", []byte("3456789"), nil)
			// The sender disconnects after two of the seven declared bytes arrive.
			err := resumeFolderUpload(t, store, root, payload[:len(payload)-5])
			require.Error(t, err)
			_, err = store.Stat(complete)
			assert.True(t, os.IsNotExist(err), "a failed transfer must not create the final file")
			data, err := store.ReadFile(partial)
			require.NoError(t, err)
			require.Equal(t, "01234", string(data), "received progress must remain resumable")

			// A later retry appends the remaining bytes and only then finalizes the file.
			require.NoError(t, resumeFolderUpload(t, store, root, flatFileBytes("resume.txt", []byte("56789"), nil)))
			data, err = store.ReadFile(complete)
			require.NoError(t, err)
			assert.Equal(t, "0123456789", string(data))
			_, err = store.Stat(partial)
			assert.True(t, os.IsNotExist(err))
		})
	}
}

type resumeCloseErrorStore struct {
	FileStore
	err error
}

func (s resumeCloseErrorStore) OpenFile(name string, flags int, perm fs.FileMode) (io.WriteCloser, error) {
	w, err := s.FileStore.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return resumeCloseErrorWriter{WriteCloser: w, err: s.err}, nil
}

type resumeCloseErrorWriter struct {
	io.WriteCloser
	err error
}

func (w resumeCloseErrorWriter) Close() error {
	return errors.Join(w.WriteCloser.Close(), w.err)
}

func TestUploadFolderResumeDoesNotFinalizeOnCloseError(t *testing.T) {
	closeErr := errors.New("storage flush failed")
	store := resumeCloseErrorStore{FileStore: NewMemFileStore(), err: closeErr}
	root := t.TempDir()
	partial := filepath.Join(root, "resume.txt"+IncompleteFileSuffix)
	require.NoError(t, store.WriteFile(partial, []byte("012"), 0644))
	err := resumeFolderUpload(t, store, root, flatFileBytes("resume.txt", []byte("345"), nil))
	require.ErrorIs(t, err, closeErr)
	_, err = store.Stat(filepath.Join(root, "resume.txt"))
	assert.True(t, os.IsNotExist(err))
	_, err = store.Stat(partial)
	assert.NoError(t, err)
}
