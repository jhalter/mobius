package mobius

import (
	"io"
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
