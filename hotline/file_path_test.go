package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFilePath_Write(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		want    FilePath
		wantErr bool
	}{
		{
			name: "unmarshals bytes into struct",
			args: args{b: []byte{
				0x00, 0x02,
				0x00, 0x00,
				0x0f,
				0x46, 0x69, 0x72, 0x73, 0x74, 0x20, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x20, 0x44, 0x69, 0x72,
				0x00, 0x00,
				0x08,
				0x41, 0x20, 0x53, 0x75, 0x62, 0x44, 0x69, 0x72,
			}},
			want: FilePath{
				ItemCount: [2]byte{0x00, 0x02},
				Items: []FilePathItem{
					{
						Len:  0x0f,
						Name: []byte("First Level Dir"),
					},
					{
						Len:  0x08,
						Name: []byte("A SubDir"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handles empty data payload",
			args: args{b: []byte{
				0x00, 0x00,
			}},
			want: FilePath{
				ItemCount: [2]byte{0x00, 0x00},
				Items:     []FilePathItem(nil),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fp FilePath
			if _, err := fp.Write(tt.args.b); (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !assert.Equal(t, tt.want, fp) {
				t.Errorf("Read() got = %v, want %v", fp, tt.want)
			}
		})
	}
}

func Test_readPath(t *testing.T) {
	type args struct {
		fileRoot string
		filePath []byte
		fileName []byte
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "when filePath is invalid",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: []byte{
					0x61,
				},
				fileName: []byte{
					0x61, 0x61, 0x61,
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "when filePath is nil",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: nil,
				fileName: []byte("foo"),
			},
			want: "/usr/local/var/mobius/Files/foo",
		},
		{
			name: "when fileName contains .. ",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: nil,
				fileName: []byte("../../../foo"),
			},
			want: "/usr/local/var/mobius/Files/foo",
		},
		{
			name: "when filePath contains .. ",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: []byte{
					0x00, 0x02,
					0x00, 0x00,
					0x03,
					0x2e, 0x2e, 0x2f,
					0x00, 0x00,
					0x08,
					0x41, 0x20, 0x53, 0x75, 0x62, 0x44, 0x69, 0x72,
				},
				fileName: []byte("foo"),
			},
			want: "/usr/local/var/mobius/Files/A SubDir/foo",
		},
		{
			name: "when a filePath entry contains .. ",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: []byte{
					0x00, 0x01,
					0x00, 0x00,
					0x0b,
					0x2e, 0x2e, 0x2f, 0x41, 0x20, 0x53, 0x75, 0x62, 0x44, 0x69, 0x72,
				},
				fileName: []byte("foo"),
			},
			want: "/usr/local/var/mobius/Files/A SubDir/foo",
		},
		{
			name: "when filePath and fileName are nil",
			args: args{
				fileRoot: "/usr/local/var/mobius/Files",
				filePath: nil,
				fileName: nil,
			},
			want: "/usr/local/var/mobius/Files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPath(tt.args.fileRoot, tt.args.filePath, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReadPath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_fileItemScanner(t *testing.T) {
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
			name: "when a full fileItem is provided",
			args: args{
				data: []byte{
					0, 0,
					0x09,
					0x73, 0x75, 0x62, 0x66, 0x6f, 0x6c, 0x64, 0x65, 0x72,
				},
				in1: false,
			},
			wantAdvance: 12,
			wantToken: []byte{
				0, 0,
				0x09,
				0x73, 0x75, 0x62, 0x66, 0x6f, 0x6c, 0x64, 0x65, 0x72,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when a full fileItem with extra bytes is provided",
			args: args{
				data: []byte{
					0, 0,
					0x09,
					0x73, 0x75, 0x62, 0x66, 0x6f, 0x6c, 0x64, 0x65, 0x72,
					1, 1, 1, 1, 1, 1,
				},
				in1: false,
			},
			wantAdvance: 12,
			wantToken: []byte{
				0, 0,
				0x09,
				0x73, 0x75, 0x62, 0x66, 0x6f, 0x6c, 0x64, 0x65, 0x72,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when insufficient bytes are provided",
			args: args{
				data: []byte{
					0, 0,
				},
				in1: false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := fileItemScanner(tt.args.data, tt.args.in1)
			if !tt.wantErr(t, err, fmt.Sprintf("fileItemScanner(%v, %v)", tt.args.data, tt.args.in1)) {
				return
			}
			assert.Equalf(t, tt.wantAdvance, gotAdvance, "fileItemScanner(%v, %v)", tt.args.data, tt.args.in1)
			assert.Equalf(t, tt.wantToken, gotToken, "fileItemScanner(%v, %v)", tt.args.data, tt.args.in1)
		})
	}
}
