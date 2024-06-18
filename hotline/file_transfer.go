package hotline

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"sync"
)

// File transfer types
const (
	FileDownload = iota
	FileUpload
	FolderDownload
	FolderUpload
	bannerDownload
)

type FileTransfer struct {
	FileName         []byte
	FilePath         []byte
	ReferenceNumber  []byte
	refNum           [4]byte
	Type             int
	TransferSize     []byte
	FolderItemCount  []byte
	fileResumeData   *FileResumeData
	options          []byte
	bytesSentCounter *WriteCounter
	ClientConn       *ClientConn
}

// WriteCounter counts the number of bytes written to it.
type WriteCounter struct {
	mux   sync.Mutex
	Total int64 // Total # of bytes written
}

// Write implements the io.Writer interface.
//
// Always completes and never returns an error.
func (wc *WriteCounter) Write(p []byte) (int, error) {
	wc.mux.Lock()
	defer wc.mux.Unlock()
	n := len(p)
	wc.Total += int64(n)
	return n, nil
}

func (cc *ClientConn) newFileTransfer(transferType int, fileName, filePath, size []byte) *FileTransfer {
	var transactionRef [4]byte
	_, _ = rand.Read(transactionRef[:])

	ft := &FileTransfer{
		FileName:         fileName,
		FilePath:         filePath,
		ReferenceNumber:  transactionRef[:],
		refNum:           transactionRef,
		Type:             transferType,
		TransferSize:     size,
		ClientConn:       cc,
		bytesSentCounter: &WriteCounter{},
	}

	cc.transfersMU.Lock()
	defer cc.transfersMU.Unlock()
	cc.transfers[transferType][transactionRef] = ft

	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()
	cc.Server.fileTransfers[transactionRef] = ft

	return ft
}

// String returns a string representation of a file transfer and its progress for display in the GetInfo window
// Example:
// MasterOfOrionII1.4.0. 0%   197.9M
func (ft *FileTransfer) String() string {
	trunc := fmt.Sprintf("%.21s", ft.FileName)
	return fmt.Sprintf("%-21s %.3s%%  %6s\n", trunc, ft.percentComplete(), ft.formattedTransferSize())
}

func (ft *FileTransfer) percentComplete() string {
	ft.bytesSentCounter.mux.Lock()
	defer ft.bytesSentCounter.mux.Unlock()
	return fmt.Sprintf(
		"%v",
		math.RoundToEven(float64(ft.bytesSentCounter.Total)/float64(binary.BigEndian.Uint32(ft.TransferSize))*100),
	)
}

func (ft *FileTransfer) formattedTransferSize() string {
	sizeInKB := float32(binary.BigEndian.Uint32(ft.TransferSize)) / 1024
	if sizeInKB > 1024 {
		return fmt.Sprintf("%.1fM", sizeInKB/1024)
	} else {
		return fmt.Sprintf("%.0fK", sizeInKB)
	}
}

func (ft *FileTransfer) ItemCount() int {
	return int(binary.BigEndian.Uint16(ft.FolderItemCount))
}

type folderUpload struct {
	DataSize      [2]byte
	IsFolder      [2]byte
	PathItemCount [2]byte
	FileNamePath  []byte
}

func (fu *folderUpload) FormattedPath() string {
	pathItemLen := binary.BigEndian.Uint16(fu.PathItemCount[:])

	var pathSegments []string
	pathData := fu.FileNamePath

	// TODO: implement scanner interface instead?
	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		pathSegments = append(pathSegments, string(pathData[3:3+segLen]))
		pathData = pathData[3+segLen:]
	}

	return filepath.Join(pathSegments...)
}
