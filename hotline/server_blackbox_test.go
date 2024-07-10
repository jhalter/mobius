package hotline

import (
	"cmp"
	"encoding/binary"
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"slices"
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

	clean := slices.Concat(
		got[:92],
		make([]byte, 16),
		got[108:],
	)
	return assert.Equal(t, wantHexDump, hex.Dump(clean))
}

var tranSortFunc = func(a, b Transaction) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.clientID[:]),
		binary.BigEndian.Uint16(b.clientID[:]),
	)
}

// tranAssertEqual compares equality of transactions slices after stripping out the random transaction Type
func tranAssertEqual(t *testing.T, tran1, tran2 []Transaction) bool {
	var newT1 []Transaction
	var newT2 []Transaction

	for _, trans := range tran1 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []Field
		for _, field := range trans.Fields {
			if field.Type == FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []Field
		for _, field := range trans.Fields {
			if field.Type == FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT2 = append(newT2, trans)
	}

	slices.SortFunc(newT1, tranSortFunc)
	slices.SortFunc(newT2, tranSortFunc)

	return assert.Equal(t, newT1, newT2)
}
