package main

import (
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