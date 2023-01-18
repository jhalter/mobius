package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_byteToInt(t *testing.T) {
	type args struct {
		bytes []byte
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "with 2 bytes of input",
			args:    args{bytes: []byte{0, 1}},
			want:    1,
			wantErr: assert.NoError,
		},
		{
			name:    "with 4 bytes of input",
			args:    args{bytes: []byte{0, 1, 0, 0}},
			want:    65536,
			wantErr: assert.NoError,
		},
		{
			name:    "with invalid number of bytes of input",
			args:    args{bytes: []byte{1, 0, 0, 0, 0, 0, 0, 0}},
			want:    0,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := byteToInt(tt.args.bytes)
			if !tt.wantErr(t, err, fmt.Sprintf("byteToInt(%v)", tt.args.bytes)) {
				return
			}
			assert.Equalf(t, tt.want, got, "byteToInt(%v)", tt.args.bytes)
		})
	}
}
