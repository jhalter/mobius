package mobius

import (
	"strings"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/hotline/hltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/encoding/charmap"
)

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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
		{
			name: "with temporary ban option",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x1}).Return(&hotline.ClientConn{
								ID:         hotline.ClientID{0x0, 0x1},
								UserName:   []byte("baduser"),
								RemoteAddr: "10.0.0.1:12345",
								Connection: nopReadWriteCloser{},
								Account: &hotline.Account{
									Login: "baduser",
									Access: func() hotline.AccessBitmap {
										var bits hotline.AccessBitmap
										return bits
									}(),
								},
								Server: &hotline.Server{
									ClientMgr: hotline.NewMemClientMgr(),
									Logger:    NewTestLogger(),
								},
							})
							return &m
						}(),
						BanList: func() *hltest.MockBanMgr {
							m := hltest.MockBanMgr{}
							m.On("Add", "10.0.0.1", mock.AnythingOfType("*time.Time")).Return(nil)
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
					hotline.TranDisconnectUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     hotline.TranServerMsg,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte(ErrMsgTemporaryBan)),
						hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
					},
				},
				{
					IsReply: 0x01,
				},
			},
		},
		{
			name: "with permanent ban option",
			args: args{
				cc: &hotline.ClientConn{
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("Get", hotline.ClientID{0x0, 0x1}).Return(&hotline.ClientConn{
								ID:         hotline.ClientID{0x0, 0x1},
								UserName:   []byte("baduser"),
								RemoteAddr: "10.0.0.2:12345",
								Connection: nopReadWriteCloser{},
								Account: &hotline.Account{
									Login: "baduser",
									Access: func() hotline.AccessBitmap {
										var bits hotline.AccessBitmap
										return bits
									}(),
								},
								Server: &hotline.Server{
									ClientMgr: hotline.NewMemClientMgr(),
									Logger:    NewTestLogger(),
								},
							})
							return &m
						}(),
						BanList: func() *hltest.MockBanMgr {
							m := hltest.MockBanMgr{}
							m.On("Add", "10.0.0.2", (*time.Time)(nil)).Return(nil)
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
					hotline.TranDisconnectUser,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     hotline.TranServerMsg,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte(ErrMsgPermanentBan)),
						hotline.NewField(hotline.FieldChatOptions, []byte{0, 0}),
					},
				},
				{
					IsReply: 0x01,
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							BannerFile: "Banner.jpg",
						},
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
		{
			name: "with gif banner",
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							BannerFile: "Banner.gif",
						},
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{})
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
						hotline.NewField(hotline.FieldBannerType, []byte("GIFf")),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
		{
			name: "when client has AccessAnyName and changes username",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessAnyName)
							return bits
						}(),
					},
					ID:       [2]byte{0, 1},
					UserName: []byte("Guest"),
					Flags:    [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
					hotline.NewField(hotline.FieldUserName, []byte("NewName")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserName, []byte("NewName"))},
				},
			},
		},
		{
			name: "when client updates icon with 4-byte data",
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
					hotline.NewField(hotline.FieldUserIconID, []byte{0, 1, 0, 5}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 5}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserName, []byte("Guest"))},
				},
			},
		},
		{
			name: "when client updates icon with 2-byte data",
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
					hotline.NewField(hotline.FieldUserIconID, []byte{0, 3}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 3}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserName, []byte("Guest"))},
				},
			},
		},
		{
			name: "when client sets user options with auto-reply",
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
					Version:  []byte{0x01, 0x03},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
					hotline.NewField(hotline.FieldOptions, []byte{0, 4}),
					hotline.NewField(hotline.FieldAutomaticResponse, []byte("Away message")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserName, []byte("Guest"))},
				},
			},
		},
		{
			name: "when client sets refuse private messages flag",
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
					Version:  []byte{0x01, 0x03},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
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
					hotline.NewField(hotline.FieldOptions, []byte{0, 2}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0x01, 0x2d},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 9}),
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

func TestHandleKeepAlive(t *testing.T) {
	cc := &hotline.ClientConn{
		ID: [2]byte{0, 1},
	}
	tran := &hotline.Transaction{
		ID: [4]byte{0, 0, 0, 1},
	}

	got := HandleKeepAlive(cc, tran)

	assert.Len(t, got, 1)
	assert.Equal(t, byte(1), got[0].IsReply)
	assert.Equal(t, tran.ID, got[0].ID)
}

