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

// 00 28 // DataSize
// 00 00 // IsFolder
// 00 02 // PathItemCount
//
// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72 // "subfolder"
//
// 00 00
// 15
// 73 75 62 66 6f 6c 64 65 72 2d 74 65 73 74 66 69 6c 65 2d 35 6b // "subfolder-testfile-5k"
func readFolderUpload(buf []byte) folderUpload {
	dataLen := binary.BigEndian.Uint16(buf[0:2])

	fu := folderUpload{
		DataSize:      [2]byte{buf[0], buf[1]}, // Size of this structure (not including data size element itself)
		IsFolder:      [2]byte{buf[2], buf[3]},
		PathItemCount: [2]byte{buf[4], buf[5]},
		FileNamePath:  buf[6 : dataLen+2],
	}

	return fu
}

func (fu *folderUpload) UnmarshalBinary(b []byte) error {
	fu.DataSize = [2]byte{b[0], b[1]}
	fu.IsFolder = [2]byte{b[2], b[3]}
	fu.PathItemCount = [2]byte{b[4], b[5]}

	return nil
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
