package hotline

import (
	"bytes"
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
	"reflect"
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

//
//func TestNewFlattenedFileObject(t *testing.T) {
//	ffo := NewFlattenedFileObject("test/config/files", "testfile.txt")
//
//	dataSize := ffo.FlatFileDataForkHeader.DataSize
//	want := []byte{0, 0, 0, 0x17}
//	if bytes.Compare(dataSize, want) != 0 {
//		t.Errorf("%q, want %q", dataSize, want)
//	}
//
//	comment := ffo.FlatFileInformationFork.Comment
//	want = []byte("Test Comment")
//	if bytes.Compare(ffo.FlatFileInformationFork.Comment, want) != 0 {
//		t.Errorf("%q, want %q", comment, want)
//	}
//}

func TestNewFlattenedFileObject(t *testing.T) {
	type args struct {
		filePath string
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    *flattenedFileObject
		wantErr bool
	}{
		{
			name: "when file path is valid",
			args: args{
				filePath: "./test/config/Files/",
				fileName: "testfile.txt",
			},
			want: &flattenedFileObject{
				FlatFileHeader:                NewFlatFileHeader(),
				FlatFileInformationForkHeader: FlatFileInformationForkHeader{},
				FlatFileInformationFork:       NewFlatFileInformationFork("testfile.txt"),
				FlatFileDataForkHeader:        FlatFileDataForkHeader{
					ForkType:        []byte("DATA"),
					CompressionType: []byte{0, 0, 0, 0},
					RSVD:            []byte{0, 0, 0, 0},
					DataSize:        []byte{0x00, 0x00, 0x00, 0x17},
				},
				FileData:                      nil,
			},
			wantErr: false,
		},
		{
			name: "when file path is invalid",
			args: args{
				filePath: "./nope/",
				fileName: "also-nope.txt",
			},
			want: nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFlattenedFileObject(tt.args.filePath, tt.args.fileName)
			spew.Dump(got)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFlattenedFileObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewFlattenedFileObject() got = %v, want %v", got, tt.want)
			}
		})
	}
}