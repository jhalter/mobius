package hotline

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestStats_Increment(t *testing.T) {
	tests := []struct {
		name     string
		keys     []int
		expected map[int]int
	}{
		{
			name: "single key increment",
			keys: []int{StatCurrentlyConnected},
			expected: map[int]int{
				StatCurrentlyConnected: 1,
			},
		},
		{
			name: "multiple keys increment",
			keys: []int{StatCurrentlyConnected, StatDownloadCounter, StatUploadCounter},
			expected: map[int]int{
				StatCurrentlyConnected: 1,
				StatDownloadCounter:    1,
				StatUploadCounter:      1,
			},
		},
		{
			name: "duplicate keys increment",
			keys: []int{StatCurrentlyConnected, StatCurrentlyConnected},
			expected: map[int]int{
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
		name        string
		setupValue  int
		key         int
		expected    int
	}{
		{
			name:        "decrement from positive value",
			setupValue:  5,
			key:         StatCurrentlyConnected,
			expected:    4,
		},
		{
			name:        "decrement from zero stays zero",
			setupValue:  0,
			key:         StatCurrentlyConnected,
			expected:    0,
		},
		{
			name:        "decrement from one",
			setupValue:  1,
			key:         StatCurrentlyConnected,
			expected:    0,
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
		key      int
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
		key      int
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
	
	expectedDefaults := map[int]int{
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
	
	assert.Equal(t, 0, values["CurrentlyConnected"])
	assert.Equal(t, 0, values["DownloadsInProgress"])
	assert.Equal(t, 0, values["UploadsInProgress"])
	assert.Equal(t, 0, values["WaitingDownloads"])
	assert.Equal(t, 0, values["ConnectionPeak"])
	assert.Equal(t, 0, values["ConnectionCounter"])
	assert.Equal(t, 0, values["DownloadCounter"])
	assert.Equal(t, 0, values["UploadCounter"])
	assert.NotNil(t, values["Since"])
	
	// Verify Since is a time.Time
	_, ok := values["Since"].(time.Time)
	assert.True(t, ok, "Since should be a time.Time")
}

func TestStats_Values_WithModifiedStats(t *testing.T) {
	stats := NewStats()
	
	// Modify some stats
	stats.Set(StatCurrentlyConnected, 10)
	stats.Set(StatDownloadsInProgress, 5)
	stats.Increment(StatConnectionCounter)
	stats.Increment(StatDownloadCounter, StatUploadCounter)
	
	values := stats.Values()
	
	assert.Equal(t, 10, values["CurrentlyConnected"])
	assert.Equal(t, 5, values["DownloadsInProgress"])
	assert.Equal(t, 0, values["UploadsInProgress"])
	assert.Equal(t, 0, values["WaitingDownloads"])
	assert.Equal(t, 0, values["ConnectionPeak"])
	assert.Equal(t, 1, values["ConnectionCounter"])
	assert.Equal(t, 1, values["DownloadCounter"])
	assert.Equal(t, 1, values["UploadCounter"])
}

func TestStats_Values_ContainsAllKeys(t *testing.T) {
	stats := NewStats()
	values := stats.Values()
	
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
		_, exists := values[key]
		assert.True(t, exists, "Key %s should exist in Values() output", key)
	}
	
	// Should have exactly 9 keys
	assert.Equal(t, 9, len(values))
}