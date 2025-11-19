package mobius

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_InvalidBannerFileExtension(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "mobius-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	_, err = LoadConfig(configPath)

	// Verify that we get the improved error message
	if err == nil {
		t.Fatal("Expected error for invalid banner file extension, got nil")
	}

	expectedMsg := "BannerFile must have a .jpg, .jpeg, or .gif extension (got: banner.png)"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
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
			tmpDir, err := os.MkdirTemp("", "mobius-config-test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

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
