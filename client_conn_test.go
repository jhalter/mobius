package hotline

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestHandleGetUserNameList(t *testing.T) {
	type args struct {
		cc *ClientConn
		t  *Transaction
	}
	tests := []struct {
		name    string
		args    args
		want    []egressTransaction
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
								UserName: &[]byte{0, 4},
							},
						},
					},
				},
				t: &Transaction{
					ID:   []byte{0, 0, 0, 1},
					Type: []byte{0, 1},
				},
			},
			want: []egressTransaction{
				{
					ClientID: &[]byte{1, 1},
					Transaction: &Transaction{
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
						},
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
		want    []egressTransaction
		wantErr bool
	}{
		{
			name: "sends chat msg transaction to all clients",
			args: args{
				cc: &ClientConn{
					UserName: &[]byte{0x00, 0x01},
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
			want: []egressTransaction{
				{
					ClientID: &[]byte{0, 1},
					Transaction: &Transaction{
						Flags:     0x00,
						IsReply:   0x00,
						Type:      []byte{0, 0x6a},
						ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
						ErrorCode: []byte{0, 0, 0, 0},
						Fields: []Field{
							NewField(fieldData, []byte{0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69, 0x0d}),
						},
					},
				},
				{
					ClientID: &[]byte{0, 2},
					Transaction: &Transaction{
						Flags:     0x00,
						IsReply:   0x00,
						Type:      []byte{0, 0x6a},
						ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
						ErrorCode: []byte{0, 0, 0, 0},
						Fields: []Field{
							NewField(fieldData, []byte{0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69, 0x0d}),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only sends chat msg to clients with accessReadChat permission",
			args: args{
				cc: &ClientConn{
					UserName: &[]byte{0x00, 0x01},
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
			want: []egressTransaction{
				{
					ClientID: &[]byte{0, 1},
					Transaction: &Transaction{
						Flags:     0x00,
						IsReply:   0x00,
						Type:      []byte{0, 0x6a},
						ID:        []byte{0x9a, 0xcb, 0x04, 0x42}, // Random ID from rand.Seed(1)
						ErrorCode: []byte{0, 0, 0, 0},
						Fields: []Field{
							NewField(fieldData, []byte{0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x01, 0x3a, 0x20, 0x20, 0x68, 0x61, 0x69, 0x0d}),
						},
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
			if !assert.Equal(t, got, tt.want) {
				t.Errorf("HandleChatSend() got = %v, want %v", got, tt.want)
			}
		})
	}
}
