package hotline

import (
	"encoding/binary"
	"slices"
	"time"
)

type Time [8]byte

// NewTime converts a time.Time to the 8 byte Hotline time format:
// Year (2 bytes), milliseconds (2 bytes) and seconds (4 bytes)
func NewTime(t time.Time) (b Time) {
	yearBytes := make([]byte, 2)
	secondBytes := make([]byte, 4)

	// Get a time.Time for January 1st 00:00 from t so we can calculate the difference in seconds from t
	startOfYear := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.Local)

	binary.BigEndian.PutUint16(yearBytes, uint16(t.Year()))
	binary.BigEndian.PutUint32(secondBytes, uint32(t.Sub(startOfYear).Seconds()))

	return [8]byte(slices.Concat(
		yearBytes,
		[]byte{0, 0},
		secondBytes,
	))
}
