package hotline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadFields(t *testing.T) {
	type args struct {
		paramCount []byte
		buf        []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []Field
		wantErr bool
	}{
		{
			name: "valid field data",
			args: args{
				paramCount: []byte{0x00, 0x02},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 2 bytes
					0x01, 0x02, 0x03, 0x04, // Data
					0x00, 0x66, // ID: fieldUserName
					0x00, 0x02, // Size: 2 bytes
					0x00, 0x01, // Data
				},
			},
			want: []Field{
				{
					ID:        []byte{0x00, 0x65},
					FieldSize: []byte{0x00, 0x04},
					Data:      []byte{0x01, 0x02, 0x03, 0x04},
				},
				{
					ID:        []byte{0x00, 0x66},
					FieldSize: []byte{0x00, 0x02},
					Data:      []byte{0x00, 0x01},
				},
			},
			wantErr: false,
		},
		{
			name: "empty bytes",
			args: args{
				paramCount: []byte{0x00, 0x00},
				buf:        []byte{},
			},
			want:    []Field(nil),
			wantErr: false,
		},
		{
			name: "when field size does not match data length",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 4 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field(nil),
			wantErr: true,
		},
		{
			name: "when field size of second field does not match data length",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x02, // Size: 2 bytes
					0x01, 0x02, // Data
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 4 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field(nil),
			wantErr: true,
		},
		{
			name: "when field data has extra bytes",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x02, // Size: 2 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field(nil),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFields(tt.args.paramCount, tt.args.buf)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !assert.Equal(t, tt.want, got) {
				t.Errorf("ReadFields() got = %v, want %v", got, tt.want)
			}
		})
	}
}
