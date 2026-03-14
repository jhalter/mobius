package hotline

import "time"

// BanDuration is the length of time for temporary bans.
const BanDuration = 30 * time.Minute

// Redis key constants for ban management and online user tracking
const (
	RedisKeyBannedIPs       = "mobius:banned:ips"
	RedisKeyBannedUsers     = "mobius:banned:users"
	RedisKeyBannedNicknames = "mobius:banned:nicknames"
	RedisKeyTempBannedIPs   = "mobius:temp_banned:ips:"
	RedisKeyOnline          = "mobius:online"
)

type BanMgr interface {
	Add(ip string, until *time.Time) error
	IsBanned(ip string) (bool, *time.Time)
	
	// IP banning
	UnbanIP(ip string) error
	
	// Username banning
	BanUsername(username string) error
	UnbanUsername(username string) error
	IsUsernameBanned(username string) bool
	
	// Nickname banning
	BanNickname(nickname string) error
	UnbanNickname(nickname string) error
	IsNicknameBanned(nickname string) bool
	
	// List operations
	ListBannedIPs() ([]string, error)
	ListBannedUsernames() ([]string, error)
	ListBannedNicknames() ([]string, error)
}
