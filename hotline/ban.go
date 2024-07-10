package hotline

import "time"

const tempBanDuration = 30 * time.Minute

type BanMgr interface {
	Add(ip string, until *time.Time) error
	IsBanned(ip string) (bool, *time.Time)
}
