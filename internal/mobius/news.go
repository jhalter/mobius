package mobius

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
)

type FlatNews struct {
	data     []byte
	filePath string

	mu         sync.Mutex
	readOffset int // Internal offset to track read progress
}

func NewFlatNews(path string) (*FlatNews, error) {
	flatNews := &FlatNews{filePath: path}
	if err := flatNews.Reload(); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}

	return flatNews, nil
}

func (f *FlatNews) Reload() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := os.ReadFile(f.filePath)
	if err != nil {
		return err
	}

	// Swap line breaks
	agreement := strings.ReplaceAll(string(data), "\n", "\r")
	agreement = strings.ReplaceAll(agreement, "\r\n", "\r")

	f.data = []byte(agreement)

	return nil
}

// It returns the number of bytes read and any error encountered.
func (f *FlatNews) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.readOffset >= len(f.data) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, f.data[f.readOffset:])

	f.readOffset += n

	return n, nil
}

// Write implements io.Writer for flat news.
// p is guaranteed to contain the full data of a news post.
func (f *FlatNews) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.data = slices.Concat(p, f.data)

	tempFilePath := f.filePath + ".tmp"

	if err := os.WriteFile(tempFilePath, f.data, 0644); err != nil {
		return 0, fmt.Errorf("write to temporary file: %v", err)
	}

	// Atomically rename the temporary file to the final file path.
	if err := os.Rename(tempFilePath, f.filePath); err != nil {
		return 0, fmt.Errorf("rename temporary file to final file: %v", err)
	}

	return len(p), os.WriteFile(f.filePath, f.data, 0644)
}

func (f *FlatNews) Seek(offset int64, _ int) (int64, error) {
	f.readOffset = int(offset)

	return 0, nil
}
