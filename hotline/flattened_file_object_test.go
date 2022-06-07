package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewFlattenedFileObject(t *testing.T) {
	type args struct {
		fileRoot string
		filePath []byte
		fileName []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *flattenedFileObject
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "with valid file",
			args: args{
				fileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
				fileName: []byte("testfile.txt"),
				filePath: []byte{0, 0},
			},
			want: &flattenedFileObject{
				FlatFileHeader:                NewFlatFileHeader(),
				FlatFileInformationForkHeader: FlatFileInformationForkHeader{},
				FlatFileInformationFork:       NewFlatFileInformationFork("testfile.txt", make([]byte, 8), "", ""),
				FlatFileDataForkHeader: FlatFileDataForkHeader{
					ForkType:        [4]byte{0x4d, 0x41, 0x43, 0x52}, // DATA
					CompressionType: [4]byte{0, 0, 0, 0},
					RSVD:            [4]byte{0, 0, 0, 0},
					DataSize:        [4]byte{0x00, 0x00, 0x00, 0x17},
				},
				FileData: nil,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when file path is invalid",
			args: args{
				fileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
				fileName: []byte("nope.txt"),
			},
			want:    nil,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFlattenedFileObject(tt.args.fileRoot, tt.args.filePath, tt.args.fileName, 0)
			if tt.wantErr(t, err, fmt.Sprintf("NewFlattenedFileObject(%v, %v, %v)", tt.args.fileRoot, tt.args.filePath, tt.args.fileName)) {
				return
			}

			// Clear the file timestamp fields to work around problems running the tests in multiple timezones
			// TODO: revisit how to test this by mocking the stat calls
			got.FlatFileInformationFork.CreateDate = make([]byte, 8)
			got.FlatFileInformationFork.ModifyDate = make([]byte, 8)
			assert.Equalf(t, tt.want, got, "NewFlattenedFileObject(%v, %v, %v)", tt.args.fileRoot, tt.args.filePath, tt.args.fileName)
		})
	}
}

func TestFlatFileInformationFork_UnmarshalBinary(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when zero length comment size is omitted (Nostalgia client behavior)",
			args: args{
				b: []byte{
					0x41, 0x4d, 0x41, 0x43, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x62, 0x65, 0x61, 0x72, 0x2e, 0x74, 0x69, 0x66, 0x66,
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when zero length comment size is included",
			args: args{
				b: []byte{
					0x41, 0x4d, 0x41, 0x43, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x62, 0x65, 0x61, 0x72, 0x2e, 0x74, 0x69, 0x66, 0x66, 0x00, 0x00,
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffif := &FlatFileInformationFork{}
			tt.wantErr(t, ffif.UnmarshalBinary(tt.args.b), fmt.Sprintf("UnmarshalBinary(%v)", tt.args.b))
		})
	}
}
