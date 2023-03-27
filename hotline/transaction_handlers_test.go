package hotline

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
											Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 1},
									},
									uint16(2): {
										Account: &Account{
											Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 2},
									},
								},
							},
						},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
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
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
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
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
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
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldChatSubject, []byte("Test Subject")),
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
											Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 1},
									},
									uint16(2): {
										Account: &Account{
											Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
										},
										ID: &[]byte{0, 2},
									},
								},
							},
						},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: NewTransaction(TranDeleteUser, nil, NewField(FieldChatID, []byte{0, 0, 0, 1})),
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
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldUserID, []byte{0, 2}),
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
								Icon:     []byte{0, 2},
								Flags:    []byte{0, 3},
								UserName: []byte{0, 4},
							},
							uint16(2): {
								ID:       &[]byte{0, 2},
								Icon:     []byte{0, 2},
								Flags:    []byte{0, 3},
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
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 1},
					ErrorCode: []byte{0, 0, 0, 0},
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
			assert.Equal(t, tt.want, got)
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
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
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
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
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
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "treats Chat ID 00 00 00 00 as a public chat message",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
						NewField(FieldChatID, []byte{0, 0, 0, 0}),
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
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
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
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranChatSend, &[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			want: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to participate in chat.")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "sends chat msg as emote if FieldChatOptions is set to 1",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("performed action")),
						NewField(FieldChatOptions, []byte{0x00, 0x01}),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0xf0, 0xc5, 0x34, 0x1e},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("\r*** Testy McTest performed action")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "does not send chat msg as emote if FieldChatOptions is set to 0",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte("Testy McTest"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hello")),
						NewField(FieldChatOptions, []byte{0x00, 0x00}),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0xf0, 0xc5, 0x34, 0x1e},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("\r Testy McTest:  hello")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only sends chat msg to clients with accessReadChat permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: func() accessBitmap {
										var bits accessBitmap
										bits.Set(accessReadChat)
										return bits
									}()},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
								},
								ID: &[]byte{0, 2},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
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
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only sends private chat msg to members of private chat",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendChat)
							return bits
						}(),
					},
					UserName: []byte{0x00, 0x01},
					Server: &Server{
						PrivateChats: map[uint32]*PrivateChat{
							uint32(1): {
								ClientConn: map[uint16]*ClientConn{
									uint16(1): {
										ID: &[]byte{0, 1},
									},
									uint16(2): {
										ID: &[]byte{0, 2},
									},
								},
							},
						},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Access: accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
								},
								ID: &[]byte{0, 1},
							},
							uint16(2): {
								Account: &Account{
									Access: accessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
								},
								ID: &[]byte{0, 2},
							},
							uint16(3): {
								Account: &Account{
									Access: accessBitmap{0, 0, 0, 0, 0, 0, 0, 0},
								},
								ID: &[]byte{0, 3},
							},
						},
					},
				},
				t: &Transaction{
					Fields: []Field{
						NewField(FieldData, []byte("hai")),
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
					},
				},
			},
			want: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x6a},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldChatID, []byte{0, 0, 0, 1}),
						NewField(FieldData, []byte{0x0d, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69}),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleChatSend(tt.args.cc, tt.args.t)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleChatSend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tranAssertEqual(t, tt.want, got)
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
						FS: &OSFileStore{},
						Config: &Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileInfo, nil,
					NewField(FieldFileName, []byte("testfile.txt")),
					NewField(FieldFilePath, []byte{0x00, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldFileName, []byte("testfile.txt")),
						NewField(FieldFileTypeString, []byte("Text File")),
						NewField(FieldFileCreatorString, []byte("ttxt")),
						NewField(FieldFileComment, []byte{}),
						NewField(FieldFileType, []byte("TEXT")),
						NewField(FieldFileCreateDate, make([]byte, 8)),
						NewField(FieldFileModifyDate, make([]byte, 8)),
						NewField(FieldFileSize, []byte{0x0, 0x0, 0x0, 0x17}),
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

			// Clear the fileWrapper timestamp fields to work around problems running the tests in multiple timezones
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
		name    string
		args    args
		wantRes []Transaction
		wantErr bool
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
					accessCreateFolder,
					&[]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create folders.")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when path is nested",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateFolder)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
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
					TranNewFolder, &[]byte{0, 1},
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
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
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateFolder)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
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
					TranNewFolder, &[]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			},
			wantErr: false,
		},
		{
			name: "when Write returns an err",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateFolder)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
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
					TranNewFolder, &[]byte{0, 1},
					NewField(FieldFileName, []byte("testFolder")),
					NewField(FieldFilePath, []byte{
						0x00,
					}),
				),
			},
			wantRes: []Transaction{},
			wantErr: true,
		},
		{
			name: "FieldFileName does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateFolder)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
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
					TranNewFolder, &[]byte{0, 1},
					NewField(FieldFileName, []byte("../../testFolder")),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			}, wantErr: false,
		},
		{
			name: "FieldFilePath does not allow directory traversal",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateFolder)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						Config: &Config{
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
					TranNewFolder, &[]byte{0, 1},
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
					ErrorCode: []byte{0, 0, 0, 0},
				},
			}, wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

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
			name: "when request is valid and user has Upload Anywhere permission",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						FS:            &OSFileStore{},
						fileTransfers: map[[4]byte]*FileTransfer{},
						Config: &Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						}},
					transfers: map[int]map[[4]byte]*FileTransfer{
						FileUpload: {},
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessUploadFile)
							bits.Set(accessUploadAnywhere)
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranUploadFile, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}), // rand.Seed(1)
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
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranUploadFile, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to upload files.")), // rand.Seed(1)
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

			tranAssertEqual(t, tt.wantRes, gotRes)

		})
	}
}

