package hotline

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleTransactionBytes is a valid TranChatSend transaction with one FieldData field ("hai"),
// lifted from TestTransaction_Write.
var sampleTransactionBytes = []byte{
	0x00, 0x00, 0x00, 0x69, 0x00, 0x00, 0x15, 0x72,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09,
	0x00, 0x00, 0x00, 0x09, 0x00, 0x01, 0x00, 0x65,
	0x00, 0x03, 0x68, 0x61, 0x69,
}

// FuzzTransactionScanner verifies that the split func used to frame transactions from the
// network never panics, never advances past its input, and only produces tokens that the
// Transaction decoder can consume without panicking.
func FuzzTransactionScanner(f *testing.F) {
	f.Add(sampleTransactionBytes)
	f.Add(sampleTransactionBytes[:16])                       // header only, no fields
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})                    // shorter than the size field
	f.Add(bytes.Repeat([]byte{0xff}, 64))                    // absurd declared size
	f.Add(append(sampleTransactionBytes, 0xde, 0xad, 0xbe)) // trailing partial transaction

	f.Fuzz(func(t *testing.T, data []byte) {
		advance, token, err := transactionScanner(data, false)
		if err != nil {
			return
		}
		if advance == 0 {
			assert.Nil(t, token, "no advance must produce no token")
			return
		}
		require.LessOrEqual(t, advance, len(data), "scanner advanced past its input")
		require.Len(t, token, advance, "token length must match advance")
		require.GreaterOrEqual(t, advance, tranHeaderLen, "token cannot be smaller than a transaction header")

		// A framed token must be decodable without panicking (errors are fine).
		_, _ = (&Transaction{}).Write(token)
	})
}

// FuzzTransactionWrite feeds raw untrusted bytes to the Transaction decoder. The size and
// param-count fields come straight off the network, so decoding must error rather than panic.
func FuzzTransactionWrite(f *testing.F) {
	f.Add(sampleTransactionBytes)
	f.Add(sampleTransactionBytes[:22])
	// Declared total size smaller than the minimum (previously panicked with tranLen < 22).
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x69, 0x00, 0x00, 0x15, 0x72,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	})
	// Declared total size larger than the buffer (previously panicked with tranLen > len(p)).
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x69, 0x00, 0x00, 0x15, 0x72,
		0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x00, 0x09, 0x00, 0x01,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		var tran Transaction
		if _, err := tran.Write(data); err != nil {
			return
		}

		// A successfully decoded transaction must re-encode and decode back to the same value.
		encoded, err := io.ReadAll(&tran)
		require.NoError(t, err)

		var reDecoded Transaction
		_, err = reDecoded.Write(encoded)
		require.NoError(t, err, "re-encoded transaction failed to decode")
		assert.Equal(t, tran.Type, reDecoded.Type)
		assert.Equal(t, tran.Fields, reDecoded.Fields)
	})
}

// TestTransaction_RoundTrip pins encode→decode symmetry for representative transactions.
func TestTransaction_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		tran Transaction
	}{
		{
			name: "no fields",
			tran: Transaction{
				Type: TranKeepAlive,
				ID:   [4]byte{0, 0, 0, 1},
			},
		},
		{
			name: "single field",
			tran: Transaction{
				Type:   TranChatSend,
				ID:     [4]byte{0, 0, 0, 2},
				Fields: []Field{NewField(FieldData, []byte("hello world"))},
			},
		},
		{
			name: "multiple fields including empty data",
			tran: Transaction{
				IsReply:   1,
				ErrorCode: [4]byte{0, 0, 0, 1},
				Type:      TranLogin,
				ID:        [4]byte{0, 0, 0, 3},
				Fields: []Field{
					NewField(FieldUserLogin, []byte("guest")),
					NewField(FieldUserPassword, []byte{}),
					NewField(FieldUserIconID, []byte{0x07, 0xd1}),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := io.ReadAll(&tt.tran)
			require.NoError(t, err)

			var decoded Transaction
			n, err := decoded.Write(encoded)
			require.NoError(t, err)
			assert.Equal(t, len(encoded), n)

			assert.Equal(t, tt.tran.Flags, decoded.Flags)
			assert.Equal(t, tt.tran.IsReply, decoded.IsReply)
			assert.Equal(t, tt.tran.Type, decoded.Type)
			assert.Equal(t, tt.tran.ID, decoded.ID)
			assert.Equal(t, tt.tran.ErrorCode, decoded.ErrorCode)
			if len(tt.tran.Fields) == 0 {
				assert.Empty(t, decoded.Fields)
			} else {
				assert.Equal(t, tt.tran.Fields, decoded.Fields)
			}

			// Encoding the decoded transaction must reproduce the original bytes.
			reEncoded, err := io.ReadAll(&decoded)
			require.NoError(t, err)
			assert.Equal(t, encoded, reEncoded)
		})
	}
}
