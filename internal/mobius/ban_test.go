package mobius

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
				filePath: path.Join(cwd, "test", "config", "Banlist.yaml"),
				banList:  map[string]*time.Time{"192.168.86.29": &testTime},
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
