package hotline

type Config struct {
	Name                      string   `yaml:"Name" validate:"required,max=50"`         // Name used for Tracker registration
	Description               string   `yaml:"Description" validate:"required,max=200"` // Description used for Tracker registration
	BannerFile                string   `yaml:"BannerFile"`                              // Path to banner jpg
	FileRoot                  string   `yaml:"FileRoot" validate:"required"`            // Path to Files
	EnableTrackerRegistration bool     `yaml:"EnableTrackerRegistration"`               // Toggle Tracker Registration
	Trackers                  []string `yaml:"Trackers" validate:"dive,hostname_port"`  // List of trackers that the server should register with
	NewsDelimiter             string   `yaml:"NewsDelimiter"`                           // String used to separate news posts
	NewsDateFormat            string   `yaml:"NewsDateFormat"`                          // Go template string to customize news date format
	MaxDownloads              int      `yaml:"MaxDownloads"`                            // Global simultaneous download limit
	MaxDownloadsPerClient     int      `yaml:"MaxDownloadsPerClient"`                   // Per client simultaneous download limit
	MaxConnectionsPerIP       int      `yaml:"MaxConnectionsPerIP"`                     // Max connections per IP
	PreserveResourceForks     bool     `yaml:"PreserveResourceForks"`                   // Enable preservation of file info and resource forks in sidecar files
	IgnoreFiles               []string `yaml:"IgnoreFiles"`                             // List of regular expression for filtering files from the file list
}
