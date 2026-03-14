package mobius

import (
	"fmt"
	"os"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBanFile(t *testing.T) {
	cwd, _ := os.Getwd()
	str := "2024-06-29T11:34:43.245899-07:00"
	testTime, _ := time.Parse(time.RFC3339Nano, str)

	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    *BanFile
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Valid path with valid content",
			args: args{path: path.Join(cwd, "test", "config", "Banlist.yaml")},
			want: &BanFile{
				filePath:    path.Join(cwd, "test", "config", "Banlist.yaml"),
				banList:     map[string]*time.Time{"192.168.86.29": &testTime},
				bannedUsers: make(map[string]bool),
				bannedNicks: make(map[string]bool),
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewBanFile(tt.args.path)
			if !tt.wantErr(t, err, fmt.Sprintf("NewBanFile(%v)", tt.args.path)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NewBanFile(%v)", tt.args.path)
		})
	}
}

// TestAdd tests the Add function.
func TestAdd(t *testing.T) {
	// Create a temporary directory.
	tmpDir, err := os.MkdirTemp("", "banfile_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() // Clean up the temporary directory.

	// Path to the temporary ban file.
	tmpFilePath := path.Join(tmpDir, "banfile.yaml")

	// Initialize BanFile.
	bf := &BanFile{
		filePath: tmpFilePath,
		banList:  make(map[string]*time.Time),
	}

	// Define the test cases.
	tests := []struct {
		name   string
		ip     string
		until  *time.Time
		expect map[string]*time.Time
	}{
		{
			name:  "Add IP with no expiration",
			ip:    "192.168.1.1",
			until: nil,
			expect: map[string]*time.Time{
				"192.168.1.1": nil,
			},
		},
		{
			name:  "Add IP with expiration",
			ip:    "192.168.1.2",
			until: func() *time.Time { t := time.Date(2024, 6, 29, 11, 34, 43, 245899000, time.UTC); return &t }(),
			expect: map[string]*time.Time{
				"192.168.1.1": nil,
				"192.168.1.2": func() *time.Time { t := time.Date(2024, 6, 29, 11, 34, 43, 245899000, time.UTC); return &t }(),
			},
		},
	}

	// Run the test cases.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bf.Add(tt.ip, tt.until)
			assert.NoError(t, err, "Add() error")

			// Load the file to check its contents.
			loadedBanFile := &BanFile{filePath: tmpFilePath}
			err = loadedBanFile.Load()
			assert.NoError(t, err, "Load() error")
			assert.Equal(t, tt.expect, loadedBanFile.banList, "Ban list does not match")
		})
	}
}

func TestBanFile_IsBanned(t *testing.T) {
	type fields struct {
		banList map[string]*time.Time
	}
	type args struct {
		ip string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
		want1  *time.Time
	}{
		{
			name: "with permanent ban",
			fields: fields{
				banList: map[string]*time.Time{
					"192.168.86.1": nil,
				},
			},
			args:  args{ip: "192.168.86.1"},
			want:  true,
			want1: nil,
		},
		{
			name: "with no ban",
			fields: fields{
				banList: map[string]*time.Time{},
			},
			args:  args{ip: "192.168.86.1"},
			want:  false,
			want1: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bf := &BanFile{
				banList: tt.fields.banList,
				Mutex:   sync.Mutex{},
			}
			got, got1 := bf.IsBanned(tt.args.ip)
			assert.Equalf(t, tt.want, got, "IsBanned(%v)", tt.args.ip)
			assert.Equalf(t, tt.want1, got1, "IsBanned(%v)", tt.args.ip)
		})
	}
}

func newTempBanFile(t *testing.T) *BanFile {
	t.Helper()
	tmpDir := t.TempDir()
	return &BanFile{
		filePath:    path.Join(tmpDir, "banfile.yaml"),
		banList:     make(map[string]*time.Time),
		bannedUsers: make(map[string]bool),
		bannedNicks: make(map[string]bool),
	}
}

