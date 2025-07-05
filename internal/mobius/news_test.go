package mobius

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewFlatNews(t *testing.T) {
	tests := []struct {
		name        string
		setupFile   func(string) error
		filePath    string
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name: "valid file with content",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte("test news content\nwith newlines"), 0644)
			},
			filePath: "test_news.txt",
			wantErr:  false,
		},
		{
			name: "valid empty file",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte(""), 0644)
			},
			filePath: "empty_news.txt",
			wantErr:  false,
		},
		{
			name:        "nonexistent file",
			setupFile:   func(path string) error { return nil },
			filePath:    "nonexistent.txt",
			wantErr:     true,
			wantErrMsg:  "reload:",
		},
		{
			name: "file with mixed line endings",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte("line1\nline2\r\nline3\r"), 0644)
			},
			filePath: "mixed_endings.txt",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			fullPath := filepath.Join(tempDir, tt.filePath)
			
			if err := tt.setupFile(fullPath); err != nil {
				t.Fatalf("Failed to setup test file: %v", err)
			}

			flatNews, err := NewFlatNews(fullPath)
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.wantErrMsg != "" && !containsSubstring(err.Error(), tt.wantErrMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.wantErrMsg, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if flatNews == nil {
				t.Error("Expected FlatNews instance but got nil")
				return
			}
			
			if flatNews.filePath != fullPath {
				t.Errorf("Expected filePath %q, got %q", fullPath, flatNews.filePath)
			}
		})
	}
}

func TestFlatNews_Reload(t *testing.T) {
	tests := []struct {
		name         string
		initialData  string
		newData      string
		expectData   string
		wantErr      bool
		deleteFile   bool
	}{
		{
			name:        "reload with new content",
			initialData: "initial content",
			newData:     "new content\nwith newlines",
			expectData:  "new content\rwith newlines",
			wantErr:     false,
		},
		{
			name:        "reload with empty content",
			initialData: "some content",
			newData:     "",
			expectData:  "",
			wantErr:     false,
		},
		{
			name:        "reload with mixed line endings",
			initialData: "old",
			newData:     "line1\nline2\r\nline3\r",
			expectData:  "line1\rline2\r\rline3\r",
			wantErr:     false,
		},
		{
			name:        "reload after file deletion",
			initialData: "content",
			newData:     "",
			expectData:  "",
			wantErr:     true,
			deleteFile:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.txt")
			
			if err := os.WriteFile(filePath, []byte(tt.initialData), 0644); err != nil {
				t.Fatalf("Failed to create initial file: %v", err)
			}
			
			flatNews, err := NewFlatNews(filePath)
			if err != nil {
				t.Fatalf("Failed to create FlatNews: %v", err)
			}
			
			if tt.deleteFile {
				if err := os.Remove(filePath); err != nil {
					t.Fatalf("Failed to delete file: %v", err)
				}
			} else {
				if err := os.WriteFile(filePath, []byte(tt.newData), 0644); err != nil {
					t.Fatalf("Failed to write new data: %v", err)
				}
			}
			
			err = flatNews.Reload()
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if string(flatNews.data) != tt.expectData {
				t.Errorf("Expected data %q, got %q", tt.expectData, string(flatNews.data))
			}
		})
	}
}

