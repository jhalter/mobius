package hotline

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadFlattenedFileObject(t *testing.T) {
	testData, _ := hex.DecodeString("46494c500001000000000000000000000000000000000002494e464f000000000000000000000052414d414354455854747478740000000000000100000000000000000000000000000000000000000000000000000000000000000007700000ba74247307700000ba74247300000008746573742e74787400004441544100000000000000000000000474657374")

	ffo := ReadFlattenedFileObject(testData)

	format := ffo.FlatFileHeader.Format[:]
	want := []byte("FILP")
	if !bytes.Equal(format, want) {
		t.Errorf("ReadFlattenedFileObject() = %q, want %q", format, want)
	}
}

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
				FlatFileInformationFork:       NewFlatFileInformationFork("testfile.txt"),
				FlatFileDataForkHeader: FlatFileDataForkHeader{
					ForkType:        []byte("DATA"),
					CompressionType: []byte{0, 0, 0, 0},
					RSVD:            []byte{0, 0, 0, 0},
					DataSize:        []byte{0x00, 0x00, 0x00, 0x17},
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
			got, err := NewFlattenedFileObject(tt.args.fileRoot, tt.args.filePath, tt.args.fileName)
			if !tt.wantErr(t, err, fmt.Sprintf("NewFlattenedFileObject(%v, %v, %v)", tt.args.fileRoot, tt.args.filePath, tt.args.fileName)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NewFlattenedFileObject(%v, %v, %v)", tt.args.fileRoot, tt.args.filePath, tt.args.fileName)
		})
	}
}
