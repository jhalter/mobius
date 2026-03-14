package mobius

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisBanMgr_TemporalBans(t *testing.T) {
	// Start mini redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	defer func() { _ = client.Close() }()

	// Create a silent logger for tests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	banMgr := NewRedisBanMgr(client, logger)

	tests := []struct {
		name           string
		ip             string
		until          *time.Time
		expectedBanned bool
		expectedUntil  *time.Time
		setup          func() error
		cleanup        func() error
	}{
		{
			name:           "Permanent ban via Add method",
			ip:             "192.168.1.100",
			until:          nil,
			expectedBanned: true,
			expectedUntil:  nil,
		},
		{
			name:           "Temporary ban via Add method",
			ip:             "192.168.1.101",
			until:          func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
			expectedBanned: true,
			expectedUntil:  func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
		},
		{
			name:           "Expired temporary ban",
			ip:             "192.168.1.102",
			until:          func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
			expectedBanned: false,
			expectedUntil:  nil,
		},
		{
			name:           "UnbanIP removes both permanent and temporary bans",
			ip:             "192.168.1.104",
			until:          nil, // Not used since we have custom setup
			expectedBanned: false,
			expectedUntil:  nil,
			setup: func() error {
				// Add both permanent and temporary ban
				if err := banMgr.Add("192.168.1.104", nil); err != nil {
					return err
				}
				expiration := time.Now().Add(1 * time.Hour)
				if err := banMgr.Add("192.168.1.104", &expiration); err != nil {
					return err
				}
				// First verify the IP is banned (permanent takes precedence)
				banned, until := banMgr.IsBanned("192.168.1.104")
				if !banned || until != nil {
					return fmt.Errorf("setup failed: IP should be permanently banned")
				}
				// Now unban to test the cleanup
				return banMgr.UnbanIP("192.168.1.104")
			},
			cleanup: func() error {
				// Additional cleanup in case something went wrong
				return banMgr.UnbanIP("192.168.1.104")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup if needed
			if tt.setup != nil {
				err := tt.setup()
				assert.NoError(t, err)
			} else {
				// Default setup: add the ban
				err := banMgr.Add(tt.ip, tt.until)
				assert.NoError(t, err)
			}

			// Check ban status
			banned, until := banMgr.IsBanned(tt.ip)
			assert.Equal(t, tt.expectedBanned, banned)

			if tt.expectedUntil != nil {
				assert.NotNil(t, until)
				assert.WithinDuration(t, *tt.expectedUntil, *until, 1*time.Second)
			} else {
				assert.Equal(t, tt.expectedUntil, until)
			}

			// Cleanup
			if tt.cleanup != nil {
				err := tt.cleanup()
				assert.NoError(t, err)
			} else {
				err := banMgr.UnbanIP(tt.ip)
				assert.NoError(t, err)
			}
		})
	}

	t.Run("ListBannedIPs includes both permanent and temporary", func(t *testing.T) {
		// Add permanent ban
		permanentIP := "192.168.1.105"
		err := banMgr.Add(permanentIP, nil)
		assert.NoError(t, err)
		
		// Add temporary ban
		temporaryIP := "192.168.1.106"
		expiration := time.Now().Add(1 * time.Hour)
		err = banMgr.Add(temporaryIP, &expiration)
		assert.NoError(t, err)
		
		// List should include both
		ips, err := banMgr.ListBannedIPs()
		assert.NoError(t, err)
		assert.Contains(t, ips, permanentIP)
		assert.Contains(t, ips, temporaryIP)
		
		// Cleanup
		err = banMgr.UnbanIP(permanentIP)
		assert.NoError(t, err)
		err = banMgr.UnbanIP(temporaryIP)
		assert.NoError(t, err)
	})
}

func TestRedisBanMgr_UserAndNicknameBans(t *testing.T) {
	// Start mini redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	defer func() { _ = client.Close() }()

	// Create a silent logger for tests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	banMgr := NewRedisBanMgr(client, logger)

	tests := []struct {
		name         string
		banType      string
		value        string
		banFunc      func(string) error
		unbanFunc    func(string) error
		isBannedFunc func(string) bool
		listFunc     func() ([]string, error)
	}{
		{
			name:         "User banning",
			banType:      "username",
			value:        "testuser",
			banFunc:      banMgr.BanUsername,
			unbanFunc:    banMgr.UnbanUsername,
			isBannedFunc: banMgr.IsUsernameBanned,
			listFunc:     banMgr.ListBannedUsernames,
		},
		{
			name:         "Nickname banning",
			banType:      "nickname",
			value:        "testnick",
			banFunc:      banMgr.BanNickname,
			unbanFunc:    banMgr.UnbanNickname,
			isBannedFunc: banMgr.IsNicknameBanned,
			listFunc:     banMgr.ListBannedNicknames,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initially not banned
			assert.False(t, tt.isBannedFunc(tt.value))
			
			// Ban the item
			err := tt.banFunc(tt.value)
			assert.NoError(t, err)
			
			// Should be banned
			assert.True(t, tt.isBannedFunc(tt.value))
			
			// Should appear in list
			items, err := tt.listFunc()
			assert.NoError(t, err)
			assert.Contains(t, items, tt.value)
			
			// Unban the item
			err = tt.unbanFunc(tt.value)
			assert.NoError(t, err)
			
			// Should not be banned
			assert.False(t, tt.isBannedFunc(tt.value))
		})
	}
}