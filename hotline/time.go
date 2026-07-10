package hotline

import (
	"encoding/binary"
	"slices"
	"time"
)

type Time [8]byte

// macEpoch is the classic Mac OS time epoch: seconds are counted from
// midnight, January 1, 1904 (local time). The 4-byte seconds field of the
// Hotline time format overflows a uint32 on 2040-02-06; the original protocol
// shares this limitation.
var macEpoch = time.Date(1904, time.January, 1, 0, 0, 0, 0, time.Local)

// NewTime converts a time.Time to the 8 byte Hotline time format:
// Year (2 bytes), milliseconds (2 bytes) and seconds (4 bytes).
//
// The seconds field holds seconds since the classic Mac OS epoch
// (1904-01-01), matching the original Hotline server. Some clients (e.g.
// Pitbull Pro) ignore the year field and decode the seconds field as a raw
// Mac timestamp; encoding seconds-since-start-of-year instead made those
// clients always display the year 1904 (see issue #166).
func NewTime(t time.Time) (b Time) {
	yearBytes := make([]byte, 2)
	secondBytes := make([]byte, 4)

	binary.BigEndian.PutUint16(yearBytes, uint16(t.Year()))
	binary.BigEndian.PutUint32(secondBytes, uint32(t.Sub(macEpoch).Seconds()))

	return [8]byte(slices.Concat(
		yearBytes,
		[]byte{0, 0},
		secondBytes,
	))
}

// Time converts the Hotline Time format to a Go time.Time.
// The Hotline format stores: Year (2 bytes) + milliseconds (2 bytes, unused) +
// seconds since the Mac OS epoch (4 bytes).
func (t Time) Time() time.Time {
	seconds := binary.BigEndian.Uint32(t[4:8])

	return macEpoch.Add(time.Duration(seconds) * time.Second)
}

// Format returns the time formatted according to the layout string.
// This is a convenience wrapper around Time().Format().
func (t Time) Format(layout string) string {
	return t.Time().Format(layout)
}
