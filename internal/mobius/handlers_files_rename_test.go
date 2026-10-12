package mobius

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSetFileInfoRejectsInvalidRename(t *testing.T) {
	for _, newName := range []string{
		"../outside.txt", "sub/../../outside.txt", "/outside.txt",
		`..\outside.txt`, `sub\..\..\outside.txt`,
		"", ".", "..", "bad\x00name",
	} {
		for _, isDir := range []bool{false, true} {
			kind := "file"
			if isDir {
				kind = "folder"
			}
			t.Run(kind+"/"+newName, func(t *testing.T) {
				base := t.TempDir()
				root := filepath.Join(base, "Files")
				require.NoError(t, os.Mkdir(root, 0755))
				source := filepath.Join(root, "source")
				if isDir {
					require.NoError(t, os.Mkdir(source, 0755))
				} else {
					require.NoError(t, os.WriteFile(source, []byte("source content"), 0644))
				}
				outside := filepath.Join(base, "outside.txt")
				require.NoError(t, os.WriteFile(outside, []byte("outside content"), 0644))
				cc := newRenameTestClient(t, root, "macintosh")
				req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
					hotline.NewField(hotline.FieldFileName, []byte("source")),
					hotline.NewField(hotline.FieldFileNewName, []byte(newName)),
					hotline.NewField(hotline.FieldFileComment, []byte("must not be saved")),
				)

				replies := HandleSetFileInfo(cc, &req)

				require.Len(t, replies, 1)
				assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
				info, err := os.Stat(source)
				require.NoError(t, err, "invalid rename must leave the source in place")
				assert.Equal(t, isDir, info.IsDir())
				if !isDir {
					data, err := os.ReadFile(source)
					require.NoError(t, err)
					assert.Equal(t, "source content", string(data))
				}
				data, err := os.ReadFile(outside)
				require.NoError(t, err)
				assert.Equal(t, "outside content", string(data))
				_, err = os.Stat(filepath.Join(root, ".info_source"))
				assert.True(t, os.IsNotExist(err), "invalid rename must be rejected before writing a comment")
			})
		}
	}
}

func TestHandleSetFileInfoRenamesEncodedNames(t *testing.T) {
	for _, encoding := range []string{"macintosh", "utf8"} {
		for _, isDir := range []bool{false, true} {
			kind := "file"
			if isDir {
				kind = "folder"
			}
			t.Run(encoding+"/"+kind, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "Café")
				dir := filepath.Join(root, "nested")
				require.NoError(t, os.MkdirAll(dir, 0755))
				cc := newRenameTestClient(t, root, encoding)
				const oldName, newName = "source", "renamé"
				files := map[string]string{}
				if isDir {
					require.NoError(t, os.Mkdir(filepath.Join(dir, oldName), 0755))
					files[filepath.Join(oldName, "child.txt")] = "child content"
				} else {
					files[oldName] = "data fork"
					files[oldName+hotline.IncompleteFileSuffix] = "partial data"
					files[".rsrc_"+oldName] = "resource fork"
					info := hotline.NewFlatFileInformationFork(oldName, [8]byte{}, "TEXT", "TEXT")
					data, err := io.ReadAll(&info)
					require.NoError(t, err)
					files[".info_"+oldName] = string(data)
				}
				for name, content := range files {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
				}
				encodedName, err := cc.TextEncoder().String(newName)
				require.NoError(t, err)
				req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
					hotline.NewField(hotline.FieldFileName, []byte(oldName)),
					hotline.NewField(hotline.FieldFilePath, hotline.EncodeFilePath("nested")),
					hotline.NewField(hotline.FieldFileNewName, []byte(encodedName)),
				)

				replies := HandleSetFileInfo(cc, &req)

				require.Len(t, replies, 1)
				require.Equal(t, [4]byte{}, replies[0].ErrorCode)
				destinations := map[string]string{
					oldName:                                newName,
					oldName + hotline.IncompleteFileSuffix: newName + hotline.IncompleteFileSuffix,
					".rsrc_" + oldName:                     ".rsrc_" + newName,
					".info_" + oldName:                     ".info_" + newName,
					filepath.Join(oldName, "child.txt"):    filepath.Join(newName, "child.txt"),
				}
				for name, content := range files {
					data, err := os.ReadFile(filepath.Join(dir, destinations[name]))
					require.NoError(t, err)
					assert.Equal(t, content, string(data))
					_, err = os.Stat(filepath.Join(dir, name))
					assert.True(t, os.IsNotExist(err), "old path must be removed: %s", name)
				}
			})
		}
	}
}

