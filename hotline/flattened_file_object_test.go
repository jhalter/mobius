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

func Test_flattenedFileObject_BinaryMarshal(t *testing.T) {

	testData, _ := hex.DecodeString("46494c500001000000000000000000000000000000000002494e464f000000000000000000000052414d414354455854747478740000000000000100000000000000000000000000000000000000000000000000000000000000000007700000ba74247307700000ba74247300000008746573742e74787400004441544100000000000000000000000474657374")
	testFile := ReadFlattenedFileObject(testData)
	testFile.FlatFileInformationFork.Comment = []byte("test!")
	testFile.FlatFileInformationFork.CommentSize = []byte{0x00, 0x05}

	type fields struct {
		FlatFileHeader                FlatFileHeader
		FlatFileInformationForkHeader FlatFileInformationForkHeader
		FlatFileInformationFork       FlatFileInformationFork
		FlatFileDataForkHeader        FlatFileDataForkHeader
		FileData                      []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{
			name: "with a valid file",
			fields: fields{
				FlatFileHeader:                testFile.FlatFileHeader,
				FlatFileInformationForkHeader: testFile.FlatFileInformationForkHeader,
				FlatFileInformationFork:       testFile.FlatFileInformationFork,
				FlatFileDataForkHeader:        testFile.FlatFileDataForkHeader,
				FileData:                      testFile.FileData,
			},
			want: []byte{
				0x46, 0x49, 0x4c, 0x50, 0x00, 0x01, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
				0x49, 0x4e, 0x46, 0x4f, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x57,
				0x41, 0x4d, 0x41, 0x43, 0x54, 0x45, 0x58, 0x54,
				0x74, 0x74, 0x78, 0x74, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00, 0x00,
				0xba, 0x74, 0x24, 0x73, 0x07, 0x70, 0x00, 0x00,
				0xba, 0x74, 0x24, 0x73, 0x00, 0x00, 0x00, 0x08,
				0x74, 0x65, 0x73, 0x74, 0x2e, 0x74, 0x78, 0x74,
				0x00, 0x05, 0x74, 0x65, 0x73, 0x74, 0x21, 0x44,
				0x41, 0x54, 0x41, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := flattenedFileObject{
				FlatFileHeader:                tt.fields.FlatFileHeader,
				FlatFileInformationForkHeader: tt.fields.FlatFileInformationForkHeader,
				FlatFileInformationFork:       tt.fields.FlatFileInformationFork,
				FlatFileDataForkHeader:        tt.fields.FlatFileDataForkHeader,
				FileData:                      tt.fields.FileData,
			}
			assert.Equalf(t, tt.want, f.BinaryMarshal(), "BinaryMarshal()")
		})
	}
}
