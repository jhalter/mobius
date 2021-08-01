package hotline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFileNameWithInfo_Read(t *testing.T) {
	type fields struct {
		Type       []byte
		Creator    []byte
		FileSize   []byte
		NameScript []byte
		NameSize   []byte
		Name       []byte
	}
	type args struct {
		p []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *FileNameWithInfo
		wantN   int
		wantErr bool
	}{
		{
			name:   "reads bytes into struct",
			fields: fields{},
			args: args{
				p: []byte{
					0x54, 0x45, 0x58, 0x54, // TEXT
					0x54, 0x54, 0x58, 0x54, // TTXT
					0x00, 0x43, 0x16, 0xd3, // File Size
					0x00, 0x00, 0x00, 0x00, // RSVD
					0x00, 0x00, // NameScript
					0x00, 0x0e, // Name Size
					0x41, 0x75, 0x64, 0x69, 0x6f, 0x6e, 0x2e, 0x61, 0x70, 0x70, 0x2e, 0x7a, 0x69, 0x70,
				},
			},
			want: &FileNameWithInfo{
				Type:       []byte("TEXT"),
				Creator:    []byte("TTXT"),
				FileSize:   []byte{0x00, 0x43, 0x16, 0xd3},
				RSVD:       []byte{0, 0, 0, 0},
				NameScript: []byte{0, 0},
				NameSize:   []byte{0x00, 0x0e},
				Name:       []byte("Audion.app.zip"),
			},
			wantN:   34,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FileNameWithInfo{
				Type:       tt.fields.Type,
				Creator:    tt.fields.Creator,
				FileSize:   tt.fields.FileSize,
				NameScript: tt.fields.NameScript,
				NameSize:   tt.fields.NameSize,
				Name:       tt.fields.Name,
			}
			gotN, err := f.Read(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("Read() gotN = %v, want %v", gotN, tt.wantN)
			}
			if !assert.Equal(t, tt.want, f) {
				t.Errorf("Read() got = %v, want %v", f, tt.want)

			}
		})
	}
}
