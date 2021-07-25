package hotline

import "fmt"

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
}

func (ft *FileTransfer) String() string {
	percentComplete := 10
	out := fmt.Sprintf("%s\t %v", ft.FileName, percentComplete)

	return out
}
