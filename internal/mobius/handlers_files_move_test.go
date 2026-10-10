package mobius

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMoveFileDestination(t *testing.T) {
	const conflict = "Cannot move \"Ambrosia Software\" because there is already a file/folder in the destination folder with the same name."
	const encodedConflict = "Cannot move \"Café\" because there is already a file/folder in the destination folder with the same name."
	tests := []struct {
		name        string
		isDir       bool
		destination string
		entryName   string
		alias       bool
		fileName    string
		encoding    string
		denied      bool
		lookupErr   error
		lookupEntry string
		readLinkErr error
		wantError   string
	}{
		{name: "file onto file", destination: "file", wantError: conflict},
		{name: "folder onto folder", isDir: true, destination: "folder", wantError: conflict},
		{name: "folder onto empty folder", isDir: true, destination: "empty folder", wantError: conflict},
		{name: "file onto folder", destination: "folder", wantError: conflict},
		{name: "folder onto file", isDir: true, destination: "file", wantError: conflict},
		{name: "file into current folder", destination: "same", wantError: conflict},
		{name: "folder into current folder", isDir: true, destination: "same", wantError: conflict},
		{name: "successful file move"},
		{name: "successful folder move", isDir: true},
		{name: "destination permission error", lookupErr: fs.ErrPermission, wantError: ErrMsgMoveFile},
		{name: "destination storage error", lookupErr: errors.New("storage unavailable"), wantError: ErrMsgMoveFile},
		{name: "file permission denied before conflict check", destination: "file", denied: true, wantError: ErrMsgNotAllowedMoveFiles},
		{name: "folder permission denied before conflict check", isDir: true, destination: "folder", denied: true, wantError: ErrMsgNotAllowedMoveFolders},
		{name: "Mac Roman filename conflict", fileName: "Café", destination: "file", wantError: encodedConflict},
		{name: "UTF-8 filename conflict", fileName: "Café", encoding: "utf8", destination: "file", wantError: encodedConflict},
		{name: "partial upload with forks", destination: "partial", wantError: conflict},
		{name: "partial file alone", destination: "entry", entryName: "%s.incomplete", wantError: conflict},
		{name: "resource fork alone", destination: "entry", entryName: hotline.RsrcForkNameTemplate, wantError: conflict},
		{name: "info fork alone", destination: "entry", entryName: hotline.InfoForkNameTemplate, wantError: conflict},
		{name: "folder onto partial upload", isDir: true, destination: "partial", wantError: conflict},
		{name: "folder onto info fork", isDir: true, destination: "entry", entryName: hotline.InfoForkNameTemplate, wantError: conflict},
		{name: "broken alias", destination: "entry", entryName: "%s", alias: true, wantError: conflict},
		{name: "folder onto broken alias", isDir: true, destination: "entry", entryName: "%s", alias: true, wantError: conflict},
		{name: "broken partial alias", destination: "entry", entryName: "%s.incomplete", alias: true, wantError: conflict},
		{name: "broken resource alias", destination: "entry", entryName: hotline.RsrcForkNameTemplate, alias: true, wantError: conflict},
		{name: "broken info alias", destination: "entry", entryName: hotline.InfoForkNameTemplate, alias: true, wantError: conflict},
		{name: "fork lookup error", lookupEntry: hotline.InfoForkNameTemplate, lookupErr: fs.ErrPermission, wantError: ErrMsgMoveFile},
		{name: "alias lookup error", readLinkErr: fs.ErrPermission, wantError: ErrMsgMoveFile},
		{name: "backend without alias support", readLinkErr: &fs.PathError{Op: "readlink", Err: errors.ErrUnsupported}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Café")
			sourceDir := filepath.Join(root, "source")
			destinationDir := filepath.Join(root, "destination")
			require.NoError(t, os.MkdirAll(sourceDir, 0755))
			require.NoError(t, os.MkdirAll(destinationDir, 0755))
			fileName := tt.fileName
			if fileName == "" {
				fileName = "Ambrosia Software"
			}
			encoding := tt.encoding
			if encoding == "" {
				encoding = "macintosh"
			}

			sourceFiles := writeMoveTestItem(t, sourceDir, fileName, tt.isDir, "source")
			var destinationFiles map[string][]byte
			var aliasPath string
			switch tt.destination {
			case "file", "folder":
				destinationFiles = writeMoveTestItem(t, destinationDir, fileName, tt.destination == "folder", "destination")
			case "empty folder":
				require.NoError(t, os.Mkdir(filepath.Join(destinationDir, fileName), 0755))
			case "same":
				destinationDir = sourceDir
			case "partial":
				destinationFiles = writeMoveTestItem(t, destinationDir, fileName, false, "destination")
				completePath := filepath.Join(destinationDir, fileName)
				require.NoError(t, os.Remove(completePath))
				delete(destinationFiles, completePath)
				if !tt.isDir {
					// A completed source need not have its own partial upload.
					partialPath := filepath.Join(sourceDir, fileName+hotline.IncompleteFileSuffix)
					require.NoError(t, os.Remove(partialPath))
					delete(sourceFiles, partialPath)
				}
			case "entry":
				entryPath := filepath.Join(destinationDir, fmt.Sprintf(tt.entryName, fileName))
				if tt.alias {
					aliasPath = entryPath
					require.NoError(t, os.Symlink("missing-target", aliasPath))
				} else {
					destinationFiles = map[string][]byte{entryPath: []byte("existing destination data")}
					require.NoError(t, os.WriteFile(entryPath, destinationFiles[entryPath], 0644))
				}
			}

			store := &moveDestinationTestStore{
				FileStore:   &hotline.OSFileStore{},
				destination: filepath.Join(destinationDir, fileName),
				lookupErr:   tt.lookupErr,
				readLinkErr: tt.readLinkErr,
			}
			store.lookupPath = store.destination
			if tt.lookupEntry != "" {
				store.lookupPath = filepath.Join(destinationDir, fmt.Sprintf(tt.lookupEntry, fileName))
			}
			srv, err := hotline.NewServer(
				hotline.WithConfig(hotline.Config{FileRoot: filepath.Join(root, "unused-global-root"), Encoding: encoding}),
				hotline.WithFileStore(store),
				hotline.WithLogger(NewTestLogger()),
			)
			require.NoError(t, err)
			cc := srv.NewClientConn(nil, "127.0.0.1:1")
			cc.Logger = srv.Logger
			cc.Account = &hotline.Account{FileRoot: root}
			if !tt.denied {
				if tt.isDir {
					cc.Account.Access.Set(hotline.AccessMoveFolder)
				} else {
					cc.Account.Access.Set(hotline.AccessMoveFile)
				}
			}
			encodedName, err := cc.TextEncoder().String(fileName)
			require.NoError(t, err)
			req := hotline.NewTransaction(hotline.TranMoveFile, cc.ID,
				hotline.NewField(hotline.FieldFileName, []byte(encodedName)),
				hotline.NewField(hotline.FieldFilePath, hotline.EncodeFilePath("source")),
				hotline.NewField(hotline.FieldFileNewPath, hotline.EncodeFilePath(filepath.Base(destinationDir))),
			)

			replies := HandleMoveFile(cc, &req)

			require.Len(t, replies, 1)
			assert.Equal(t, req.ID, replies[0].ID)
			assert.Equal(t, cc.ID, replies[0].ClientID)
			assert.Equal(t, uint8(1), replies[0].IsReply)
			if tt.wantError != "" {
				assert.Equal(t, [4]byte{0, 0, 0, 1}, replies[0].ErrorCode)
				encodedError, err := cc.TextEncoder().String(tt.wantError)
				require.NoError(t, err)
				assert.Equal(t, []hotline.Field{hotline.NewField(hotline.FieldError, []byte(encodedError))}, replies[0].Fields)
				assert.Zero(t, store.renameCalls, "conflicts and lookup errors must not move any forks")
				if tt.denied {
					assert.Zero(t, store.destinationChecks, "authorization must precede the destination check")
				}
				for name, want := range sourceFiles {
					got, err := os.ReadFile(name)
					require.NoError(t, err)
					assert.Equal(t, want, got, "source must be preserved: %s", name)
				}
				for name, want := range destinationFiles {
					got, err := os.ReadFile(name)
					require.NoError(t, err)
					assert.Equal(t, want, got, "destination must be preserved: %s", name)
				}
				if tt.destination == "empty folder" {
					entries, err := os.ReadDir(store.destination)
					require.NoError(t, err)
					assert.Empty(t, entries)
				}
				if aliasPath != "" {
					target, err := os.Readlink(aliasPath)
					require.NoError(t, err, "the existing alias must be preserved")
					assert.Equal(t, "missing-target", target)
				}
				return
			}

			assert.Equal(t, [4]byte{}, replies[0].ErrorCode)
			assert.Empty(t, replies[0].Fields)
			for name, want := range sourceFiles {
				rel, err := filepath.Rel(sourceDir, name)
				require.NoError(t, err)
				got, err := os.ReadFile(filepath.Join(destinationDir, rel))
				require.NoError(t, err)
				assert.Equal(t, want, got)
				_, err = os.Stat(name)
				assert.ErrorIs(t, err, fs.ErrNotExist)
			}
			_, err = os.Stat(filepath.Join(sourceDir, fileName))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	}
}

// Use real files and forks so the tests detect data loss as well as protocol errors.
func writeMoveTestItem(t *testing.T, dir, name string, isDir bool, content string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if isDir {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0755))
		files[filepath.Join(dir, name, "child.txt")] = []byte(content)
	} else {
		files[filepath.Join(dir, name)] = []byte(content)
		files[filepath.Join(dir, name+hotline.IncompleteFileSuffix)] = []byte(content + " partial")
		files[filepath.Join(dir, ".rsrc_"+name)] = []byte(content + " resource")
		info := hotline.NewFlatFileInformationFork(name, [8]byte{}, "TEXT", "TEXT")
		require.NoError(t, info.SetComment([]byte(content)))
		data, err := io.ReadAll(&info)
		require.NoError(t, err)
		files[filepath.Join(dir, ".info_"+name)] = data
	}
	for path, data := range files {
		require.NoError(t, os.WriteFile(path, data, 0644))
	}
	return files
}

type moveDestinationTestStore struct {
	hotline.FileStore
	destination       string
	lookupPath        string
	lookupErr         error
	readLinkErr       error
	destinationChecks int
	renameCalls       int
}

func (s *moveDestinationTestStore) Stat(name string) (fs.FileInfo, error) {
	if name == s.destination {
		s.destinationChecks++
	}
	if name == s.lookupPath && s.lookupErr != nil {
		return nil, s.lookupErr
	}
	return s.FileStore.Stat(name)
}

func (s *moveDestinationTestStore) ReadLink(name string) (string, error) {
	if s.readLinkErr != nil {
		return "", s.readLinkErr
	}
	return s.FileStore.ReadLink(name)
}

func (s *moveDestinationTestStore) Rename(oldpath, newpath string) error {
	s.renameCalls++
	return s.FileStore.Rename(oldpath, newpath)
}