func newRenameTestClient(t *testing.T, root, encoding string) *hotline.ClientConn {
	t.Helper()
	srv, err := hotline.NewServer(
		hotline.WithConfig(hotline.Config{FileRoot: filepath.Join(root, "unused-global-root"), Encoding: encoding}),
		hotline.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	require.NoError(t, err)
	cc := srv.NewClientConn(nil, "127.0.0.1:1")
	cc.Logger = srv.Logger
	cc.Account = &hotline.Account{FileRoot: root}
	for _, access := range []int{hotline.AccessRenameFile, hotline.AccessRenameFolder, hotline.AccessSetFileComment, hotline.AccessSetFolderComment} {
		cc.Account.Access.Set(access)
	}
	return cc
}

func TestHandleSetFileInfoRenameDestination(t *testing.T) {
	const conflict = "Cannot rename to \"renamé\" because there is already a file/folder in the destination folder with the same name."
	for _, tt := range []struct {
		name        string
		isDir       bool
		destination string
		entry       string
		alias       bool
		denied      bool
		lookupErr   error
		readLinkErr error
		wantError   string
	}{
		{name: "file onto file", destination: "file", wantError: conflict},
		{name: "file onto folder", destination: "folder", wantError: conflict},
		{name: "folder onto file", isDir: true, destination: "file", wantError: conflict},
		{name: "folder onto folder", isDir: true, destination: "folder", wantError: conflict},
		{name: "folder onto empty folder", isDir: true, destination: "empty folder", wantError: conflict},
		{name: "partial upload alone", entry: "%s.incomplete", wantError: conflict},
		{name: "resource fork alone", entry: hotline.RsrcForkNameTemplate, wantError: conflict},
		{name: "info fork alone", entry: hotline.InfoForkNameTemplate, wantError: conflict},
		{name: "folder onto partial upload", isDir: true, entry: "%s.incomplete", wantError: conflict},
		{name: "folder onto info fork", isDir: true, entry: hotline.InfoForkNameTemplate, wantError: conflict},
		{name: "broken alias", entry: "%s", alias: true, wantError: conflict},
		{name: "folder onto broken alias", isDir: true, entry: "%s", alias: true, wantError: conflict},
		{name: "broken partial alias", entry: "%s.incomplete", alias: true, wantError: conflict},
		{name: "broken resource alias", entry: hotline.RsrcForkNameTemplate, alias: true, wantError: conflict},
		{name: "broken info alias", entry: hotline.InfoForkNameTemplate, alias: true, wantError: conflict},
		{name: "destination permission error", lookupErr: fs.ErrPermission, wantError: ErrMsgRenameFile},
		{name: "destination storage error", lookupErr: errors.New("storage unavailable"), wantError: ErrMsgRenameFile},
		{name: "folder lookup error", isDir: true, lookupErr: fs.ErrPermission, wantError: ErrMsgRenameFolder},
		{name: "fork lookup error", entry: hotline.InfoForkNameTemplate, lookupErr: fs.ErrPermission, wantError: ErrMsgRenameFile},
		{name: "alias lookup error", readLinkErr: fs.ErrPermission, wantError: ErrMsgRenameFile},
		{name: "file permission denied", destination: "file", denied: true, wantError: ErrMsgNotAllowedRenameFiles},
		{name: "folder permission denied", isDir: true, destination: "folder", denied: true, wantError: ErrMsgNotAllowedRenameFolders},
		{name: "backend without aliases", readLinkErr: &fs.PathError{Op: "readlink", Err: errors.ErrUnsupported}},
	} {
		for _, encoding := range []string{"macintosh", "utf8"} {
			t.Run(encoding+"/"+tt.name, func(t *testing.T) {
				root := t.TempDir()
				const oldName, newName = "source", "renamé"
				sourceFiles := writeMoveTestItem(t, root, oldName, tt.isDir, "source")
				var destinationFiles map[string][]byte
				destination := filepath.Join(root, newName)
				switch tt.destination {
				case "file", "folder":
					destinationFiles = writeMoveTestItem(t, root, newName, tt.destination == "folder", "destination")
				case "empty folder":
					require.NoError(t, os.Mkdir(destination, 0755))
				}
				lookupPath := destination
				var aliasPath string
				if tt.entry != "" {
					lookupPath = filepath.Join(root, fmt.Sprintf(tt.entry, newName))
					if tt.lookupErr == nil {
						if tt.alias {
							aliasPath = lookupPath
							require.NoError(t, os.Symlink("missing-target", aliasPath))
						} else {
							destinationFiles = map[string][]byte{lookupPath: []byte("destination")}
							require.NoError(t, os.WriteFile(lookupPath, destinationFiles[lookupPath], 0644))
						}
					}
				}
				store := &moveDestinationTestStore{
					FileStore: &hotline.OSFileStore{}, destination: destination,
					lookupPath: lookupPath, lookupErr: tt.lookupErr, readLinkErr: tt.readLinkErr,
				}
				cc := newRenameTestClient(t, root, encoding)
				cc.Server.FS = store
				if tt.denied {
					cc.Account.Access = hotline.AccessBitmap{}
					cc.Account.Access.Set(hotline.AccessSetFileComment)
					cc.Account.Access.Set(hotline.AccessSetFolderComment)
				}
				encodedName, err := cc.TextEncoder().String(newName)
				require.NoError(t, err)
				req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
					hotline.NewField(hotline.FieldFileName, []byte(oldName)),
					hotline.NewField(hotline.FieldFileNewName, []byte(encodedName)),
					hotline.NewField(hotline.FieldFileComment, []byte("updated comment")),
				)
				replies := HandleSetFileInfo(cc, &req)
				require.Len(t, replies, 1)
				if tt.wantError == "" {
					require.Equal(t, [4]byte{}, replies[0].ErrorCode)
					data, err := os.ReadFile(destination)
					require.NoError(t, err)
					assert.Equal(t, "source", string(data))
					_, err = os.Stat(filepath.Join(root, oldName))
					assert.ErrorIs(t, err, fs.ErrNotExist)
					return
				}
				assert.Equal(t, [4]byte{0, 0, 0, 1}, replies[0].ErrorCode)
				encodedError, err := cc.TextEncoder().String(tt.wantError)
				require.NoError(t, err)
				assert.Equal(t, encodedError, string(replies[0].GetField(hotline.FieldError).Data))
				assert.Zero(t, store.renameCalls)
				if tt.denied {
					assert.Zero(t, store.destinationChecks)
				}
				for _, files := range []map[string][]byte{sourceFiles, destinationFiles} {
					for name, want := range files {
						got, err := os.ReadFile(name)
						require.NoError(t, err)
						assert.Equal(t, want, got, "preserve file and metadata: %s", name)
					}
				}
				if tt.isDir {
					_, err = os.Stat(filepath.Join(root, ".info_"+oldName))
					assert.ErrorIs(t, err, fs.ErrNotExist, "failed rename must not save a comment")
				}
				if tt.destination == "empty folder" {
					entries, err := os.ReadDir(destination)
					require.NoError(t, err)
					assert.Empty(t, entries)
				}
				if aliasPath != "" {
					target, err := os.Readlink(aliasPath)
					require.NoError(t, err)
					assert.Equal(t, "missing-target", target)
				}
			})
		}
	}
}

