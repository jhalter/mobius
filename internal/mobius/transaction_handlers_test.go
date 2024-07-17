package mobius

import (
	"cmp"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type mockReadWriteSeeker struct {
	mock.Mock
}

func (m *mockReadWriteSeeker) Read(p []byte) (int, error) {
	args := m.Called(p)

	return args.Int(0), args.Error(1)
}

func (m *mockReadWriteSeeker) Write(p []byte) (int, error) {
	args := m.Called(p)

	return args.Int(0), args.Error(1)
}

func (m *mockReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	args := m.Called(offset, whence)

	return args.Get(0).(int64), args.Error(1)
}

func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

// assertTransferBytesEqual takes a string with a hexdump in the same format that `hexdump -C` produces and compares with
// a hexdump for the bytes in got, after stripping the create/modify timestamps.
// I don't love this, but as git does not  preserve file create/modify timestamps, we either need to fully mock the
// filesystem interactions or work around in this way.
// TODO: figure out a better solution
func assertTransferBytesEqual(t *testing.T, wantHexDump string, got []byte) bool {
	if wantHexDump == "" {
		return true
	}

	clean := slices.Concat(
		got[:92],
		make([]byte, 16),
		got[108:],
	)
	return assert.Equal(t, wantHexDump, hex.Dump(clean))
}

var tranSortFunc = func(a, b hotline.Transaction) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ClientID[:]),
		binary.BigEndian.Uint16(b.ClientID[:]),
	)
}

