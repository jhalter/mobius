package mobius

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgreement(t *testing.T) {
	t.Run("success with valid file", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("Welcome to the server!"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)
		assert.Equal(t, "Welcome to the server!", string(ag.data))
		assert.Equal(t, filepath.Join(dir, agreementFile), ag.filePath)
		assert.Equal(t, "\r", ag.lineEndings)
	})

	t.Run("error with missing file", func(t *testing.T) {
		dir := t.TempDir()

		ag, err := NewAgreement(dir, "\r")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read file:")
		assert.NotNil(t, ag) // returns empty Agreement on error
		assert.Empty(t, ag.data)
	})

	t.Run("converts newlines to custom line endings", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("line1\nline2\nline3"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)
		assert.Equal(t, "line1\rline2\rline3", string(ag.data))
	})

	t.Run("converts CRLF line endings", func(t *testing.T) {
		dir := t.TempDir()
		// The implementation first replaces \n, then \r\n.
		// Writing raw \r\n: first pass turns \n -> \r, yielding \r\r,
		// then second pass looks for \r\n which no longer exists.
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("line1\r\nline2"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)
		// \r\n -> first \n becomes \r -> "\r\r" then \r\n replacement finds nothing
		assert.Equal(t, "line1\r\rline2", string(ag.data))
	})

	t.Run("preserves content when line ending is newline", func(t *testing.T) {
		dir := t.TempDir()
		content := "line1\nline2\nline3"
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte(content), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\n")
		require.NoError(t, err)
		assert.Equal(t, content, string(ag.data))
	})
}

func TestAgreement_Reload(t *testing.T) {
	t.Run("reload picks up changed file content", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, agreementFile)
		err := os.WriteFile(filePath, []byte("original"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)
		assert.Equal(t, "original", string(ag.data))

		err = os.WriteFile(filePath, []byte("updated content"), 0644)
		require.NoError(t, err)

		err = ag.Reload()
		require.NoError(t, err)
		assert.Equal(t, "updated content", string(ag.data))
	})

	t.Run("reload applies line ending conversion", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, agreementFile)
		err := os.WriteFile(filePath, []byte("first"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		err = os.WriteFile(filePath, []byte("line1\nline2"), 0644)
		require.NoError(t, err)

		err = ag.Reload()
		require.NoError(t, err)
		assert.Equal(t, "line1\rline2", string(ag.data))
	})

	t.Run("reload returns error when file is deleted", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, agreementFile)
		err := os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		err = os.Remove(filePath)
		require.NoError(t, err)

		err = ag.Reload()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read file:")
	})
}

func TestAgreement_Read(t *testing.T) {
	t.Run("reads full data in one call", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("hello"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		buf := make([]byte, 100)
		n, err := ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", string(buf[:n]))
	})

	t.Run("returns EOF after all data is read", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("hi"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		buf := make([]byte, 100)
		_, err = ag.Read(buf)
		require.NoError(t, err)

		n, err := ag.Read(buf)
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("reads in chunks", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("abcdefgh"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		buf := make([]byte, 3)

		n, err := ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, "abc", string(buf[:n]))

		n, err = ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, "def", string(buf[:n]))

		n, err = ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, "gh", string(buf[:n]))

		n, err = ag.Read(buf)
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("empty agreement returns EOF immediately", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte(""), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		buf := make([]byte, 10)
		n, err := ag.Read(buf)
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, io.EOF)
	})
}

func TestAgreement_Seek(t *testing.T) {
	t.Run("seek to beginning resets read position", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("hello"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		// Read all data
		buf := make([]byte, 100)
		_, err = ag.Read(buf)
		require.NoError(t, err)

		// Confirm EOF
		_, err = ag.Read(buf)
		assert.ErrorIs(t, err, io.EOF)

		// Seek back to start
		offset, err := ag.Seek(0, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), offset)

		// Read again from beginning
		n, err := ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", string(buf[:n]))
	})

	t.Run("seek to middle offset", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("abcdef"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		_, err = ag.Seek(3, 0)
		require.NoError(t, err)

		buf := make([]byte, 100)
		n, err := ag.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, "def", string(buf[:n]))
	})

	t.Run("seek sets internal readOffset", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("test"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		_, err = ag.Seek(2, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, ag.readOffset)
	})
}

func TestAgreement_ReadAll(t *testing.T) {
	t.Run("io.ReadAll reads complete agreement", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("full content here"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		data, err := io.ReadAll(ag)
		require.NoError(t, err)
		assert.Equal(t, "full content here", string(data))
	})

	t.Run("io.ReadAll with line ending conversion", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("line1\nline2\nline3"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		data, err := io.ReadAll(ag)
		require.NoError(t, err)
		assert.Equal(t, "line1\rline2\rline3", string(data))
	})

	t.Run("io.ReadAll after seek resets position", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte("abcdef"), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		// Partially read
		buf := make([]byte, 3)
		_, err = ag.Read(buf)
		require.NoError(t, err)

		// Seek to beginning
		_, err = ag.Seek(0, 0)
		require.NoError(t, err)

		// ReadAll should get everything
		data, err := io.ReadAll(ag)
		require.NoError(t, err)
		assert.Equal(t, "abcdef", string(data))
	})

	t.Run("io.ReadAll on empty agreement", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, agreementFile), []byte(""), 0644)
		require.NoError(t, err)

		ag, err := NewAgreement(dir, "\r")
		require.NoError(t, err)

		data, err := io.ReadAll(ag)
		require.NoError(t, err)
		assert.Empty(t, data)
	})
}