func TestHandleMakeAlias(t *testing.T) {
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
			name: "with valid input and required permissions",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessMakeAlias)
							return bits
						}(),
					},
					Server: &Server{
						Config: &Config{
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
					TranMakeFileAlias, &[]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					NewField(FieldFileNewPath, EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: false,
		},
		{
			name: "when symlink returns an error",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessMakeAlias)
							return bits
						}(),
					},
					Server: &Server{
						Config: &Config{
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
					TranMakeFileAlias, &[]byte{0, 1},
					NewField(FieldFileName, []byte("testFile")),
					NewField(FieldFilePath, EncodeFilePath(strings.Join([]string{"foo"}, "/"))),
					NewField(FieldFileNewPath, EncodeFilePath(strings.Join([]string{"bar"}, "/"))),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Error creating alias")),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
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
					TranMakeFileAlias, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to make aliases.")),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleMakeAlias(tt.args.cc, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleMakeAlias(%v, %v)", tt.args.cc, tt.args.t)
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when account is valid",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						Accounts: map[string]*Account{
							"guest": {
								Login:    "guest",
								Name:     "Guest",
								Password: "password",
								Access:   accessBitmap{},
							},
						},
					},
				},
				t: NewTransaction(
					TranGetUser, &[]byte{0, 1},
					NewField(FieldUserLogin, []byte("guest")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldUserName, []byte("Guest")),
						NewField(FieldUserLogin, negateString([]byte("guest"))),
						NewField(FieldUserPassword, []byte("password")),
						NewField(FieldUserAccess, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
					},
				},
			},
			wantErr: assert.NoError,
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetUser, &[]byte{0, 1},
					NewField(FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when account does not exist",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetUser, &[]byte{0, 1},
					NewField(FieldUserLogin, []byte("nonExistentUser")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Account does not exist.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetUser(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetUser(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when user dataFile",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDeleteUser)
							return bits
						}(),
					},
					Server: &Server{
						Accounts: map[string]*Account{
							"testuser": {
								Login:    "testuser",
								Name:     "Testy McTest",
								Password: "password",
								Access:   accessBitmap{},
							},
						},
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("Remove", "Users/testuser.yaml").Return(nil)
							return mfs
						}(),
					},
				},
				t: NewTransaction(
					TranDeleteUser, &[]byte{0, 1},
					NewField(FieldUserLogin, negateString([]byte("testuser"))),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: assert.NoError,
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDeleteUser, &[]byte{0, 1},
					NewField(FieldUserLogin, negateString([]byte("testuser"))),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDeleteUser(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDeleteUser(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetMsgs(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "returns news data",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessNewsReadArt)
							return bits
						}(),
					},
					Server: &Server{
						FlatNews: []byte("TEST"),
					},
				},
				t: NewTransaction(
					TranGetMsgs, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("TEST")),
					},
				},
			},
			wantErr: assert.NoError,
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetMsgs, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetMsgs(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetMsgs(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranNewUser, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when user attempts to create account with greater access",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessCreateUser)
							return bits
						}(),
					},
					Server: &Server{
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranNewUser, &[]byte{0, 1},
					NewField(FieldUserLogin, []byte("userB")),
					NewField(
						FieldUserAccess,
						func() []byte {
							var bits accessBitmap
							bits.Set(accessDisconUser)
							return bits[:]
						}(),
					),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("Cannot create account with more access than yourself.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleNewUser(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleNewUser(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleListUsers(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranNewUser, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessOpenUser)
							return bits
						}(),
					},
					Server: &Server{
						Accounts: map[string]*Account{
							"guest": {
								Name:     "guest",
								Login:    "guest",
								Password: "zz",
								Access:   accessBitmap{255, 255, 255, 255, 255, 255, 255, 255},
							},
						},
					},
				},
				t: NewTransaction(
					TranGetClientInfoText, &[]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte{
							0x00, 0x04, 0x00, 0x66, 0x00, 0x05, 0x67, 0x75, 0x65, 0x73, 0x74, 0x00, 0x69, 0x00, 0x05, 0x98,
							0x8a, 0x9a, 0x8c, 0x8b, 0x00, 0x6e, 0x00, 0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
							0x00, 0x6a, 0x00, 0x01, 0x78,
						}),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleListUsers(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleListUsers(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
				t: NewTransaction(TranDownloadFile, &[]byte{0, 1}),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to download files.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "with a valid file",
			args: args{
				cc: &ClientConn{
					transfers: map[int]map[[4]byte]*FileTransfer{
						FileDownload: {},
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDownloadFile)
							return bits
						}(),
					},
					Server: &Server{
						FS:            &OSFileStore{},
						fileTransfers: map[[4]byte]*FileTransfer{},
						Config: &Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					accessDownloadFile,
					&[]byte{0, 1},
					NewField(FieldFileName, []byte("testfile.txt")),
					NewField(FieldFilePath, []byte{0x0, 0x00}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldWaitingCount, []byte{0x00, 0x00}),
						NewField(FieldTransferSize, []byte{0x00, 0x00, 0x00, 0xa5}),
						NewField(FieldFileSize, []byte{0x00, 0x00, 0x00, 0x17}),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when client requests to resume 1k test file at offset 256",
			args: args{
				cc: &ClientConn{
					transfers: map[int]map[[4]byte]*FileTransfer{
						FileDownload: {},
					}, Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDownloadFile)
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
						fileTransfers: map[[4]byte]*FileTransfer{},
						Config: &Config{
							FileRoot: func() string { path, _ := os.Getwd(); return path + "/test/config/Files" }(),
						},
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					accessDownloadFile,
					&[]byte{0, 1},
					NewField(FieldFileName, []byte("testfile-1k")),
					NewField(FieldFilePath, []byte{0x00, 0x00}),
					NewField(
						FieldFileResumeData,
						func() []byte {
							frd := FileResumeData{
								Format:    [4]byte{},
								Version:   [2]byte{},
								RSVD:      [34]byte{},
								ForkCount: [2]byte{0, 2},
								ForkInfoList: []ForkInfoList{
									{
										Fork:     [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
										DataSize: [4]byte{0, 0, 0x01, 0x00},       // request offset 256
										RSVDA:    [4]byte{},
										RSVDB:    [4]byte{},
									},
									{
										Fork:     [4]byte{0x4d, 0x41, 0x43, 0x52}, // "MACR"
										DataSize: [4]byte{0, 0, 0, 0},
										RSVDA:    [4]byte{},
										RSVDB:    [4]byte{},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldRefNum, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldWaitingCount, []byte{0x00, 0x00}),
						NewField(FieldTransferSize, []byte{0x00, 0x00, 0x03, 0x8d}),
						NewField(FieldFileSize, []byte{0x00, 0x00, 0x03, 0x00}),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDownloadFile(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDownloadFile(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleUpdateUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when action is create user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{
						Logger: NewTestLogger(),
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
					&[]byte{0, 0},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create new accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when action is modify user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{
						Logger: NewTestLogger(),
						Accounts: map[string]*Account{
							"bbb": {},
						},
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
					&[]byte{0, 0},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to modify accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when action is delete user without required permission",
			args: args{
				cc: &ClientConn{
					logger: NewTestLogger(),
					Server: &Server{
						Accounts: map[string]*Account{
							"bbb": {},
						},
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
					&[]byte{0, 0},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete accounts.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleUpdateUser(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleUpdateUser(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsArt(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
					&[]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news articles.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDelNewsArt(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDelNewsArt(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDisconnectUser(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
					&[]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to disconnect users.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when target user has 'cannot be disconnected' priv",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								Account: &Account{
									Login: "unnamed",
									Access: func() accessBitmap {
										var bits accessBitmap
										bits.Set(accessCannotBeDiscon)
										return bits
									}(),
								},
							},
						},
					},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDisconUser)
							return bits
						}(),
					},
				},
				t: NewTransaction(
					TranDelNewsArt,
					&[]byte{0, 0},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("unnamed is not allowed to be disconnected.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDisconnectUser(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDisconnectUser(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSendInstantMsg(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
					&[]byte{0, 0},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to send private messages.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when client 1 sends a message to client 2",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendPrivMsg)
							return bits
						}(),
					},
					ID:       &[]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(2): {
								AutoReply: []byte(nil),
								Flags:     []byte{0, 0},
							},
						},
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					&[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				*NewTransaction(
					TranServerMsg,
					&[]byte{0, 2},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserName, []byte("User1")),
					NewField(FieldUserID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when client 2 has autoreply enabled",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendPrivMsg)
							return bits
						}(),
					},
					ID:       &[]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(2): {
								Flags:     []byte{0, 0},
								ID:        &[]byte{0, 2},
								UserName:  []byte("User2"),
								AutoReply: []byte("autohai"),
							},
						},
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					&[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				*NewTransaction(
					TranServerMsg,
					&[]byte{0, 2},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserName, []byte("User1")),
					NewField(FieldUserID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				*NewTransaction(
					TranServerMsg,
					&[]byte{0, 1},
					NewField(FieldData, []byte("autohai")),
					NewField(FieldUserName, []byte("User2")),
					NewField(FieldUserID, []byte{0, 2}),
					NewField(FieldOptions, []byte{0, 1}),
				),
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when client 2 has refuse private messages enabled",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessSendPrivMsg)
							return bits
						}(),
					},
					ID:       &[]byte{0, 1},
					UserName: []byte("User1"),
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(2): {
								Flags:    []byte{255, 255},
								ID:       &[]byte{0, 2},
								UserName: []byte("User2"),
							},
						},
					},
				},
				t: NewTransaction(
					TranSendInstantMsg,
					&[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				*NewTransaction(
					TranServerMsg,
					&[]byte{0, 1},
					NewField(FieldData, []byte("User2 does not accept private messages.")),
					NewField(FieldUserName, []byte("User2")),
					NewField(FieldUserID, []byte{0, 2}),
					NewField(FieldOptions, []byte{0, 2}),
				),
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleSendInstantMsg(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleSendInstantMsg(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDeleteFile(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Config: &Config{
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDeleteFile, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete files.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "deletes all associated metadata files",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDeleteFile)
							return bits
						}(),
					},
					Server: &Server{
						Config: &Config{
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranDeleteFile, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x0, 0x0, 0x0, 0x0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field(nil),
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDeleteFile(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDeleteFile(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)

			tt.args.cc.Server.FS.(*MockFileStore).AssertExpectations(t)
		})
	}
}

func TestHandleGetFileNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when FieldFilePath is a drop box, but user does not have accessViewDropBoxes ",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					Server: &Server{

						Config: &Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileNameList, &[]byte{0, 1},
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
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to view drop boxes.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "with file root",
			args: args{
				cc: &ClientConn{
					Server: &Server{
						Config: &Config{
							FileRoot: func() string {
								path, _ := os.Getwd()
								return filepath.Join(path, "/test/config/Files/getFileNameListTestDir")
							}(),
						},
					},
				},
				t: NewTransaction(
					TranGetFileNameList, &[]byte{0, 1},
					NewField(FieldFilePath, []byte{
						0x00, 0x00,
						0x00, 0x00,
					}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
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
									name: []byte("testfile-1k"),
								}
								b, _ := fnwi.MarshalBinary()
								return b
							}(),
						),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetFileNameList(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetFileNameList(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetClientInfoText(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetClientInfoText, &[]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to get client info.")),
					},
				},
			},
			wantErr: assert.NoError,
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
							bits.Set(accessGetClientInfo)
							return bits
						}(),
						Name:  "test",
						Login: "test",
					},
					Server: &Server{
						Accounts: map[string]*Account{},
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								UserName:   []byte("Testy McTest"),
								RemoteAddr: "1.2.3.4:12345",
								Account: &Account{
									Access: func() accessBitmap {
										var bits accessBitmap
										bits.Set(accessGetClientInfo)
										return bits
									}(),
									Name:  "test",
									Login: "test",
								},
							},
						},
					},
					transfers: map[int]map[[4]byte]*FileTransfer{
						FileDownload:   {},
						FileUpload:     {},
						FolderDownload: {},
						FolderUpload:   {},
					},
				},
				t: NewTransaction(
					TranGetClientInfoText, &[]byte{0, 1},
					NewField(FieldUserID, []byte{0, 1}),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte(
							strings.Replace(`Nickname:   Testy McTest
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

`, "\n", "\r", -1)),
						),
						NewField(FieldUserName, []byte("Testy McTest")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetClientInfoText(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetClientInfoText(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranAgreed(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "normal request flow",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessDisconUser)
							bits.Set(accessAnyName)
							return bits
						}()},
					Icon:    []byte{0, 1},
					Flags:   []byte{0, 1},
					Version: []byte{0, 1},
					ID:      &[]byte{0, 1},
					logger:  NewTestLogger(),
					Server: &Server{
						Config: &Config{
							BannerFile: "banner.jpg",
						},
					},
				},
				t: NewTransaction(
					TranAgreed, nil,
					NewField(FieldUserName, []byte("username")),
					NewField(FieldUserIconID, []byte{0, 1}),
					NewField(FieldOptions, []byte{0, 0}),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x7a},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldBannerType, []byte("JPEG")),
					},
				},
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field{},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleTranAgreed(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleTranAgreed(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleSetClientUserInfo(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when client does not have accessAnyName",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							return bits
						}(),
					},
					ID:       &[]byte{0, 1},
					UserName: []byte("Guest"),
					Flags:    []byte{0, 1},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(1): {
								ID: &[]byte{0, 1},
							},
						},
					},
				},
				t: NewTransaction(
					TranSetClientUserInfo, nil,
					NewField(FieldUserIconID, []byte{0, 1}),
					NewField(FieldUserName, []byte("NOPE")),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0x01, 0x2d},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 1}),
						NewField(FieldUserName, []byte("Guest"))},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleSetClientUserInfo(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleSetClientUserInfo(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsItem(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when user does not have permission to delete a news category",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						ThreadedNews: &ThreadedNews{Categories: map[string]NewsCategoryListData15{
							"test": {
								Type:     []byte{0, 3},
								Count:    nil,
								NameSize: 0,
								Name:     "zz",
							},
						}},
					},
				},
				t: NewTransaction(
					TranDelNewsItem, nil,
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news categories.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when user does not have permission to delete a news folder",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: accessBitmap{},
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						ThreadedNews: &ThreadedNews{Categories: map[string]NewsCategoryListData15{
							"testcat": {
								Type:     []byte{0, 2},
								Count:    nil,
								NameSize: 0,
								Name:     "test",
							},
						}},
					},
				},
				t: NewTransaction(
					TranDelNewsItem, nil,
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to delete news folders.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when user deletes a news folder",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessNewsDeleteFldr)
							return bits
						}(),
					},
					ID: &[]byte{0, 1},
					Server: &Server{
						ConfigDir: "/fakeConfigRoot",
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("WriteFile", "/fakeConfigRoot/ThreadedNews.yaml", mock.Anything, mock.Anything).Return(nil, os.ErrNotExist)
							return mfs
						}(),
						ThreadedNews: &ThreadedNews{Categories: map[string]NewsCategoryListData15{
							"testcat": {
								Type:     []byte{0, 2},
								Count:    nil,
								NameSize: 0,
								Name:     "test",
							},
						}},
					},
				},
				t: NewTransaction(
					TranDelNewsItem, nil,
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field{},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDelNewsItem(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDelNewsItem(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDownloadBanner(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "returns expected response",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					transfers: map[int]map[[4]byte]*FileTransfer{
						bannerDownload: {},
					},
					Server: &Server{
						ConfigDir: "/config",
						Config: &Config{
							BannerFile: "banner.jpg",
						},
						fileTransfers: map[[4]byte]*FileTransfer{},
						FS: func() *MockFileStore {
							mfi := &MockFileInfo{}
							mfi.On("Size").Return(int64(100))

							mfs := &MockFileStore{}
							mfs.On("Stat", "/config/banner.jpg").Return(mfi, nil)
							return mfs
						}(),
					},
				},
				t: NewTransaction(TranDownloadBanner, nil),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldRefNum, []byte{1, 2, 3, 4}),
						NewField(FieldTransferSize, []byte{0, 0, 0, 0x64}),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleDownloadBanner(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleDownloadBanner(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranOldPostNews(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
				t: NewTransaction(
					TranOldPostNews, &[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to post news.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when user posts news update",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessNewsPostArt)
							return bits
						}(),
					},
					Server: &Server{
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("WriteFile", "/fakeConfigRoot/MessageBoard.txt", mock.Anything, mock.Anything).Return(nil, os.ErrNotExist)
							return mfs
						}(),
						ConfigDir: "/fakeConfigRoot",
						Config:    &Config{},
					},
				},
				t: NewTransaction(
					TranOldPostNews, &[]byte{0, 1},
					NewField(FieldData, []byte("hai")),
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleTranOldPostNews(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleTranOldPostNews(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}

			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleInviteNewChat(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
				t: NewTransaction(TranInviteNewChat, &[]byte{0, 1}),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to request private chat.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when userA invites userB to new private chat",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    []byte{0, 0},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(2): {
								ID:       &[]byte{0, 2},
								UserName: []byte("UserB"),
								Flags:    []byte{0, 0},
							},
						},
						PrivateChats: make(map[uint32]*PrivateChat),
					},
				},
				t: NewTransaction(
					TranInviteNewChat, &[]byte{0, 1},
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 2},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x71},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
					},
				},

				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 0}),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when userA invites userB to new private chat, but UserB has refuse private chat enabled",
			args: args{
				cc: &ClientConn{
					ID: &[]byte{0, 1},
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessOpenChat)
							return bits
						}(),
					},
					UserName: []byte("UserA"),
					Icon:     []byte{0, 1},
					Flags:    []byte{0, 0},
					Server: &Server{
						Clients: map[uint16]*ClientConn{
							uint16(2): {
								ID:       &[]byte{0, 2},
								UserName: []byte("UserB"),
								Flags:    []byte{255, 255},
							},
						},
						PrivateChats: make(map[uint32]*PrivateChat),
					},
				},
				t: NewTransaction(
					TranInviteNewChat, &[]byte{0, 1},
					NewField(FieldUserID, []byte{0, 2}),
				),
			},
			wantRes: []Transaction{
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x00,
					Type:      []byte{0, 0x68},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldData, []byte("UserB does not accept private chats.")),
						NewField(FieldUserName, []byte("UserB")),
						NewField(FieldUserID, []byte{0, 2}),
						NewField(FieldOptions, []byte{0, 2}),
					},
				},
				{
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields: []Field{
						NewField(FieldChatID, []byte{0x52, 0xfd, 0xfc, 0x07}),
						NewField(FieldUserName, []byte("UserA")),
						NewField(FieldUserID, []byte{0, 1}),
						NewField(FieldUserIconID, []byte{0, 1}),
						NewField(FieldUserFlags, []byte{0, 0}),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rand.Seed(1)
			gotRes, err := HandleInviteNewChat(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleInviteNewChat(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtData(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetNewsArtData, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0x9a, 0xcb, 0x04, 0x42},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetNewsArtData(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetNewsArtData(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleGetNewsArtNameList(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleGetNewsArtNameList(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)

		})
	}
}

func TestHandleNewNewsFldr(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []Transaction
		wantErr assert.ErrorAssertionFunc
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
						Accounts: map[string]*Account{},
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, &[]byte{0, 1},
				),
			},
			wantRes: []Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ErrorCode: []byte{0, 0, 0, 1},
					Fields: []Field{
						NewField(FieldError, []byte("You are not allowed to create news folders.")),
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "with a valid request",
			args: args{
				cc: &ClientConn{
					Account: &Account{
						Access: func() accessBitmap {
							var bits accessBitmap
							bits.Set(accessNewsCreateFldr)
							return bits
						}(),
					},
					logger: NewTestLogger(),
					ID:     &[]byte{0, 1},
					Server: &Server{
						ConfigDir: "/fakeConfigRoot",
						FS: func() *MockFileStore {
							mfs := &MockFileStore{}
							mfs.On("WriteFile", "/fakeConfigRoot/ThreadedNews.yaml", mock.Anything, mock.Anything).Return(nil)
							return mfs
						}(),
						ThreadedNews: &ThreadedNews{Categories: map[string]NewsCategoryListData15{
							"test": {
								Type:     []byte{0, 2},
								Count:    nil,
								NameSize: 0,
								Name:     "test",
								SubCats:  make(map[string]NewsCategoryListData15),
							},
						}},
					},
				},
				t: NewTransaction(
					TranGetNewsArtNameList, &[]byte{0, 1},
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
					clientID:  &[]byte{0, 1},
					Flags:     0x00,
					IsReply:   0x01,
					Type:      []byte{0, 0},
					ID:        []byte{0, 0, 0, 0},
					ErrorCode: []byte{0, 0, 0, 0},
					Fields:    []Field{},
				},
			},
			wantErr: assert.NoError,
		},
		//{
		//	name: "when there is an error writing the threaded news file",
		//	args: args{
		//		cc: &ClientConn{
		//			Account: &Account{
		//				Access: func() accessBitmap {
		//					var bits accessBitmap
		//					bits.Set(accessNewsCreateFldr)
		//					return bits
		//				}(),
		//			},
		//			logger: NewTestLogger(),
		//			ID:     &[]byte{0, 1},
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
		//			TranGetNewsArtNameList, &[]byte{0, 1},
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
		//			clientID:  &[]byte{0, 1},
		//			Flags:     0x00,
		//			IsReply:   0x01,
		//			Type:      []byte{0, 0},
		//			ErrorCode: []byte{0, 0, 0, 1},
		//			Fields: []Field{
		//				NewField(FieldError, []byte("Error creating news folder.")),
		//			},
		//		},
		//	},
		//	wantErr: assert.Error,
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleNewNewsFldr(tt.args.cc, tt.args.t)
			if !tt.wantErr(t, err, fmt.Sprintf("HandleNewNewsFldr(%v, %v)", tt.args.cc, tt.args.t)) {
				return
			}
			tranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}
