package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheService struct {
	client    *redis.Client
	local     *LocalCache
	localOnly bool
}

type LocalCache struct {
	mu      sync.RWMutex
	items   map[string]cacheItem
	expires map[string]time.Time
	stopCh  chan struct{}
}

type cacheItem struct {
	Value interface{}
}

func NewLocalCache() *LocalCache {
	lc := &LocalCache{
		items:   make(map[string]cacheItem),
		expires: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
	go lc.cleanup()
	return lc
}

func (lc *LocalCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-lc.stopCh:
			return
		case <-ticker.C:
			lc.cleanupOnce()
		}
	}
}

func (lc *LocalCache) Stop() {
	close(lc.stopCh)
}

func (lc *LocalCache) cleanupOnce() {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	now := time.Now()
	for key, expires := range lc.expires {
		if now.After(expires) {
			delete(lc.items, key)
			delete(lc.expires, key)
		}
	}
}

func (lc *LocalCache) Get(key string) (interface{}, bool) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if expires, ok := lc.expires[key]; ok && time.Now().After(expires) {
		return nil, false
	}

	if item, ok := lc.items[key]; ok {
		return item.Value, true
	}
	return nil, false
}

func (lc *LocalCache) Set(key string, value interface{}, expiration time.Duration) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.items[key] = cacheItem{Value: value}
	if expiration > 0 {
		lc.expires[key] = time.Now().Add(expiration)
	}
}

func (lc *LocalCache) Delete(key string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	delete(lc.items, key)
	delete(lc.expires, key)
}

func (lc *LocalCache) Clear() {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.items = make(map[string]cacheItem)
	lc.expires = make(map[string]time.Time)
}

func NewCacheService(host, password string, db int, enableLocal bool) *CacheService {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:6379", host),
		Password: password,
		DB:       db,
	})

	cs := &CacheService{
		client: client,
	}

	if enableLocal {
		cs.local = NewLocalCache()
	} else {
		cs.localOnly = true
	}

	return cs
}

func NewCacheServiceFromClient(client *redis.Client, enableLocal bool) *CacheService {
	cs := &CacheService{
		client: client,
	}

	if enableLocal {
		cs.local = NewLocalCache()
	} else {
		cs.localOnly = true
	}

	return cs
}

func (cs *CacheService) Ping(ctx context.Context) error {
	return cs.client.Ping(ctx).Err()
}

func (cs *CacheService) Close() error {
	if cs.local != nil {
		cs.local.Stop()
		cs.local.Clear()
	}
	return cs.client.Close()
}

func (cs *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	if cs.local != nil {
		if val, ok := cs.local.Get(key); ok {
			data, err := json.Marshal(val)
			if err == nil {
				if err := json.Unmarshal(data, dest); err == nil {
					return nil
				}
			}
		}
	}

	if cs.localOnly {
		return redis.Nil
	}

	data, err := cs.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

func (cs *CacheService) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if cs.local != nil {
		cs.local.Set(key, value, expiration)
	}

	if cs.localOnly {
		return nil
	}

	return cs.client.Set(ctx, key, data, expiration).Err()
}

func (cs *CacheService) Delete(ctx context.Context, key string) error {
	if cs.local != nil {
		cs.local.Delete(key)
	}

	if cs.localOnly {
		return nil
	}

	return cs.client.Del(ctx, key).Err()
}

func (cs *CacheService) Exists(ctx context.Context, key string) (bool, error) {
	if cs.local != nil {
		if _, ok := cs.local.Get(key); ok {
			return true, nil
		}
	}

	if cs.localOnly {
		return false, nil
	}

	n, err := cs.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (cs *CacheService) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if cs.local != nil {
		if val, ok := cs.local.Get(key); ok {
			cs.local.Set(key, val, expiration)
		}
	}

	if cs.localOnly {
		return nil
	}

	return cs.client.Expire(ctx, key, expiration).Err()
}

