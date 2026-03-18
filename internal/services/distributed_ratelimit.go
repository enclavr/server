package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type SlidingRateLimiter struct {
	cache  *CacheService
	limit  int
	window time.Duration
}

type RateLimitResult struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	Current    int
}

func NewSlidingRateLimiter(cache *CacheService, limit int, window time.Duration) *SlidingRateLimiter {
	return &SlidingRateLimiter{
		cache:  cache,
		limit:  limit,
		window: window,
	}
}

func (rl *SlidingRateLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return rl.AllowWithCost(ctx, key, 1)
}

func (rl *SlidingRateLimiter) AllowWithCost(ctx context.Context, key string, cost int64) (*RateLimitResult, error) {
	redisKey := fmt.Sprintf("ratelimit:sliding:%s", key)

	pipe := rl.cache.client.Pipeline()

	currentTime := time.Now().Unix()
	windowStart := currentTime - int64(rl.window.Seconds())

	pipe.ZRemRangeByScore(ctx, redisKey, "0", strconv.FormatInt(windowStart, 10))

	countCmd := pipe.ZCard(ctx, redisKey)

	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(currentTime),
		Member: fmt.Sprintf("%d:%d", currentTime, cost),
	})

	pipe.Expire(ctx, redisKey, rl.window)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to execute rate limit pipeline: %w", err)
	}

	current := int(countCmd.Val())
	remaining := rl.limit - current

	allowed := current < rl.limit

	result := &RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		Current:    current,
		RetryAfter: 0,
	}

	if !allowed && current > 0 {
		oldestCmd := rl.cache.client.ZRange(ctx, redisKey, 0, 0)
		oldest, err := oldestCmd.Result()
		if err == nil && len(oldest) > 0 {
			if score, err := rl.cache.client.ZScore(ctx, redisKey, oldest[0]).Result(); err == nil {
				expiresAt := time.Unix(int64(score), 0).Add(rl.window)
				result.RetryAfter = time.Until(expiresAt)
			}
		}
	}

	return result, nil
}

func (rl *SlidingRateLimiter) SetLimit(limit int) {
	rl.limit = limit
}

func (rl *SlidingRateLimiter) SetWindow(window time.Duration) {
	rl.window = window
}

func (rl *SlidingRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("ratelimit:sliding:%s", key)
	return rl.cache.client.Del(ctx, redisKey).Err()
}

type FixedRateLimiter struct {
	cache  *CacheService
	limit  int
	window time.Duration
}

func NewFixedRateLimiter(cache *CacheService, limit int, window time.Duration) *FixedRateLimiter {
	return &FixedRateLimiter{
		cache:  cache,
		limit:  limit,
		window: window,
	}
}

func (rl *FixedRateLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	windowKey := fmt.Sprintf("ratelimit:fixed:%s", key)

	currentWindow := time.Now().Unix() / int64(rl.window.Seconds())
	windowKey = fmt.Sprintf("%s:%d", windowKey, currentWindow)

	count, err := rl.cache.client.IncrBy(ctx, windowKey, 1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	if count == 1 {
		rl.cache.client.Expire(ctx, windowKey, rl.window)
	}

	remaining := rl.limit - int(count)
	allowed := count <= int64(rl.limit)

	result := &RateLimitResult{
		Allowed:   allowed,
		Remaining: remaining,
		Current:   int(count),
	}

	if !allowed {
		ttl, _ := rl.cache.client.TTL(ctx, windowKey).Result()
		result.RetryAfter = ttl
	}

	return result, nil
}

func (rl *FixedRateLimiter) Reset(ctx context.Context, key string) error {
	pattern := fmt.Sprintf("ratelimit:fixed:%s:*", key)
	keys, err := rl.cache.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil
	}
	if len(keys) > 0 {
		return rl.cache.client.Del(ctx, keys...).Err()
	}
	return nil
}

type DistributedTokenBucketLimiter struct {
	cache      *CacheService
	capacity   int64
	refillRate float64
	window     time.Duration
}

func NewDistributedTokenBucketLimiter(cache *CacheService, capacity int64, refillPerSecond float64) *DistributedTokenBucketLimiter {
	return &DistributedTokenBucketLimiter{
		cache:      cache,
		capacity:   capacity,
		refillRate: refillPerSecond,
		window:     time.Hour,
	}
}

func (rl *DistributedTokenBucketLimiter) Allow(ctx context.Context, key string, tokens int64) (*RateLimitResult, error) {
	redisKey := fmt.Sprintf("ratelimit:token:%s", key)

	var tokensLeft int64
	var lastRefill int64

	tokensData, err := rl.cache.client.HMGet(ctx, redisKey, "tokens", "last_refill").Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get token bucket state: %w", err)
	}

	if len(tokensData) >= 2 && tokensData[0] != nil {
		if f, ok := tokensData[0].(float64); ok {
			tokensLeft = int64(f)
		}
		if f, ok := tokensData[1].(float64); ok {
			lastRefill = int64(f)
		}
	} else {
		tokensLeft = rl.capacity
		lastRefill = time.Now().Unix()
	}

	now := time.Now().Unix()
	secondsPassed := now - lastRefill
	if secondsPassed > 0 {
		newTokens := float64(secondsPassed) * rl.refillRate
		totalTokens := float64(tokensLeft) + newTokens
		if totalTokens > float64(rl.capacity) {
			totalTokens = float64(rl.capacity)
		}
		tokensLeft = int64(totalTokens)
		lastRefill = now
	}

	if tokensLeft >= tokens {
		tokensLeft -= tokens
		allowed := true
		result := &RateLimitResult{
			Allowed:    allowed,
			Remaining:  int(tokensLeft),
			Current:    int(rl.capacity - tokensLeft),
			RetryAfter: 0,
		}

		rl.cache.client.HSet(ctx, redisKey, map[string]interface{}{
			"tokens":      float64(tokensLeft),
			"last_refill": float64(lastRefill),
		})
		rl.cache.client.Expire(ctx, redisKey, rl.window)

		return result, nil
	}

	waitTime := float64(tokens-tokensLeft) / rl.refillRate
	result := &RateLimitResult{
		Allowed:    false,
		Remaining:  int(tokensLeft),
		Current:    int(rl.capacity - tokensLeft),
		RetryAfter: time.Duration(waitTime * float64(time.Second)),
	}

	return result, nil
}

func (rl *DistributedTokenBucketLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("ratelimit:token:%s", key)
	return rl.cache.client.Del(ctx, redisKey).Err()
}

type MultiLimiter struct {
	limiters []Limiter
}

type Limiter interface {
	Allow(ctx context.Context, key string) (*RateLimitResult, error)
	Reset(ctx context.Context, key string) error
}

func NewMultiLimiter(limiters ...Limiter) *MultiLimiter {
	return &MultiLimiter{
		limiters: limiters,
	}
}

func (m *MultiLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	for _, limiter := range m.limiters {
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			return nil, err
		}
		if !result.Allowed {
			return result, nil
		}
	}
	return &RateLimitResult{Allowed: true, Remaining: 0}, nil
}

func (m *MultiLimiter) Reset(ctx context.Context, key string) error {
	for _, limiter := range m.limiters {
		if err := limiter.Reset(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
