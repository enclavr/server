package services

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type UserRateLimiter struct {
	cache  *CacheService
	config RateLimiterConfig
}

type RateLimiterConfig struct {
	RequestsPerMinute int
	RequestsPerHour   int
	RequestsPerDay    int
	BurstLimit        int
	BlockDuration     time.Duration
}

var defaultRateLimiterConfig = RateLimiterConfig{
	RequestsPerMinute: 60,
	RequestsPerHour:   1000,
	RequestsPerDay:    10000,
	BurstLimit:        10,
	BlockDuration:     5 * time.Minute,
}

func NewUserRateLimiter(cache *CacheService, config RateLimiterConfig) *UserRateLimiter {
	if config.RequestsPerMinute == 0 {
		config = defaultRateLimiterConfig
	}
	return &UserRateLimiter{
		cache:  cache,
		config: config,
	}
}

func (r *UserRateLimiter) Allow(ctx context.Context, userID string) (bool, error) {
	if r.cache == nil {
		return true, nil
	}

	minuteKey := fmt.Sprintf("ratelimit:user:%s:minute", userID)
	hourKey := fmt.Sprintf("ratelimit:user:%s:hour", userID)
	dayKey := fmt.Sprintf("ratelimit:user:%s:day", userID)
	blockKey := fmt.Sprintf("ratelimit:user:%s:blocked", userID)

	blocked, _ := r.cache.Exists(ctx, blockKey)
	if blocked {
		return false, fmt.Errorf("user is temporarily blocked")
	}

	minuteCount, _ := r.cache.Incr(ctx, minuteKey)
	if minuteCount == 1 {
		if err := r.cache.Expire(ctx, minuteKey, time.Minute); err != nil {
			return false, err
		}
	}
	if int(minuteCount) > r.config.RequestsPerMinute {
		r.blockUser(ctx, userID)
		return false, fmt.Errorf("minute rate limit exceeded")
	}

	hourCount, _ := r.cache.Incr(ctx, hourKey)
	if hourCount == 1 {
		if err := r.cache.Expire(ctx, hourKey, time.Hour); err != nil {
			return false, err
		}
	}
	if int(hourCount) > r.config.RequestsPerHour {
		r.blockUser(ctx, userID)
		return false, fmt.Errorf("hourly rate limit exceeded")
	}

	dayCount, _ := r.cache.Incr(ctx, dayKey)
	if dayCount == 1 {
		if err := r.cache.Expire(ctx, dayKey, 24*time.Hour); err != nil {
			return false, err
		}
	}
	if int(dayCount) > r.config.RequestsPerDay {
		r.blockUser(ctx, userID)
		return false, fmt.Errorf("daily rate limit exceeded")
	}

	return true, nil
}

func (r *UserRateLimiter) blockUser(ctx context.Context, userID string) {
	blockKey := fmt.Sprintf("ratelimit:user:%s:blocked", userID)
	if err := r.cache.Set(ctx, blockKey, true, r.config.BlockDuration); err != nil {
		return
	}
}

func (r *UserRateLimiter) Unblock(ctx context.Context, userID string) error {
	if r.cache == nil {
		return nil
	}
	blockKey := fmt.Sprintf("ratelimit:user:%s:blocked", userID)
	return r.cache.Delete(ctx, blockKey)
}

func (r *UserRateLimiter) GetRemaining(ctx context.Context, userID string) (minute, hour, day int) {
	if r.cache == nil {
		return r.config.RequestsPerMinute, r.config.RequestsPerHour, r.config.RequestsPerDay
	}

	minuteKey := fmt.Sprintf("ratelimit:user:%s:minute", userID)
	hourKey := fmt.Sprintf("ratelimit:user:%s:hour", userID)
	dayKey := fmt.Sprintf("ratelimit:user:%s:day", userID)

	minuteTTL, _ := r.cache.TTL(ctx, minuteKey)
	hourTTL, _ := r.cache.TTL(ctx, hourKey)
	dayTTL, _ := r.cache.TTL(ctx, dayKey)

	var minuteUsed, hourUsed, dayUsed int64
	var tmp int
	if err := r.cache.Get(ctx, minuteKey, &tmp); err == nil {
		minuteUsed = int64(tmp)
	}
	if err := r.cache.Get(ctx, hourKey, &tmp); err == nil {
		hourUsed = int64(tmp)
	}
	if err := r.cache.Get(ctx, dayKey, &tmp); err == nil {
		dayUsed = int64(tmp)
	}

	minute = r.config.RequestsPerMinute - int(minuteUsed)
	hour = r.config.RequestsPerHour - int(hourUsed)
	day = r.config.RequestsPerDay - int(dayUsed)

	if minuteTTL <= 0 {
		minute = r.config.RequestsPerMinute
	}
	if hourTTL <= 0 {
		hour = r.config.RequestsPerHour
	}
	if dayTTL <= 0 {
		day = r.config.RequestsPerDay
	}

	return minute, hour, day
}

