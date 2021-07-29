package hotline

import (
	"reflect"
	"testing"
)

func TestNewFileHeader(t *testing.T) {
	type args struct {
		fileName string
		isDir    bool
	}
	tests := []struct {
		name string
		args args
		want FileHeader
	}{
		{
			name: "when path is file",
			args: args{
				fileName: "foo",
				isDir:    false,
			},
			want: FileHeader{
				Size:     []byte{0x00, 0x0a},
				Type:     []byte{0x00, 0x00},
				FilePath: EncodeFilePath("foo"),
			},
		},
		{
			name: "when path is dir",
			args: args{
				fileName: "foo",
				isDir:    true,
			},
			want: FileHeader{
				Size:     []byte{0x00, 0x0a},
				Type:     []byte{0x00, 0x01},
				FilePath: EncodeFilePath("foo"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewFileHeader(tt.args.fileName, tt.args.isDir); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewFileHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileHeader_Payload(t *testing.T) {
	type fields struct {
		Size     []byte
		Type     []byte
		FilePath []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{
			name: "has expected payload bytes",
			fields: fields{
				Size:     []byte{0x00, 0x0a},
				Type:     []byte{0x00, 0x00},
				FilePath: EncodeFilePath("foo"),
			},
			want: []byte{
				0x00, 0x0a, // total size
				0x00, 0x00, // type
				0x00, 0x01, // path item count
				0x00, 0x00, // path separator
				0x03,             // pathName len
				0x66, 0x6f, 0x6f, // "foo"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fh := &FileHeader{
				Size:     tt.fields.Size,
				Type:     tt.fields.Type,
				FilePath: tt.fields.FilePath,
			}
			if got := fh.Payload(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Payload() = %v, want %v", got, tt.want)
			}
		})
	}
}
