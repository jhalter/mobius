package hotline

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
			if got := NewFileHeader(tt.args.fileName, tt.args.isDir); !assert.Equal(t, tt.want, got) {
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
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_folderUpload_FormattedPath(t *testing.T) {
	tests := []struct {
		name          string
		pathItemCount [2]byte
		fileNamePath  []byte
		want          string
	}{
		{
			name:          "empty path",
			pathItemCount: [2]byte{0x00, 0x00},
			fileNamePath:  []byte{},
			want:          "",
		},
		{
			name:          "single path segment",
			pathItemCount: [2]byte{0x00, 0x01},
			fileNamePath: []byte{
				0x00, 0x00, // path separator
				0x03,             // segment length
				0x66, 0x6f, 0x6f, // "foo"
			},
			want: "foo",
		},
		{
			name:          "multiple path segments",
			pathItemCount: [2]byte{0x00, 0x03},
			fileNamePath: []byte{
				0x00, 0x00, // path separator
				0x04,                   // segment length
				0x68, 0x6f, 0x6d, 0x65, // "home"
				0x00, 0x00, // path separator
				0x04,                   // segment length
				0x75, 0x73, 0x65, 0x72, // "user"
				0x00, 0x00, // path separator
				0x09,                                                 // segment length
				0x64, 0x6f, 0x63, 0x75, 0x6d, 0x65, 0x6e, 0x74, 0x73, // "documents"
			},
			want: "home/user/documents",
		},
		{
			name:          "path with spaces",
			pathItemCount: [2]byte{0x00, 0x02},
			fileNamePath: []byte{
				0x00, 0x00, // path separator
				0x07,                                     // segment length
				0x4d, 0x79, 0x20, 0x46, 0x69, 0x6c, 0x65, // "My File"
				0x00, 0x00, // path separator
				0x0d,                                                                         // segment length (13 bytes)
				0x49, 0x6d, 0x70, 0x6f, 0x72, 0x74, 0x61, 0x6e, 0x74, 0x2e, 0x74, 0x78, 0x74, // "Important.txt"
			},
			want: "My File/Important.txt",
		},
		{
			name:          "single character segments",
			pathItemCount: [2]byte{0x00, 0x03},
			fileNamePath: []byte{
				0x00, 0x00, // path separator
				0x01,       // segment length
				0x61,       // "a"
				0x00, 0x00, // path separator
				0x01,       // segment length
				0x62,       // "b"
				0x00, 0x00, // path separator
				0x01, // segment length
				0x63, // "c"
			},
			want: "a/b/c",
		},
		{
			name:          "path with special characters",
			pathItemCount: [2]byte{0x00, 0x01},
			fileNamePath: []byte{
				0x00, 0x00, // path separator
				0x08,                                           // segment length
				0x74, 0x65, 0x73, 0x74, 0x40, 0x24, 0x25, 0x26, // "test@$%&"
			},
			want: "test@$%&",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fu := &folderUpload{
				PathItemCount: tt.pathItemCount,
				FileNamePath:  tt.fileNamePath,
			}
			got := fu.FormattedPath()
			assert.Equal(t, tt.want, got)
		})
	}
}