func (cs *CacheService) Incr(ctx context.Context, key string) (int64, error) {
	if cs.localOnly {
		return 0, redis.Nil
	}

	return cs.client.Incr(ctx, key).Result()
}

func (cs *CacheService) Decr(ctx context.Context, key string) (int64, error) {
	if cs.localOnly {
		return 0, redis.Nil
	}

	return cs.client.Decr(ctx, key).Result()
}

func (cs *CacheService) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	if cs.localOnly {
		return 0, redis.Nil
	}

	return cs.client.IncrBy(ctx, key, value).Result()
}

func (cs *CacheService) TTL(ctx context.Context, key string) (time.Duration, error) {
	if cs.localOnly {
		return 0, nil
	}

	return cs.client.TTL(ctx, key).Result()
}

func (cs *CacheService) GetMulti(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	if cs.localOnly {
		result := make(map[string]interface{})
		return result, nil
	}

	results, err := cs.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for i, val := range results {
		if val != nil {
			str, ok := val.(string)
			if !ok {
				continue
			}
			var dest interface{}
			if err := json.Unmarshal([]byte(str), &dest); err == nil {
				result[keys[i]] = dest
			}
		}
	}

	return result, nil
}

func (cs *CacheService) SetMulti(ctx context.Context, items map[string]interface{}, expiration time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	if cs.local != nil {
		for key, value := range items {
			cs.local.Set(key, value, expiration)
		}
	}

	if cs.localOnly {
		return nil
	}

	pipe := cs.client.Pipeline()
	for key, value := range items {
		data, err := json.Marshal(value)
		if err != nil {
			continue
		}
		pipe.Set(ctx, key, data, expiration)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (cs *CacheService) DeleteMulti(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	if cs.local != nil {
		for _, key := range keys {
			cs.local.Delete(key)
		}
	}

	if cs.localOnly {
		return nil
	}

	return cs.client.Del(ctx, keys...).Err()
}

func (cs *CacheService) Flush(ctx context.Context) error {
	if cs.local != nil {
		cs.local.Clear()
	}

	if cs.localOnly {
		return nil
	}

	return cs.client.FlushDB(ctx).Err()
}

type RateLimiterStore struct {
	cache *CacheService
}

func NewRateLimiterStore(cache *CacheService) *RateLimiterStore {
	return &RateLimiterStore{cache: cache}
}

func (r *RateLimiterStore) GetLimiter(key string) ([]time.Time, int, time.Duration, bool) {
	var requests []time.Time
	err := r.cache.Get(context.Background(), fmt.Sprintf("ratelimit:%s:requests", key), &requests)
	if err != nil {
		return nil, 0, 0, false
	}

	ttl, err := r.cache.TTL(context.Background(), fmt.Sprintf("ratelimit:%s:requests", key))
	if err != nil {
		return nil, 0, 0, false
	}

	return requests, len(requests), ttl, true
}

func (r *RateLimiterStore) SetLimiter(key string, requests []time.Time, limit int, window time.Duration) {
	r.cache.Set(context.Background(), fmt.Sprintf("ratelimit:%s:requests", key), requests, window)
}

func (r *RateLimiterStore) Increment(key string) (int, error) {
	count, err := r.cache.IncrBy(context.Background(), fmt.Sprintf("ratelimit:%s:count", key), 1)
	if err != nil {
		return 0, err
	}

	ttl, err := r.cache.TTL(context.Background(), fmt.Sprintf("ratelimit:%s:count", key))
	if err != nil || ttl <= 0 {
		r.cache.Expire(context.Background(), fmt.Sprintf("ratelimit:%s:count", key), time.Minute)
		count, _ = r.cache.IncrBy(context.Background(), fmt.Sprintf("ratelimit:%s:count", key), 0)
	}

	return int(count), nil
}

func (r *RateLimiterStore) Reset(key string) {
	r.cache.Delete(context.Background(), fmt.Sprintf("ratelimit:%s:requests", key))
	r.cache.Delete(context.Background(), fmt.Sprintf("ratelimit:%s:count", key))
}
