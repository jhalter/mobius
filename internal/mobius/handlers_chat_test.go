package mobius

import (
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/encoding/charmap"
)

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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
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

func TestHandleJoinChat(t *testing.T) {
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
			name: "joins private chat and notifies existing members",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("NewUser"),
					ID:       [2]byte{0, 3},
					Icon:     []byte{0, 5},
					Flags:    [2]byte{0, 1},
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							// Mock existing members before join
							m.On("Members", hotline.ChatID{0, 0, 0, 1}).Return([]*hotline.ClientConn{
								{
									UserName: []byte("User1"),
									ID:       [2]byte{0, 1},
									Icon:     []byte{0, 2},
									Flags:    [2]byte{0, 0},
								},
								{
									UserName: []byte("User2"),
									ID:       [2]byte{0, 2},
									Icon:     []byte{0, 3},
									Flags:    [2]byte{0, 0},
								},
							})
							// Mock join operation
							m.On("Join", hotline.ChatID{0, 0, 0, 1}, mock.AnythingOfType("*hotline.ClientConn")).Return()
							// Mock members after join (including new user)
							m.On("Members", hotline.ChatID{0, 0, 0, 1}).Return([]*hotline.ClientConn{
								{
									UserName: []byte("User1"),
									ID:       [2]byte{0, 1},
									Icon:     []byte{0, 2},
									Flags:    [2]byte{0, 0},
								},
								{
									UserName: []byte("User2"),
									ID:       [2]byte{0, 2},
									Icon:     []byte{0, 3},
									Flags:    [2]byte{0, 0},
								},
								{
									UserName: []byte("NewUser"),
									ID:       [2]byte{0, 3},
									Icon:     []byte{0, 5},
									Flags:    [2]byte{0, 1},
								},
							})
							// Mock chat subject
							m.On("GetSubject", hotline.ChatID{0, 0, 0, 1}).Return("Test Chat Room")
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x74}, // TranJoinChat
					ID:   [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
					},
				},
			},
			want: []hotline.Transaction{
				// Notification to existing member 1
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x75}, // TranNotifyChatChangeUser
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("NewUser")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 3}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 5}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
					},
				},
				// Notification to existing member 2
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x75}, // TranNotifyChatChangeUser
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("NewUser")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 3}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 5}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 1}),
					},
				},
				// Reply to joining user
				{
					ClientID: [2]byte{0, 3},
					IsReply:  0x01,
					Type:     [2]byte{0, 0}, // Reply type is [0, 0]
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatSubject, []byte("Test Chat Room")),
						// User1 info
						hotline.NewField(hotline.FieldUsernameWithInfo, []byte{
							0x00, 0x01, // User ID
							0x00, 0x02, // Icon ID
							0x00, 0x00, // Flags
							0x00, 0x05, // Username length
							0x55, 0x73, 0x65, 0x72, 0x31, // "User1"
						}),
						// User2 info
						hotline.NewField(hotline.FieldUsernameWithInfo, []byte{
							0x00, 0x02, // User ID
							0x00, 0x03, // Icon ID
							0x00, 0x00, // Flags
							0x00, 0x05, // Username length
							0x55, 0x73, 0x65, 0x72, 0x32, // "User2"
						}),
					},
				},
			},
		},
		{
			name: "joins empty private chat",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("OnlyUser"),
					ID:       [2]byte{0, 1},
					Icon:     []byte{0, 1},
					Flags:    [2]byte{0, 0},
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							// Mock empty chat before join
							m.On("Members", hotline.ChatID{0, 0, 0, 2}).Return([]*hotline.ClientConn{})
							// Mock join operation
							m.On("Join", hotline.ChatID{0, 0, 0, 2}, mock.AnythingOfType("*hotline.ClientConn")).Return()
							// Mock members after join
							m.On("Members", hotline.ChatID{0, 0, 0, 2}).Return([]*hotline.ClientConn{
								{
									UserName: []byte("OnlyUser"),
									ID:       [2]byte{0, 1},
									Icon:     []byte{0, 1},
									Flags:    [2]byte{0, 0},
								},
							})
							// Mock chat subject
							m.On("GetSubject", hotline.ChatID{0, 0, 0, 2}).Return("Empty Chat")
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x74}, // TranJoinChat
					ID:   [4]byte{0, 0, 0, 2},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 2}),
					},
				},
			},
			want: []hotline.Transaction{
				// Only reply to joining user (no existing members to notify)
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Type:     [2]byte{0, 0}, // Reply type is [0, 0]
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatSubject, []byte("Empty Chat")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleJoinChat(tt.args.cc, &tt.args.t)
			if !TranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleJoinChat() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleRejectChatInvite(t *testing.T) {
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
			name: "rejects invite and notifies chat members",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("RejectUser"),
					ID:       [2]byte{0, 3},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							// Mock current members of the chat
							m.On("Members", hotline.ChatID{0, 0, 0, 1}).Return([]*hotline.ClientConn{
								{
									UserName: []byte("Member1"),
									ID:       [2]byte{0, 1},
								},
								{
									UserName: []byte("Member2"),
									ID:       [2]byte{0, 2},
								},
							})
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x72}, // TranRejectChatInvite
					ID:   [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 1}),
					},
				},
			},
			want: []hotline.Transaction{
				// Notification to member 1
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6A}, // TranChatMsg
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("RejectUser declined invitation to chat")),
					},
				},
				// Notification to member 2
				{
					ClientID: [2]byte{0, 2},
					Type:     [2]byte{0, 0x6A}, // TranChatMsg
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("RejectUser declined invitation to chat")),
					},
				},
			},
		},
		{
			name: "rejects invite to empty chat",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("LoneRejecter"),
					ID:       [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							// Mock empty chat (no members)
							m.On("Members", hotline.ChatID{0, 0, 0, 2}).Return([]*hotline.ClientConn{})
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x72}, // TranRejectChatInvite
					ID:   [4]byte{0, 0, 0, 2},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 2}),
					},
				},
			},
			want: []hotline.Transaction{
				// No notifications (no members to notify)
			},
		},
		{
			name: "rejects invite with single member",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("Shy"),
					ID:       [2]byte{0, 2},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ChatMgr: func() *hotline.MockChatManager {
							m := hotline.MockChatManager{}
							// Mock chat with single member
							m.On("Members", hotline.ChatID{0, 0, 0, 3}).Return([]*hotline.ClientConn{
								{
									UserName: []byte("OnlyMember"),
									ID:       [2]byte{0, 1},
								},
							})
							return &m
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x72}, // TranRejectChatInvite
					ID:   [4]byte{0, 0, 0, 3},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 3}),
					},
				},
			},
			want: []hotline.Transaction{
				// Notification to the single member
				{
					ClientID: [2]byte{0, 1},
					Type:     [2]byte{0, 0x6A}, // TranChatMsg
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("Shy declined invitation to chat")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleRejectChatInvite(tt.args.cc, &tt.args.t)
			if !TranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleRejectChatInvite() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleInviteToChat(t *testing.T) {
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
			name: "invites user to chat successfully",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("Inviter"),
					ID:       [2]byte{0, 1},
					Icon:     []byte{0, 2},
					Flags:    [2]byte{0, 3},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							access := hotline.AccessBitmap{}
							access.Set(hotline.AccessOpenChat)
							return access
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x71}, // TranInviteToChat
					ID:   [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 5}),
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 10}),
					},
				},
			},
			want: []hotline.Transaction{
				// Invite sent to target user
				{
					ClientID: [2]byte{0, 5},
					Type:     [2]byte{0, 0x71}, // TranInviteToChat
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("Inviter")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
					},
				},
				// Reply to inviting user
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Type:     [2]byte{0, 0}, // Reply type is [0, 0]
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("Inviter")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 1}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 2}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 3}),
					},
				},
			},
		},
		{
			name: "returns error when user lacks permission",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("NoPermUser"),
					ID:       [2]byte{0, 2},
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{}, // No permissions
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x71}, // TranInviteToChat
					ID:   [4]byte{0, 0, 0, 2},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 3}),
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 5}),
					},
				},
			},
			want: []hotline.Transaction{
				// Error reply to requesting user
				{
					ClientID:  [2]byte{0, 2},
					IsReply:   0x01,
					Type:      [2]byte{0, 0}, // Reply type is [0, 0]
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to request private chat.")),
					},
				},
			},
		},
		{
			name: "invites to different chat room",
			args: args{
				cc: &hotline.ClientConn{
					UserName: []byte("Host"),
					ID:       [2]byte{0, 10},
					Icon:     []byte{0, 15},
					Flags:    [2]byte{0, 20},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							access := hotline.AccessBitmap{}
							access.Set(hotline.AccessOpenChat)
							return access
						}(),
					},
				},
				t: hotline.Transaction{
					Type: [2]byte{0, 0x71}, // TranInviteToChat
					ID:   [4]byte{0, 0, 0, 3},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserID, []byte{0, 99}),
						hotline.NewField(hotline.FieldChatID, []byte{0, 0, 0, 25}),
					},
				},
			},
			want: []hotline.Transaction{
				// Invite sent to target user
				{
					ClientID: [2]byte{0, 99},
					Type:     [2]byte{0, 0x71}, // TranInviteToChat
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("Host")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 10}),
					},
				},
				// Reply to inviting user
				{
					ClientID: [2]byte{0, 10},
					IsReply:  0x01,
					Type:     [2]byte{0, 0}, // Reply type is [0, 0]
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldUserName, []byte("Host")),
						hotline.NewField(hotline.FieldUserID, []byte{0, 10}),
						hotline.NewField(hotline.FieldUserIconID, []byte{0, 15}),
						hotline.NewField(hotline.FieldUserFlags, []byte{0, 20}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleInviteToChat(tt.args.cc, &tt.args.t)
			if !TranAssertEqual(t, tt.want, got) {
				t.Errorf("HandleInviteToChat() got = %v, want %v", got, tt.want)
			}
		})
	}
}
