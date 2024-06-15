package hotline

import (
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"testing"
)

func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

// assertTransferBytesEqual takes a string with a hexdump in the same format that `hexdump -C` produces and compares with
// a hexdump for the bytes in got, after stripping the create/modify timestamps.
// I don't love this, but as git does not  preserve file create/modify timestamps, we either need to fully mock the
// filesystem interactions or work around in this way.
// TODO: figure out a better solution
func assertTransferBytesEqual(t *testing.T, wantHexDump string, got []byte) bool {
	if wantHexDump == "" {
		return true
	}

	var clean []byte
	clean = append(clean, got[:92]...)         // keep the first 92 bytes
	clean = append(clean, make([]byte, 16)...) // replace the next 16 bytes for create/modify timestamps
	clean = append(clean, got[108:]...)        // keep the rest

	return assert.Equal(t, wantHexDump, hex.Dump(clean))
}

// tranAssertEqual compares equality of transactions slices after stripping out the random ID
func tranAssertEqual(t *testing.T, tran1, tran2 []Transaction) bool {
	var newT1 []Transaction
	var newT2 []Transaction

	for _, trans := range tran1 {
		trans.ID = []byte{0, 0, 0, 0}
		var fs []Field
		for _, field := range trans.Fields {
			if field.ID == [2]byte{0x00, 0x6b} {
				continue
			}
			fs = append(fs, field)
		}
		trans.Fields = fs
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = []byte{0, 0, 0, 0}
		var fs []Field
		for _, field := range trans.Fields {
			if field.ID == [2]byte{0x00, 0x6b} {
				continue
			}
			fs = append(fs, field)
		}
		trans.Fields = fs
		newT2 = append(newT2, trans)
	}

	return assert.Equal(t, newT1, newT2)
}
