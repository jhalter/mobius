package mobius

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const agreementFile = "Agreement.txt"

type Agreement struct {
	data        []byte
	filePath    string
	lineEndings string

	mu         sync.RWMutex
	readOffset int // Internal offset to track read progress
}

func NewAgreement(path, lineEndings string) (*Agreement, error) {
	data, err := os.ReadFile(filepath.Join(path, agreementFile))
	if err != nil {
		return &Agreement{}, fmt.Errorf("read file: %w", err)
	}

	// Swap line breaks
	agreement := strings.ReplaceAll(string(data), "\n", lineEndings)
	agreement = strings.ReplaceAll(agreement, "\r\n", lineEndings)

	return &Agreement{
		data:        []byte(agreement),
		filePath:    filepath.Join(path, agreementFile),
		lineEndings: lineEndings,
	}, nil
}

func (a *Agreement) Reload() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Swap line breaks
	agreement := strings.ReplaceAll(string(data), "\n", a.lineEndings)
	agreement = strings.ReplaceAll(agreement, "\r\n", a.lineEndings)

	a.data = []byte(agreement)

	return nil
}

// It returns the number of bytes read and any error encountered.
func (a *Agreement) Read(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.readOffset >= len(a.data) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, a.data[a.readOffset:])

	a.readOffset += n

	return n, nil
}

func (a *Agreement) Seek(offset int64, _ int) (int64, error) {
	a.readOffset = int(offset)

	return 0, nil
}
