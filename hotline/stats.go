package hotline

import (
	"fmt"
	"time"
)

type Stats struct {
	LoginCount int           `yaml:"login count"`
	StartTime  time.Time     `yaml:"start time"`
	Uptime     time.Duration `yaml:"uptime"`
}

func (s *Stats) String() string {
	template := `
Server Stats:
  Start Time:		%v
  Uptime:			%s
  Login Count:	%v
`
	d := time.Since(s.StartTime)
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute

	return fmt.Sprintf(
		template,
		s.StartTime.Format(time.RFC1123Z),
		fmt.Sprintf("%02d:%02d", h, m),
		s.LoginCount,
	)
}
