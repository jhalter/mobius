package hotline

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"io"
	"os"
	"sync"
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
		Port          int
		Accounts      map[string]*Account
		Agreement     []byte
		Clients       map[uint16]*ClientConn
		ThreadedNews  *ThreadedNews
		fileTransfers map[[4]byte]*FileTransfer
		Config        *Config
		ConfigDir     string
		Logger        *zap.SugaredLogger
		PrivateChats  map[uint32]*PrivateChat
		NextGuestID   *uint16
		TrackerPassID [4]byte
		Stats         *Stats
		FS            FileStore
		outbox        chan Transaction
		mux           sync.Mutex
		flatNewsMux   sync.Mutex
		FlatNews      []byte
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
			name: "with invalid transfer ID",
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
				FS: &OSFileStore{},
				Config: &Config{
					FileRoot: func() string {
						path, _ := os.Getwd()
						return path + "/test/config/Files"
					}()},
				Logger: NewTestLogger(),
				Stats:  &Stats{},
				fileTransfers: map[[4]byte]*FileTransfer{
					[4]byte{0, 0, 0, 5}: {
						ReferenceNumber: []byte{0, 0, 0, 5},
						Type:            FileDownload,
						FileName:        []byte("testfile-8b"),
						FilePath:        []byte{},
						ClientConn: &ClientConn{
							Account: &Account{
								Login: "foo",
							},
							transfersMU: sync.Mutex{},
							transfers: map[int]map[[4]byte]*FileTransfer{
								FileDownload: {
									[4]byte{0, 0, 0, 5}: &FileTransfer{},
								},
							},
						},
						bytesSentCounter: &WriteCounter{},
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
				Port:          tt.fields.Port,
				Accounts:      tt.fields.Accounts,
				Agreement:     tt.fields.Agreement,
				Clients:       tt.fields.Clients,
				ThreadedNews:  tt.fields.ThreadedNews,
				fileTransfers: tt.fields.fileTransfers,
				Config:        tt.fields.Config,
				ConfigDir:     tt.fields.ConfigDir,
				Logger:        tt.fields.Logger,
				Stats:         tt.fields.Stats,
				FS:            tt.fields.FS,
			}
			tt.wantErr(t, s.handleFileTransfer(tt.args.ctx, tt.args.rwc), fmt.Sprintf("handleFileTransfer(%v, %v)", tt.args.ctx, tt.args.rwc))

			assertTransferBytesEqual(t, tt.wantDump, tt.args.rwc.(mockReadWriter).WBuf.Bytes())
		})
	}
}
