package mobius

import (
	"context"
	"log/slog"
	"strings"

	"github.com/redis/go-redis/v9"
)

// redisKeyOnline is the Redis set holding currently online users.
const redisKeyOnline = "mobius:online"

// OnlineUser describes a currently online user as reported by an OnlineLister.
type OnlineUser struct {
	Login    string `json:"login"`
	Nickname string `json:"nickname"`
	IP       string `json:"ip"`
}

// RedisPresenceTracker implements hotline.PresenceTracker backed by a Redis set. Set members
// use the legacy formats "login::ip" (nickname unknown) and "login:nickname:ip" so existing
// deployments and tooling continue to work.
type RedisPresenceTracker struct {
	client *redis.Client
	logger *slog.Logger
}

// NewRedisPresenceTracker creates a new Redis-backed presence tracker.
func NewRedisPresenceTracker(client *redis.Client, logger *slog.Logger) *RedisPresenceTracker {
	return &RedisPresenceTracker{client: client, logger: logger}
}

func (r *RedisPresenceTracker) UserConnected(login, ip string) {
	if err := r.client.SAdd(context.Background(), redisKeyOnline, login+"::"+ip).Err(); err != nil {
		r.logger.Warn("Failed to record online user in Redis", "err", err)
	}
}

func (r *RedisPresenceTracker) UserRenamed(login, oldNickname, newNickname, ip string) {
	ctx := context.Background()
	// Remove the pre-nickname entry and, if present, the previous nickname entry.
	r.client.SRem(ctx, redisKeyOnline, login+"::"+ip)
	if oldNickname != "" {
		r.client.SRem(ctx, redisKeyOnline, login+":"+oldNickname+":"+ip)
	}
	if err := r.client.SAdd(ctx, redisKeyOnline, login+":"+newNickname+":"+ip).Err(); err != nil {
		r.logger.Warn("Failed to update online user in Redis", "err", err)
	}
}

func (r *RedisPresenceTracker) UserDisconnected(login, nickname, ip string) {
	ctx := context.Background()
	r.client.SRem(ctx, redisKeyOnline, login+"::"+ip)
	if nickname != "" {
		r.client.SRem(ctx, redisKeyOnline, login+":"+nickname+":"+ip)
	}
}

// Clear removes all online-user entries. Call on startup to discard stale state from a
// previous run.
func (r *RedisPresenceTracker) Clear(ctx context.Context) error {
	return r.client.Del(ctx, redisKeyOnline).Err()
}

// Online returns the list of currently online users parsed from the Redis set.
func (r *RedisPresenceTracker) Online(ctx context.Context) ([]OnlineUser, error) {
	members, err := r.client.SMembers(ctx, redisKeyOnline).Result()
	if err != nil {
		return nil, err
	}

	var users []OnlineUser
	for _, m := range members {
		parts := strings.SplitN(m, ":", 3)
		if len(parts) == 3 {
			users = append(users, OnlineUser{Login: parts[0], Nickname: parts[1], IP: parts[2]})
		}
	}
	return users, nil
}
