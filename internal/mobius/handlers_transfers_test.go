package mobius

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/encoding/charmap"
)

func TestHandleUploadFile(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when request is valid and user has Upload Anywhere permission",
			args: args{
				cc: &hotline.ClientConn{
					Server: &hotline.Server{
						TextDecoder:     charmap.Macintosh.NewDecoder(),
						TextEncoder:     charmap.Macintosh.NewEncoder(),
						FS:              &hotline.OSFileStore{},
						FileTransferMgr: hotline.NewMemFileTransferMgr(),
						Config: hotline.Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						}},
					ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessUploadFile)
							bits.Set(hotline.AccessUploadAnywhere)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUploadFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}), // rand.Seed(1)
					},
				},
			},
		},
		{
			name: "when user does not have required access",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUploadFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to upload files.")), // rand.Seed(1)
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleUploadFile(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

// When a client requests to resume an upload but no partial (.incomplete) file
// exists, the handler should fall back to a normal upload reply (reference number,
// no resume-data field) rather than discarding the reply and returning nothing.
func TestHandleUploadFile_resumeFallbackWhenNoPartialFile(t *testing.T) {
	cc := &hotline.ClientConn{
		Logger: NewTestLogger(),
		Server: &hotline.Server{
			TextDecoder:     charmap.Macintosh.NewDecoder(),
			TextEncoder:     charmap.Macintosh.NewEncoder(),
			FS:              &hotline.OSFileStore{},
			FileTransferMgr: hotline.NewMemFileTransferMgr(),
			Config: hotline.Config{
				FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
			},
		},
		ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
		Account: &hotline.Account{
			Access: func() hotline.AccessBitmap {
				var bits hotline.AccessBitmap
				bits.Set(hotline.AccessUploadFile)
				bits.Set(hotline.AccessUploadAnywhere)
				return bits
			}(),
		},
	}
	tr := hotline.NewTransaction(
		hotline.TranUploadFile, [2]byte{0, 1},
		hotline.NewField(hotline.FieldFileName, []byte("doesNotExistYet")),
		hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x01, 0x00, 0x00, 0x03, 0x2e, 0x2e, 0x2f}),
		hotline.NewField(hotline.FieldFileTransferOptions, []byte{0x00, 0x02}), // request resume
	)

	gotRes := HandleUploadFile(cc, &tr)

	if len(gotRes) != 1 {
		t.Fatalf("expected 1 reply transaction, got %d", len(gotRes))
	}
	if gotRes[0].ErrorCode != [4]byte{} {
		t.Errorf("expected a non-error reply, got error code %v", gotRes[0].ErrorCode)
	}
	if gotRes[0].GetField(hotline.FieldRefNum).Data == nil {
		t.Errorf("expected a reference number field in the reply")
	}
	if gotRes[0].GetField(hotline.FieldFileResumeData).Data != nil {
		t.Errorf("expected no resume data field when there is no partial file to resume")
	}
}

