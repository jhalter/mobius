package hotline

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePathComponentLengths(t *testing.T) {
	for _, size := range []int{0, 1, 252, 253, 254, 255} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			name := bytes.Repeat([]byte{'x'}, size)
			component := append([]byte{0, 0, byte(size)}, name...)
			var item FilePathItem
			n, err := item.Write(component)
			require.NoError(t, err)
			assert.Equal(t, len(component), n)
			assert.Equal(t, name, item.Name)
			var fp FilePath
			_, err = fp.Write(append([]byte{0, 1}, component...))
			require.NoError(t, err)
			require.Len(t, fp.Items, 1)
			assert.Equal(t, name, fp.Items[0].Name)
		})
	}
}

func TestFilePathRejectsTruncatedComponents(t *testing.T) {
	for _, size := range []int{1, 252, 253, 254, 255} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			component := append([]byte{0, 0, byte(size)}, bytes.Repeat([]byte{'x'}, size-1)...)
			var item FilePathItem
			_, err := item.Write(component)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			advance, token, err := fileItemScanner(component, false)
			assert.Zero(t, advance)
			assert.Nil(t, token)
			require.NoError(t, err) // Wait for more data until EOF.
			_, _, err = fileItemScanner(component, true)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			var fp FilePath
			_, err = fp.Write(append([]byte{0, 1}, component...))
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		})
	}
	for _, data := range [][]byte{{0}, {0, 1}, {0, 1, 0}, {0, 1, 0, 0}, {0, 2, 0, 0, 1, 'x'}} {
		var fp FilePath
		_, err := fp.Write(data)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	}
}

func FuzzFilePathWrite(f *testing.F) {
	f.Add([]byte{0, 1, 0, 0, 1, 'x'})
	f.Add([]byte{0, 1, 0, 0, 255})
	f.Add(append([]byte{0, 1, 0, 0, 255}, bytes.Repeat([]byte{'x'}, 255)...))
	f.Fuzz(func(t *testing.T, data []byte) {
		var fp FilePath
		_, err := fp.Write(data)
		if err != nil {
			return
		}
		require.Equal(t, int(fp.Len()), len(fp.Items))
		for _, item := range fp.Items {
			require.Equal(t, int(item.Len), len(item.Name))
		}
	})
}
