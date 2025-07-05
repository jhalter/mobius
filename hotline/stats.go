package hotline

import (
	"sync"
	"time"
)

// Stat counter keys
const (
	StatCurrentlyConnected = iota
	StatDownloadsInProgress
	StatUploadsInProgress
	StatWaitingDownloads
	StatConnectionPeak
	StatConnectionCounter
	StatDownloadCounter
	StatUploadCounter
)

type Counter interface {
	Increment(keys ...int)
	Decrement(key int)
	Set(key, val int)
	Get(key int) int
	Values() map[string]interface{}
}

type Stats struct {
	stats map[int]int
	since time.Time

	mu sync.RWMutex
}

func NewStats() *Stats {
	return &Stats{
		since: time.Now(),
		stats: map[int]int{
			StatCurrentlyConnected:  0,
			StatDownloadsInProgress: 0,
			StatUploadsInProgress:   0,
			StatWaitingDownloads:    0,
			StatConnectionPeak:      0,
			StatDownloadCounter:     0,
			StatUploadCounter:       0,
			StatConnectionCounter:   0,
		},
	}
}

func (s *Stats) Increment(keys ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		s.stats[key]++
	}
}

func (s *Stats) Decrement(key int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stats[key] > 0 {
		s.stats[key]--
	}
}

func (s *Stats) Set(key, val int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats[key] = val
}

func (s *Stats) Get(key int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.stats[key]
}

func (s *Stats) Values() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"CurrentlyConnected":  s.stats[StatCurrentlyConnected],
		"DownloadsInProgress": s.stats[StatDownloadsInProgress],
		"UploadsInProgress":   s.stats[StatUploadsInProgress],
		"WaitingDownloads":    s.stats[StatWaitingDownloads],
		"ConnectionPeak":      s.stats[StatConnectionPeak],
		"ConnectionCounter":   s.stats[StatConnectionCounter],
		"DownloadCounter":     s.stats[StatDownloadCounter],
		"UploadCounter":       s.stats[StatUploadCounter],
		"Since":               s.since,
	}
}
