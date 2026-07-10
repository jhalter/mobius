package hotline

import (
	"cmp"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

// flatFileTimestampOffset is the byte offset of the information fork's CreateDate within a
// flattened file object stream: FlatFileHeader (24) + INFO FlatFileForkHeader (16) + the info
// fork's fixed fields up to CreateDate (Platform+TypeSignature+CreatorSignature+Flags+
// PlatformFlags = 20, then RSVD = 32). CreateDate and ModifyDate are 8 bytes each and adjacent.
const (
	flatFileTimestampOffset = 24 + 16 + 20 + 32
	flatFileTimestampLen    = 16 // CreateDate (8) + ModifyDate (8)
)

// assertTransferBytesEqual takes a string with a hexdump in the same format that `hexdump -C`
// produces and compares with a hexdump for the bytes in got, after zeroing the info fork's
// create/modify timestamps. Git does not preserve file create/modify times, so those bytes vary
// between checkouts; the offset is derived structurally from the flattened file object layout
// rather than hardcoded (see flatFileTimestampOffset).
func assertTransferBytesEqual(t *testing.T, wantHexDump string, got []byte) bool {
	if wantHexDump == "" {
		return true
	}

	clean := slices.Clone(got)
	if len(clean) >= flatFileTimestampOffset+flatFileTimestampLen {
		for i := flatFileTimestampOffset; i < flatFileTimestampOffset+flatFileTimestampLen; i++ {
			clean[i] = 0
		}
	}
	return assert.Equal(t, wantHexDump, hex.Dump(clean))
}

var tranSortFunc = func(a, b Transaction) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ClientID[:]),
		binary.BigEndian.Uint16(b.ClientID[:]),
	)
}

// TranAssertEqual compares equality of transactions slices after stripping out the random transaction Type
func TranAssertEqual(t *testing.T, tran1, tran2 []Transaction) bool {
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
