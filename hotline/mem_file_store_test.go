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
	"golang.org/x/text/encoding"
)

func TestMemFileStore_WriteReadAppend(t *testing.T) {
	m := NewMemFileStore()

	// Create + write.
	w, err := m.Create("/files/foo.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	got, err := m.ReadFile("/files/foo.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got)

	// Append via OpenFile with O_APPEND (mirrors the .incomplete upload path).
	aw, err := m.OpenFile("/files/foo.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = aw.Write([]byte(" world"))
	require.NoError(t, err)
	require.NoError(t, aw.Close())

	got, err = m.ReadFile("/files/foo.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), got)

	// Open returns a reader over the committed bytes.
	r, err := m.Open("/files/foo.txt")
	require.NoError(t, err)
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Equal(t, []byte("hello world"), b)
}

func TestMemFileStore_StatAndParents(t *testing.T) {
	m := NewMemFileStore()

	require.NoError(t, m.WriteFile("/files/sub/bar.txt", []byte("data"), 0644))

	// Nested write auto-registers parent directories.
	fi, err := m.Stat("/files/sub")
	require.NoError(t, err)
	assert.True(t, fi.IsDir())

	fi, err = m.Stat("/files/sub/bar.txt")
	require.NoError(t, err)
	assert.False(t, fi.IsDir())
	assert.Equal(t, int64(4), fi.Size())

	_, err = m.Stat("/files/missing")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestMemFileStore_ReadDir(t *testing.T) {
	m := NewMemFileStore()
	require.NoError(t, m.WriteFile("/root/a.txt", []byte("a"), 0644))
	require.NoError(t, m.WriteFile("/root/b.txt", []byte("bb"), 0644))
	require.NoError(t, m.Mkdir("/root/sub", 0755))
	require.NoError(t, m.WriteFile("/root/sub/deep.txt", []byte("x"), 0644))

	entries, err := m.ReadDir("/root")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"a.txt", "b.txt", "sub"}, names)
}

func TestMemFileStore_RenameRemove(t *testing.T) {
	m := NewMemFileStore()
	require.NoError(t, m.WriteFile("/f/a.incomplete", []byte("payload"), 0644))

	// Rename mirrors the incomplete->final upload commit.
	require.NoError(t, m.Rename("/f/a.incomplete", "/f/a"))
	_, err := m.Stat("/f/a.incomplete")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	got, err := m.ReadFile("/f/a")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)

	require.NoError(t, m.WriteFile("/f/sub/x", []byte("x"), 0644))
	require.NoError(t, m.RemoveAll("/f/sub"))
	_, err = m.Stat("/f/sub/x")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestMemFileStore_WalkCounts(t *testing.T) {
	m := NewMemFileStore()
	require.NoError(t, m.Mkdir("/w", 0755))
	require.NoError(t, m.WriteFile("/w/a.txt", []byte("1234"), 0644))
	require.NoError(t, m.WriteFile("/w/sub/b.txt", []byte("567"), 0644))

	// CalcTotalSize and CalcItemCount are pure Walk consumers — exercising them proves Walk
	// works against a non-OS backend.
	size, err := CalcTotalSize(m, "/w")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x07}, size) // 4 + 3 bytes

	count, err := CalcItemCount(m, "/w")
	require.NoError(t, err)
	// Walk visits /w, /w/a.txt, /w/sub, /w/sub/b.txt = 4 non-hidden entries, minus the root.
	assert.Equal(t, []byte{0x00, 0x03}, count)
}

func TestMemFileStore_SymlinkUnsupported(t *testing.T) {
	m := NewMemFileStore()
	err := m.Symlink("/a", "/b")
	assert.True(t, errors.Is(err, errors.ErrUnsupported))

	_, err = m.ReadLink("/b")
	assert.True(t, errors.Is(err, errors.ErrUnsupported))
}

// TestMemFileStore_DownloadRoundTrip proves the abstraction is genuinely swappable: a file
// staged entirely in memory (no disk, no *os.File) is served through the real DownloadHandler.
func TestMemFileStore_DownloadRoundTrip(t *testing.T) {
	m := NewMemFileStore()

	fileData := []byte("the quick brown fox")
	require.NoError(t, m.WriteFile("/files/story.txt", fileData, 0644))

	ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}

	var out bytes.Buffer
	err := DownloadHandler(&out, "/files/story.txt", ft, m, slog.Default(), true)
	require.NoError(t, err)

	// The download stream is the flattened file object header followed by the data fork; the
	// raw file bytes must appear in the output.
	assert.True(t, bytes.Contains(out.Bytes(), fileData), "download output should contain the file data")
	assert.Equal(t, int64(len(fileData)), ft.bytesSentCounter.Total)
}

// TestMemFileStore_GetFileNameList proves directory listing works against the in-memory backend.
func TestMemFileStore_GetFileNameList(t *testing.T) {
	m := NewMemFileStore()
	require.NoError(t, m.WriteFile(filepath.Join("/library", "readme.txt"), []byte("12345"), 0644))

	fields, err := GetFileNameList(m, "/library", nil, encoding.Nop.NewEncoder(), slog.Default())
	require.NoError(t, err)
	require.Len(t, fields, 1)

	var fnwi FileNameWithInfo
	_, err = fnwi.Write(fields[0].Data)
	require.NoError(t, err)
	assert.Equal(t, "readme.txt", string(fnwi.Name))
}
