package mobius

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type TestData struct {
	Name  string `yaml:"name"`
	Value int    `yaml:"value"`
}

func TestLoadFromYAMLFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  string
		wantData TestData
		wantErr  bool
	}{
		{
			name:     "Valid YAML file",
			fileName: "valid.yaml",
			content:  "name: Test\nvalue: 123\n",
			wantData: TestData{Name: "Test", Value: 123},
			wantErr:  false,
		},
		{
			name:     "File not found",
			fileName: "nonexistent.yaml",
			content:  "",
			wantData: TestData{},
			wantErr:  true,
		},
		{
			name:     "Invalid YAML content",
			fileName: "invalid.yaml",
			content:  "name: Test\nvalue: invalid_int\n",
			wantData: TestData{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create a temporary file with the provided content if content is not empty
			if tt.content != "" {
				err := os.WriteFile(tt.fileName, []byte(tt.content), 0644)
				assert.NoError(t, err)
				defer os.Remove(tt.fileName) // Cleanup the file after the test
			}

			var data TestData
			err := loadFromYAMLFile(tt.fileName, &data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantData, data)
			}
		})
	}
}
