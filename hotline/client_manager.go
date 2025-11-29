package hotline

import (
	"cmp"
	"encoding/binary"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/stretchr/testify/mock"
)

type ClientID [2]byte

type ClientManager interface {
	List() []*ClientConn // Returns list of sorted clients
	Get(id ClientID) *ClientConn
	Add(cc *ClientConn)
	Delete(id ClientID)
}

type MockClientMgr struct {
	mock.Mock
}

func (m *MockClientMgr) List() []*ClientConn {
	args := m.Called()

	return args.Get(0).([]*ClientConn)
}

func (m *MockClientMgr) Get(id ClientID) *ClientConn {
	args := m.Called(id)

	return args.Get(0).(*ClientConn)
}

func (m *MockClientMgr) Add(cc *ClientConn) {
	m.Called(cc)
}
func (m *MockClientMgr) Delete(id ClientID) {
	m.Called(id)
}

type MemClientMgr struct {
	clients map[ClientID]*ClientConn

	mu           sync.Mutex
	nextClientID atomic.Uint32
}

func NewMemClientMgr() *MemClientMgr {
	return &MemClientMgr{
		clients: make(map[ClientID]*ClientConn),
	}
}

// List returns slice of sorted clients.
func (cm *MemClientMgr) List() []*ClientConn {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var clients []*ClientConn
	for _, client := range cm.clients {
		clients = append(clients, client)
	}

	slices.SortFunc(clients, func(a, b *ClientConn) int {
		return cmp.Compare(
			binary.BigEndian.Uint16(a.ID[:]),
			binary.BigEndian.Uint16(b.ID[:]),
		)
	})

	return clients
}

func (cm *MemClientMgr) Get(id ClientID) *ClientConn {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.clients[id]
}

func (cm *MemClientMgr) Add(cc *ClientConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.nextClientID.Add(1)
	binary.BigEndian.PutUint16(cc.ID[:], uint16(cm.nextClientID.Load()))

	cm.clients[cc.ID] = cc
}

func (cm *MemClientMgr) Delete(id ClientID) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.clients, id)
}
