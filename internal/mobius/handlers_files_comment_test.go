package mobius

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSetFileInfoPersistsComments(t *testing.T) {
	for _, backend := range []string{"os", "memory"} {
		for _, kind := range []string{"file", "folder"} {
			t.Run(backend+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				var store hotline.FileStore = &hotline.OSFileStore{}
				if backend == "memory" {
					store = hotline.NewMemFileStore()
				}
				name := filepath.Join(root, "source")
				if kind == "folder" {
					require.NoError(t, store.Mkdir(name, 0755))
				} else {
					require.NoError(t, store.WriteFile(name, []byte("file content"), 0644))
				}
				cc := newRenameTestClient(t, root, "macintosh")
				cc.Server.FS = store

				for _, comment := range []string{"saved longer comment", "short", ""} {
					req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
						hotline.NewField(hotline.FieldFileName, []byte("source")),
						hotline.NewField(hotline.FieldFileComment, []byte(comment)),
					)
					replies := HandleSetFileInfo(cc, &req)
					require.Len(t, replies, 1)
					require.Equal(t, [4]byte{}, replies[0].ErrorCode)

					getReq := hotline.NewTransaction(hotline.TranGetFileInfo, cc.ID,
						hotline.NewField(hotline.FieldFileName, []byte("source")),
					)
					replies = HandleGetFileInfo(cc, &getReq)
					require.Len(t, replies, 1)
					require.Equal(t, [4]byte{}, replies[0].ErrorCode)
					assert.Equal(t, comment, string(replies[0].GetField(hotline.FieldFileComment).Data))
				}
			})
		}
	}
}

func TestHandleSetFileInfoClosesCommentBeforeRename(t *testing.T) {
	writeErr := errors.New("comment write failed")
	closeErr := errors.New("comment upload failed")
	for _, tc := range []struct {
		name     string
		writeErr error
		closeErr error
	}{
		{name: "success"},
		{name: "write failure", writeErr: writeErr},
		{name: "close failure", closeErr: closeErr},
		{name: "write and close failure", writeErr: writeErr, closeErr: closeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writer := &commentTestWriter{writeErr: tc.writeErr, closeErr: tc.closeErr}
			store := &commentTestStore{FileStore: hotline.NewMemFileStore(), writer: writer}
			source := filepath.Join(root, "source")
			destination := filepath.Join(root, "renamed")
			require.NoError(t, store.WriteFile(source, []byte("file content"), 0644))
			cc := newRenameTestClient(t, root, "macintosh")
			cc.Server.FS = store
			req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
				hotline.NewField(hotline.FieldFileName, []byte("source")),
				hotline.NewField(hotline.FieldFileNewName, []byte("renamed")),
				hotline.NewField(hotline.FieldFileComment, []byte("saved comment")),
			)

			replies := HandleSetFileInfo(cc, &req)

			require.Len(t, replies, 1)
			assert.Equal(t, 1, writer.closes, "close the writer exactly once, including after a failed write")
			if tc.writeErr != nil || tc.closeErr != nil {
				assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
				assert.Equal(t, ErrMsgSetFileInfo, string(replies[0].GetField(hotline.FieldError).Data))
				data, err := store.ReadFile(source)
				require.NoError(t, err)
				assert.Equal(t, "file content", string(data))
				_, err = store.Stat(destination)
				assert.ErrorIs(t, err, fs.ErrNotExist, "a failed comment save must prevent the rename")
				return
			}

			require.Equal(t, [4]byte{}, replies[0].ErrorCode)
			file, err := hotline.NewFile(store, destination, 0)
			require.NoError(t, err)
			assert.Equal(t, "saved comment", string(file.Ffo.FlatFileInformationFork.Comment))
			_, err = store.Stat(filepath.Join(root, ".info_source"))
			assert.ErrorIs(t, err, fs.ErrNotExist, "the comment must be committed before its sidecar is renamed")
		})
	}
}

type commentTestStore struct {
	hotline.FileStore
	writer *commentTestWriter
}

func (s *commentTestStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	w, err := s.FileStore.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	s.writer.WriteCloser = w
	return s.writer, nil
}

type commentTestWriter struct {
	io.WriteCloser
	writeErr error
	closeErr error
	closes   int
}

func (w *commentTestWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.WriteCloser.Write(p)
}

func (w *commentTestWriter) Close() error {
	w.closes++
	return errors.Join(w.WriteCloser.Close(), w.closeErr)
}
