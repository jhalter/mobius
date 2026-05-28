package hotline

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStats_Increment(t *testing.T) {
	tests := []struct {
		name     string
		keys     []StatKey
		expected map[StatKey]int
	}{
		{
			name: "single key increment",
			keys: []StatKey{StatCurrentlyConnected},
			expected: map[StatKey]int{
				StatCurrentlyConnected: 1,
			},
		},
		{
			name: "multiple keys increment",
			keys: []StatKey{StatCurrentlyConnected, StatDownloadCounter, StatUploadCounter},
			expected: map[StatKey]int{
				StatCurrentlyConnected: 1,
				StatDownloadCounter:    1,
				StatUploadCounter:      1,
			},
		},
		{
			name: "duplicate keys increment",
			keys: []StatKey{StatCurrentlyConnected, StatCurrentlyConnected},
			expected: map[StatKey]int{
				StatCurrentlyConnected: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStats()
			stats.Increment(tt.keys...)

			for key, expectedVal := range tt.expected {
				assert.Equal(t, expectedVal, stats.Get(key))
			}
		})
	}
}

func TestStats_Increment_Multiple_Calls(t *testing.T) {
	stats := NewStats()

	stats.Increment(StatCurrentlyConnected)
	assert.Equal(t, 1, stats.Get(StatCurrentlyConnected))

	stats.Increment(StatCurrentlyConnected)
	assert.Equal(t, 2, stats.Get(StatCurrentlyConnected))

	stats.Increment(StatCurrentlyConnected, StatDownloadCounter)
	assert.Equal(t, 3, stats.Get(StatCurrentlyConnected))
	assert.Equal(t, 1, stats.Get(StatDownloadCounter))
}

func TestStats_Decrement(t *testing.T) {
	tests := []struct {
		name       string
		setupValue int
		key        StatKey
		expected   int
	}{
		{
			name:       "decrement from positive value",
			setupValue: 5,
			key:        StatCurrentlyConnected,
			expected:   4,
		},
		{
			name:       "decrement from zero stays zero",
			setupValue: 0,
			key:        StatCurrentlyConnected,
			expected:   0,
		},
		{
			name:       "decrement from one",
			setupValue: 1,
			key:        StatCurrentlyConnected,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStats()
			stats.Set(tt.key, tt.setupValue)

			stats.Decrement(tt.key)

			assert.Equal(t, tt.expected, stats.Get(tt.key))
		})
	}
}

func TestStats_Decrement_Multiple_Calls(t *testing.T) {
	stats := NewStats()

	stats.Set(StatCurrentlyConnected, 10)
	assert.Equal(t, 10, stats.Get(StatCurrentlyConnected))

	stats.Decrement(StatCurrentlyConnected)
	assert.Equal(t, 9, stats.Get(StatCurrentlyConnected))

	stats.Decrement(StatCurrentlyConnected)
	assert.Equal(t, 8, stats.Get(StatCurrentlyConnected))
}

func TestStats_Set(t *testing.T) {
	tests := []struct {
		name     string
		key      StatKey
		value    int
		expected int
	}{
		{
			name:     "set positive value",
			key:      StatCurrentlyConnected,
			value:    42,
			expected: 42,
		},
		{
			name:     "set zero value",
			key:      StatDownloadCounter,
			value:    0,
			expected: 0,
		},
		{
			name:     "set negative value",
			key:      StatUploadCounter,
			value:    -5,
			expected: -5,
		},
		{
			name:     "overwrite existing value",
			key:      StatConnectionPeak,
			value:    100,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStats()

			if tt.name == "overwrite existing value" {
				stats.Set(tt.key, 50)
			}

			stats.Set(tt.key, tt.value)

			assert.Equal(t, tt.expected, stats.Get(tt.key))
		})
	}
}

