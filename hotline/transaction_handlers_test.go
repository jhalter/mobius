package hotline

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleSetChatSubject(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name string
		args args
		want []Transaction
	}{
		{
			name: "sends chat subject to private chat members",
			args: args{
				cc: &ClientConn{
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						ChatMgr: func() *MockChatManager {
							m := MockChatManager{}
							m.On("Members", ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							})
							m.On("SetSubject", ChatID{0x0, 0x0, 0x0, 0x1}, "Test Subject")
							return &m
						}(),
						//PrivateChats: map[[4]byte]*PrivateChat{
						//	[4]byte{0, 0, 0, 1}: {
						//		Subject: "unset",
						//		ClientConn: map[[2]byte]*ClientConn{
						//			[2]byte{0, 1}: {
						//				Account: &Account{
						//					Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
						//				},
						//				ID: [2]byte{0, 1},
						//			},
						//			[2]byte{0, 2}: {
						//				Account: &Account{
						//					Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
						//				},
						//				ID: [2]byte{0, 2},
						//			},
						//		},
						//	},
						//},
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Type: [2]byte{0, 0x6a},
					ID:   [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x77},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x77},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleSetChatSubject(tt.args.cc, &tt.args.t)
			if !tranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleSetChatSubject() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleLeaveChat(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name string
		args args
		want []Transaction
	}{
		{
			name: "when client 2 leaves chat",
			args: args{
				cc: &ClientConn{
					ID: [2]byte{0, 2},
					Server: &Server{
						ChatMgr: func() *MockChatManager {
							m := MockChatManager{}
							m.On("Members", ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
							})
							m.On("Leave", ChatID{0x0, 0x0, 0x0, 0x1}, [2]uint8{0x0, 0x2})
							m.On("GetSubject").Return("unset")
							return &m
						}(),
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: NewTransaction(TranDeleteUser, [2]byte{}, NewField(FieldChatID, []byte{0, 0, 0, 1})),
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x76},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldUserID, []byte{0, 2}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleLeaveChat(tt.args.cc, &tt.args.t)
			if !tranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleLeaveChat() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleGetUserNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name string
		args args
		want []Transaction
	}{
		{
			name: "replies with userlist transaction",
			args: args{
				cc: &ClientConn{
					ID: [2]byte{0, 1},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									ID:       [2]byte{0, 1},
									Icon:     []byte{0, 2},
									Flags:    [2]byte{0, 3},
									UserName: []byte{0, 4},
								},
								{
									ID:       [2]byte{0, 2},
									Icon:     []byte{0, 2},
									Flags:    [2]byte{0, 3},
									UserName: []byte{0, 4},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []Field{
						NewField(
							FieldUsernameWithInfo,
							[]byte{00, 01, 00, 02, 00, 03, 00, 02, 00, 04},
						),
						NewField(
							FieldUsernameWithInfo,
							[]byte{00, 02, 00, 02, 00, 03, 00, 02, 00, 04},
						),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleGetUserNameList(tt.args.cc, &tt.args.t)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandleChatSend(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name string
		args args
		want []Transaction
	}{
		{
			name: "sends chat msg transaction to all clients",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
		{
			name: "treats Chat Type 00 00 00 00 as a public chat message",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
						NewField(FieldChatID, []byte{0, 0, 0, 0}),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranChatSend, [2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			want: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to participate in chat.")),
					},
				},
			},
		},
		{
			name: "sends chat msg as emote if FieldChatOptions is set to 1",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("performed action")),
						NewField(FieldChatOptions, []byte{0x00, 0x01}),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
			},
		},
		{
			name: "does not send chat msg as emote if FieldChatOptions is set to 0",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hello")),
						NewField(FieldChatOptions, []byte{0x00, 0x00}),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
			},
		},
		{
			name: "only sends chat msg to clients with AccessReadChat permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: func() accessBitmap {
											var bits accessBitmap
											bits.Set(AccessReadChat)
											return bits
										}(),
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{},
									ID:      [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
		{
			name: "only sends private chat msg to members of private chat",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						ChatMgr: func() *MockChatManager {
							m := MockChatManager{}
							m.On("Members", ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*ClientConn{
								{
									ID: [2]byte{0, 1},
								},
								{
									ID: [2]byte{0, 2},
								},
							})
							m.On("GetSubject").Return("unset")
							return &m
						}(),
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									Account: &Account{
										Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &Account{
										Access: accessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
									},
									ID: [2]byte{0, 2},
								},
								{
									Account: &Account{
										Access: accessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
									},
									ID: [2]byte{0, 3},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
					},
				},
			},
			want: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					clientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleChatSend(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.want, got)
		})
	}
}

func TestHandleGetFileInfo(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "returns expected fields when a valid file is requested",
			args: args{
				cc: &ClientConn{
					ID: [2]byte{0x00, 0x01},
					Server: &Server{
						FS: &OSFileStore{},
						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileInfo, [2]byte{},
					NewField(FieldFileName, []byte("testfile.txt")),
					NewField(FieldFilePath, []byte{0x00, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Type:     [2]byte{0, 0},
					Fields: []Field{
						NewField(FieldFileName, []byte("testfile.txt")),
						NewField(FieldFileTypeString, []byte("Text File")),
						NewField(FieldFileCreatorString, []byte("ttxt")),
						NewField(FieldFileType, []byte("TEXT")),
						NewField(FieldFileCreateDate, make([]byte, 8)),
						NewField(FieldFileModifyDate, make([]byte, 8)),
						NewField(FieldFileSize, []byte{0x0, 0x0, 0x0, 0x17}),
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
			gotRes[0].Fields[4].Data = make([]byte, 8)
			gotRes[0].Fields[5].Data = make([]byte, 8)

			if !tranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleGetFileInfo() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleNewFolder(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder,
					[2]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create folders.")),
					},
				},
			},
		},
		{
			name: "when path is nested",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						Config: Config{
							FileRoot: "/Files/",
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "when path is not nested",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						Config: Config{
							FileRoot: "/Files",
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "when Write returns an err",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						Config: Config{
							FileRoot: "/Files/",
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Mkdir", "/Files/aaa/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/aaa/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
					NewField(FieldFilePath, []byte{
						0x00,
					}),
				),
			},
			wantRes: []Transaction{},
		},
		{
			name: "FieldFileName does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						Config: Config{
							FileRoot: "/Files/",
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Mkdir", "/Files/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder, [2]byte{0, 1},
					NewField(FieldFileName, []byte("../../testFolder")),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
		{
			name: "FieldFilePath does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateFolder)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						Config: Config{
							FileRoot: "/Files/",
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Mkdir", "/Files/foo/testFolder", fs.FileMode(0777)).Return(nil)
							mfs.On("Stat", "/Files/foo/testFolder").Return(nil, os.ErrNotExist)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranNewFolder, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
					NewField(FieldFilePath, []byte{
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
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewFolder(tt.args.cc, &tt.args.t)

			if !tranAssertEqual(t, tt.wantRes, gotRes) {
				t.Errorf("HandleNewFolder() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
		})
	}
}

func TestHandleUploadFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when request is valid and user has Upload Anywhere permission",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						FS:              &OSFileStore{},
						FileTransferMgr: NewMemFileTransferMgr(),
						Config: Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						}},
					ClientFileTransferMgr: NewClientFileTransferMgr(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessUploadFile)
							bits.Set(AccessUploadAnywhere)
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranUploadFile, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}), // rand.Seed(1)
					},
				},
			},
		},
		{
			name: "when user does not have required access",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranUploadFile, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2f,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to upload files.")), // rand.Seed(1)
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleUploadFile(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleMakeAlias(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "with valid input and required permissions",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessMakeAlias)
							return bits
						}(),
					},
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
						Logger: NewTestLogger(),
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
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
				t: NewTransaction(
					TranMakeFileAlias, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					NewField(FieldFileNewPath, EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields:  []Field(nil),
				},
			},
		},
		{
			name: "when symlink returns an error",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessMakeAlias)
							return bits
						}(),
					},
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
						Logger: NewTestLogger(),
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
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
				t: NewTransaction(
					TranMakeFileAlias, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					NewField(FieldFileNewPath, EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Error creating alias")),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: accessBitmap{},
					},
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
						},
					},
				},
				t: NewTransaction(
					TranMakeFileAlias, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2e,
					}),
					NewField(FieldFileNewPath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x2e, 0x2e, 0x2e,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to make aliases.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleMakeAlias(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when account is valid",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "guest").Return(&Account{
								Login:    "guest",
								Name:     "Guest",
								Password: "password",
								Access:   accessBitmap{},
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetUser, [2]byte{0, 1},
					NewField(FieldUserLogin, []byte("guest")),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldUserName, []byte("Guest")),
						NewField(FieldUserLogin, encodeString([]byte("guest"))),
						NewField(FieldUserPassword, []byte("password")),
						NewField(FieldUserAccess, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetUser, [2]byte{0, 1},
					NewField(FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
		},
		{
			name: "when account does not exist",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "nonExistentUser").Return((*Account)(nil))
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetUser, [2]byte{0, 1},
					NewField(FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Account does not exist.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetUser(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user exists",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDeleteUser)
							return bits
						}(),
					},
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{}) // TODO
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranDeleteUser, [2]byte{0, 1},
					NewField(FieldUserLogin, encodeString([]byte("testuser"))),
				),
			},
			wantRes: []Transaction{
				{
					Flags:   0x00,
					IsReply: 0x01,
					Type:    [2]byte{0, 0},
					Fields:  []Field(nil),
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDeleteUser, [2]byte{0, 1},
					NewField(FieldUserLogin, encodeString([]byte("testuser"))),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDeleteUser(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetMsgs(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "returns news data",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &Server{
						MessageBoard: func() *mockReadWriteSeeker {
							m := mockReadWriteSeeker{}
							m.On("Seek", int64(0), 0).Return(int64(0), nil)
							m.On("Read", mock.AnythingOfType("[]uint8")).Run(func(args mock.Arguments) {
								arg := args.Get(0).([]uint8)
								copy(arg, "TEST")
							}).Return(4, io.EOF)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldData, []byte("TEST")),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetMsgs(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranNewUser, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
		},
		{
			name: "when user attempts to create account with greater access",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessCreateUser)
							return bits
						}(),
					},
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "userB").Return((*Account)(nil))
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranNewUser, [2]byte{0, 1},
					NewField(FieldUserLogin, encodeString([]byte("userB"))),
					NewField(
						FieldUserAccess,
						func() []byte {
							var bits accessBitmap
							bits.Set(AccessDisconUser)
							return bits[:]
						}(),
					),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Cannot create account with more access than yourself.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewUser(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleListUsers(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranNewUser, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("List").Return([]Account{
								{
									Name:     "guest",
									Login:    "guest",
									Password: "zz",
									Access:   accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetClientInfoText, [2]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldData, []byte{
							0x00, 0x04, 0x00, 0x66, 0x00, 0x05, 0x67, 0x75, 0x65, 0x73, 0x74, 0x00, 0x69, 0x00, 0x05, 0x98,
							0x8a, 0x9a, 0x8c, 0x8b, 0x00, 0x6e, 0x00, 0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
							0x00, 0x6a, 0x00, 0x01, 0x78,
						}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleListUsers(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{},
				},
				t: NewTransaction(TranDownloadFile, [2]byte{0, 1}),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to download files.")),
					},
				},
			},
		},
		{
			name: "with a valid file",
			args: args{
				cc: &ClientConn{
					ClientFileTransferMgr: NewClientFileTransferMgr(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDownloadFile)
							return bits
						}(),
					},
					Server: &Server{
						FS:              &OSFileStore{},
						FileTransferMgr: NewMemFileTransferMgr(),
						Config: Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
					},
				},
				t: NewTransaction(
					TranDownloadFile,
					[2]byte{0, 1},
					NewField(FieldFileName, []byte("testfile.txt")),
					NewField(FieldFilePath, []byte{0x0, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldWaitingCount, []byte{0x00, 0x00}),
						NewField(FieldTransferSize, []byte{0x00, 0x00, 0x00, 0xa5}),
						NewField(FieldFileSize, []byte{0x00, 0x00, 0x00, 0x17}),
					},
				},
			},
		},
		{
			name: "when client requests to resume 1k test file at offset 256",
			args: args{
				cc: &ClientConn{
					ClientFileTransferMgr: NewClientFileTransferMgr(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDownloadFile)
							return bits
						}(),
					},
					Server: &Server{
						FS: &OSFileStore{},

						// FS: func() *MockFileStore {
						// 	path, _ := os.Getwd()
						// 	testFile, err := os.Open(path + "/test/config/Files/testfile-1k")
						// 	if err != nil {
						// 		panic(err)
						// 	}
						//
						// 	mfi := &MockFileInfo{}
						// 	mfi.On("Mode").Return(fs.FileMode(0))
						// 	mfs := &MockFileStore{}
						// 	mfs.On("Stat", "/fakeRoot/Files/testfile.txt").Return(mfi, nil)
						// 	mfs.On("Open", "/fakeRoot/Files/testfile.txt").Return(testFile, nil)
						// 	mfs.On("Stat", "/fakeRoot/Files/.info_testfile.txt").Return(nil, errors.New("no"))
						// 	mfs.On("Stat", "/fakeRoot/Files/.rsrc_testfile.txt").Return(nil, errors.New("no"))
						//
						// 	return mfs
						// }(),
						FileTransferMgr: NewMemFileTransferMgr(),
						Config: Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDownloadFile,
					[2]byte{0, 1},
					NewField(FieldFileName, []byte("testfile-1k")),
					NewField(FieldFilePath, []byte{0x00, 0x00}),
					NewField(
						FieldFileResumeData,
						func() []byte {
							frd := FileResumeData{
								ForkCount: [2]byte{0, 2},
								ForkInfoList: []ForkInfoList{
									{
										Fork:     [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
										DataSize: [4]byte{0, 0, 0x01, 0x00},       // request offset 256
									},
									{
										Fork:     [4]byte{0x4d, 0x41, 0x43, 0x52}, // "MACR"
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
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldWaitingCount, []byte{0x00, 0x00}),
						NewField(FieldTransferSize, []byte{0x00, 0x00, 0x03, 0x8d}),
						NewField(FieldFileSize, []byte{0x00, 0x00, 0x03, 0x00}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDownloadFile(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleUpdateUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when action is create user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return((*Account)(nil))
							return &m
						}(),
						Logger: NewTestLogger(),
					},
					Account: &Account{
						Access: accessBitmap{},
					},
				},
				t: NewTransaction(
					TranUpdateUser,
					[2]byte{0, 0},
					NewField(FieldData, []byte{
						0x00, 0x04, // field count

						0x00, 0x69, // FieldUserLogin = 105
						0x00, 0x03,
						0x9d, 0x9d, 0x9d,

						0x00, 0x6a, // FieldUserPassword = 106
						0x00, 0x03,
						0x9c, 0x9c, 0x9c,

						0x00, 0x66, // FieldUserName = 102
						0x00, 0x03,
						0x61, 0x61, 0x61,

						0x00, 0x6e, // FieldUserAccess = 110
						0x00, 0x08,
						0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
		},
		{
			name: "when action is modify user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{
						Logger: NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return(&Account{})
							return &m
						}(),
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranUpdateUser,
					[2]byte{0, 0},
					NewField(FieldData, []byte{
						0x00, 0x04, // field count

						0x00, 0x69, // FieldUserLogin = 105
						0x00, 0x03,
						0x9d, 0x9d, 0x9d,

						0x00, 0x6a, // FieldUserPassword = 106
						0x00, 0x03,
						0x9c, 0x9c, 0x9c,

						0x00, 0x66, // FieldUserName = 102
						0x00, 0x03,
						0x61, 0x61, 0x61,

						0x00, 0x6e, // FieldUserAccess = 110
						0x00, 0x08,
						0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to modify accounts.")),
					},
				},
			},
		},
		{
			name: "when action is delete user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{},
					Account: &Account{
						Access: accessBitmap{},
					},
				},
				t: NewTransaction(
					TranUpdateUser,
					[2]byte{0, 0},
					NewField(FieldData, []byte{
						0x00, 0x01,
						0x00, 0x65,
						0x00, 0x03,
						0x88, 0x9e, 0x8b,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleUpdateUser(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsArt(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news articles.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsArt(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDisconnectUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to disconnect users.")),
					},
				},
			},
		},
		{
			name: "when target user has 'cannot be disconnected' priv",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x1}).Return(&ClientConn{
								Account: &Account{
									Login: "unnamed",
									Access: func() accessBitmap {
										var bits accessBitmap
										bits.Set(AccessCannotBeDiscon)
										return bits
									}(),
								},
							},
							)
							return &m
						}(),
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDisconUser)
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsArt,
					[2]byte{0, 0},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("unnamed is not allowed to be disconnected.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDisconnectUser(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSendInstantMsg(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to send private messages.")),
					},
				},
			},
		},
		{
			name: "when client 1 sends a message to client 2",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x2}).Return(&ClientConn{
								AutoReply: []byte(nil),
								Flags:     [2]byte{0, 0},
							},
							)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					[2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				NewTransaction(
					TranServerMsg,
					[2]byte{0, 2},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserName, []byte("User1")),
					NewField(FieldUserID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field(nil),
				},
			},
		},
		{
			name: "when client 2 has autoreply enabled",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x2}).Return(&ClientConn{
								Flags:     [2]byte{0, 0},
								ID:        [2]byte{0, 2},
								UserName:  []byte("User2"),
								AutoReply: []byte("autohai"),
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					[2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				NewTransaction(
					TranServerMsg,
					[2]byte{0, 2},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserName, []byte("User1")),
					NewField(FieldUserID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				NewTransaction(
					TranServerMsg,
					[2]byte{0, 1},
					NewField(FieldData, []byte("autohai")),
					NewField(FieldUserName, []byte("User2")),
					NewField(FieldUserID, []byte{0, 2}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field(nil),
				},
			},
		},
		{
			name: "when client 2 has refuse private messages enabled",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x2}).Return(&ClientConn{
								Flags:    [2]byte{255, 255},
								ID:       [2]byte{0, 2},
								UserName: []byte("User2"),
							},
							)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					[2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				NewTransaction(
					TranServerMsg,
					[2]byte{0, 1},
					NewField(FieldData, []byte("User2 does not accept private messages.")),
					NewField(FieldUserName, []byte("User2")),
					NewField(FieldUserID, []byte{0, 2}),
					NewField(FieldOptions, []byte{0, 2}),
				),
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field(nil),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleSendInstantMsg(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission to delete a folder",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								return "/fakeRoot/Files"
							}(),
						},
						FS: func() *MockFileStore {
							mfi := &MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &MockFileStore{}
							mfs.On("Stat", "/fakeRoot/Files/aaa/testfile").Return(mfi, nil)
							mfs.On("Stat", "/fakeRoot/Files/aaa/.info_testfile").Return(nil, errors.New("err"))
							mfs.On("Stat", "/fakeRoot/Files/aaa/.rsrc_testfile").Return(nil, errors.New("err"))

							return mfs
						}(),
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDeleteFile, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testfile")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete files.")),
					},
				},
			},
		},
		{
			name: "deletes all associated metadata files",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDeleteFile)
							return bits
						}(),
					},
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								return "/fakeRoot/Files"
							}(),
						},
						FS: func() *MockFileStore {
							mfi := &MockFileInfo{}
							mfi.On("Mode").Return(fs.FileMode(0))
							mfi.On("Size").Return(int64(100))
							mfi.On("ModTime").Return(time.Parse(time.Layout, time.Layout))
							mfi.On("IsDir").Return(false)
							mfi.On("Name").Return("testfile")

							mfs := &MockFileStore{}
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
				t: NewTransaction(
					TranDeleteFile, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testfile")),
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x03,
						0x61, 0x61, 0x61,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields:  []Field(nil),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDeleteFile(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)

			tt.args.cc.Server.FS.(*MockFileStore).AssertExpectations(t)
		})
	}
}

func TestHandleGetFileNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when FieldFilePath is a drop box, but user does not have AccessViewDropBoxes ",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{

						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileNameList, [2]byte{0, 1},
					NewField(FieldFilePath, []byte{
						0x00, 0x01,
						0x00, 0x00,
						0x08,
						0x64, 0x72, 0x6f, 0x70, 0x20, 0x62, 0x6f, 0x78, // "drop box"
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view drop boxes.")),
					},
				},
			},
		},
		{
			name: "with file root",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						Config: Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileNameList, [2]byte{0, 1},
					NewField(FieldFilePath, []byte{
						0x00, 0x00,
						0x00, 0x00,
					}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(
							FieldFileNameWithInfo,
							func() []byte {
								fnwi := FileNameWithInfo{
									fileNameWithInfoHeader: fileNameWithInfoHeader{
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
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetClientInfoText(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetClientInfoText, [2]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to get client info.")),
					},
				},
			},
		},
		{
			name: "with a valid user",
			args: args{
				cc: &ClientConn{
					UserName:   []byte("Testy McTest"),
					RemoteAddr: "1.2.3.4:12345",
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessGetClientInfo)
							return bits
						}(),
						Name:  "test",
						Login: "test",
					},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x1}).Return(&ClientConn{
								UserName:   []byte("Testy McTest"),
								RemoteAddr: "1.2.3.4:12345",
								Account: &Account{
									Access: func() accessBitmap {
										var bits accessBitmap
										bits.Set(AccessGetClientInfo)
										return bits
									}(),
									Name:  "test",
									Login: "test",
								},
							},
							)
							return &m
						}(),
					},
					ClientFileTransferMgr: ClientFileTransferMgr{},
				},
				t: NewTransaction(
					TranGetClientInfoText, [2]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
					Fields: []Field{
						NewField(FieldData, []byte(
							strings.ReplaceAll(`Nickname:   Testy McTest
Name:       test
Account:    test
Address:    1.2.3.4:12345

-------- File Downloads ---------

None.

------- Folder Downloads --------

None.

--------- File Uploads ----------

None.

-------- Folder Uploads ---------

None.

------- Waiting Downloads -------

None.

`, "\n", "\r")),
						),
						NewField(FieldUserName, []byte("Testy McTest")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetClientInfoText(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranAgreed(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "normal request flow",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessDisconUser)
							bits.Set(AccessAnyName)
							return bits
						}()},
					Icon:    []byte{0, 1},
					Flags:   [2]byte{0, 1},
					Version: []byte{0, 1},
					ID:      [2]byte{0, 1},
					logger:  NewTestLogger(),
					Server: &Server{
						Config: Config{
							BannerFile: "banner.jpg",
						},
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								//{
								//	ID:       [2]byte{0, 2},
								//	UserName: []byte("UserB"),
								//},
							},
							)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranAgreed, [2]byte{},
					NewField(FieldUserName, []byte("username")),
					NewField(FieldUserIconID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 0}),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x7a},
					Fields: []Field{
						NewField(FieldBannerType, []byte("JPEG")),
					},
				},
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleTranAgreed(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSetClientUserInfo(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when client does not have AccessAnyName",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("Guest"),
					Flags:    [2]byte{0, 1},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{
								{
									ID: [2]byte{0, 1},
								},
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranSetClientUserInfo, [2]byte{},
					NewField(FieldUserIconID, []byte{0, 1}),
					NewField(FieldUserName, []byte("NOPE")),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []Field{
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 1}),
						NewField(FieldUserName, []byte("Guest"))},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleSetClientUserInfo(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsItem(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have permission to delete a news category",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(NewsCategoryListData15{
								Type: NewsCategory,
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsItem, [2]byte{},
					NewField(FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news categories.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to delete a news folder",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(NewsCategoryListData15{
								Type: NewsBundle,
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsItem, [2]byte{},
					NewField(FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news folders.")),
					},
				},
			},
		},
		{
			name: "when user deletes a news folder",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsDeleteFldr)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(NewsCategoryListData15{Type: NewsBundle})
							m.On("DeleteNewsItem", []string{"test"}).Return(nil)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsItem, [2]byte{},
					NewField(FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsItem(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranOldPostNews(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
				},
				t: NewTransaction(
					TranOldPostNews, [2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to post news.")),
					},
				},
			},
		},
		{
			name: "when user posts news update",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsPostArt)
							return bits
						}(),
					},
					Server: &Server{
						Config: Config{
							NewsDateFormat: "",
						},
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("List").Return([]*ClientConn{})
							return &m
						}(),
						MessageBoard: func() *mockReadWriteSeeker {
							m := mockReadWriteSeeker{}
							m.On("Seek", int64(0), 0).Return(int64(0), nil)
							m.On("Read", mock.AnythingOfType("[]uint8")).Run(func(args mock.Arguments) {
								arg := args.Get(0).([]uint8)
								copy(arg, "TEST")
							}).Return(4, io.EOF)
							m.On("Write", mock.AnythingOfType("[]uint8")).Return(3, nil)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranOldPostNews, [2]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleTranOldPostNews(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleInviteNewChat(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(TranInviteNewChat, [2]byte{0, 1}),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to request private chat.")),
					},
				},
			},
		},
		{
			name: "when userA invites userB to new private chat",
			args: args{
				cc: &ClientConn{
					ID: [2]byte{0, 1},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    [2]byte{0, 0},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0x0, 0x2}).Return(&ClientConn{
								ID:       [2]byte{0, 2},
								UserName: []byte("UserB"),
							})
							return &m
						}(),
						ChatMgr: func() *MockChatManager {
							m := MockChatManager{}
							m.On("New", mock.AnythingOfType("*hotline.ClientConn")).Return(ChatID{0x52, 0xfd, 0xfc, 0x07})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranInviteNewChat, [2]byte{0, 1},
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x71},
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
					},
				},
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 0}),
					},
				},
			},
		},
		{
			name: "when userA invites userB to new private chat, but UserB has refuse private chat enabled",
			args: args{
				cc: &ClientConn{
					ID: [2]byte{0, 1},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    [2]byte{0, 0},
					Server: &Server{
						ClientMgr: func() *MockClientMgr {
							m := MockClientMgr{}
							m.On("Get", ClientID{0, 2}).Return(&ClientConn{
								ID:       [2]byte{0, 2},
								Icon:     []byte{0, 1},
								UserName: []byte("UserB"),
								Flags:    [2]byte{255, 255},
							})
							return &m
						}(),
						ChatMgr: func() *MockChatManager {
							m := MockChatManager{}
							m.On("New", mock.AnythingOfType("*hotline.ClientConn")).Return(ChatID{0x52, 0xfd, 0xfc, 0x07})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranInviteNewChat, [2]byte{0, 1},
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x68},
					Fields: []Field{
						NewField(FieldData, []byte("UserB does not accept private chats.")),
						NewField(FieldUserName, []byte("UserB")),
						NewField(FieldUserID, []byte{0, 2}),
						NewField(FieldOptions, []byte{0, 2}),
					},
				},
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 0}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotRes := HandleInviteNewChat(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtData(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{Account: &Account{}},
				t: NewTransaction(
					TranGetNewsArtData, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("GetArticle", []string{"Example Category"}, uint32(1)).Return(&NewsArtData{
								Title:         "title",
								Poster:        "poster",
								Date:          [8]byte{},
								PrevArt:       [4]byte{0, 0, 0, 1},
								NextArt:       [4]byte{0, 0, 0, 2},
								ParentArt:     [4]byte{0, 0, 0, 3},
								FirstChildArt: [4]byte{0, 0, 0, 4},
								DataFlav:      []byte("text/plain"),
								Data:          "article data",
							})
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetNewsArtData, [2]byte{0, 1},
					NewField(FieldNewsPath, []byte{
						// Example Category
						0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
					}),
					NewField(FieldNewsArtID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply: 1,
					Fields: []Field{
						NewField(FieldNewsArtTitle, []byte("title")),
						NewField(FieldNewsArtPoster, []byte("poster")),
						NewField(FieldNewsArtDate, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
						NewField(FieldNewsArtPrevArt, []byte{0, 0, 0, 1}),
						NewField(FieldNewsArtNextArt, []byte{0, 0, 0, 2}),
						NewField(FieldNewsArtParentArt, []byte{0, 0, 0, 3}),
						NewField(FieldNewsArt1stChildArt, []byte{0, 0, 0, 4}),
						NewField(FieldNewsArtDataFlav, []byte("text/plain")),
						NewField(FieldNewsArtData, []byte("article data")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetNewsArtData(tt.args.cc, &tt.args.t)
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		//{
		//	name: "when user has required access",
		//	args: args{
		//		cc: &ClientConn{
		//			Account: &Account{
		//				Access: func() accessBitmap {
		//					var bits accessBitmap
		//					bits.Set(AccessNewsReadArt)
		//					return bits
		//				}(),
		//			},
		//			Server: &Server{
		//				ThreadedNewsMgr: func() *mockThreadNewsMgr {
		//					m := mockThreadNewsMgr{}
		//					m.On("ListArticles", []string{"Example Category"}).Return(NewsArtListData{
		//						Name:        []byte("testTitle"),
		//						NewsArtList: []byte{},
		//					})
		//					return &m
		//				}(),
		//			},
		//		},
		//		t: NewTransaction(
		//			TranGetNewsArtNameList,
		//			[2]byte{0, 1},
		//			//  00000000  00 01 00 00 10 45 78 61  6d 70 6c 65 20 43 61 74  |.....Example Cat|
		//			//  00000010  65 67 6f 72 79                                    |egory|
		//			NewField(FieldNewsPath, []byte{
		//				0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
		//			}),
		//		),
		//	},
		//	wantRes: []Transaction{
		//		{
		//			IsReply: 0x01,
		//			Fields: []Field{
		//				NewField(FieldNewsArtListData, []byte{
		//					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
		//					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		//					0x09, 0x74, 0x65, 0x73, 0x74, 0x54, 0x69, 0x74, 0x6c, 0x65, 0x0a, 0x74, 0x65, 0x73, 0x74, 0x50,
		//					0x6f, 0x73, 0x74, 0x65, 0x72, 0x0a, 0x74, 0x65, 0x78, 0x74, 0x2f, 0x70, 0x6c, 0x61, 0x69, 0x6e,
		//					0x00, 0x08,
		//				},
		//				),
		//			},
		//		},
		//	},
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetNewsArtNameList(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewNewsFldr(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create news folders.")),
					},
				},
			},
		},
		{
			name: "with a valid request",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsCreateFldr)
							return bits
						}(),
					},
					logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "testFolder", NewsBundle).Return(nil)
							return &m
						}(),
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, [2]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
					NewField(FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []Transaction{
				{
					clientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []Field{},
				},
			},
		},
		//{
		//	Name: "when there is an error writing the threaded news file",
		//	args: args{
		//		cc: &ClientConn{
		//			Account: &Account{
		//				Access: func() accessBitmap {
		//					var bits accessBitmap
		//					bits.Set(AccessNewsCreateFldr)
		//					return bits
		//				}(),
		//			},
		//			logger: NewTestLogger(),
		//			Type:     [2]byte{0, 1},
		//			Server: &Server{
		//				ConfigDir: "/fakeConfigRoot",
		//				FS: func() *MockFileStore {
		//					mfs := &MockFileStore{}
		//					mfs.On("WriteFile", "/fakeConfigRoot/ThreadedNews.yaml", mock.Anything, mock.Anything).Return(os.ErrNotExist)
		//					return mfs
		//				}(),
		//				ThreadedNews: &ThreadedNews{Categories: map[string]NewsCategoryListData15{
		//					"test": {
		//						Type:     []byte{0, 2},
		//						Count:    nil,
		//						NameSize: 0,
		//						Name:     "test",
		//						SubCats:  make(map[string]NewsCategoryListData15),
		//					},
		//				}},
		//			},
		//		},
		//		t: NewTransaction(
		//			TranGetNewsArtNameList, [2]byte{0, 1},
		//			NewField(FieldFileName, []byte("testFolder")),
		//			NewField(FieldNewsPath,
		//				[]byte{
		//					0, 1,
		//					0, 0,
		//					4,
		//					0x74, 0x65, 0x73, 0x74,
		//				},
		//			),
		//		),
		//	},
		//	wantRes: []Transaction{
		//		{
		//			clientID:  [2]byte{0, 1},
		//			Flags:     0x00,
		//			IsReply:   0x01,
		//			Type:      [2]byte{0, 0},
		//			ErrorCode: [4]byte{0, 0, 0, 1},
		//			Fields: []Field{
		//				NewField(FieldError, []byte("Error creating news folder.")),
		//			},
		//		},
		//	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewNewsFldr(tt.args.cc, &tt.args.t)

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadBanner(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDownloadBanner(tt.args.cc, &tt.args.t)

			assert.Equalf(t, tt.wantRes, gotRes, "HandleDownloadBanner(%v, %v)", tt.args.cc, &tt.args.t)
		})
	}
}

func TestHandlePostNewsArt(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranPostNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to post news articles.")),
					},
				},
			},
		},
		{
			name: "with required permission",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						ThreadedNewsMgr: func() *mockThreadNewsMgr {
							m := mockThreadNewsMgr{}
							m.On("PostArticle", []string{"www"}, uint32(0), mock.AnythingOfType("hotline.NewsArtData")).Return(nil)
							return &m
						}(),
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(AccessNewsPostArt)
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranPostNewsArt,
					[2]byte{0, 0},
					NewField(FieldNewsPath, []byte{0x00, 0x01, 0x00, 0x00, 0x03, 0x77, 0x77, 0x77}),
					NewField(FieldNewsArtID, []byte{0x00, 0x00, 0x00, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 0},
					Fields:    []Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tranAssertEqual(t, tt.wantRes, HandlePostNewsArt(tt.args.cc, &tt.args.t))
		})
	}
}
