package hotline

import (
	"sync"
	"time"
)

// StatKey identifies a single stat counter.
type StatKey int

// Stat counter keys. numStats must remain last; it sizes the counter array.
const (
	StatCurrentlyConnected StatKey = iota
	StatDownloadsInProgress
	StatUploadsInProgress
	StatWaitingDownloads
	StatConnectionPeak
	StatConnectionCounter
	StatDownloadCounter
	StatUploadCounter

	numStats
)

type Counter interface {
	Increment(keys ...StatKey)
	Decrement(keys ...StatKey)
	Set(key StatKey, val int)
	Max(key StatKey, val int)
	Get(key StatKey) int
	Values() StatValues
}

// StatValues is a point-in-time snapshot of all counters. Its JSON tags define
// the wire format served at GET /api/v1/stats.
type StatValues struct {
	CurrentlyConnected  int       `json:"CurrentlyConnected"`
	DownloadsInProgress int       `json:"DownloadsInProgress"`
	UploadsInProgress   int       `json:"UploadsInProgress"`
	WaitingDownloads    int       `json:"WaitingDownloads"`
	ConnectionPeak      int       `json:"ConnectionPeak"`
	ConnectionCounter   int       `json:"ConnectionCounter"`
	DownloadCounter     int       `json:"DownloadCounter"`
	UploadCounter       int       `json:"UploadCounter"`
	Since               time.Time `json:"Since"`
}

type Stats struct {
	stats [numStats]int
	since time.Time

	mu sync.RWMutex
}

func NewStats() *Stats {
	return &Stats{since: time.Now()}
}

func (s *Stats) Increment(keys ...StatKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		s.stats[key]++
	}
}

func (s *Stats) Decrement(keys ...StatKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		if s.stats[key] > 0 {
			s.stats[key]--
		}
	}
}

func (s *Stats) Set(key StatKey, val int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats[key] = val
}

// Max sets key to val only if val is greater than the current value. The
// compare-and-set happens under a single lock so concurrent callers cannot
// clobber each other's update.
func (s *Stats) Max(key StatKey, val int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if val > s.stats[key] {
		s.stats[key] = val
	}
}

func (s *Stats) Get(key StatKey) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.stats[key]
}

func (s *Stats) Values() StatValues {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StatValues{
		CurrentlyConnected:  s.stats[StatCurrentlyConnected],
		DownloadsInProgress: s.stats[StatDownloadsInProgress],
		UploadsInProgress:   s.stats[StatUploadsInProgress],
		WaitingDownloads:    s.stats[StatWaitingDownloads],
		ConnectionPeak:      s.stats[StatConnectionPeak],
		ConnectionCounter:   s.stats[StatConnectionCounter],
		DownloadCounter:     s.stats[StatDownloadCounter],
		UploadCounter:       s.stats[StatUploadCounter],
		Since:               s.since,
	}
}
