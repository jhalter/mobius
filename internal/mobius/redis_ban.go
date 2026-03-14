package mobius

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/redis/go-redis/v9"
)

// RedisBanMgr implements the BanMgr interface using Redis as the backend
type RedisBanMgr struct {
	client *redis.Client
	logger *slog.Logger
}

// NewRedisBanMgr creates a new Redis-based ban manager
func NewRedisBanMgr(client *redis.Client, logger *slog.Logger) *RedisBanMgr {
	return &RedisBanMgr{
		client: client,
		logger: logger,
	}
}

// Add adds an IP ban (maintains compatibility with existing interface)
func (r *RedisBanMgr) Add(ip string, until *time.Time) error {
	ctx := context.Background()

	if until == nil {
		// Permanent ban - add to the permanent ban set
		return r.client.SAdd(ctx, hotline.RedisKeyBannedIPs, ip).Err()
	}

	// Temporary ban - use a separate key with expiration
	tempKey := hotline.RedisKeyTempBannedIPs + ip
	duration := time.Until(*until)
	if duration <= 0 {
		// Already expired, don't add the ban
		return nil
	}

	return r.client.Set(ctx, tempKey, "1", duration).Err()
}

// IsBanned checks if an IP is banned (maintains compatibility with existing interface)
// On Redis errors, logs the error and returns true (fail-safe: deny access)
func (r *RedisBanMgr) IsBanned(ip string) (bool, *time.Time) {
	ctx := context.Background()

	// Check permanent ban first
	banned, err := r.client.SIsMember(ctx, hotline.RedisKeyBannedIPs, ip).Result()
	if err != nil {
		r.logger.Error("Redis error checking IP ban, denying access (fail-safe)", "ip", ip, "err", err)
		return true, nil // Fail-safe: deny access on error
	}
	if banned {
		return true, nil // Permanent ban
	}

	// Check temporary ban
	tempKey := hotline.RedisKeyTempBannedIPs + ip
	exists, err := r.client.Exists(ctx, tempKey).Result()
	if err != nil {
		r.logger.Error("Redis error checking temp IP ban, denying access (fail-safe)", "ip", ip, "err", err)
		return true, nil // Fail-safe: deny access on error
	}
	if exists > 0 {
		// Get TTL to calculate expiration time
		ttl, err := r.client.TTL(ctx, tempKey).Result()
		if err != nil {
			r.logger.Error("Redis error getting TTL for temp ban, denying access (fail-safe)", "ip", ip, "err", err)
			return true, nil // Fail-safe: deny access on error
		}
		if ttl > 0 {
			expiration := time.Now().Add(ttl)
			return true, &expiration
		}
	}

	return false, nil
}

// UnbanIP removes an IP from both permanent and temporary ban lists
func (r *RedisBanMgr) UnbanIP(ip string) error {
	ctx := context.Background()

	pipe := r.client.Pipeline()
	pipe.SRem(ctx, hotline.RedisKeyBannedIPs, ip)
	pipe.Del(ctx, hotline.RedisKeyTempBannedIPs+ip)

	_, err := pipe.Exec(ctx)
	return err
}

// BanUsername adds a username to the banned users set
func (r *RedisBanMgr) BanUsername(username string) error {
	ctx := context.Background()
	return r.client.SAdd(ctx, hotline.RedisKeyBannedUsers, username).Err()
}

// UnbanUsername removes a username from the banned users set
func (r *RedisBanMgr) UnbanUsername(username string) error {
	ctx := context.Background()
	return r.client.SRem(ctx, hotline.RedisKeyBannedUsers, username).Err()
}

// IsUsernameBanned checks if a username is banned
// On Redis errors, logs the error and returns true (fail-safe: deny access)
func (r *RedisBanMgr) IsUsernameBanned(username string) bool {
	ctx := context.Background()
	banned, err := r.client.SIsMember(ctx, hotline.RedisKeyBannedUsers, username).Result()
	if err != nil {
		r.logger.Error("Redis error checking username ban, denying access (fail-safe)", "username", username, "err", err)
		return true // Fail-safe: deny access on error
	}
	return banned
}

// BanNickname adds a nickname to the banned nicknames set
func (r *RedisBanMgr) BanNickname(nickname string) error {
	ctx := context.Background()
	return r.client.SAdd(ctx, hotline.RedisKeyBannedNicknames, nickname).Err()
}

// UnbanNickname removes a nickname from the banned nicknames set
func (r *RedisBanMgr) UnbanNickname(nickname string) error {
	ctx := context.Background()
	return r.client.SRem(ctx, hotline.RedisKeyBannedNicknames, nickname).Err()
}

// IsNicknameBanned checks if a nickname is banned
// On Redis errors, logs the error and returns true (fail-safe: deny access)
func (r *RedisBanMgr) IsNicknameBanned(nickname string) bool {
	ctx := context.Background()
	banned, err := r.client.SIsMember(ctx, hotline.RedisKeyBannedNicknames, nickname).Result()
	if err != nil {
		r.logger.Error("Redis error checking nickname ban, denying access (fail-safe)", "nickname", nickname, "err", err)
		return true // Fail-safe: deny access on error
	}
	return banned
}

// ListBannedIPs returns all banned IP addresses (both permanent and temporary)
func (r *RedisBanMgr) ListBannedIPs() ([]string, error) {
	ctx := context.Background()

	// Get permanent bans
	permanentIPs, err := r.client.SMembers(ctx, hotline.RedisKeyBannedIPs).Result()
	if err != nil {
		return nil, err
	}

	// Get temporary bans by scanning for temp ban keys (non-blocking unlike Keys())
	var tempIPs []string
	var cursor uint64
	for {
		var keys []string
		keys, cursor, err = r.client.Scan(ctx, cursor, hotline.RedisKeyTempBannedIPs+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			ip := strings.TrimPrefix(key, hotline.RedisKeyTempBannedIPs)
			tempIPs = append(tempIPs, ip)
		}
		if cursor == 0 {
			break
		}
	}

	// Combine and deduplicate
	allIPs := append(permanentIPs, tempIPs...)
	seen := make(map[string]bool)
	var uniqueIPs []string
	for _, ip := range allIPs {
		if !seen[ip] {
			seen[ip] = true
			uniqueIPs = append(uniqueIPs, ip)
		}
	}

	return uniqueIPs, nil
}

// ListBannedUsernames returns all banned usernames
func (r *RedisBanMgr) ListBannedUsernames() ([]string, error) {
	ctx := context.Background()
	return r.client.SMembers(ctx, hotline.RedisKeyBannedUsers).Result()
}

// ListBannedNicknames returns all banned nicknames
func (r *RedisBanMgr) ListBannedNicknames() ([]string, error) {
	ctx := context.Background()
	return r.client.SMembers(ctx, hotline.RedisKeyBannedNicknames).Result()
}

// Ensure RedisBanMgr implements the BanMgr interface
var _ hotline.BanMgr = (*RedisBanMgr)(nil)