func TestBanFile_UsernameBanning(t *testing.T) {
	bf := newTempBanFile(t)

	// Ban a username.
	require.NoError(t, bf.BanUsername("baduser"))
	assert.True(t, bf.IsUsernameBanned("baduser"))
	assert.False(t, bf.IsUsernameBanned("gooduser"))

	// Persist and reload.
	bf2 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf2.Load())
	assert.True(t, bf2.IsUsernameBanned("baduser"))

	// Unban.
	require.NoError(t, bf2.UnbanUsername("baduser"))
	assert.False(t, bf2.IsUsernameBanned("baduser"))

	// Verify unban persists.
	bf3 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf3.Load())
	assert.False(t, bf3.IsUsernameBanned("baduser"))
}

func TestBanFile_NicknameBanning(t *testing.T) {
	bf := newTempBanFile(t)

	// Ban a nickname.
	require.NoError(t, bf.BanNickname("troll"))
	assert.True(t, bf.IsNicknameBanned("troll"))
	assert.False(t, bf.IsNicknameBanned("friend"))

	// Persist and reload.
	bf2 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf2.Load())
	assert.True(t, bf2.IsNicknameBanned("troll"))

	// Unban.
	require.NoError(t, bf2.UnbanNickname("troll"))
	assert.False(t, bf2.IsNicknameBanned("troll"))

	// Verify unban persists.
	bf3 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf3.Load())
	assert.False(t, bf3.IsNicknameBanned("troll"))
}

func TestBanFile_UnbanIP(t *testing.T) {
	bf := newTempBanFile(t)

	require.NoError(t, bf.Add("10.0.0.1", nil))
	banned, _ := bf.IsBanned("10.0.0.1")
	assert.True(t, banned)

	require.NoError(t, bf.UnbanIP("10.0.0.1"))
	banned, _ = bf.IsBanned("10.0.0.1")
	assert.False(t, banned)

	// Verify unban persists.
	bf2 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf2.Load())
	banned, _ = bf2.IsBanned("10.0.0.1")
	assert.False(t, banned)
}

func TestBanFile_ListOperations(t *testing.T) {
	bf := newTempBanFile(t)

	require.NoError(t, bf.Add("1.2.3.4", nil))
	require.NoError(t, bf.Add("5.6.7.8", nil))
	require.NoError(t, bf.BanUsername("user1"))
	require.NoError(t, bf.BanUsername("user2"))
	require.NoError(t, bf.BanNickname("nick1"))

	ips, err := bf.ListBannedIPs()
	require.NoError(t, err)
	sort.Strings(ips)
	assert.Equal(t, []string{"1.2.3.4", "5.6.7.8"}, ips)

	usernames, err := bf.ListBannedUsernames()
	require.NoError(t, err)
	sort.Strings(usernames)
	assert.Equal(t, []string{"user1", "user2"}, usernames)

	nicknames, err := bf.ListBannedNicknames()
	require.NoError(t, err)
	assert.Equal(t, []string{"nick1"}, nicknames)
}

func TestBanFile_PermanentBanViaAdd(t *testing.T) {
	bf := newTempBanFile(t)

	require.NoError(t, bf.Add("172.16.0.1", nil))

	banned, until := bf.IsBanned("172.16.0.1")
	assert.True(t, banned)
	assert.Nil(t, until, "Add with nil should create a permanent ban")

	// Verify persistence.
	bf2 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf2.Load())
	banned, until = bf2.IsBanned("172.16.0.1")
	assert.True(t, banned)
	assert.Nil(t, until)
}

func TestBanFile_NewFormatPersistence(t *testing.T) {
	bf := newTempBanFile(t)

	expiry := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, bf.Add("10.0.0.1", nil))
	require.NoError(t, bf.Add("10.0.0.2", &expiry))
	require.NoError(t, bf.BanUsername("admin"))
	require.NoError(t, bf.BanNickname("spammer"))

	// Reload into a fresh BanFile.
	bf2 := &BanFile{filePath: bf.filePath}
	require.NoError(t, bf2.Load())

	// Verify IPs.
	banned, until := bf2.IsBanned("10.0.0.1")
	assert.True(t, banned)
	assert.Nil(t, until)

	banned, until = bf2.IsBanned("10.0.0.2")
	assert.True(t, banned)
	require.NotNil(t, until)
	assert.True(t, expiry.Equal(*until))

	// Verify username and nickname.
	assert.True(t, bf2.IsUsernameBanned("admin"))
	assert.True(t, bf2.IsNicknameBanned("spammer"))
}
