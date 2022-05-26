package hotline

import (
	"github.com/stretchr/testify/assert"
	"io/fs"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

func TestHandleSetChatSubject(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		want    []Transaction
		wantErr bool
	}{
		{
			name: "sends chat subject to private chat members",
			args: args{
				cc: &ClientConn{
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						PrivateChats: map[uint32]*PrivateChat{
							uint32(1): {
								Subject: "unset",
								ClientConn: map[uint16]*ClientConn{
									uint16(1): {
										Account: &Account{
											Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 1},
									},
									uint16(2): {
										Account: &Account{
											Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 2},
									},
								},
							},
						},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0, 0, 0, 1},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldChatID, []byte{0, 0, 0, 1}),
						NewField(fieldChatSubject, []byte("Test Subject")),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x77},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldChatID, []byte{0, 0, 0, 1}),
						NewField(fieldChatSubject, []byte("Test Subject")),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x77},
					ID:        []byte{0xf0, 0xc5, 0x34, 0x1e}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldChatID, []byte{0, 0, 0, 1}),
						NewField(fieldChatSubject, []byte("Test Subject")),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		rand.Seed(1) // reset seed between tests to make transaction IDs predictable

		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleSetChatSubject(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleSetChatSubject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("HandleSetChatSubject() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleLeaveChat(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		want    []Transaction
		wantErr bool
	}{
		{
			name: "returns expected transactions",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 2},
					Server: &Server{
						PrivateChats: map[uint32]*PrivateChat{
							uint32(1): {
								ClientConn: map[uint16]*ClientConn{
									uint16(1): {
										Account: &Account{
											Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 1},
									},
									uint16(2): {
										Account: &Account{
											Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 2},
									},
								},
							},
						},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: NewTransaction(tranDeleteUser, nil, NewField(fieldChatID, []byte{0, 0, 0, 1})),
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x76},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldChatID, []byte{0, 0, 0, 1}),
						NewField(fieldUserID, []byte{0, 2}),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		rand.Seed(1)
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleLeaveChat(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleLeaveChat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("HandleLeaveChat() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleGetUserNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		want    []Transaction
		wantErr bool
	}{
		{
			name: "replies with userlist transaction",
			args: args{
				cc: &ClientConn{

					ID: &[]byte{1, 1},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								ID:       &[]byte{0, 1},
								Icon:     &[]byte{0, 2},
								Flags:    &[]byte{0, 3},
								UserName: []byte{0, 4},
							},
							uint16(2): {
								ID:       &[]byte{0, 2},
								Icon:     &[]byte{0, 2},
								Flags:    &[]byte{0, 3},
								UserName: []byte{0, 4},
							},
						},
					},
				},
				t: &Transaction{
					ID:   []byte{0, 0, 0, 1},
					Type: []byte{0, 1},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{1, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 1},
					ID:        []byte{0, 0, 0, 1},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(
							fieldUsernameWithInfo,
							[]byte{00, 01, 00, 02, 00, 03, 00, 02, 00, 04},
						),
						NewField(
							fieldUsernameWithInfo,
							[]byte{00, 02, 00, 02, 00, 03, 00, 02, 00, 04},
						),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleGetUserNameList(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleGetUserNameList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HandleGetUserNameList() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleChatSend(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		want    []Transaction
		wantErr bool
	}{
		{
			name: "sends chat msg transaction to all clients",
			args: args{
				cc: &ClientConn{
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(fieldData, []byte("hai")),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0xf0, 0xc5, 0x34, 0x1e}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "sends chat msg as emote if fieldChatOptions is set",
			args: args{
				cc: &ClientConn{
					UserName: []byte("Testy McTest"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(fieldData, []byte("performed action")),
						NewField(fieldChatOptions, []byte{0x00, 0x01}),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0xf0, 0xc5, 0x34, 0x1e}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only sends chat msg to clients with accessReadChat permission",
			args: args{
				cc: &ClientConn{
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: &[]byte{0, 0, 0, 0, 0, 0, 0, 0},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(fieldData, []byte("hai")),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		rand.Seed(1) // reset seed between tests to make transaction IDs predictable
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleChatSend(tt.args.cc, tt.args.t)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleChatSend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("HandleChatSend() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleGetFileInfo(t *testing.T) {
	rand.Seed(1) // reset seed between tests to make transaction IDs predictable

	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr bool
	}{
		{
			name: "returns expected fields when a valid file is requested",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0x00, 0x01},
					Server: &Server{
						Config: &Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
					},
				},
				t: NewTransaction(
					tranGetFileInfo, nil,
					NewField(fieldFileName, []byte("testfile.txt")),
					NewField(fieldFilePath, []byte{0x00, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xce},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldFileName, []byte("testfile.txt")),
						NewField(fieldFileTypeString, []byte("TEXT")),
						NewField(fieldFileCreatorString, []byte("ttxt")),
						NewField(fieldFileComment, []byte{}),
						NewField(fieldFileType, []byte("TEXT")),
						NewField(fieldFileCreateDate, make([]byte, 8)),
						NewField(fieldFileModifyDate, make([]byte, 8)),
						NewField(fieldFileSize, []byte{0x0, 0x0, 0x0, 0x17}),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rand.Seed(1) // reset seed between tests to make transaction IDs predictable

			gotRes, err := HandleGetFileInfo(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleGetFileInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Clear the file timestamp fields to work around problems running the tests in multiple timezones
			// TODO: revisit how to test this by mocking the stat calls
			gotRes[0].Fields[5].Data = make([]byte, 8)
			gotRes[0].Fields[6].Data = make([]byte, 8)
			if !assert.Equal(t, tt.wantRes, gotRes) {
				t.Errorf("HandleGetFileInfo() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleNewFolder(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		setup   func()
		name    string
		args    args
		wantRes []Transaction
		wantErr bool
	}{
		{
			name: "when path is nested",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
							FileRoot: "/Files/",
						},
					},
				},
				t: NewTransaction(
					tranNewFolder, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFolder")),
					NewField(fieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			setup: func() {
				mfs := MockFileStore{}
				mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
				mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
				FS = mfs
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xcd},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			},
			wantErr: false,
		},
		{
			name: "when path is not nested",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
							FileRoot: "/Files",
						},
					},
				},
				t: NewTransaction(
					tranNewFolder, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFolder")),
				),
			},
			setup: func() {
				mfs := MockFileStore{}
				mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
				mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
				FS = mfs
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xcd},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			},
			wantErr: false,
		},
		{
			name: "when UnmarshalBinary returns an err",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
							FileRoot: "/Files/",
						},
					},
				},
				t: NewTransaction(
					tranNewFolder, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFolder")),
					NewField(fieldFilePath, []byte{
						0x00,
					}),
				),
			},
			setup: func() {
				mfs := MockFileStore{}
				mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
				mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
				FS = mfs
			},
			wantRes: []Transaction{},
			wantErr: true,
		},
		{
			name: "fieldFileName does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
							FileRoot: "/Files/",
						},
					},
				},
				t: NewTransaction(
					tranNewFolder, &[]byte{0, 1},
					NewField(fieldFileName, []byte("../../testFolder")),
				),
			},
			setup: func() {
				mfs := MockFileStore{}
				mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
				mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
				FS = mfs
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xcd},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			}, wantErr: false,
		},
		{
			name: "fieldFilePath does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
							FileRoot: "/Files/",
						},
					},
				},
				t: NewTransaction(
					tranNewFolder, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFolder")),
					NewField(fieldFilePath, []byte{
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
			setup: func() {
				mfs := MockFileStore{}
				mfs.On("Mkdir", "/Files/foo/testFolder", fs.FileMode(0777)).Return(nil)
				mfs.On("Stat", "/Files/foo/testFolder").Return(nil, os.ErrNotExist)
				FS = mfs
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xcd},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			}, wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			gotRes, err := HandleNewFolder(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleNewFolder() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleNewFolder() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleUploadFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr bool
	}{
		{
			name: "when request is valid",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						FileTransfers: map[uint32]*FileTransfer{},
					},
					Account: &Account{
						Access: func() *[]byte {
							var bits accessBitmap
							bits.Set(accessUploadFile)
							access := bits[:]
							return &access
						}(),
					},
				},
				t: NewTransaction(
					tranUploadFile, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFile")),
					NewField(fieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0xcb},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(fieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}), // rand.Seed(1)
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when user does not have required access",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() *[]byte {
							var bits accessBitmap
							access := bits[:]
							return &access
						}(),
					},
					Server: &Server{
						FileTransfers: map[uint32]*FileTransfer{},
					},
				},
				t: NewTransaction(
					tranUploadFile, &[]byte{0, 1},
					NewField(fieldFileName, []byte("testFile")),
					NewField(fieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0x00},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(fieldError, []byte("You are not allowed to upload files.")), // rand.Seed(1)
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rand.Seed(1)
			gotRes, err := HandleUploadFile(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleUploadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleUploadFile() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}
