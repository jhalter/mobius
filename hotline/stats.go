package hotline

import (
	"time"
)

type Stats struct {
	CurrentlyConnected  int
	DownloadsInProgress int
	UploadsInProgress   int
	ConnectionPeak      int
	DownloadCounter     int
	UploadCounter       int

	LoginCount int       `yaml:"login count"`
	StartTime  time.Time `yaml:"start time"`
}
