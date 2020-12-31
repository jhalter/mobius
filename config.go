package hotline

const newsTemplate = `From %s (%s):

%s

__________________________________________________________
`

type Config struct {
	Name                      string   `yaml:"Name"`                      // Name used for Tracker registration
	Description               string   `yaml:"Description"`               // Description used for Tracker registration
	BannerID                  int      `yaml:"BannerID"`                  // Unimplemented
	FileRoot                  string   `yaml:"FileRoot"`                  // Path to Files
	EnableTrackerRegistration bool     `yaml:"EnableTrackerRegistration"` // Toggle Tracker Registration
	Trackers                  []string `yaml:"Trackers"`                  // List of trackers that the server should register with
}
