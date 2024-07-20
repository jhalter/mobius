package hotline

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"log/slog"
	"os"
	"testing"
)

type mockReadWriter struct {
	RBuf bytes.Buffer
	WBuf *bytes.Buffer
}

func (mrw mockReadWriter) Read(p []byte) (n int, err error) {
	return mrw.RBuf.Read(p)
}

func (mrw mockReadWriter) Write(p []byte) (n int, err error) {
	return mrw.WBuf.Write(p)
}

func TestServer_handleFileTransfer(t *testing.T) {
	type fields struct {
		ThreadedNews    *ThreadedNews
		FileTransferMgr FileTransferMgr
		Config          Config
		ConfigDir       string
		Stats           *Stats
		Logger          *slog.Logger
		FS              FileStore
	}
	type args struct {
		ctx context.Context
		rwc io.ReadWriter
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		wantErr  assert.ErrorAssertionFunc
		wantDump string
	}{
		{
			name: "with invalid protocol",
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0, 0, 0, 0,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.Error,
		},
		{
			name: "with invalid transfer Type",
			fields: fields{
				FileTransferMgr: NewMemFileTransferMgr(),
			},
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0x48, 0x54, 0x58, 0x46,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.Error,
		},
		{
			name: "file download",
			fields: fields{
				FS:     &OSFileStore{},
				Logger: NewTestLogger(),
				Stats:  NewStats(),
				FileTransferMgr: &MemFileTransferMgr{
					fileTransfers: map[FileTransferID]*FileTransfer{
						{0, 0, 0, 5}: {
							RefNum:   [4]byte{0, 0, 0, 5},
							Type:     FileDownload,
							FileName: []byte("testfile-8b"),
							FilePath: []byte{},
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
							ClientConn: &ClientConn{
								Account: &Account{
									Login: "foo",
								},
								ClientFileTransferMgr: ClientFileTransferMgr{
									transfers: map[FileTransferType]map[FileTransferID]*FileTransfer{
										FileDownload: {
											[4]byte{0, 0, 0, 5}: &FileTransfer{},
										},
									},
								},
							},
							bytesSentCounter: &WriteCounter{},
						},
					},
				},
			},
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0x48, 0x54, 0x58, 0x46,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.NoError,
			wantDump: `00000000  46 49 4c 50 00 01 00 00  00 00 00 00 00 00 00 00  |FILP............|
00000010  00 00 00 00 00 00 00 02  49 4e 46 4f 00 00 00 00  |........INFO....|
00000020  00 00 00 00 00 00 00 55  41 4d 41 43 54 45 58 54  |.......UAMACTEXT|
00000030  54 54 58 54 00 00 00 00  00 00 01 00 00 00 00 00  |TTXT............|
00000040  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
00000050  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
00000060  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 0b  |................|
00000070  74 65 73 74 66 69 6c 65  2d 38 62 00 00 44 41 54  |testfile-8b..DAT|
00000080  41 00 00 00 00 00 00 00  00 00 00 00 08 7c 39 e0  |A............|9.|
00000090  bc 64 e2 cd de 4d 41 43  52 00 00 00 00 00 00 00  |.d...MACR.......|
000000a0  00 00 00 00 00                                    |.....|
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				FileTransferMgr: tt.fields.FileTransferMgr,
				Config:          tt.fields.Config,
				Logger:          tt.fields.Logger,
				Stats:           tt.fields.Stats,
				FS:              tt.fields.FS,
			}

			tt.wantErr(t, s.handleFileTransfer(tt.args.ctx, tt.args.rwc), fmt.Sprintf("handleFileTransfer(%v, %v)", tt.args.ctx, tt.args.rwc))

			assertTransferBytesEqual(t, tt.wantDump, tt.args.rwc.(mockReadWriter).WBuf.Bytes())
		})
	}
}
