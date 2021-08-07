package hotline

type Config struct {
	Name                      string   `yaml:"Name"`                      // Name used for Tracker registration
	Description               string   `yaml:"Description"`               // Description used for Tracker registration
	BannerID                  int      `yaml:"BannerID"`                  // Unimplemented
	FileRoot                  string   `yaml:"FileRoot"`                  // Path to Files
	EnableTrackerRegistration bool     `yaml:"EnableTrackerRegistration"` // Toggle Tracker Registration
	Trackers                  []string `yaml:"Trackers"`                  // List of trackers that the server should register with
	NewsDelimiter             string   `yaml:"NewsDelimiter"`             // String used to separate news posts
	NewsDateFormat            string   `yaml:"NewsDateFormat"`            // Go template string to customize news date format
	MaxDownloads              int      `yaml:"MaxDownloads"`              // Global simultaneous download limit
	MaxDownloadsPerClient     int      `yaml:"MaxDownloadsPerClient"`     // Per client simultaneous download limit
	MaxConnectionsPerIP       int      `yaml:"MaxConnectionsPerIP"`       // Max connections per IP
}