func TestStats_Get(t *testing.T) {
	tests := []struct {
		name     string
		key      StatKey
		setValue int
		expected int
	}{
		{
			name:     "get initialized value",
			key:      StatCurrentlyConnected,
			setValue: 0,
			expected: 0,
		},
		{
			name:     "get set value",
			key:      StatDownloadCounter,
			setValue: 25,
			expected: 25,
		},
		{
			name:     "get after increment",
			key:      StatUploadCounter,
			setValue: -1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStats()

			if tt.name == "get after increment" {
				stats.Increment(tt.key)
			} else {
				stats.Set(tt.key, tt.setValue)
			}

			result := stats.Get(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStats_Get_Default_Values(t *testing.T) {
	stats := NewStats()

	expectedDefaults := map[StatKey]int{
		StatCurrentlyConnected:  0,
		StatDownloadsInProgress: 0,
		StatUploadsInProgress:   0,
		StatWaitingDownloads:    0,
		StatConnectionPeak:      0,
		StatDownloadCounter:     0,
		StatUploadCounter:       0,
		StatConnectionCounter:   0,
	}

	for key, expected := range expectedDefaults {
		assert.Equal(t, expected, stats.Get(key))
	}
}

func TestStats_Values(t *testing.T) {
	stats := NewStats()

	// Test default values
	values := stats.Values()

	assert.Equal(t, 0, values.CurrentlyConnected)
	assert.Equal(t, 0, values.DownloadsInProgress)
	assert.Equal(t, 0, values.UploadsInProgress)
	assert.Equal(t, 0, values.WaitingDownloads)
	assert.Equal(t, 0, values.ConnectionPeak)
	assert.Equal(t, 0, values.ConnectionCounter)
	assert.Equal(t, 0, values.DownloadCounter)
	assert.Equal(t, 0, values.UploadCounter)
	assert.False(t, values.Since.IsZero(), "Since should be set")
}

func TestStats_Values_WithModifiedStats(t *testing.T) {
	stats := NewStats()

	// Modify some stats
	stats.Set(StatCurrentlyConnected, 10)
	stats.Set(StatDownloadsInProgress, 5)
	stats.Increment(StatConnectionCounter)
	stats.Increment(StatDownloadCounter, StatUploadCounter)

	values := stats.Values()

	assert.Equal(t, 10, values.CurrentlyConnected)
	assert.Equal(t, 5, values.DownloadsInProgress)
	assert.Equal(t, 0, values.UploadsInProgress)
	assert.Equal(t, 0, values.WaitingDownloads)
	assert.Equal(t, 0, values.ConnectionPeak)
	assert.Equal(t, 1, values.ConnectionCounter)
	assert.Equal(t, 1, values.DownloadCounter)
	assert.Equal(t, 1, values.UploadCounter)
}

// TestStats_Values_JSONKeys locks the JSON wire format served at /api/v1/stats.
func TestStats_Values_JSONKeys(t *testing.T) {
	b, err := json.Marshal(NewStats().Values())
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &decoded))

	expectedKeys := []string{
		"CurrentlyConnected",
		"DownloadsInProgress",
		"UploadsInProgress",
		"WaitingDownloads",
		"ConnectionPeak",
		"ConnectionCounter",
		"DownloadCounter",
		"UploadCounter",
		"Since",
	}

	for _, key := range expectedKeys {
		_, exists := decoded[key]
		assert.True(t, exists, "Key %s should exist in JSON output", key)
	}

	assert.Equal(t, len(expectedKeys), len(decoded))
}

func TestStats_Max(t *testing.T) {
	stats := NewStats()

	stats.Max(StatConnectionPeak, 5)
	assert.Equal(t, 5, stats.Get(StatConnectionPeak), "raises to a larger value")

	stats.Max(StatConnectionPeak, 3)
	assert.Equal(t, 5, stats.Get(StatConnectionPeak), "no-op for a smaller value")

	stats.Max(StatConnectionPeak, 5)
	assert.Equal(t, 5, stats.Get(StatConnectionPeak), "no-op for an equal value")

	stats.Max(StatConnectionPeak, 10)
	assert.Equal(t, 10, stats.Get(StatConnectionPeak), "raises again")
}

// TestStats_Concurrent exercises the counter from many goroutines so the race
// detector can catch unsynchronized access (run with -race).
func TestStats_Concurrent(t *testing.T) {
	stats := NewStats()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			stats.Increment(StatConnectionCounter)
			stats.Max(StatConnectionPeak, n)
			stats.Set(StatCurrentlyConnected, n)
			_ = stats.Get(StatCurrentlyConnected)
			_ = stats.Values()
			stats.Decrement(StatConnectionCounter)
		}(i)
	}

	wg.Wait()

	assert.Equal(t, 0, stats.Get(StatConnectionCounter), "increments and decrements balance out")
	assert.Equal(t, goroutines-1, stats.Get(StatConnectionPeak), "peak captures the largest value")
}
