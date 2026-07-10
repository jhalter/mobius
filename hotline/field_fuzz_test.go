package hotline

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzFieldScanner verifies the field-framing split func never panics or advances past its input.
func FuzzFieldScanner(f *testing.F) {
	f.Add([]byte{0x00, 0x65, 0x00, 0x03, 0x68, 0x61, 0x69}) // FieldData "hai"
	f.Add([]byte{0x00, 0x65, 0xff, 0xff})                   // declared size larger than input
	f.Add([]byte{0x00})                                     // shorter than the size field

	f.Fuzz(func(t *testing.T, data []byte) {
		advance, token, err := FieldScanner(data, false)
		if err != nil {
			return
		}
		if advance == 0 {
			assert.Nil(t, token, "no advance must produce no token")
			return
		}
		require.LessOrEqual(t, advance, len(data), "scanner advanced past its input")
		require.Len(t, token, advance, "token length must match advance")
	})
}

// FuzzFieldWrite feeds raw untrusted bytes to the Field decoder. If decoding succeeds,
// re-encoding must reproduce exactly the bytes that were consumed.
func FuzzFieldWrite(f *testing.F) {
	f.Add([]byte{0x00, 0x65, 0x00, 0x03, 0x68, 0x61, 0x69}) // FieldData "hai"
	f.Add([]byte{0x00, 0x65, 0x00, 0x00})                   // empty data
	f.Add([]byte{0x00, 0x65, 0xff, 0xff, 0x00})             // declared size overruns buffer

	f.Fuzz(func(t *testing.T, data []byte) {
		var field Field
		n, err := field.Write(data)
		if err != nil {
			return
		}

		encoded, err := io.ReadAll(&field)
		require.NoError(t, err)
		assert.Equal(t, data[:n], encoded, "re-encoding a decoded field must reproduce the consumed bytes")
	})
}

// TestField_RoundTrip pins encode→decode symmetry for fields built with NewField.
func TestField_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		field Field
	}{
		{name: "with data", field: NewField(FieldData, []byte("hello"))},
		{name: "empty data", field: NewField(FieldUserPassword, []byte{})},
		{name: "binary data", field: NewField(FieldUserIconID, []byte{0x07, 0xd1})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := io.ReadAll(&tt.field)
			require.NoError(t, err)

			var decoded Field
			n, err := decoded.Write(encoded)
			require.NoError(t, err)
			assert.Equal(t, len(encoded), n)
			assert.Equal(t, tt.field.Type, decoded.Type)
			assert.Equal(t, tt.field.FieldSize, decoded.FieldSize)
			assert.Equal(t, tt.field.Data, decoded.Data)
		})
	}
}
