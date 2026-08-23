package mobius

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_InvalidBannerFileExtension(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test config file with an invalid banner file extension
	configContent := `
Name: "Test Server"
Description: "Test Description"
BannerFile: "banner.png"
FileRoot: "files"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Attempt to load the config
	_, err := LoadConfig(configPath)

	// Verify that we get the improved error message
	if err == nil {
		t.Fatal("Expected error for invalid banner file extension, got nil")
	}

	expectedMsg := "BannerFile must have a .jpg, .jpeg, or .gif extension (got: banner.png)"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
	}
}

// TestLoadConfig_FileRootKeptVerbatim guards against LoadConfig resolving FileRoot to a host
// filesystem path.  FileRoot is a path within the selected file store's namespace; rewriting it to
// a local absolute path here would leak the host's directory layout into object-store keys.
func TestLoadConfig_FileRootKeptVerbatim(t *testing.T) {
	tests := []struct {
		name     string
		fileRoot string
	}{
		{"relative path", "files"},
		{"nested relative path", "library/files"},
		{"absolute path", "/srv/hotline/files"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			configContent := `
Name: "Test Server"
Description: "Test Description"
FileRoot: "` + tt.fileRoot + `"
`
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			config, err := LoadConfig(configPath)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if config.FileRoot != tt.fileRoot {
				t.Errorf("Expected FileRoot to be %q, got %q", tt.fileRoot, config.FileRoot)
			}
		})
	}
}

func TestLoadConfig_ValidBannerFileExtensions(t *testing.T) {
	tests := []struct {
		name       string
		bannerFile string
	}{
		{"jpg extension", "banner.jpg"},
		{"jpeg extension", "banner.jpeg"},
		{"gif extension", "banner.gif"},
		{"empty banner", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for test files
			tmpDir := t.TempDir()

			// Create files subdirectory
			filesDir := filepath.Join(tmpDir, "files")
			if err := os.Mkdir(filesDir, 0755); err != nil {
				t.Fatalf("Failed to create files dir: %v", err)
			}

			// Create a test config file with a valid banner file extension
			configContent := `
Name: "Test Server"
Description: "Test Description"
BannerFile: "` + tt.bannerFile + `"
FileRoot: "files"
`
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Attempt to load the config
			config, err := LoadConfig(configPath)

			// Verify that we don't get a validation error
			if err != nil {
				t.Errorf("Expected no error for %s, got: %v", tt.bannerFile, err)
			}

			if config.BannerFile != tt.bannerFile {
				t.Errorf("Expected BannerFile to be %q, got %q", tt.bannerFile, config.BannerFile)
			}
		})
	}
}

func TestLoadConfig_NewsFeeds(t *testing.T) {
	t.Run("accepts an auto-detected RSS or Atom feed", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		configContent := `
Name: Test Server
Description: Test Description
FileRoot: Files
NewsFeeds:
  - CategoryPath: [Afterglow Releases]
    URL: https://morphing.cloud/afterglow/appcast.xml
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		config, err := LoadConfig(configPath)
		require.NoError(t, err)
		require.Len(t, config.NewsFeeds, 1)
		assert.Equal(t, []string{"Afterglow Releases"}, config.NewsFeeds[0].CategoryPath)
		assert.Equal(t, "https://morphing.cloud/afterglow/appcast.xml", config.NewsFeeds[0].URL)
	})

	tests := []struct {
		name     string
		feedYAML string
		wantErr  string
	}{
		{
			name: "rejects non HTTP URL",
			feedYAML: `
  - CategoryPath: [News]
    URL: file:///tmp/feed.xml`,
			wantErr: "absolute HTTP or HTTPS URL",
		},
		{
			name: "rejects credentials",
			feedYAML: `
  - CategoryPath: [News]
    URL: https://user:password@example.com/feed.xml`,
			wantErr: "must not contain credentials",
		},
		{
			name: "rejects duplicate category",
			feedYAML: `
  - CategoryPath: [News]
    URL: https://example.com/one.xml
  - CategoryPath: [News]
    URL: https://example.com/two.xml`,
			wantErr: "duplicates another news feed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			configContent := `
Name: Test Server
Description: Test Description
FileRoot: Files
NewsFeeds:` + tt.feedYAML + "\n"
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

			_, err := LoadConfig(configPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	for _, removed := range []string{"Format: rss", "RefreshInterval: 1h", "MaxItems: 25"} {
		t.Run("rejects removed setting "+removed, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			configContent := `
Name: Test Server
Description: Test Description
FileRoot: Files
NewsFeeds:
  - CategoryPath: [Releases]
    URL: http://example.com/releases.xml
    ` + removed + "\n"
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

			_, err := LoadConfig(configPath)
			require.ErrorContains(t, err, "unknown NewsFeeds setting")
		})
	}
}
