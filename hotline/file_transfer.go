package hotline

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// File transfer types
const (
	FileDownload   = 0
	FileUpload     = 1
	FolderDownload = 2
	FolderUpload   = 3
)

type FileTransfer struct {
	FileName        []byte
	FilePath        []byte
	ReferenceNumber []byte
	Type            int
	TransferSize    []byte // total size of all items in the folder. Only used in FolderUpload action
	FolderItemCount []byte
	BytesSent       int
	clientID        uint16
	fileResumeData  *FileResumeData
	options         []byte
}

func (ft *FileTransfer) String() string {
	percentComplete := 10
	out := fmt.Sprintf("%s\t %v", ft.FileName, percentComplete)

	return out
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

	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		pathSegments = append(pathSegments, string(pathData[3:3+segLen]))
		pathData = pathData[3+segLen:]
	}

	return strings.Join(pathSegments, pathSeparator)
}
