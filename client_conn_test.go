package hotline

import (
	"net"
	"testing"
)

func TestClientConn_handleTransaction(t *testing.T) {
	type fields struct {
		Connection net.Conn
		ID         *[]byte
		Icon       *[]byte
		Flags      *[]byte
		UserName   *[]byte
		Account    *Account
		IdleTime   *int
		Server     *Server
		Version    *[]byte
		Idle       bool
		AutoReply  *[]byte
	}
	type args struct {
		transaction *Transaction
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ClientConn{
				Connection: tt.fields.Connection,
				ID:         tt.fields.ID,
				Icon:       tt.fields.Icon,
				Flags:      tt.fields.Flags,
				UserName:   tt.fields.UserName,
				Account:    tt.fields.Account,
				IdleTime:   tt.fields.IdleTime,
				Server:     tt.fields.Server,
				Version:    tt.fields.Version,
				Idle:       tt.fields.Idle,
				AutoReply:  tt.fields.AutoReply,
			}
			if err := cc.handleTransaction(tt.args.transaction); (err != nil) != tt.wantErr {
				t.Errorf("handleTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}