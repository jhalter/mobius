package hotline

import "time"

// BanDuration is the length of time for temporary bans.
const BanDuration = 30 * time.Minute

type BanMgr interface {
	Add(ip string, until *time.Time) error
	IsBanned(ip string) (bool, *time.Time)
}
