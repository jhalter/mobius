package mobius

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"golang.org/x/text/encoding/charmap"
)

func TestHandleGetFileInfo(t *testing.T) {
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
			name: "returns expected fields when a valid file is requested",
			args: args{
				cc: &hotline.ClientConn{
					ID:      [2]byte{0, 1},
					Account: &hotline.Account{},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						FS:          &hotline.OSFileStore{},
						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files")
							}(),
						},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetFileInfo, [2]byte{},
					hotline.NewField(hotline.FieldFileName, []byte("testfile.txt")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Type:     [2]byte{0, 0},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldFileName, []byte("testfile.txt")),
						hotline.NewField(hotline.FieldFileTypeString, []byte("Text File")),
						hotline.NewField(hotline.FieldFileCreatorString, []byte("ttxt")),
						hotline.NewField(hotline.FieldFileType, []byte("TEXT")),
						hotline.NewField(hotline.FieldFileCreateDate, make([]byte, 8)),
						hotline.NewField(hotline.FieldFileModifyDate, make([]byte, 8)),
						hotline.NewField(hotline.FieldFileSize, []byte{0x0, 0x0, 0x0, 0x17}),
					},
				},
			},
		},
		{
			name: "returns an error reply when the file path cannot be parsed",
			args: args{
				cc: &hotline.ClientConn{
					ID:      [2]byte{0, 1},
					Account: &hotline.Account{},
					Logger:  NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						FS:          &hotline.OSFileStore{},
						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files")
							}(),
						},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetFileInfo, [2]byte{},
					hotline.NewField(hotline.FieldFileName, []byte("testfile.txt")),
					// Malformed path: claims 1 item but supplies fewer than the
					// 3 bytes a path item requires, so ReadPath fails to parse it.
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x01, 0x00, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgFileNotFound)),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetFileInfo(tt.args.cc, &tt.args.t)

			// Clear the file timestamp fields to work around problems running the tests in multiple timezones
			// TODO: revisit how to test this by mocking the stat calls
			if len(gotRes) > 0 && gotRes[0].ErrorCode == [4]byte{} {
				gotRes[0].Fields[4].Data = make([]byte, 8)
				gotRes[0].Fields[5].Data = make([]byte, 8)
			}

			if !TranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleGetFileInfo() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleNewFolder(t *testing.T) {
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
			name: "without required permission",
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
					hotline.TranNewFolder,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to create folders.")),
					},
				},
			},
		},
		{
			name: "when path is nested",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/Files/",
						},
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewFolder, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "when path is not nested",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewFolder, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "when Write returns an err",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateFolder)
							return bits
						}(),
					},
					ID:     [2]byte{0, 1},
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/Files/",
						},
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewFolder, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgCreateFolder)),
					},
				},
			},
		},
		{
			name: "FieldFileName does not allow directory traversal",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/Files/",
						},
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewFolder, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("../../testFolder")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "FieldFilePath does not allow directory traversal",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/Files/",
						},
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							mfs.On("Mkdir", "/Files/foo/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/foo/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewFolder, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x02,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
						0x00, 0x00,
						0x03,
						0x66, 0x6f, 0x6f,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewFolder(tt.args.cc, &tt.args.t)

			if !TranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleNewFolder() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleMakeAlias(t *testing.T) {
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
			name: "with valid input and required permissions",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessMakeAlias)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
						Logger: NewTestLogger(),
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							path, _ := os.Getwd()
							mfs.On(
								"Symlink",
								path+"/test/config/Files/foo/testFile",
								path+"/test/config/Files/bar/testFile",
							).Return(nil)
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranMakeFileAlias, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, hotline.EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					hotline.NewField(hotline.FieldFileNewPath, hotline.EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields:  []hotline.Field(nil),
				},
			},
		},
		{
			name: "when symlink returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessMakeAlias)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
						Logger: NewTestLogger(),
						FS: func() *hotline.MockFileStore {
							mfs := &hotline.MockFileStore{}
							path, _ := os.Getwd()
							mfs.On(
								"Symlink",
								path+"/test/config/Files/foo/testFile",
								path+"/test/config/Files/bar/testFile",
							).Return(errors.New("ohno"))
							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranMakeFileAlias, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, hotline.EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					hotline.NewField(hotline.FieldFileNewPath, hotline.EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Error creating alias")),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranMakeFileAlias, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2e,
					}),
					hotline.NewField(hotline.FieldFileNewPath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2e,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to make aliases.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleMakeAlias(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteFile(t *testing.T) {
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
			name: "when user does not have required permission to delete a folder",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: func() string {
								return "/fakeRoot/Files"
							}(),
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/aaa/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/aaa/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/aaa/.rsrc_testfile").Return(nil, errors.New("err"))

							return mfs
						}(),
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDeleteFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete files.")),
					},
				},
			},
		},
		{
			name: "deletes all associated metadata files",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteFile)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: func() string {
								return "/fakeRoot/Files"
							}(),
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/aaa/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/aaa/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/aaa/.rsrc_testfile").Return(nil, errors.New("err"))

							mfs.On("RemoveAll", "/fakeRoot/Files/aaa/testfile").Return(nil)
							mfs.On("Remove", "/fakeRoot/Files/aaa/testfile.incomplete").Return(nil)
							mfs.On("Remove", "/fakeRoot/Files/aaa/.rsrc_testfile").Return(nil)
							mfs.On("Remove", "/fakeRoot/Files/aaa/.info_testfile").Return(nil)

							return mfs
						}(),
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDeleteFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields:  []hotline.Field(nil),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDeleteFile(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)

			tt.args.cc.Server.FS.(*hotline.MockFileStore).AssertExpectations(t)
		})
	}
}

