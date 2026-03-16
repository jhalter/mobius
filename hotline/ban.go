package hotline

import (
	"time"

	"github.com/stretchr/testify/mock"
)

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

type MockBanMgr struct {
	mock.Mock
}

func (m *MockBanMgr) Add(ip string, until *time.Time) error {
	args := m.Called(ip, until)
	return args.Error(0)
}

func (m *MockBanMgr) IsBanned(ip string) (bool, *time.Time) {
	args := m.Called(ip)
	return args.Bool(0), args.Get(1).(*time.Time)
}

func (m *MockBanMgr) UnbanIP(ip string) error {
	args := m.Called(ip)
	return args.Error(0)
}

func (m *MockBanMgr) BanUsername(username string) error {
	args := m.Called(username)
	return args.Error(0)
}

func (m *MockBanMgr) UnbanUsername(username string) error {
	args := m.Called(username)
	return args.Error(0)
}

func (m *MockBanMgr) IsUsernameBanned(username string) bool {
	args := m.Called(username)
	return args.Bool(0)
}

func (m *MockBanMgr) BanNickname(nickname string) error {
	args := m.Called(nickname)
	return args.Error(0)
}

func (m *MockBanMgr) UnbanNickname(nickname string) error {
	args := m.Called(nickname)
	return args.Error(0)
}

func (m *MockBanMgr) IsNicknameBanned(nickname string) bool {
	args := m.Called(nickname)
	return args.Bool(0)
}

func (m *MockBanMgr) ListBannedIPs() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBanMgr) ListBannedUsernames() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBanMgr) ListBannedNicknames() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}
