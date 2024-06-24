package hotline

import (
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFileTransfer_String(t *testing.T) {
	type fields struct {
		FileName         []byte
		FilePath         []byte
		refNum           [4]byte
		Type             int
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
				refNum:           tt.fields.refNum,
				Type:             tt.fields.Type,
				TransferSize:     tt.fields.TransferSize,
				FolderItemCount:  tt.fields.FolderItemCount,
				fileResumeData:   tt.fields.fileResumeData,
				options:          tt.fields.options,
				bytesSentCounter: tt.fields.bytesSentCounter,
				ClientConn:       tt.fields.ClientConn,
			}
			assert.Equalf(t, tt.want, ft.String(), "String()")
		})
	}
}
