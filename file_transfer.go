package hotline

const (
	FileDownload   int = 0
	FileUpload     int = 1
	FolderDownload int = 2
	FolderUpload   int = 3
)

type FileTransfer struct {
	FileName        []byte
	FilePath        []byte
	ReferenceNumber []byte
	Type            int
}