func (r *UserRateLimiter) Reset(ctx context.Context, userID string) error {
	if r.cache == nil {
		return nil
	}

	keys := []string{
		fmt.Sprintf("ratelimit:user:%s:minute", userID),
		fmt.Sprintf("ratelimit:user:%s:hour", userID),
		fmt.Sprintf("ratelimit:user:%s:day", userID),
		fmt.Sprintf("ratelimit:user:%s:blocked", userID),
	}
	return r.cache.DeleteMulti(ctx, keys)
}

type SlidingWindowRateLimiter struct {
	client *redis.Client
	config RateLimiterConfig
}

func NewSlidingWindowRateLimiter(client *redis.Client, config RateLimiterConfig) *SlidingWindowRateLimiter {
	if config.RequestsPerMinute == 0 {
		config = defaultRateLimiterConfig
	}
	return &SlidingWindowRateLimiter{
		client: client,
		config: config,
	}
}

func (r *SlidingWindowRateLimiter) Allow(ctx context.Context, key string) (bool, int, error) {
	now := time.Now()
	windowStart := now.Add(-time.Minute)

	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])

		local requests = redis.call('ZRANGEBYSCORE', key, window_start, now)
		local count = #requests

		if count >= limit then
			return {0, count}
		end

		local score = tostring(now) .. '.' .. math.random(1000000)
		redis.call('ZADD', key, score, score)
		redis.call('EXPIRE', key, 60)

		return {1, count + 1}
	`)

	result, err := script.Run(ctx, r.client, []string{"sliding:" + key},
		now.UnixNano(),
		windowStart.UnixNano(),
		r.config.RequestsPerMinute,
	).Slice()

	if err != nil {
		return false, 0, err
	}

	if len(result) < 2 {
		return false, 0, fmt.Errorf("unexpected result from redis script")
	}

	result0, ok := result[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected result type for allowed")
	}
	result1, ok := result[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected result type for count")
	}

	allowed := result0 == 1
	count := int(result1)

	return allowed, count, nil
}

type TokenBucketRateLimiter struct {
	client     *redis.Client
	capacity   int64
	refillRate int64
	window     time.Duration
}

func NewTokenBucketRateLimiter(client *redis.Client, capacity int64, refillRate int64, window time.Duration) *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		client:     client,
		capacity:   capacity,
		refillRate: refillRate,
		window:     window,
	}
}

func (r *TokenBucketRateLimiter) Allow(ctx context.Context, key string) (bool, int64, error) {
	script := redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local window_secs = tonumber(ARGV[4])

		local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1])
		local last_refill = tonumber(bucket[2])

		if not tokens then
			tokens = capacity
			last_refill = now
		end

		local elapsed = (now - last_refill) / 1000000000
		local new_tokens = math.min(capacity, tokens + (elapsed * refill_rate / window_secs))

		if new_tokens < 1 then
			return {0, new_tokens}
		end

		redis.call('HMSET', key, 'tokens', new_tokens - 1, 'last_refill', now)
		redis.call('EXPIRE', key, 3600)

		return {1, new_tokens - 1}
	`)

	result, err := script.Run(ctx, r.client, []string{"tokenbucket:" + key},
		r.capacity,
		r.refillRate,
		time.Now().UnixNano(),
		int64(r.window.Seconds()),
	).Slice()

	if err != nil {
		return false, 0, err
	}

	if len(result) < 2 {
		return false, 0, fmt.Errorf("unexpected result from redis script")
	}

	result0, ok := result[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected result type for allowed")
	}
	result1, ok := result[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected result type for tokens")
	}

	allowed := result0 == 1
	tokens := result1

	return allowed, tokens, nil
}