func TestHandleGetFileNameList(t *testing.T) {
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
			name: "when FieldFilePath is a drop box, but user does not have AccessViewDropBoxes ",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),

						Config: hotline.Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
							}(),
						},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetFileNameList, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x08,
						0x64, 0x72, 0x6f, 0x70, 0x20, 0x62, 0x6f, 0x78, // "drop box"
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to view drop boxes.")),
					},
				},
			},
		},
		{
			name: "with file root",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						FileRoot: func() string {
							path, _ := os.Getwd()
							return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
						}(),
					},
					Server: &hotline.Server{FS: &hotline.OSFileStore{}, TextDecoder: charmap.Macintosh.NewDecoder(), TextEncoder: charmap.Macintosh.NewEncoder()},
				},
				t: hotline.NewTransaction(
					hotline.TranGetFileNameList, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x00,
						0x00, 0x00,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(
							hotline.FieldFileNameWithInfo,
							func() []byte {
								fnwi := hotline.FileNameWithInfo{
									FileNameWithInfoHeader: hotline.FileNameWithInfoHeader{
										Type:       [4]byte{0x54, 0x45, 0x58, 0x54},
										Creator:    [4]byte{0x54, 0x54, 0x58, 0x54},
										FileSize:   [4]byte{0, 0, 0x04, 0},
										RSVD:       [4]byte{},
										NameScript: [2]byte{},
										NameSize:   [2]byte{0, 0x0b},
									},
									Name: []byte("testfile-1k"),
								}
								b, _ := io.ReadAll(&fnwi)
								return b
							}(),
						),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetFileNameList(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleMoveFile(t *testing.T) {
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
			name: "when user does not have permission to move a file",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/fakeRoot/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &hotline.MockFileStore{}
							// NewFile calls: Stat data, Stat info, Stat rsrc
							mfs.On("Stat", "/fakeRoot/Files/aaa/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/aaa/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/aaa/.rsrc_testfile").Return(nil, errors.New("err"))

							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranMoveFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
					hotline.NewField(hotline.FieldFileNewPath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x62, 0x62, 0x62,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to move files.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to move a folder",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/fakeRoot/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.ModeDir)
							mfi.On("Size").Return(int64(0))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(true)
							mfi.On("Name").Return("testfolder")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/aaa/testfolder").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/aaa/.info_testfolder").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/aaa/.rsrc_testfolder").Return(nil, errors.New("err"))

							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranMoveFile, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
					hotline.NewField(hotline.FieldFileNewPath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x62, 0x62, 0x62,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to move folders.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleMoveFile(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSetFileInfo(t *testing.T) {
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
			name: "when user does not have permission to set file comment",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/fakeRoot/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/.rsrc_testfile").Return(nil, errors.New("err"))

							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetFileInfo, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
					hotline.NewField(hotline.FieldFileComment, []byte("a comment")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to set comments for files.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to rename a file",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/fakeRoot/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/.rsrc_testfile").Return(nil, errors.New("err"))

							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetFileInfo, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfile")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
					hotline.NewField(hotline.FieldFileNewName, []byte("newname")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to rename files.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to set folder comment",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							FileRoot: "/fakeRoot/Files",
						},
						FS: func() *hotline.MockFileStore {
							mfi := &hotline.MockFileInfo{}
							mfi.On("Mode").Return(fs.ModeDir)
							mfi.On("Size").Return(int64(0))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(true)
							mfi.On("Name").Return("testfolder")

							mfs := &hotline.MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/testfolder").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/.info_testfolder").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/.rsrc_testfolder").Return(nil, errors.New("err"))

							return mfs
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetFileInfo, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testfolder")),
					hotline.NewField(hotline.FieldFilePath, []byte{0x00, 0x00}),
					hotline.NewField(hotline.FieldFileComment, []byte("a comment")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to set comments for folders.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleSetFileInfo(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}
