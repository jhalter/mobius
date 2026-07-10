package main

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/jhalter/mobius/internal/mobius"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyDir(t *testing.T) {
	// Test using the actual embedded config directory
	dstDir := t.TempDir()

	// Execute copyDir with the embedded mobius/config directory
	err := copyDir("mobius/config", dstDir)
	require.NoError(t, err)

	// Verify some expected files exist (based on the embedded config)
	expectedFiles := []string{
		"config.yaml",
		"Agreement.txt",
		"MessageBoard.txt",
		"ThreadedNews.yaml",
		"Users/admin.yaml",
		"Users/guest.yaml",
		"banner.jpg",
	}

	for _, expectedFile := range expectedFiles {
		fullPath := path.Join(dstDir, expectedFile)
		assert.FileExists(t, fullPath, "Expected file %s to exist", expectedFile)

		// Verify file is not empty
		info, err := os.Stat(fullPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0), "File %s should not be empty", expectedFile)
	}

	// Verify directories were created
	expectedDirs := []string{
		"Users",
		"Files",
	}

	for _, expectedDir := range expectedDirs {
		fullPath := path.Join(dstDir, expectedDir)
		info, err := os.Stat(fullPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "Expected %s to be a directory", expectedDir)
	}
}

func TestCopyDirNonexistentSource(t *testing.T) {
	dstDir := t.TempDir()

	err := copyDir("nonexistent/directory", dstDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read source directory")
}

func TestCopyDirRecursive(t *testing.T) {
	// Test the recursive functionality using embedded config
	dstDir := t.TempDir()

	err := copyDirRecursive("mobius/config", dstDir)
	require.NoError(t, err)

	// Verify nested structure is copied correctly
	nestedPath := path.Join(dstDir, "Users", "admin.yaml")
	assert.FileExists(t, nestedPath)

	// Verify nested Files directory
	filesDir := path.Join(dstDir, "Files")
	info, err := os.Stat(filesDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCopyFile(t *testing.T) {
	dstDir := t.TempDir()
	dstFile := path.Join(dstDir, "copied.yaml")

	// Copy a single file from embedded config
	err := copyFile("mobius/config/config.yaml", dstFile)
	require.NoError(t, err)

	// Verify file was copied correctly
	assert.FileExists(t, dstFile)

	// Verify file is not empty
	info, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestCopyFileErrors(t *testing.T) {
	t.Run("source file does not exist", func(t *testing.T) {
		dstDir := t.TempDir()
		err := copyFile("nonexistent.txt", path.Join(dstDir, "dest.txt"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open source file")
	})

	t.Run("destination directory does not exist", func(t *testing.T) {
		err := copyFile("mobius/config/config.yaml", "/nonexistent/directory/dest.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create destination file")
	})
}

func TestCopyDirPermissions(t *testing.T) {
	dstDir := t.TempDir()

	err := copyDir("mobius/config", dstDir)
	require.NoError(t, err)

	// Check directory permissions
	info, err := os.Stat(path.Join(dstDir, "Users"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Check that directory has reasonable permissions (at least readable/executable)
	mode := info.Mode()
	assert.True(t, mode&0400 != 0, "Directory should be readable")
	assert.True(t, mode&0100 != 0, "Directory should be executable")
}

func TestFindConfigPath(t *testing.T) {
	// Test function behavior by checking it returns one of the expected paths or fallback
	t.Run("returns valid path", func(t *testing.T) {
		result := findConfigPath()

		// Should return either one of the search paths that exists, or "config" fallback
		validPaths := append([]string{"config"}, mobius.ConfigSearchOrder...)

		found := false
		for _, validPath := range validPaths {
			if result == validPath {
				found = true
				break
			}
		}

		assert.True(t, found, "findConfigPath should return one of the valid paths or fallback, got: %s", result)
	})

	// Test directory vs file validation
	t.Run("validates directory vs file", func(t *testing.T) {
		// This test verifies the function logic but can't control system directories
		// The function correctly validates that only directories are returned
		result := findConfigPath()

		// Verify result is an actual directory if it exists
		if result != "config" {
			info, err := os.Stat(result)
			require.NoError(t, err, "Returned path should exist")
			assert.True(t, info.IsDir(), "Returned path should be a directory")
		}
	})

	// Test with existing directory
	t.Run("finds existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(originalDir) }()

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Create a config directory
		err = os.Mkdir("config", 0755)
		require.NoError(t, err)

		result := findConfigPath()
		assert.Equal(t, "config", result)
	})
}

// r2EnvVars are every R2_* variable newR2FileStore reads. Each case clears all of them and sets
// only what it needs, so ambient credentials in the test environment can't leak in.
var r2EnvVars = []string{
	"R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY",
	"R2_ENDPOINT", "R2_ACCOUNT_ID", "R2_PREFIX", "R2_STAGING_DIR",
}

func TestNewR2FileStore_EnvValidation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string // substring; empty means the store must construct successfully
	}{
		{
			name:    "missing bucket and credentials",
			env:     map[string]string{},
			wantErr: "R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY must be set",
		},
		{
			name: "missing secret key",
			env: map[string]string{
				"R2_BUCKET":        "b",
				"R2_ACCESS_KEY_ID": "ak",
			},
			wantErr: "R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY must be set",
		},
		{
			name: "credentials set but no endpoint or account id",
			env: map[string]string{
				"R2_BUCKET":            "b",
				"R2_ACCESS_KEY_ID":     "ak",
				"R2_SECRET_ACCESS_KEY": "sk",
			},
			wantErr: "either R2_ENDPOINT or R2_ACCOUNT_ID must be set",
		},
		{
			name: "account id derives the endpoint",
			env: map[string]string{
				"R2_BUCKET":            "b",
				"R2_ACCESS_KEY_ID":     "ak",
				"R2_SECRET_ACCESS_KEY": "sk",
				"R2_ACCOUNT_ID":        "acct123",
			},
		},
		{
			name: "explicit endpoint",
			env: map[string]string{
				"R2_BUCKET":            "b",
				"R2_ACCESS_KEY_ID":     "ak",
				"R2_SECRET_ACCESS_KEY": "sk",
				"R2_ENDPOINT":          "https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range r2EnvVars {
				t.Setenv(k, "")
			}
			// Keep staging off the shared temp dir even on the success paths.
			t.Setenv("R2_STAGING_DIR", t.TempDir())
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			store, err := newR2FileStore(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, store)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, store)
		})
	}
}
