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
			gotAdvance, gotToken, err := FieldScanner(tt.args.data, tt.args.in1)
			if !tt.wantErr(t, err, fmt.Sprintf("FieldScanner(%v, %v)", tt.args.data, tt.args.in1)) {
				return
			}
			assert.Equalf(t, tt.wantAdvance, gotAdvance, "FieldScanner(%v, %v)", tt.args.data, tt.args.in1)
			assert.Equalf(t, tt.wantToken, gotToken, "FieldScanner(%v, %v)", tt.args.data, tt.args.in1)
		})
	}
}

func TestField_Read(t *testing.T) {
	type fields struct {
		ID         [2]byte
		FieldSize  [2]byte
		Data       []byte
		readOffset int
	}
	type args struct {
		p []byte
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      int
		wantErr   assert.ErrorAssertionFunc
		wantBytes []byte
	}{
		{
			name: "returns field bytes",
			fields: fields{
				ID:        [2]byte{0x00, 0x62},
				FieldSize: [2]byte{0x00, 0x03},
				Data:      []byte("hai!"),
			},
			args: args{
				p: make([]byte, 512),
			},
			want:    8,
			wantErr: assert.NoError,
			wantBytes: []byte{
				0x00, 0x62,
				0x00, 0x03,
				0x68, 0x61, 0x69, 0x21,
			},
		},
		{
			name: "returns field bytes from readOffset",
			fields: fields{
				ID:         [2]byte{0x00, 0x62},
				FieldSize:  [2]byte{0x00, 0x03},
				Data:       []byte("hai!"),
				readOffset: 4,
			},
			args: args{
				p: make([]byte, 512),
			},
			want:    4,
			wantErr: assert.NoError,
			wantBytes: []byte{
				0x68, 0x61, 0x69, 0x21,
			},
		},
		{
			name: "returns io.EOF when all bytes read",
			fields: fields{
				ID:         [2]byte{0x00, 0x62},
				FieldSize:  [2]byte{0x00, 0x03},
				Data:       []byte("hai!"),
				readOffset: 8,
			},
			args: args{
				p: make([]byte, 512),
			},
			want:      0,
			wantErr:   assert.Error,
			wantBytes: []byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Field{
				Type:       tt.fields.ID,
				FieldSize:  tt.fields.FieldSize,
				Data:       tt.fields.Data,
				readOffset: tt.fields.readOffset,
			}
			got, err := f.Read(tt.args.p)
			if !tt.wantErr(t, err, fmt.Sprintf("Read(%v)", tt.args.p)) {
				return
			}
			assert.Equalf(t, tt.want, got, "Read(%v)", tt.args.p)
			assert.Equalf(t, tt.wantBytes, tt.args.p[:got], "Read(%v)", tt.args.p)
		})
	}
}

func TestField_DecodeInt(t *testing.T) {
	type fields struct {
		Data []byte
	}
	tests := []struct {
		name    string
		fields  fields
		want    int
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "with 2 bytes of input",
			fields:  fields{Data: []byte{0, 1}},
			want:    1,
			wantErr: assert.NoError,
		},
		{
			name:    "with 4 bytes of input",
			fields:  fields{Data: []byte{0, 1, 0, 0}},
			want:    65536,
			wantErr: assert.NoError,
		},
		{
			name:    "with invalid number of bytes of input",
			fields:  fields{Data: []byte{1, 0, 0, 0, 0, 0, 0, 0}},
			want:    0,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Field{
				Data: tt.fields.Data,
			}
			got, err := f.DecodeInt()
			if !tt.wantErr(t, err) {
				return
			}
			assert.Equalf(t, tt.want, got, "DecodeInt()")
		})
	}
}
