package hotline

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestTransfer_Read(t *testing.T) {
	type fields struct {
		Protocol        [4]byte
		ReferenceNumber [4]byte
		DataSize        [4]byte
		RSVD            [4]byte
	}
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "when b is a valid transfer",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x48, 0x54, 0x58, 0x46,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00, 0x00,
				},
			},
			want:    16,
			wantErr: false,
		},
		{
			name: "when b contains invalid transfer protocol",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x11, 0x11, 0x11, 0x11,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00, 0x00,
				},
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "when b does not contain expected len of bytes",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x48, 0x54, 0x58, 0x46,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00,
				},
			},
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf := &transfer{
				Protocol:        tt.fields.Protocol,
				ReferenceNumber: tt.fields.ReferenceNumber,
				DataSize:        tt.fields.DataSize,
				RSVD:            tt.fields.RSVD,
			}
			got, err := tf.Write(tt.args.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Read() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_receiveFile(t *testing.T) {
	type args struct {
		conn io.Reader
	}
	tests := []struct {
		name            string
		args            args
		wantTargetFile  []byte
		wantResForkFile []byte
		wantErr         assert.ErrorAssertionFunc
	}{
		{
			name: "transfers file",
			args: args{
				conn: func() io.Reader {
					testFile := flattenedFileObject{
						FlatFileHeader:                NewFlatFileHeader(),
						FlatFileInformationForkHeader: FlatFileInformationForkHeader{},
						FlatFileInformationFork:       NewFlatFileInformationFork("testfile.txt", make([]byte, 8), "TEXT", "TEXT"),
						FlatFileDataForkHeader: FlatFileDataForkHeader{
							ForkType:        [4]byte{0x4d, 0x41, 0x43, 0x52}, // DATA
							CompressionType: [4]byte{0, 0, 0, 0},
							RSVD:            [4]byte{0, 0, 0, 0},
							DataSize:        [4]byte{0x00, 0x00, 0x00, 0x03},
						},
						FileData: nil,
					}
					fakeFileData := []byte{1, 2, 3}
					b := testFile.BinaryMarshal()
					b = append(b, fakeFileData...)
					return bytes.NewReader(b)
				}(),
			},
			wantTargetFile:  []byte{1, 2, 3},
			wantResForkFile: []byte(nil),

			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetFile := &bytes.Buffer{}
			resForkFile := &bytes.Buffer{}
			err := receiveFile(tt.args.conn, targetFile, resForkFile)
			if !tt.wantErr(t, err, fmt.Sprintf("receiveFile(%v, %v, %v)", tt.args.conn, targetFile, resForkFile)) {
				return
			}

			assert.Equalf(t, tt.wantTargetFile, targetFile.Bytes(), "receiveFile(%v, %v, %v)", tt.args.conn, targetFile, resForkFile)
			assert.Equalf(t, tt.wantResForkFile, resForkFile.Bytes(), "receiveFile(%v, %v, %v)", tt.args.conn, targetFile, resForkFile)
		})
	}
}
