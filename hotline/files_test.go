package hotline

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeFilePath(t *testing.T) {
	var tests = []struct {
		filePath string
		want     []byte
	}{
		{
			filePath: "kitten1.jpg",
			want: []byte{
				0x00, 0x01, // number of items in path
				0x00, 0x00, // leading path separator
				0x0b,                                                             // length of next path section (11)
				0x6b, 0x69, 0x74, 0x74, 0x65, 0x6e, 0x31, 0x2e, 0x6a, 0x70, 0x67, // kitten1.jpg
			},
		},
		{
			filePath: "foo/kitten1.jpg",
			want: []byte{
				0x00, 0x02, // number of items in path
				0x00, 0x00,
				0x03,
				0x66, 0x6f, 0x6f,
				0x00, 0x00, // leading path separator
				0x0b,                                                             // length of next path section (11)
				0x6b, 0x69, 0x74, 0x74, 0x65, 0x6e, 0x31, 0x2e, 0x6a, 0x70, 0x67, // kitten1.jpg
			},
		},
	}

	for _, test := range tests {
		got := EncodeFilePath(test.filePath)
		if !bytes.Equal(got, test.want) {
			t.Errorf("field mismatch:  want: %#v got: %#v", test.want, got)
		}
	}
}

func TestCalcTotalSize(t *testing.T) {
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()

	_ = os.Chdir("test/config/Files")

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Foo",
			args: args{
				filePath: "test",
			},
			want:    []byte{0x00, 0x00, 0x18, 0x00},
			wantErr: false,
		},
		// TODO: Add more test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalcTotalSize(tt.args.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalcTotalSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("CalcTotalSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func createTestDirStructure(baseDir string, structure map[string]string) error {
	// First pass: create directories
	for path, content := range structure {
		if content == "dir" {
			if err := os.MkdirAll(filepath.Join(baseDir, path), 0755); err != nil {
				return err
			}
		}
	}

	// Second pass: create files
	for path, content := range structure {
		if content != "dir" {
			fullPath := filepath.Join(baseDir, path)
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestCalcItemCount(t *testing.T) {
	tests := []struct {
		name      string
		structure map[string]string
		expected  uint16
	}{
		{
			name: "directory with files",
			structure: map[string]string{
				"file1.txt":        "content1",
				"file2.txt":        "content2",
				"subdir/":          "dir",
				"subdir/file3.txt": "content3",
			},
			expected: 4, // 3 files and 1 directory, should count 4 items
		},
		{
			name: "directory with hidden files",
			structure: map[string]string{
				".hiddenfile": "hiddencontent",
				"file1.txt":   "content1",
			},
			expected: 1, // 1 non-hidden file
		},
		{
			name:      "empty directory",
			structure: map[string]string{},
			expected:  0, // 0 files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tempDir, err := os.MkdirTemp("", "test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create the test directory structure
			if err := createTestDirStructure(tempDir, tt.structure); err != nil {
				t.Fatalf("Failed to create test dir structure: %v", err)
			}

			// Calculate item count
			result, err := CalcItemCount(tempDir)
			if err != nil {
				t.Fatalf("CalcItemCount returned an error: %v", err)
			}

			// Convert result to uint16
			count := binary.BigEndian.Uint16(result)
			if count != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, count)
			}
		})
	}
}
