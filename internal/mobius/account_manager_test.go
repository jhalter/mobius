package mobius

import (
	"os"
	"path"
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// copyTestFiles copies test config files to a temporary directory
func copyTestFiles(t *testing.T, srcDir, dstDir string) {
	files := []string{"admin.yaml", "guest.yaml", "user-with-old-access-format.yaml"}
	
	for _, file := range files {
		srcPath := path.Join(srcDir, file)
		dstPath := path.Join(dstDir, file)
		
		data, err := os.ReadFile(srcPath)
		require.NoError(t, err, "Failed to read %s", srcPath)
		
		err = os.WriteFile(dstPath, data, 0644)
		require.NoError(t, err, "Failed to write %s", dstPath)
	}
}

func TestNewYAMLAccountManager(t *testing.T) {
	t.Run("loads accounts from a directory", func(t *testing.T) {
		// Create temporary directory and copy test files
		tempDir := t.TempDir()
		copyTestFiles(t, "test/config/Users", tempDir)
		
		// Use temp directory instead of original test directory
		got, err := NewYAMLAccountManager(tempDir)
		require.NoError(t, err)

		assert.Equal(t,
			&hotline.Account{
				Name:     "admin",
				Login:    "admin",
				Password: "$2a$04$2itGEYx8C1N5bsfRSoC9JuonS3I4YfnyVPZHLSwp7kEInRX0yoB.a",
				Access:   hotline.AccessBitmap{0xff, 0xff, 0xef, 0xff, 0xff, 0x80, 0x00, 0x00},
			},
			got.Get("admin"),
		)

		assert.Equal(t,
			&hotline.Account{
				Login:    "guest",
				Name:     "guest",
				Password: "$2a$04$6Yq/TIlgjSD.FbARwtYs9ODnkHawonu1TJ5W2jJKfhnHwBIQTk./y",
				Access:   hotline.AccessBitmap{0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00},
			},
			got.Get("guest"),
		)

		assert.Equal(t,
			&hotline.Account{
				Login:    "test-user",
				Name:     "Test User Name",
				Password: "$2a$04$9P/jgLn1fR9TjSoWL.rKxuN6g.1TSpf2o6Hw.aaRuBwrWIJNwsKkS",
				Access:   hotline.AccessBitmap{0x7d, 0xf0, 0x0c, 0xef, 0xab, 0x80, 0x00, 0x00},
			},
			got.Get("test-user"),
		)
	})
}
