package hotline

import (
	"crypto/rand"
	"github.com/stretchr/testify/mock"
	"slices"
	"sync"
)

type PrivateChat struct {
	Subject    string
	ClientConn map[[2]byte]*ClientConn
}

type ChatID [4]byte

type ChatManager interface {
	New(cc *ClientConn) ChatID
	GetSubject(id ChatID) string
	Join(id ChatID, cc *ClientConn)
	Leave(id ChatID, clientID [2]byte)
	SetSubject(id ChatID, subject string)
	Members(id ChatID) []*ClientConn
}

type MockChatManager struct {
	mock.Mock
}

func (m *MockChatManager) New(cc *ClientConn) ChatID {
	args := m.Called(cc)

	return args.Get(0).(ChatID)
}

func (m *MockChatManager) GetSubject(id ChatID) string {
	args := m.Called(id)

	return args.String(0)
}

func (m *MockChatManager) Join(id ChatID, cc *ClientConn) {
	m.Called(id, cc)
}

func (m *MockChatManager) Leave(id ChatID, clientID [2]byte) {
	m.Called(id, clientID)
}

func (m *MockChatManager) SetSubject(id ChatID, subject string) {
	m.Called(id, subject)

}

func (m *MockChatManager) Members(id ChatID) []*ClientConn {
	args := m.Called(id)

	return args.Get(0).([]*ClientConn)
}

type MemChatManager struct {
	chats map[ChatID]*PrivateChat

	mu sync.Mutex
}

func NewMemChatManager() *MemChatManager {
	return &MemChatManager{
		chats: make(map[ChatID]*PrivateChat),
	}
}

func (cm *MemChatManager) New(cc *ClientConn) ChatID {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var randID [4]byte
	_, _ = rand.Read(randID[:])

	cm.chats[randID] = &PrivateChat{ClientConn: make(map[[2]byte]*ClientConn)}

	cm.chats[randID].ClientConn[cc.ID] = cc

	return randID
}

func (cm *MemChatManager) Join(id ChatID, cc *ClientConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	chat := cm.chats[id]
	chat.ClientConn[cc.ID] = cc
}

func (cm *MemChatManager) Leave(id ChatID, clientID [2]byte) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	privChat, ok := cm.chats[id]
	if !ok {
		return
	}

	delete(privChat.ClientConn, clientID)
}

func (cm *MemChatManager) GetSubject(id ChatID) string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.chats[id].Subject
}

func (cm *MemChatManager) Members(id ChatID) []*ClientConn {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	chat := cm.chats[id]

	var members []*ClientConn
	for _, cc := range chat.ClientConn {
		members = append(members, cc)
	}

	slices.SortFunc(members, clientConnSortFunc)

	return members
}

func (cm *MemChatManager) SetSubject(id ChatID, subject string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	chat := cm.chats[id]

	chat.Subject = subject
}
