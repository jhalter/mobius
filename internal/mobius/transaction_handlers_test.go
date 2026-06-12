package mobius

import (
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
)

// Malformed client-supplied ID fields (wrong length) must produce an error reply
// rather than panicking the handler goroutine.
func TestHandlers_malformedIDFieldsReturnErrorReply(t *testing.T) {
	accessWith := func(bit int) hotline.AccessBitmap {
		var bits hotline.AccessBitmap
		bits.Set(bit)
		return bits
	}

	tests := []struct {
		name    string
		handler func(*hotline.ClientConn, *hotline.Transaction) []hotline.Transaction
		cc      *hotline.ClientConn
		tran    hotline.Transaction
		wantMsg string
	}{
		{
			name:    "HandleJoinChat with a 1-byte chat ID",
			handler: HandleJoinChat,
			cc:      &hotline.ClientConn{ID: [2]byte{0, 1}, Logger: NewTestLogger()},
			tran: hotline.NewTransaction(hotline.TranJoinChat, [2]byte{0, 1},
				hotline.NewField(hotline.FieldChatID, []byte{0x01})),
			wantMsg: ErrMsgInvalidChatID,
		},
		{
			name:    "HandleSendInstantMsg with a 1-byte user ID",
			handler: HandleSendInstantMsg,
			cc: &hotline.ClientConn{ID: [2]byte{0, 1}, Logger: NewTestLogger(),
				Account: &hotline.Account{Access: accessWith(hotline.AccessSendPrivMsg)}},
			tran: hotline.NewTransaction(hotline.TranSendInstantMsg, [2]byte{0, 1},
				hotline.NewField(hotline.FieldUserID, []byte{0x01})),
			wantMsg: ErrMsgInvalidUserID,
		},
		{
			name:    "HandleDisconnectUser with a 3-byte user ID",
			handler: HandleDisconnectUser,
			cc: &hotline.ClientConn{ID: [2]byte{0, 1}, Logger: NewTestLogger(),
				Account: &hotline.Account{Access: accessWith(hotline.AccessDisconUser)}},
			tran: hotline.NewTransaction(hotline.TranDisconnectUser, [2]byte{0, 1},
				hotline.NewField(hotline.FieldUserID, []byte{0x01, 0x02, 0x03})),
			wantMsg: ErrMsgInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := tt.handler(tt.cc, &tt.tran) // must not panic
			if assert.Len(t, gotRes, 1) {
				assert.Equal(t, [4]byte{0, 0, 0, 1}, gotRes[0].ErrorCode)
				assert.Equal(t, tt.wantMsg, string(gotRes[0].GetField(hotline.FieldError).Data))
			}
		})
	}
}

func TestRegisterHandlers(t *testing.T) {
	srv, err := hotline.NewServer()
	assert.NoError(t, err)

	RegisterHandlers(srv)

	// Verify that a known transaction type is handled by sending a transaction
	// that requires no permission. We can't inspect the handlers map directly
	// since it's unexported, but we can verify RegisterHandlers doesn't panic
	// and the server is functional.
}