// TranAssertEqual compares equality of transactions slices after stripping out the random transaction Type
func TranAssertEqual(t *testing.T, tran1, tran2 []hotline.Transaction) bool {
	var newT1 []hotline.Transaction
	var newT2 []hotline.Transaction

	for _, trans := range tran1 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []hotline.Field
		for _, field := range trans.Fields {
			if field.Type == hotline.FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == hotline.FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []hotline.Field
		for _, field := range trans.Fields {
			if field.Type == hotline.FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == hotline.FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT2 = append(newT2, trans)
	}

	slices.SortFunc(newT1, tranSortFunc)
	slices.SortFunc(newT2, tranSortFunc)

	return assert.Equal(t, newT1, newT2)
}

func TestHandleSetChatSubject(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name string
		args args
		want []hotline.Transaction
	}{
		{
			name: "sends chat subject to private chat members",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte{0x00, 0x01},
					Server: &hotline.Server{
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							m.On("Members", hotline.ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							})
							m.On("SetSubject", hotline.ChatID{0x0, 0x0, 0x0, 0x1}, "Test Subject")
							return &m
						}(),
						//PrivateChats: map[[4]byte]*PrivateChat{
						//	[4]byte{0, 0, 0, 1}: {
						//		Subject: "unset",
						//		ClientConn: map[[2]byte]*ClientConn{
						//			[2]byte{0, 1}: {
						//				Account: &hotline.Account{
						//					Access: AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
						//				},
						//				ID: [2]byte{0, 1},
						//			},
						//			[2]byte{0, 2}: {
						//				Account: &hotline.Account{
						//					Access: AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
						//				},
						//				ID: [2]byte{0, 2},
						//			},
						//		},
						//	},
						//},
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x6a},
					ID:   [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldChatSubject, []byte("Test Subject")),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x77},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldChatSubject, []byte("Test Subject")),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x77},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldChatSubject, []byte("Test Subject")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleSetChatSubject(tt.args.cc, &tt.args.t)
			if !TranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleSetChatSubject() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleLeaveChat(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name string
		args args
		want []hotline.Transaction
	}{
		{
			name: "when client 2 leaves chat",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 2},
					Server: &hotline.Server{
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							m.On("Members", hotline.ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
							})
							m.On("Leave", hotline.ChatID{0x0, 0x0, 0x0, 0x1}, [2]uint8{0x0, 0x2})
							m.On("GetSubject").Return("unset")
							return &m
						}(),
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(hotline.TranDeleteUser, [2]byte{}, hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1})),
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x76},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleLeaveChat(tt.args.cc, &tt.args.t)
			if !TranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleLeaveChat() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleGetUserNameList(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name string
		args args
		want []hotline.Transaction
	}{
		{
			name: "replies with userlist transaction",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
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
				t: hotline.Transaction{},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []hotline.Field{
						hotline.NewField(
							hotline.FieldUsernameWithInfo,
							[]byte{00, 01, 00, 02, 00, 03, 00, 02, 00, 04},
						),
						hotline.NewField(
							hotline.FieldUsernameWithInfo,
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
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name string
		args args
		want []hotline.Transaction
	}{
		{
			name: "sends chat msg transaction to all clients",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("hai")),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
		{
			name: "treats Chat Type 00 00 00 00 as a public chat message",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("hai")),
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 0}),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranChatSend, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
				),
			},
			want: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to participate in chat.")),
					},
				},
			},
		},
		{
			name: "sends chat msg as emote if FieldChatOptions is set to 1",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("performed action")),
						hotline.NewField(hotline.FieldChatOptions, []byte{0x00, 0x01}),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Flags:    0x00,
					IsReply:  0x00,
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
			},
		},
		{
			name: "does not send chat msg as emote if FieldChatOptions is set to 0",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("hello")),
						hotline.NewField(hotline.FieldChatOptions, []byte{0x00, 0x00}),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
			},
		},
		{
			name: "only sends chat msg to clients with AccessReadChat permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: func() hotline.AccessBitmap {
											var bits hotline.AccessBitmap
											bits.Set(hotline.AccessReadChat)
											return bits
										}(),
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{},
									ID:      [2]byte{0, 2},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("hai")),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
		{
			name: "only sends private chat msg to members of private chat",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &hotline.Server{
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							m.On("Members", hotline.ChatID{0x0, 0x0, 0x0, 0x1}).Return([]*hotline.ClientConn{
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
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
									},
									ID: [2]byte{0, 1},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
									},
									ID: [2]byte{0, 2},
								},
								{
									Account: &hotline.Account{
										Access: hotline.AccessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
									},
									ID: [2]byte{0, 3},
								},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("hai")),
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
					},
				},
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleChatSend(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.want, got)
		})
	}
}

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
					ID: [2]byte{0x00, 0x01},
					Server: &hotline.Server{
						FS: &hotline.OSFileStore{},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetFileInfo(tt.args.cc, &tt.args.t)

			// Clear the file timestamp fields to work around problems running the tests in multiple timezones
			// TODO: revisit how to test this by mocking the stat calls
			gotRes[0].Fields[4].Data = make([]byte, 8)
			gotRes[0].Fields[5].Data = make([]byte, 8)

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
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
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
			wantRes: []hotline.Transaction{},
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

func TestHandleGetUser(t *testing.T) {
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
			name: "when account is valid",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessOpenUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "guest").Return(&hotline.Account{
								Login:    "guest",
								Name:     "Guest",
								Password: "password",
								Access:   hotline.AccessBitmap{},
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, []byte("guest")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("Guest")),
						hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("guest"))),
						hotline.NewField(hotline.FieldUserPassword, []byte("password")),
						hotline.NewField(hotline.FieldUserAccess, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
					},
				},
			},
		},
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
		},
		{
			name: "when account does not exist",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessOpenUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "nonExistentUser").Return((*hotline.Account)(nil))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Account does not exist.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetUser(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteUser(t *testing.T) {
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
			name: "when user exists",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{}) // TODO
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDeleteUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("testuser"))),
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:   0x00,
					IsReply: 0x01,
					Type:    [2]byte{0, 0},
					Fields:  []hotline.Field(nil),
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDeleteUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("testuser"))),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDeleteUser(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetMsgs(t *testing.T) {
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
			name: "returns news data",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
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
				t: hotline.NewTransaction(
					hotline.TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("TEST")),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetMsgs(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewUser(t *testing.T) {
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewUser, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
		},
		{
			name: "when user attempts to create account with greater access",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "userB").Return((*hotline.Account)(nil))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("userB"))),
					hotline.NewField(
						hotline.FieldUserAccess,
						func() []byte {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDisconUser)
							return bits[:]
						}(),
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Cannot create account with more access than yourself.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewUser(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleListUsers(t *testing.T) {
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewUser, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessOpenUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("List").Return([]hotline.Account{
								{
									Name:     "guest",
									Login:    "guest",
									Password: "zz",
									Access:   hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetClientInfoText, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte{
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

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
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
					Server: &hotline.Server{},
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
						FS: &hotline.OSFileStore{},

						// FS: func() *hotline.MockFileStore {
						// 	path, _ := os.Getwd()
						// 	testFile, err := os.Open(path + "/test/config/Files/testfile-1k")
						// 	if err != nil {
						// 		panic(err)
						// 	}
						//
						// 	mfi := &hotline.MockFileInfo{}
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

func TestHandleUpdateUser(t *testing.T) {
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
			name: "when action is create user without required permission",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return((*hotline.Account)(nil))
							return &m
						}(),
						Logger: NewTestLogger(),
					},
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
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
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
		},
		{
			name: "when action is modify user without required permission",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						Logger: NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return(&hotline.Account{})
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
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
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to modify accounts.")),
					},
				},
			},
		},
		{
			name: "when action is delete user without required permission",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{},
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
						0x00, 0x01,
						0x00, 0x65,
						0x00, 0x03,
						0x88, 0x9e, 0x8b,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleUpdateUser(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsArt(t *testing.T) {
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
					hotline.TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news articles.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsArt(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDisconnectUser(t *testing.T) {
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
					hotline.TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to disconnect users.")),
					},
				},
			},
		},
		{
			name: "when target user has 'cannot be disconnected' priv",
			args: args{
				cc: &hotline.ClientConn{
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x1}).Return(&hotline.ClientConn{
								Account: &hotline.Account{
									Login: "unnamed",
									Access: func() hotline.AccessBitmap {
										var bits hotline.AccessBitmap
										bits.Set(hotline.AccessCannotBeDiscon)
										return bits
									}(),
								},
							},
							)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDisconUser)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsArt,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("unnamed is not allowed to be disconnected.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDisconnectUser(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSendInstantMsg(t *testing.T) {
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
					hotline.TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to send private messages.")),
					},
				},
			},
		},
		{
			name: "when client 1 sends a message to client 2",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x2}).Return(&hotline.ClientConn{
								AutoReply: []byte(nil),
								Flags:     [2]byte{0, 0},
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSendInstantMsg,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				hotline.NewTransaction(
					hotline.TranServerMsg,
					[2]byte{0, 2},
					hotline.NewField(hotline.FieldData, []byte("hai")),
					hotline.NewField(hotline.FieldUserName, []byte("User1")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
				),
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field(nil),
				},
			},
		},
		{
			name: "when client 2 has autoreply enabled",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x2}).Return(&hotline.ClientConn{
								Flags:     [2]byte{0, 0},
								ID:        [2]byte{0, 2},
								UserName:  []byte("User2"),
								AutoReply: []byte("autohai"),
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSendInstantMsg,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				hotline.NewTransaction(
					hotline.TranServerMsg,
					[2]byte{0, 2},
					hotline.NewField(hotline.FieldData, []byte("hai")),
					hotline.NewField(hotline.FieldUserName, []byte("User1")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
				),
				hotline.NewTransaction(
					hotline.TranServerMsg,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("autohai")),
					hotline.NewField(hotline.FieldUserName, []byte("User2")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
				),
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field(nil),
				},
			},
		},
		{
			name: "when client 2 has refuse private messages enabled",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessSendPrivMsg)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("User1"),
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x2}).Return(&hotline.ClientConn{
								Flags:    [2]byte{255, 255},
								ID:       [2]byte{0, 2},
								UserName: []byte("User2"),
							},
							)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSendInstantMsg,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				hotline.NewTransaction(
					hotline.TranServerMsg,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("User2 does not accept private messages.")),
					hotline.NewField(hotline.FieldUserName, []byte("User2")),
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
				),
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field(nil),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleSendInstantMsg(tt.args.cc, &tt.args.t)
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
					Server: &hotline.Server{
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

func TestHandleGetClientInfoText(t *testing.T) {
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetClientInfoText, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to get client info.")),
					},
				},
			},
		},
		{
			name: "with a valid user",
			args: args{
				cc: &hotline.ClientConn{
					UserName:   []byte("Testy McTest"),
					RemoteAddr: "1.2.3.4:12345",
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessGetClientInfo)
							return bits
						}(),
						Name:  "test",
						Login: "test",
					},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x1}).Return(&hotline.ClientConn{
								UserName:   []byte("Testy McTest"),
								RemoteAddr: "1.2.3.4:12345",
								Account: &hotline.Account{
									Access: func() hotline.AccessBitmap {
										var bits hotline.AccessBitmap
										bits.Set(hotline.AccessGetClientInfo)
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
					ClientFileTransferMgr: hotline.ClientFileTransferMgr{},
				},
				t: hotline.NewTransaction(
					hotline.TranGetClientInfoText, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte(
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
						hotline.NewField(hotline.FieldUserName, []byte("Testy McTest")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetClientInfoText(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranAgreed(t *testing.T) {
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
			name: "normal request flow",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDisconUser)
							bits.Set(hotline.AccessAnyName)
							return bits
						}()},
					Icon:    []byte{0, 1},
					Flags:   [2]byte{0, 1},
					Version: []byte{0, 1},
					ID:      [2]byte{0, 1},
					Logger:  NewTestLogger(),
					Server: &hotline.Server{
						Config: hotline.Config{
							BannerFile: "Banner.jpg",
						},
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
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
				t: hotline.NewTransaction(
					hotline.TranAgreed, [2]byte{},
					hotline.NewField(hotline.FieldUserName, []byte("username")),
					hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 0}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x7a},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldBannerType, []byte("JPEG")),
					},
				},
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleTranAgreed(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSetClientUserInfo(t *testing.T) {
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
			name: "when client does not have AccessAnyName",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("Guest"),
					Flags:    [2]byte{0, 1},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									ID: [2]byte{0, 1},
								},
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetClientUserInfo, [2]byte{},
					hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
					hotline.NewField(hotline.FieldUserName, []byte("NOPE")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserName, []byte("Guest"))},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleSetClientUserInfo(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsItem(t *testing.T) {
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
			name: "when user does not have permission to delete a news category",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{
								Type: hotline.NewsCategory,
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news categories.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to delete a news folder",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{
								Type: hotline.NewsBundle,
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news folders.")),
					},
				},
			},
		},
		{
			name: "when user deletes a news folder",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsDeleteFldr)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{Type: hotline.NewsBundle})
							m.On("DeleteNewsItem", []string{"test"}).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsItem(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranOldPostNews(t *testing.T) {
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
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranOldPostNews, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to post news.")),
					},
				},
			},
		},
		{
			name: "when user posts news update",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsPostArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						Config: hotline.Config{
							NewsDateFormat: "",
						},
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{})
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
				t: hotline.NewTransaction(
					hotline.TranOldPostNews, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleTranOldPostNews(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleInviteNewChat(t *testing.T) {
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
				},
				t: hotline.NewTransaction(hotline.TranInviteNewChat, [2]byte{0, 1}),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to request private chat.")),
					},
				},
			},
		},
		{
			name: "when userA invites userB to new private chat",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    [2]byte{0, 0},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x2}).Return(&hotline.ClientConn{
								ID:       [2]byte{0, 2},
								UserName: []byte("UserB"),
							})
							return &m
						}(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							m.On("New", mock.AnythingOfType("*hotline.ClientConn")).Return(hotline.ChatID{0x52, 0xfd, 0xfc, 0x07})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranInviteNewChat, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x71},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						hotline.NewField(hotline.FieldUserName, []byte("UserA")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					},
				},
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						hotline.NewField(hotline.FieldUserName, []byte("UserA")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 0}),
					},
				},
			},
		},
		{
			name: "when userA invites userB to new private chat, but UserB has refuse private chat enabled",
			args: args{
				cc: &hotline.ClientConn{
					ID: [2]byte{0, 1},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    [2]byte{0, 0},
					Server: &hotline.Server{
						ClientMgr: func() *hotline.MockClientMgr {
							m := hotline.MockClientMgr{}
							m.On("Get", hotline.ClientID{0, 2}).Return(&hotline.ClientConn{
								ID:       [2]byte{0, 2},
								Icon:     []byte{0, 1},
								UserName: []byte("UserB"),
								Flags:    [2]byte{255, 255},
							})
							return &m
						}(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							m.On("New", mock.AnythingOfType("*hotline.ClientConn")).Return(hotline.ChatID{0x52, 0xfd, 0xfc, 0x07})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranInviteNewChat, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x68},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("UserB does not accept private chats.")),
						hotline.NewField(hotline.FieldUserName, []byte("UserB")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 2}),
						hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
					},
				},
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						hotline.NewField(hotline.FieldUserName, []byte("UserA")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 0}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotRes := HandleInviteNewChat(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtData(t *testing.T) {
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
				cc: &hotline.ClientConn{Account: &hotline.Account{}},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtData, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("GetArticle", []string{"Example Category"}, uint32(1)).Return(&hotline.NewsArtData{
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
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtData, [2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsPath, []byte{
						// Example Category
						0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
					}),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 1,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldNewsArtTitle, []byte("title")),
						hotline.NewField(hotline.FieldNewsArtPoster, []byte("poster")),
						hotline.NewField(hotline.FieldNewsArtDate, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
						hotline.NewField(hotline.FieldNewsArtPrevArt, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldNewsArtNextArt, []byte{0, 0, 0, 2}),
						hotline.NewField(hotline.FieldNewsArtParentArt, []byte{0, 0, 0, 3}),
						hotline.NewField(hotline.FieldNewsArt1stChildArt, []byte{0, 0, 0, 4}),
						hotline.NewField(hotline.FieldNewsArtDataFlav, []byte("text/plain")),
						hotline.NewField(hotline.FieldNewsArtData, []byte("article data")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetNewsArtData(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtNameList(t *testing.T) {
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		//{
		//	name: "when user has required access",
		//	args: args{
		//		cc: &hotline.ClientConn{
		//			Account: &hotline.Account{
		//				Access: func() hotline.AccessBitmap {
		//					var bits hotline.AccessBitmap
		//					bits.Set(hotline.AccessNewsReadArt)
		//					return bits
		//				}(),
		//			},
		//			Server: &hotline.Server{
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
		//			NewField(hotline.FieldNewsPath, []byte{
		//				0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
		//			}),
		//		),
		//	},
		//	wantRes: []hotline.Transaction{
		//		{
		//			IsReply: 0x01,
		//			Fields: []hotline.Field{
		//				NewField(hotline.FieldNewsArtListData, []byte{
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

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewNewsFldr(t *testing.T) {
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
					Server: &hotline.Server{
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to create news folders.")),
					},
				},
			},
		},
		{
			name: "with a valid request",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsCreateFldr)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "testFolder", hotline.NewsBundle).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
		//{
		//	Name: "when there is an error writing the threaded news file",
		//	args: args{
		//		cc: &hotline.ClientConn{
		//			Account: &hotline.Account{
		//				Access: func() hotline.AccessBitmap {
		//					var bits hotline.AccessBitmap
		//					bits.Set(hotline.AccessNewsCreateFldr)
		//					return bits
		//				}(),
		//			},
		//			logger: NewTestLogger(),
		//			Type:     [2]byte{0, 1},
		//			Server: &hotline.Server{
		//				ConfigDir: "/fakeConfigRoot",
		//				FS: func() *hotline.MockFileStore {
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
		//			NewField(hotline.FieldFileName, []byte("testFolder")),
		//			NewField(hotline.FieldNewsPath,
		//				[]byte{
		//					0, 1,
		//					0, 0,
		//					4,
		//					0x74, 0x65, 0x73, 0x74,
		//				},
		//			),
		//		),
		//	},
		//	wantRes: []hotline.Transaction{
		//		{
		//			ClientID:  [2]byte{0, 1},
		//			Flags:     0x00,
		//			IsReply:   0x01,
		//			Type:      [2]byte{0, 0},
		//			ErrorCode: [4]byte{0, 0, 0, 1},
		//			Fields: []hotline.Field{
		//				NewField(hotline.FieldError, []byte("Error creating news folder.")),
		//			},
		//		},
		//	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewNewsFldr(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadBanner(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
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
					hotline.TranPostNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to post news articles.")),
					},
				},
			},
		},
		{
			name: "with required permission",
			args: args{
				cc: &hotline.ClientConn{
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hotline.MockThreadNewsMgr {
							m := hotline.MockThreadNewsMgr{}
							m.On("PostArticle", []string{"www"}, uint32(0), mock.AnythingOfType("hotline.NewsArtData")).Return(nil)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsPostArt)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranPostNewsArt,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldNewsPath, []byte{0x00, 0x01, 0x00, 0x00, 0x03, 0x77, 0x77, 0x77}),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0x00, 0x00, 0x00, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 0},
					Fields:    []hotline.Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TranAssertEqual(t, tt.wantRes, HandlePostNewsArt(tt.args.cc, &tt.args.t))
		})
	}
}
