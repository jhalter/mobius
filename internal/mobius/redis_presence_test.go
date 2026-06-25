package mobius

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isMember(t *testing.T, s *miniredis.Miniredis, member string) bool {
	t.Helper()
	// miniredis returns an error (rather than false, like real Redis) when the key is absent,
	// which happens once the set is emptied.
	if !s.Exists(redisKeyOnline) {
		return false
	}
	ok, err := s.SIsMember(redisKeyOnline, member)
	require.NoError(t, err)
	return ok
}

func newTestPresenceTracker(t *testing.T) (*RedisPresenceTracker, *miniredis.Miniredis) {
	t.Helper()

	s, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(s.Close)

	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRedisPresenceTracker(client, logger), s
}

func TestRedisPresenceTracker_LegacyMemberFormats(t *testing.T) {
	// Lock the exact Redis set-member strings so existing deployments keep working.
	tracker, s := newTestPresenceTracker(t)

	t.Run("UserConnected stores login::ip", func(t *testing.T) {
		tracker.UserConnected("alice", "192.168.1.1")
		assert.True(t, isMember(t, s, "alice::192.168.1.1"))
	})

	t.Run("UserRenamed with no old nickname swaps login::ip for login:nick:ip", func(t *testing.T) {
		s.FlushAll()
		tracker.UserConnected("bob", "10.0.0.1")
		tracker.UserRenamed("bob", "", "Bobby", "10.0.0.1")

		assert.False(t, isMember(t, s, "bob::10.0.0.1"))
		assert.True(t, isMember(t, s, "bob:Bobby:10.0.0.1"))
	})

	t.Run("UserRenamed removes the previous nickname entry", func(t *testing.T) {
		s.FlushAll()
		tracker.UserRenamed("carol", "", "Carol1", "10.0.0.2")
		tracker.UserRenamed("carol", "Carol1", "Carol2", "10.0.0.2")

		assert.False(t, isMember(t, s, "carol:Carol1:10.0.0.2"))
		assert.True(t, isMember(t, s, "carol:Carol2:10.0.0.2"))
	})

	t.Run("UserDisconnected removes both possible entries", func(t *testing.T) {
		s.FlushAll()
		tracker.UserRenamed("dave", "", "Dave", "10.0.0.3")
		tracker.UserDisconnected("dave", "Dave", "10.0.0.3")

		assert.False(t, isMember(t, s, "dave::10.0.0.3"))
		assert.False(t, isMember(t, s, "dave:Dave:10.0.0.3"))
	})

	t.Run("UserDisconnected with empty nickname removes login::ip", func(t *testing.T) {
		s.FlushAll()
		tracker.UserConnected("erin", "10.0.0.4")
		tracker.UserDisconnected("erin", "", "10.0.0.4")

		assert.False(t, isMember(t, s, "erin::10.0.0.4"))
	})
}

func TestRedisPresenceTracker_Online(t *testing.T) {
	tracker, _ := newTestPresenceTracker(t)
	ctx := context.Background()

	tracker.UserRenamed("alice", "", "Alice", "192.168.1.1")
	tracker.UserRenamed("bob", "", "Bob", "192.168.1.2")

	users, err := tracker.Online(ctx)
	require.NoError(t, err)
	require.Len(t, users, 2)

	// Order is not guaranteed by the Redis set, so index by login.
	byLogin := map[string]OnlineUser{}
	for _, u := range users {
		byLogin[u.Login] = u
	}
	assert.Equal(t, OnlineUser{Login: "alice", Nickname: "Alice", IP: "192.168.1.1"}, byLogin["alice"])
	assert.Equal(t, OnlineUser{Login: "bob", Nickname: "Bob", IP: "192.168.1.2"}, byLogin["bob"])
}

func TestRedisPresenceTracker_OnlinePreNicknameEntry(t *testing.T) {
	// A connected-but-not-yet-named user is stored as "login::ip", which splits into three
	// parts with an empty nickname. The previous api.go logic included it the same way, so
	// preserve that behavior.
	tracker, _ := newTestPresenceTracker(t)

	tracker.UserConnected("alice", "192.168.1.1")

	users, err := tracker.Online(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, OnlineUser{Login: "alice", Nickname: "", IP: "192.168.1.1"}, users[0])
}

func TestRedisPresenceTracker_Clear(t *testing.T) {
	tracker, s := newTestPresenceTracker(t)

	tracker.UserRenamed("alice", "", "Alice", "192.168.1.1")
	require.NoError(t, tracker.Clear(context.Background()))

	assert.False(t, s.Exists(redisKeyOnline))
}