func TestFlatNews_Read(t *testing.T) {
	tests := []struct {
		name         string
		fileContent  string
		bufferSize   int
		expectedReads []readResult
	}{
		{
			name:        "read all at once",
			fileContent: "test content\nwith newlines",
			bufferSize:  100,
			expectedReads: []readResult{
				{data: "test content\rwith newlines", n: 26, err: nil},
				{data: "", n: 0, err: io.EOF},
			},
		},
		{
			name:        "read in chunks",
			fileContent: "hello world",
			bufferSize:  5,
			expectedReads: []readResult{
				{data: "hello", n: 5, err: nil},
				{data: " worl", n: 5, err: nil},
				{data: "d", n: 1, err: nil},
				{data: "", n: 0, err: io.EOF},
			},
		},
		{
			name:        "read empty file",
			fileContent: "",
			bufferSize:  10,
			expectedReads: []readResult{
				{data: "", n: 0, err: io.EOF},
			},
		},
		{
			name:        "small buffer large content",
			fileContent: "abcdefghij",
			bufferSize:  3,
			expectedReads: []readResult{
				{data: "abc", n: 3, err: nil},
				{data: "def", n: 3, err: nil},
				{data: "ghi", n: 3, err: nil},
				{data: "j", n: 1, err: nil},
				{data: "", n: 0, err: io.EOF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.txt")
			
			if err := os.WriteFile(filePath, []byte(tt.fileContent), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
			
			flatNews, err := NewFlatNews(filePath)
			if err != nil {
				t.Fatalf("Failed to create FlatNews: %v", err)
			}
			
			for i, expected := range tt.expectedReads {
				buf := make([]byte, tt.bufferSize)
				n, err := flatNews.Read(buf)
				
				if err != expected.err {
					t.Errorf("Read %d: expected error %v, got %v", i, expected.err, err)
				}
				
				if n != expected.n {
					t.Errorf("Read %d: expected n %d, got %d", i, expected.n, n)
				}
				
				actualData := string(buf[:n])
				if actualData != expected.data {
					t.Errorf("Read %d: expected data %q, got %q", i, expected.data, actualData)
				}
			}
		})
	}
}

func TestFlatNews_Write(t *testing.T) {
	tests := []struct {
		name         string
		initialData  string
		writeData    string
		expectedData string
		wantErr      bool
	}{
		{
			name:         "write to empty file",
			initialData:  "",
			writeData:    "new content",
			expectedData: "new content",
			wantErr:      false,
		},
		{
			name:         "prepend to existing content",
			initialData:  "existing",
			writeData:    "new ",
			expectedData: "new existing",
			wantErr:      false,
		},
		{
			name:         "write empty data",
			initialData:  "content",
			writeData:    "",
			expectedData: "content",
			wantErr:      false,
		},
		{
			name:         "write binary data",
			initialData:  "text",
			writeData:    "\x00\x01\x02",
			expectedData: "\x00\x01\x02text",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.txt")
			
			if err := os.WriteFile(filePath, []byte(tt.initialData), 0644); err != nil {
				t.Fatalf("Failed to create initial file: %v", err)
			}
			
			flatNews, err := NewFlatNews(filePath)
			if err != nil {
				t.Fatalf("Failed to create FlatNews: %v", err)
			}
			
			n, err := flatNews.Write([]byte(tt.writeData))
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if n != len(tt.writeData) {
				t.Errorf("Expected n %d, got %d", len(tt.writeData), n)
			}
			
			if string(flatNews.data) != tt.expectedData {
				t.Errorf("Expected data %q, got %q", tt.expectedData, string(flatNews.data))
			}
			
			fileData, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("Failed to read file: %v", err)
				return
			}
			
			if string(fileData) != tt.expectedData {
				t.Errorf("Expected file data %q, got %q", tt.expectedData, string(fileData))
			}
		})
	}
}

func TestFlatNews_Seek(t *testing.T) {
	tests := []struct {
		name         string
		fileContent  string
		offset       int64
		whence       int
		expectOffset int64
		expectErr    bool
	}{
		{
			name:         "seek to beginning",
			fileContent:  "test content",
			offset:       0,
			whence:       0,
			expectOffset: 0,
			expectErr:    false,
		},
		{
			name:         "seek to middle",
			fileContent:  "test content",
			offset:       5,
			whence:       0,
			expectOffset: 0,
			expectErr:    false,
		},
		{
			name:         "seek beyond end",
			fileContent:  "test",
			offset:       10,
			whence:       0,
			expectOffset: 0,
			expectErr:    false,
		},
		{
			name:         "negative offset",
			fileContent:  "test",
			offset:       -5,
			whence:       0,
			expectOffset: 0,
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.txt")
			
			if err := os.WriteFile(filePath, []byte(tt.fileContent), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
			
			flatNews, err := NewFlatNews(filePath)
			if err != nil {
				t.Fatalf("Failed to create FlatNews: %v", err)
			}
			
			offset, err := flatNews.Seek(tt.offset, tt.whence)
			
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if offset != tt.expectOffset {
				t.Errorf("Expected offset %d, got %d", tt.expectOffset, offset)
			}
			
			expectedReadOffset := int(tt.offset)
			if flatNews.readOffset != expectedReadOffset {
				t.Errorf("Expected readOffset %d, got %d", expectedReadOffset, flatNews.readOffset)
			}
		})
	}
}

func TestFlatNews_ConcurrentOperations(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "concurrent_test.txt")
	
	if err := os.WriteFile(filePath, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	flatNews, err := NewFlatNews(filePath)
	if err != nil {
		t.Fatalf("Failed to create FlatNews: %v", err)
	}
	
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			buf := make([]byte, 10)
			_, err := flatNews.Read(buf)
			if err != nil && err != io.EOF {
				errors <- fmt.Errorf("read goroutine %d: %w", id, err)
			}
		}(i)
	}
	
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			if err := flatNews.Reload(); err != nil {
				errors <- fmt.Errorf("reload goroutine %d: %w", id, err)
			}
		}(i)
	}
	
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			data := fmt.Sprintf("data%d", id)
			if _, err := flatNews.Write([]byte(data)); err != nil {
				errors <- fmt.Errorf("write goroutine %d: %w", id, err)
			}
		}(i)
	}
	
	wg.Wait()
	close(errors)
	
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

type readResult struct {
	data string
	n    int
	err  error
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (len(substr) == 0 || 
		    strings.Contains(s, substr))
}