package hotline

import (
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"io"
	"reflect"
	"testing"
)

func TestFileTransfer_String(t *testing.T) {
	type fields struct {
		FileName         []byte
		FilePath         []byte
		refNum           [4]byte
		Type             FileTransferType
		TransferSize     []byte
		FolderItemCount  []byte
		fileResumeData   *FileResumeData
		options          []byte
		bytesSentCounter *WriteCounter
		ClientConn       *ClientConn
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "50% complete 198MB file",
			fields: fields{
				FileName:     []byte("MasterOfOrionII1.4.0."),
				TransferSize: func() []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, 207618048); return b }(),
				bytesSentCounter: &WriteCounter{
					Total: 103809024,
				},
			},
			want: "MasterOfOrionII1.4.0. 50%  198.0M\n",
		},
		{
			name: "25% complete 512KB file",
			fields: fields{
				FileName:     []byte("ExampleFile.txt"),
				TransferSize: func() []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, 524288); return b }(),
				bytesSentCounter: &WriteCounter{
					Total: 131072,
				},
			},
			want: "ExampleFile.txt       25%    512K\n",
		},
		{
			name: "100% complete 2GB file",
			fields: fields{
				FileName:     []byte("LargeFile.dat"),
				TransferSize: func() []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, 2147483648); return b }(),
				bytesSentCounter: &WriteCounter{
					Total: 2147483648,
				},
			},
			want: "LargeFile.dat         100%  2048.0M\n",
		},
		{
			name: "0% complete 1MB file",
			fields: fields{
				FileName:     []byte("NewDocument.docx"),
				TransferSize: func() []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, 1048576); return b }(),
				bytesSentCounter: &WriteCounter{
					Total: 0,
				},
			},
			want: "NewDocument.docx      0%    1.0M\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &FileTransfer{
				FileName:         tt.fields.FileName,
				FilePath:         tt.fields.FilePath,
				RefNum:           tt.fields.refNum,
				Type:             tt.fields.Type,
				TransferSize:     tt.fields.TransferSize,
				FolderItemCount:  tt.fields.FolderItemCount,
				FileResumeData:   tt.fields.fileResumeData,
				Options:          tt.fields.options,
				bytesSentCounter: tt.fields.bytesSentCounter,
				ClientConn:       tt.fields.ClientConn,
			}
			assert.Equalf(t, tt.want, ft.String(), "String()")
		})
	}
}

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
				Size:     [2]byte{0x00, 0x0a},
				Type:     [2]byte{0x00, 0x00},
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
				Size:     [2]byte{0x00, 0x0a},
				Type:     [2]byte{0x00, 0x01},
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
		Size     [2]byte
		Type     [2]byte
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
				Size:     [2]byte{0x00, 0x0a},
				Type:     [2]byte{0x00, 0x00},
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
			got, _ := io.ReadAll(fh)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}
