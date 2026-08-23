package hotline

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// NewsFeedConfig maps an existing threaded-news category to an RSS or Atom
// feed. Feed entries are imported as ordinary articles in ThreadedNews.yaml.
type NewsFeedConfig struct {
	CategoryPath []string `yaml:"CategoryPath" validate:"required,min=1,dive,required"`
	URL          string   `yaml:"URL" validate:"required"`
}

func (c *NewsFeedConfig) UnmarshalYAML(value *yaml.Node) error {
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if key != "CategoryPath" && key != "URL" {
			return fmt.Errorf("unknown NewsFeeds setting %q", key)
		}
	}
	type plainNewsFeedConfig NewsFeedConfig
	return value.Decode((*plainNewsFeedConfig)(c))
}

type Config struct {
	Name                      string           `yaml:"Name" validate:"required,max=50"`                    // Name used for Tracker registration
	Description               string           `yaml:"Description" validate:"required,max=200"`            // Description used for Tracker registration
	BannerFile                string           `yaml:"BannerFile" validate:"omitempty,bannerext"`          // Path to Banner jpg or gif
	FileRoot                  string           `yaml:"FileRoot" validate:"required"`                       // Path to Files
	EnableTrackerRegistration bool             `yaml:"EnableTrackerRegistration"`                          // Toggle Tracker Registration
	Trackers                  []string         `yaml:"Trackers" validate:"dive,hostname_port"`             // List of trackers that the server should register with
	NewsDelimiter             string           `yaml:"NewsDelimiter"`                                      // String used to separate news posts
	NewsDateFormat            string           `yaml:"NewsDateFormat"`                                     // Go template string to customize news date format
	MaxDownloads              int              `yaml:"MaxDownloads"`                                       // Global simultaneous download limit
	MaxDownloadsPerClient     int              `yaml:"MaxDownloadsPerClient"`                              // Per client simultaneous download limit
	MaxConnectionsPerIP       int              `yaml:"MaxConnectionsPerIP"`                                // Max connections per IP
	PreserveResourceForks     bool             `yaml:"PreserveResourceForks"`                              // Enable preservation of file info and resource forks in sidecar files
	IgnoreFiles               []string         `yaml:"IgnoreFiles"`                                        // List of regular expression for filtering files from the file list
	EnableBonjour             bool             `yaml:"EnableBonjour"`                                      // Enable service announcement on local network with Bonjour
	Encoding                  string           `yaml:"Encoding" validate:"omitempty,oneof=macintosh utf8"` // Encoding for filesystem names and server-generated feed news
	NewsFeeds                 []NewsFeedConfig `yaml:"NewsFeeds" validate:"dive"`                          // Existing threaded-news categories populated from remote feeds
}
