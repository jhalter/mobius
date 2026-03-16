package hotline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemClientMgr_Add(t *testing.T) {
	mgr := NewMemClientMgr()

	c1 := &ClientConn{}
	c2 := &ClientConn{}
	mgr.Add(c1)
	mgr.Add(c2)

	assert.NotEqual(t, c1.ID, c2.ID)
	assert.Len(t, mgr.List(), 2)
}

func TestMemClientMgr_Get(t *testing.T) {
	mgr := NewMemClientMgr()

	assert.Nil(t, mgr.Get(ClientID{0xFF, 0xFF}))

	c := &ClientConn{}
	mgr.Add(c)
	assert.Equal(t, c, mgr.Get(c.ID))
}

func TestMemClientMgr_Delete(t *testing.T) {
	mgr := NewMemClientMgr()

	c := &ClientConn{}
	mgr.Add(c)
	id := c.ID

	mgr.Delete(id)
	assert.Nil(t, mgr.Get(id))
}

func TestMemClientMgr_List_Sorted(t *testing.T) {
	mgr := NewMemClientMgr()

	// Add 3 clients - they'll get sequential IDs
	c1 := &ClientConn{}
	c2 := &ClientConn{}
	c3 := &ClientConn{}
	mgr.Add(c1)
	mgr.Add(c2)
	mgr.Add(c3)

	list := mgr.List()
	assert.Len(t, list, 3)

	// Verify sorted by ID
	for i := 1; i < len(list); i++ {
		assert.True(t, list[i-1].ID[0] < list[i].ID[0] ||
			(list[i-1].ID[0] == list[i].ID[0] && list[i-1].ID[1] <= list[i].ID[1]),
			"clients should be sorted by ID")
	}
}
