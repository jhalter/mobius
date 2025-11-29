package hotline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemChatManager(t *testing.T) {
	cc1 := &ClientConn{ID: [2]byte{1}}
	cc2 := &ClientConn{ID: [2]byte{2}}

	cm := NewMemChatManager()

	// Create a new chat with cc1 as initial member.
	randChatID := cm.New(cc1)
	assert.Equalf(t, []*ClientConn{cc1}, cm.Members(randChatID), "Initial ChatMembers")

	// Second client joins.
	cm.Join(randChatID, cc2)
	assert.Equalf(t, []*ClientConn{cc1, cc2}, cm.Members(randChatID), "ChatMembers")

	// Initial subject is blank.
	assert.Equalf(t, "", cm.GetSubject(randChatID), "ChatMembers")

	// Update subject.
	cm.SetSubject(randChatID, "test")
	assert.Equalf(t, "test", cm.GetSubject(randChatID), "ChatMembers")

	// Second client leaves.
	cm.Leave(randChatID, cc2.ID)
	assert.Equalf(t, []*ClientConn{cc1}, cm.Members(randChatID), "ChatMembers")

	//
	//type fields struct {
	//	chats map[ChatID]*PrivateChat
	//}
	//type args struct {
	//	cc *ClientConn
	//}
	//tests := []struct {
	//	name   string
	//	fields fields
	//	args   args
	//	want   ChatID
	//}{
	//	{
	//		name: "creates new chat",
	//		fields: fields{
	//			chats: make(map[ChatID]*PrivateChat),
	//		},
	//		args: args{
	//			cc: &ClientConn{ID: [2]byte{1}},
	//		},
	//		//want: ,
	//	},
	//}
	//for _, tt := range tests {
	//	t.Run(tt.name, func(t *testing.T) {
	//		cm := &MemChatManager{
	//			chats: tt.fields.chats,
	//			mu:    sync.Mutex{},
	//		}
	//
	//		assert.Equalf(t, tt.want, cm.New(tt.args.cc), "New(%v)", tt.args.cc)
	//	})
	//}
}
