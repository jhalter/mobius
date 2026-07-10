package mobius

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
