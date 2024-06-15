package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHello(t *testing.T) {

}

func Test_fieldScanner(t *testing.T) {
	type args struct {
		data []byte
		in1  bool
	}
	tests := []struct {
		name        string
		args        args
		wantAdvance int
		wantToken   []byte
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "when too few bytes are provided to read the field size",
			args: args{
				data: []byte{},
				in1:  false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
		{
			name: "when too few bytes are provided to read the full payload",
			args: args{
				data: []byte{
					0, 1,
					0, 4,
					0, 0,
				},
				in1: false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
		{
			name: "when a full field is provided",
			args: args{
				data: []byte{
					0, 1,
					0, 4,
					0, 0,
					0, 0,
				},
				in1: false,
			},
			wantAdvance: 8,
			wantToken: []byte{
				0, 1,
				0, 4,
				0, 0,
				0, 0,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when a full field plus extra bytes are provided",
			args: args{
				data: []byte{
					0, 1,
					0, 4,
					0, 0,
					0, 0,
					1, 1,
				},
				in1: false,
			},
			wantAdvance: 8,
			wantToken: []byte{
				0, 1,
				0, 4,
				0, 0,
				0, 0,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := fieldScanner(tt.args.data, tt.args.in1)
			if !tt.wantErr(t, err, fmt.Sprintf("fieldScanner(%v, %v)", tt.args.data, tt.args.in1)) {
				return
			}
			assert.Equalf(t, tt.wantAdvance, gotAdvance, "fieldScanner(%v, %v)", tt.args.data, tt.args.in1)
			assert.Equalf(t, tt.wantToken, gotToken, "fieldScanner(%v, %v)", tt.args.data, tt.args.in1)
		})
	}
}
