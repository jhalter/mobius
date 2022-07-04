package hotline

import (
	"time"
)

type Stats struct {
	CurrentlyConnected  int
	DownloadsInProgress int
	UploadsInProgress   int
	WaitingDownloads    int
	ConnectionPeak      int
	ConnectionCounter   int
	DownloadCounter     int
	UploadCounter       int
	Since               time.Time
}