func TestHandleUserBroadcast(t *testing.T) {
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
			name: "without broadcast access returns error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{Access: hotline.AccessBitmap{}},
					ID:      [2]byte{0, 1},
				},
				t: hotline.NewTransaction(
					hotline.TranUserBroadcast, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hello")),
				),
			},
			want: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgNotAllowedSendBroadcast)),
					},
				},
			},
		},
		{
			name: "with broadcast access sends broadcast and returns success",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessBroadcast)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranUserBroadcast, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hello everyone")),
				),
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleUserBroadcast(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.want, got)
		})
	}
}

// presenceCall records a single PresenceTracker invocation for assertions.
type presenceCall struct {
	method      string
	login       string
	oldNickname string
	newNickname string
	nickname    string
	ip          string
}

type fakePresenceTracker struct {
	calls []presenceCall
}

func (f *fakePresenceTracker) UserConnected(login, ip string) {
	f.calls = append(f.calls, presenceCall{method: "UserConnected", login: login, ip: ip})
}

func (f *fakePresenceTracker) UserRenamed(login, oldNickname, newNickname, ip string) {
	f.calls = append(f.calls, presenceCall{method: "UserRenamed", login: login, oldNickname: oldNickname, newNickname: newNickname, ip: ip})
}

func (f *fakePresenceTracker) UserDisconnected(login, nickname, ip string) {
	f.calls = append(f.calls, presenceCall{method: "UserDisconnected", login: login, nickname: nickname, ip: ip})
}

func newPresenceTestServer(presence hotline.PresenceTracker) *hotline.Server {
	m := hltest.MockClientMgr{}
	m.On("List").Return([]*hotline.ClientConn{})
	return &hotline.Server{
		TextDecoder: charmap.Macintosh.NewDecoder(),
		TextEncoder: charmap.Macintosh.NewEncoder(),
		Config:      hotline.Config{BannerFile: "Banner.jpg"},
		ClientMgr:   &m,
		Presence:    presence,
	}
}

func TestHandleTranAgreed_NotifiesPresenceTracker(t *testing.T) {
	presence := &fakePresenceTracker{}
	cc := &hotline.ClientConn{
		Account: &hotline.Account{
			Login: "alice",
			Access: func() hotline.AccessBitmap {
				var bits hotline.AccessBitmap
				bits.Set(hotline.AccessAnyName)
				return bits
			}(),
		},
		ID:         [2]byte{0, 1},
		Version:    []byte{0, 1},
		RemoteAddr: "192.168.1.1:12345",
		Logger:     NewTestLogger(),
		Server:     newPresenceTestServer(presence),
	}

	tr := hotline.NewTransaction(
		hotline.TranAgreed, [2]byte{},
		hotline.NewField(hotline.FieldUserName, []byte("Alice")),
		hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}),
		hotline.NewField(hotline.FieldOptions, []byte{0, 0}),
	)

	HandleTranAgreed(cc, &tr)

	assert.Equal(t, []presenceCall{
		{method: "UserRenamed", login: "alice", oldNickname: "", newNickname: "Alice", ip: "192.168.1.1"},
	}, presence.calls)
}

func TestHandleSetClientUserInfo_NotifiesPresenceTracker(t *testing.T) {
	presence := &fakePresenceTracker{}
	cc := &hotline.ClientConn{
		Account: &hotline.Account{
			Login: "alice",
			Access: func() hotline.AccessBitmap {
				var bits hotline.AccessBitmap
				bits.Set(hotline.AccessAnyName)
				return bits
			}(),
		},
		ID:         [2]byte{0, 1},
		UserName:   []byte("Alice"),
		RemoteAddr: "192.168.1.1:12345",
		Logger:     NewTestLogger(),
		Server:     newPresenceTestServer(presence),
	}

	tr := hotline.NewTransaction(
		hotline.TranSetClientUserInfo, [2]byte{},
		hotline.NewField(hotline.FieldUserName, []byte("Alice2")),
	)

	HandleSetClientUserInfo(cc, &tr)

	assert.Equal(t, []presenceCall{
		{method: "UserRenamed", login: "alice", oldNickname: "Alice", newNickname: "Alice2", ip: "192.168.1.1"},
	}, presence.calls)
}