func TestHandleDownloadFile(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{TextDecoder: charmap.Macintosh.NewDecoder(), TextEncoder: charmap.Macintosh.NewEncoder()},
				},
				t: hotline.NewTransaction(hotline.TranDownloadFile, [2]byte{0, 1}),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to download files.")),
					},
				},
			},
		},
		{
			name: "with a valid file",
			args: args{
				cc: &hotline.ClientConn{
					ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDownloadFile)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder:     charmap.Macintosh.NewDecoder(),
						TextEncoder:     charmap.Macintosh.NewEncoder(),
						FS:              &hotline.OSFileStore{},
						FileTransferMgr: hotline.NewMemFileTransferMgr(),
						Config: hotline.Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDownloadFile,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile.txt")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x0, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}),
						hotline.NewField(hotline.FieldTransferSize, []byte{0x00, 0x00, 0x00, 0xa5}),
						hotline.NewField(hotline.FieldFileSize, []byte{0x00, 0x00, 0x00, 0x17}),
					},
				},
			},
		},
		{
			name: "when client requests to resume 1k test file at offset 256",
			args: args{
				cc: &hotline.ClientConn{
					ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDownloadFile)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						FS:          &hotline.OSFileStore{},

						// FS: func() *hltest.MockFileStore {
						// 	path, _ := os.Getwd()
						// 	testFile, err := os.Open(path + "/test/config/Files/testfile-1k")
						// 	if err != nil {
						// 		panic(err)
						// 	}
						//
						// 	mfi := &hltest.MockFileInfo{}
						// 	mfi.On("Mode").Return(fs.FileMode(0))
						// 	mfs := &MockFileStore{}
						// 	mfs.On("Stat", "/fakeRoot/Files/testfile.txt").Return(mfi, nil)
						// 	mfs.On("Open", "/fakeRoot/Files/testfile.txt").Return(testFile, nil)
						// 	mfs.On("Stat", "/fakeRoot/Files/.info_testfile.txt").Return(nil, errors.New("no"))
						// 	mfs.On("Stat", "/fakeRoot/Files/.rsrc_testfile.txt").Return(nil, errors.New("no"))
						//
						// 	return mfs
						// }(),
						FileTransferMgr: hotline.NewMemFileTransferMgr(),
						Config: hotline.Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDownloadFile,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile-1k")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
					hotline.NewField(
						hotline.FieldFileResumeData,
						func() []byte {
							frd := hotline.FileResumeData{
								ForkCount: [2]byte{0, 2},
								ForkInfoList: []hotline.ForkInfoList{
									{
										Fork:     hotline.ForkTypeDATA,
										DataSize: [4]byte{0, 0, 0x01, 0x00}, // request offset 256
									},
									{
										Fork:     hotline.ForkTypeMACR,
										DataSize: [4]byte{0, 0, 0, 0},
									},
								},
							}
							b, _ := frd.BinaryMarshal()
							return b
						}(),
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}),
						hotline.NewField(hotline.FieldTransferSize, []byte{0x00, 0x00, 0x03, 0x8d}),
						hotline.NewField(hotline.FieldFileSize, []byte{0x00, 0x00, 0x03, 0x00}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDownloadFile(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadBanner(t *testing.T) {
	t.Run("returns banner transfer info", func(t *testing.T) {
		srv := &hotline.Server{
			FileTransferMgr: hotline.NewMemFileTransferMgr(),
		}
		srv.SetBanner([]byte("test-banner-data"))

		cc := &hotline.ClientConn{
			ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
			Server:                srv,
		}
		tran := hotline.NewTransaction(hotline.TranDownloadBanner, [2]byte{0, 1})

		gotRes := HandleDownloadBanner(cc, &tran)

		assert.Len(t, gotRes, 1)
		assert.Equal(t, byte(0x01), gotRes[0].IsReply)

		// Verify transfer size field matches banner length
		transferSizeField := gotRes[0].GetField(hotline.FieldTransferSize)
		assert.NotNil(t, transferSizeField)
		gotSize := binary.BigEndian.Uint32(transferSizeField.Data)
		assert.Equal(t, uint32(len("test-banner-data")), gotSize)

		// Verify refnum field is present
		refNumField := gotRes[0].GetField(hotline.FieldRefNum)
		assert.NotNil(t, refNumField)
		assert.Len(t, refNumField.Data, 4)
	})
}

func TestHandleUploadFolder(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required access",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUploadFldr, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to upload folders.")),
					},
				},
			},
		},
		{
			name: "when user has upload access but not upload anywhere and path is not upload dir",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessUploadFolder)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUploadFldr, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("myFolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x05,
						0x46, 0x69, 0x6c, 0x65, 0x73, // "Files"
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Cannot accept upload of the folder \"myFolder\" because you are only allowed to upload to the \"Uploads\" folder.")),
					},
				},
			},
		},
		{
			name: "when user has upload access and upload anywhere permission",
			args: args{
				cc: &hotline.ClientConn{
					ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessUploadFolder)
							bits.Set(hotline.AccessUploadAnywhere)
							return bits
						}(),
					},
					Server: &hotline.Server{
						FileTransferMgr: hotline.NewMemFileTransferMgr(),
						Config:          hotline.Config{FileRoot: "/fakeRoot"},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUploadFldr, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("myFolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x05,
						0x46, 0x69, 0x6c, 0x65, 0x73, // "Files"
					}),
					hotline.NewField(hotline.FieldTransferSize, []byte{0, 0, 0x10, 0}),
					hotline.NewField(hotline.FieldFolderItemCount, []byte{0, 0, 0, 5}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldRefNum, []byte{0, 0, 0, 0}), // placeholder, TranAssertEqual strips this
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TranAssertEqual(t, tt.wantRes, HandleUploadFolder(tt.args.cc, &tt.args.t))
		})
	}
}

func TestHandleDownloadFolder(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required access",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDownloadFldr, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to download folders.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TranAssertEqual(t, tt.wantRes, HandleDownloadFolder(tt.args.cc, &tt.args.t))
		})
	}
}

func TestHandleDownloadFolder_withAccess(t *testing.T) {
	// Create a temp dir with files to act as the download folder
	tmpDir := t.TempDir()
	dlDir := filepath.Join(tmpDir, "testFolder")
	err := os.MkdirAll(dlDir, 0755)
	assert.NoError(t, err)

	// Create a test file inside the folder
	err = os.WriteFile(filepath.Join(dlDir, "file.txt"), []byte("hello"), 0644)
	assert.NoError(t, err)

	cc := &hotline.ClientConn{
		ClientFileTransferMgr: hotline.NewClientFileTransferMgr(),
		Account: &hotline.Account{
			Access: func() hotline.AccessBitmap {
				var bits hotline.AccessBitmap
				bits.Set(hotline.AccessDownloadFolder)
				return bits
			}(),
		},
		Server: &hotline.Server{
			FS:              &hotline.OSFileStore{},
			TextDecoder:     charmap.Macintosh.NewDecoder(),
			TextEncoder:     charmap.Macintosh.NewEncoder(),
			FileTransferMgr: hotline.NewMemFileTransferMgr(),
			Config:          hotline.Config{FileRoot: tmpDir},
		},
	}

	tran := hotline.NewTransaction(
		hotline.TranDownloadFldr, [2]byte{0, 1},
		hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
		hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
	)

	gotRes := HandleDownloadFolder(cc, &tran)
	assert.Len(t, gotRes, 1)
	assert.Equal(t, byte(0x01), gotRes[0].IsReply)

	// Verify expected fields are present
	assert.NotNil(t, gotRes[0].GetField(hotline.FieldRefNum))
	assert.NotNil(t, gotRes[0].GetField(hotline.FieldTransferSize))
	assert.NotNil(t, gotRes[0].GetField(hotline.FieldFolderItemCount))
}