func TestHandleSetFileInfoUnchangedName(t *testing.T) {
	for _, backend := range []string{"os", "memory"} {
		for _, isDir := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/folder=%t", backend, isDir), func(t *testing.T) {
				root := t.TempDir()
				var store hotline.FileStore = &hotline.OSFileStore{}
				if backend == "memory" {
					store = hotline.NewMemFileStore()
				}
				source := filepath.Join(root, "source")
				if isDir {
					require.NoError(t, store.Mkdir(source, 0755))
				} else {
					require.NoError(t, store.WriteFile(source, []byte("source content"), 0644))
				}
				cc := newRenameTestClient(t, root, "macintosh")
				cc.Server.FS = store
				req := hotline.NewTransaction(hotline.TranSetFileInfo, cc.ID,
					hotline.NewField(hotline.FieldFileName, []byte("source")),
					hotline.NewField(hotline.FieldFileNewName, []byte("source")),
					hotline.NewField(hotline.FieldFileComment, []byte("updated comment")),
				)
				replies := HandleSetFileInfo(cc, &req)
				require.Len(t, replies, 1)
				require.Equal(t, [4]byte{}, replies[0].ErrorCode)
				info, err := store.Stat(source)
				require.NoError(t, err)
				assert.Equal(t, isDir, info.IsDir())
				if !isDir {
					data, err := store.ReadFile(source)
					require.NoError(t, err)
					assert.Equal(t, "source content", string(data))
				}
				file, err := hotline.NewFile(store, source, 0)
				require.NoError(t, err)
				assert.Equal(t, "updated comment", string(file.Ffo.FlatFileInformationFork.Comment))
			})
		}
	}
}
