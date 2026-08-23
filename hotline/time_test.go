package hotline

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestNewTime_RoundTrip(t *testing.T) {
	// Truncate to seconds since the wire format has no sub-second precision.
	want := time.Date(2025, time.October, 27, 18, 11, 34, 0, time.Local)

	got := NewTime(want).Time()

	if !got.Equal(want) {
		t.Errorf("round trip = %v, want %v", got, want)
	}
}

func TestNewTime_YearField(t *testing.T) {
	tm := time.Date(2025, time.July, 10, 12, 0, 0, 0, time.Local)

	b := NewTime(tm)

	if year := binary.BigEndian.Uint16(b[0:2]); year != 2025 {
		t.Errorf("year field = %d, want 2025", year)
	}
}

// TestNewTime_SecondsSinceMacEpoch is the regression test for issue #166.
// Clients like Pitbull Pro ignore the year field and decode the 4-byte seconds
// field as a raw classic Mac OS timestamp (seconds since 1904-01-01). Encoding
// seconds-since-start-of-year instead made the value tiny, so those clients
// always rendered the year as 1904. Verify the seconds field decodes to the
// correct year when read as a Mac-epoch timestamp.
func TestNewTime_SecondsSinceMacEpoch(t *testing.T) {
	tm := time.Date(2025, time.July, 10, 12, 0, 0, 0, time.Local)

	b := NewTime(tm)
	seconds := binary.BigEndian.Uint32(b[4:8])

	decoded := macEpoch.Add(time.Duration(seconds) * time.Second)
	if decoded.Year() != 2025 {
		t.Errorf("Mac-epoch decode of seconds field = year %d, want 2025 (issue #166)", decoded.Year())
	}
	if !decoded.Equal(tm) {
		t.Errorf("Mac-epoch decode = %v, want %v", decoded, tm)
	}
}

func TestNewNewsTime_UsesSecondsSinceStartOfYear(t *testing.T) {
	want := time.Date(2026, time.August, 22, 18, 9, 55, 0, time.Local)

	encoded := NewNewsTime(want)
	seconds := binary.BigEndian.Uint32(encoded[4:8])
	if seconds >= 366*24*60*60 {
		t.Fatalf("news seconds field = %d, want a year-relative value", seconds)
	}
	if got := encoded.NewsTime(); !got.Equal(want) {
		t.Errorf("news time round trip = %v, want %v", got, want)
	}
}
