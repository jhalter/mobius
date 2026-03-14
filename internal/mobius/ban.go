package mobius

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/jhalter/mobius/hotline"
	"gopkg.in/yaml.v3"
)

type BanFile struct {
	banList     map[string]*time.Time
	bannedUsers map[string]bool
	bannedNicks map[string]bool
	filePath    string

	sync.Mutex
}

func NewBanFile(path string) (*BanFile, error) {
	bf := &BanFile{
		filePath:    path,
		banList:     make(map[string]*time.Time),
		bannedUsers: make(map[string]bool),
		bannedNicks: make(map[string]bool),
	}

	err := bf.Load()
	if err != nil {
		return nil, fmt.Errorf("load ban file: %w", err)
	}

	return bf, nil
}

type BanFileData struct {
	BanList     map[string]*time.Time `yaml:"banList"`
	BannedUsers map[string]bool       `yaml:"bannedUsers"`
	BannedNicks map[string]bool       `yaml:"bannedNicks"`
}

func (bf *BanFile) Load() error {
	bf.Lock()
	defer bf.Unlock()

	bf.banList = make(map[string]*time.Time)
	bf.bannedUsers = make(map[string]bool)
	bf.bannedNicks = make(map[string]bool)

	fh, err := os.Open(bf.filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer func() { _ = fh.Close() }()

	// Read all file content for proper format detection
	content, err := io.ReadAll(fh)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}

	// Try to decode as new format first
	var data BanFileData
	err = yaml.Unmarshal(content, &data)
	if err == nil && (data.BanList != nil || data.BannedUsers != nil || data.BannedNicks != nil) {
		// Successfully decoded as new format and has actual data
		if data.BanList != nil {
			bf.banList = data.BanList
		}
		if data.BannedUsers != nil {
			bf.bannedUsers = data.BannedUsers
		}
		if data.BannedNicks != nil {
			bf.bannedNicks = data.BannedNicks
		}
		return nil
	}

	// Try to decode as legacy format (simple map)
	var legacyData map[string]*time.Time
	err = yaml.Unmarshal(content, &legacyData)
	if err != nil {
		return fmt.Errorf("decode yaml: %v", err)
	}

	bf.banList = legacyData

	return nil
}

// add is the internal implementation that assumes the caller holds the lock.
func (bf *BanFile) add(ip string, until *time.Time) error {
	bf.banList[ip] = until
	return bf.save()
}

func (bf *BanFile) Add(ip string, until *time.Time) error {
	bf.Lock()
	defer bf.Unlock()

	return bf.add(ip, until)
}

// save persists the ban data to disk.
// Caller must hold bf.Lock().
func (bf *BanFile) save() error {
	data := BanFileData{
		BanList:     bf.banList,
		BannedUsers: bf.bannedUsers,
		BannedNicks: bf.bannedNicks,
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal yaml: %v", err)
	}

	err = os.WriteFile(path.Join(bf.filePath), out, 0644)
	if err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	return nil
}

func (bf *BanFile) IsBanned(ip string) (bool, *time.Time) {
	bf.Lock()
	defer bf.Unlock()

	if until, ok := bf.banList[ip]; ok {
		return true, until
	}

	return false, nil
}

// UnbanIP removes an IP from the banned IPs list
func (bf *BanFile) UnbanIP(ip string) error {
	bf.Lock()
	defer bf.Unlock()

	delete(bf.banList, ip)
	return bf.save()
}

// BanUsername adds a username to the banned users set
func (bf *BanFile) BanUsername(username string) error {
	bf.Lock()
	defer bf.Unlock()

	bf.bannedUsers[username] = true
	return bf.save()
}

// UnbanUsername removes a username from the banned users set
func (bf *BanFile) UnbanUsername(username string) error {
	bf.Lock()
	defer bf.Unlock()

	delete(bf.bannedUsers, username)
	return bf.save()
}

// IsUsernameBanned checks if a username is banned
func (bf *BanFile) IsUsernameBanned(username string) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.bannedUsers[username]
}

// BanNickname adds a nickname to the banned nicknames set
func (bf *BanFile) BanNickname(nickname string) error {
	bf.Lock()
	defer bf.Unlock()

	bf.bannedNicks[nickname] = true
	return bf.save()
}

// UnbanNickname removes a nickname from the banned nicknames set
func (bf *BanFile) UnbanNickname(nickname string) error {
	bf.Lock()
	defer bf.Unlock()

	delete(bf.bannedNicks, nickname)
	return bf.save()
}

// IsNicknameBanned checks if a nickname is banned
func (bf *BanFile) IsNicknameBanned(nickname string) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.bannedNicks[nickname]
}

// ListBannedIPs returns all banned IP addresses
func (bf *BanFile) ListBannedIPs() ([]string, error) {
	bf.Lock()
	defer bf.Unlock()

	var ips []string
	for ip := range bf.banList {
		ips = append(ips, ip)
	}
	return ips, nil
}

// ListBannedUsernames returns all banned usernames
func (bf *BanFile) ListBannedUsernames() ([]string, error) {
	bf.Lock()
	defer bf.Unlock()

	var usernames []string
	for username := range bf.bannedUsers {
		usernames = append(usernames, username)
	}
	return usernames, nil
}

// ListBannedNicknames returns all banned nicknames
func (bf *BanFile) ListBannedNicknames() ([]string, error) {
	bf.Lock()
	defer bf.Unlock()

	var nicknames []string
	for nickname := range bf.bannedNicks {
		nicknames = append(nicknames, nickname)
	}
	return nicknames, nil
}

// Ensure BanFile implements the BanMgr interface
var _ hotline.BanMgr = (*BanFile)(nil)
