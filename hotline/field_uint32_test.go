package hotline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldDecodeUint32(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		want    uint32
		wantErr bool
	}{
		{"absent", nil, 0, true},
		{"empty", []byte{}, 0, true},
		{"one byte", []byte{1}, 0, true},
		{"three bytes", []byte{0, 0, 1}, 0, true},
		{"five bytes", []byte{0, 0, 0, 0, 1}, 0, true},
		{"zero", []byte{0, 0}, 0, false},
		{"uint16", []byte{0xff, 0xff}, 65535, false},
		{"uint32 small", []byte{0, 0, 0, 42}, 42, false},
		{"2 GiB", []byte{0x80, 0, 0, 0}, 1 << 31, false},
		{"maximum uint32", []byte{0xff, 0xff, 0xff, 0xff}, 1<<32 - 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Field{Data: tc.data}).DecodeUint32()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
