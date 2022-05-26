package hotline

import (
	"encoding/binary"
	"time"
)

// toHotlineTime converts a time.Time to the 8 byte Hotline time format:
// Year (2 bytes), milliseconds (2 bytes) and seconds (4 bytes)
func toHotlineTime(t time.Time) (b []byte) {
	yearBytes := make([]byte, 2)
	secondBytes := make([]byte, 4)

	// Get a time.Time for January 1st 00:00 from t so we can calculate the difference in seconds from t
	startOfYear := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.Local)

	binary.BigEndian.PutUint16(yearBytes, uint16(t.Year()))
	binary.BigEndian.PutUint32(secondBytes, uint32(t.Sub(startOfYear).Seconds()))

	b = append(b, yearBytes...)
	b = append(b, []byte{0, 0}...)
	b = append(b, secondBytes...)

	return b
}
