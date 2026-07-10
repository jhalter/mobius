package mobius

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/hotline/hltest"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/encoding/charmap"
)

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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
		{
			name: "when user is currently connected",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									ID: [2]byte{0, 2},
									Account: &hotline.Account{
										Login: "testuser",
									},
									Connection: nopReadWriteCloser{},
									Server: &hotline.Server{
										ClientMgr: hotline.NewMemClientMgr(),
										Logger:    NewTestLogger(),
									},
								},
							})
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
					ClientID: [2]byte{0, 2},
					Type:     hotline.TranServerMsg,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
						hotline.NewField(hotline.FieldChatOptions, []byte{2}),
					},
				},
				{
					Flags:   0x00,
					IsReply: 0x01,
					Type:    [2]byte{0, 0},
					Fields:  []hotline.Field(nil),
				},
			},
		},
		{
			name: "when no matching connected clients",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteUser)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									ID: [2]byte{0, 3},
									Account: &hotline.Account{
										Login: "otheruser",
									},
								},
							})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDeleteUser(tt.args.cc, &tt.args.t)
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
		{
			name: "when account is created successfully",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							for i := 0; i < 64; i++ {
								bits.Set(i)
							}
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "userB").Return((*hotline.Account)(nil))
							m.On("Create", mock.Anything).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("userB"))),
					hotline.NewField(hotline.FieldUserName, []byte("User B")),
					hotline.NewField(hotline.FieldUserPassword, []byte("pass")),
					hotline.NewField(
						hotline.FieldUserAccess,
						func() []byte {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateUser)
							return bits[:]
						}(),
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
				},
			},
		},
		{
			name: "when account already exists",
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "userB").Return(&hotline.Account{Login: "userB"})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("userB"))),
					hotline.NewField(hotline.FieldUserName, []byte("User B")),
					hotline.NewField(hotline.FieldUserPassword, []byte("pass")),
					hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(fmt.Sprintf(ErrMsgAccountExistsTemplate, "userB"))),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
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
					Server: &hotline.Server{TextDecoder: charmap.Macintosh.NewDecoder(), TextEncoder: charmap.Macintosh.NewEncoder()},
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
		{
			name: "when action is delete user with permission",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{
								{
									Account: &hotline.Account{
										Login: "testuser",
									},
									Connection: nopReadWriteCloser{},
									Server: &hotline.Server{
										ClientMgr: hotline.NewMemClientMgr(),
										Logger:    NewTestLogger(),
									},
								},
							})
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteUser)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
						0x00, 0x01, // 1 subfield (delete)
						0x00, 0x65, // FieldData = 101
						0x00, 0x08, // length
						0x8b, 0x9a, 0x8c, 0x8b, 0x8a, 0x8c, 0x9a, 0x8d, // obfuscated "testuser"
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					Type: hotline.TranServerMsg,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
						hotline.NewField(hotline.FieldChatOptions, []byte{0}),
					},
				},
				{
					IsReply: 0x01,
				},
			},
		},
		{
			name: "when action is delete user and Delete returns error",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Delete", "testuser").Return(errors.New("disk error"))
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessDeleteUser)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
						0x00, 0x01,
						0x00, 0x65,
						0x00, 0x08,
						0x8b, 0x9a, 0x8c, 0x8b, 0x8a, 0x8c, 0x9a, 0x8d,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgDeleteAccount)),
					},
				},
			},
		},
		{
			name: "when action is modify existing user with password",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return(&hotline.Account{
								Login: "bbb",
								Name:  "old name",
							})
							m.On("Update", mock.Anything, "bbb").Return(nil)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessModifyUser)
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
					IsReply: 0x01,
				},
			},
		},
		{
			name: "when action is modify existing user with password cleared",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return(&hotline.Account{
								Login: "bbb",
								Name:  "old name",
							})
							m.On("Update", mock.Anything, "bbb").Return(nil)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessModifyUser)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUpdateUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldData, []byte{
						0x00, 0x03, // field count (3 subfields, no password)

						0x00, 0x69, // FieldUserLogin = 105
						0x00, 0x03,
						0x9d, 0x9d, 0x9d,

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
					IsReply: 0x01,
				},
			},
		},
		{
			name: "when action is create user with valid permissions",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return((*hotline.Account)(nil))
							m.On("Create", mock.Anything).Return(nil)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateUser)
							// Set all bits that the new account will have
							for i := 0; i < 64; i++ {
								bits.Set(i)
							}
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
					IsReply: 0x01,
				},
			},
		},
		{
			name: "when action is create user with escalated privileges",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return((*hotline.Account)(nil))
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessCreateUser)
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
						0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, // AccessDisconUser set (bit 22)
					}),
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
		{
			name: "when action is create user and Create returns error",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Logger:      NewTestLogger(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "bbb").Return((*hotline.Account)(nil))
							m.On("Create", mock.Anything).Return(errors.New("account exists"))
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							// Set all bits so privilege check passes
							for i := 0; i < 64; i++ {
								bits.Set(i)
							}
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
						hotline.NewField(hotline.FieldError, []byte(ErrMsgAccountExists)),
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

func TestHandleSetUser(t *testing.T) {
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
				t: hotline.NewTransaction(
					hotline.TranSetUser, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgNotAllowedModifyAccounts)),
					},
				},
			},
		},
		{
			name: "when account is not found",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessModifyUser)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "testuser").Return((*hotline.Account)(nil))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("testuser"))),
					hotline.NewField(hotline.FieldUserName, []byte("Test User")),
					hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgAccountNotFound)),
					},
				},
			},
		},
		{
			name: "when Update returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessModifyUser)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "testuser").Return(&hotline.Account{
								Login: "testuser",
								Name:  "Old Name",
							})
							m.On("Update", mock.Anything, "testuser").Return(errors.New("disk full"))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("testuser"))),
					hotline.NewField(hotline.FieldUserName, []byte("New Name")),
					hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgUpdateAccount)),
					},
				},
			},
		},
		{
			name: "when update is successful with password change",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessModifyUser)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						AccountManager: func() *MockAccountManager {
							m := MockAccountManager{}
							m.On("Get", "testuser").Return(&hotline.Account{
								Login: "testuser",
								Name:  "Old Name",
							})
							m.On("Update", mock.Anything, "testuser").Return(nil)
							return &m
						}(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranSetUser, [2]byte{0, 1},
					hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("testuser"))),
					hotline.NewField(hotline.FieldUserName, []byte("New Name")),
					hotline.NewField(hotline.FieldUserPassword, []byte("newpass")),
					hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
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
			gotRes := HandleSetUser(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}
