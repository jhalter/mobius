package hotline

import (
	"github.com/stretchr/testify/assert"
	"io"
	"reflect"
	"testing"
)

func TestFileNameWithInfo_MarshalBinary(t *testing.T) {
	type fields struct {
		fileNameWithInfoHeader fileNameWithInfoHeader
		name                   []byte
	}
	tests := []struct {
		name     string
		fields   fields
		wantData []byte
		wantErr  bool
	}{
		{
			name: "returns expected bytes",
			fields: fields{
				fileNameWithInfoHeader: fileNameWithInfoHeader{
					Type:       [4]byte{0x54, 0x45, 0x58, 0x54}, // TEXT
					Creator:    [4]byte{0x54, 0x54, 0x58, 0x54}, // TTXT
					FileSize:   [4]byte{0x00, 0x43, 0x16, 0xd3}, // File Size
					RSVD:       [4]byte{0, 0, 0, 0},
					NameScript: [2]byte{0, 0},
					NameSize:   [2]byte{0x00, 0x03},
				},
				name: []byte("foo"),
			},
			wantData: []byte{
				0x54, 0x45, 0x58, 0x54,
				0x54, 0x54, 0x58, 0x54,
				0x00, 0x43, 0x16, 0xd3,
				0, 0, 0, 0,
				0, 0,
				0x00, 0x03,
				0x66, 0x6f, 0x6f,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FileNameWithInfo{
				fileNameWithInfoHeader: tt.fields.fileNameWithInfoHeader,
				Name:                   tt.fields.name,
			}
			gotData, err := io.ReadAll(f)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotData, tt.wantData) {
				t.Errorf("MarshalBinary() gotData = %v, want %v", gotData, tt.wantData)
			}
		})
	}
}

func TestFileNameWithInfo_UnmarshalBinary(t *testing.T) {
	type fields struct {
		fileNameWithInfoHeader fileNameWithInfoHeader
		name                   []byte
	}
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *FileNameWithInfo
		wantErr bool
	}{
		{
			name: "writes bytes into struct",
			args: args{
				data: []byte{
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
				fileNameWithInfoHeader: fileNameWithInfoHeader{
					Type:       [4]byte{0x54, 0x45, 0x58, 0x54}, // TEXT
					Creator:    [4]byte{0x54, 0x54, 0x58, 0x54}, // TTXT
					FileSize:   [4]byte{0x00, 0x43, 0x16, 0xd3}, // File Size
					RSVD:       [4]byte{0, 0, 0, 0},
					NameScript: [2]byte{0, 0},
					NameSize:   [2]byte{0x00, 0x0e},
				},
				Name: []byte("Audion.app.zip"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FileNameWithInfo{
				fileNameWithInfoHeader: tt.fields.fileNameWithInfoHeader,
				Name:                   tt.fields.name,
			}
			if _, err := f.Write(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !assert.Equal(t, tt.want, f) {
				t.Errorf("Read() got = %v, want %v", f, tt.want)
			}
		})
	}
}
