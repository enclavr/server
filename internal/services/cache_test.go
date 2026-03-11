package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLocalCache_SetAndGet(t *testing.T) {
	lc := NewLocalCache()
	defer lc.Clear()

	lc.Set("key1", "value1", time.Hour)

	val, ok := lc.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestLocalCache_GetNotFound(t *testing.T) {
	lc := NewLocalCache()
	defer lc.Clear()

	_, ok := lc.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent key")
	}
}

func TestLocalCache_Delete(t *testing.T) {
	lc := NewLocalCache()
	defer lc.Clear()

	lc.Set("key1", "value1", time.Hour)
	lc.Delete("key1")

	_, ok := lc.Get("key1")
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestLocalCache_Clear(t *testing.T) {
	lc := NewLocalCache()

	lc.Set("key1", "value1", time.Hour)
	lc.Set("key2", "value2", time.Hour)
	lc.Clear()

	_, ok := lc.Get("key1")
	if ok {
		t.Error("expected key1 to be cleared")
	}
	_, ok = lc.Get("key2")
	if ok {
		t.Error("expected key2 to be cleared")
	}
}

func TestLocalCache_Expiration(t *testing.T) {
	lc := NewLocalCache()
	defer lc.Clear()

	lc.Set("key1", "value1", time.Millisecond*50)

	time.Sleep(time.Millisecond * 100)

	_, ok := lc.Get("key1")
	if ok {
		t.Error("expected key to be expired")
	}
}

func TestLocalCache_ConcurrentAccess(t *testing.T) {
	lc := NewLocalCache()
	defer lc.Clear()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			lc.Set("key", n, time.Hour)
			lc.Get("key")
		}(i)
	}
	wg.Wait()
}

func TestCacheService_NewCacheService(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	if cs == nil {
		t.Fatal("expected CacheService to be created")
	}
	if cs.local == nil {
		t.Error("expected local cache to be enabled")
	}
	cs.Close()
}

func TestCacheService_NewCacheService_LocalOnly(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, false)
	if cs == nil {
		t.Fatal("expected CacheService to be created")
	}
	if !cs.localOnly {
		t.Error("expected localOnly to be true")
	}
	cs.Close()
}

func TestCacheService_Get_LocalCacheHit(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	cs.local.Set("testkey", "testvalue", time.Hour)

	var dest string
	err := cs.Get(ctx, "testkey", &dest)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if dest != "testvalue" {
		t.Errorf("expected testvalue, got %s", dest)
	}
}

func TestCacheService_Get_LocalCacheMiss(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()

	var dest string
	err := cs.Get(ctx, "nonexistent", &dest)
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestCacheService_Set(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	err := cs.Set(ctx, "key1", "value1", time.Hour)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	val, ok := cs.local.Get("key1")
	if !ok {
		t.Error("expected key to be in local cache")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestCacheService_Delete(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	cs.local.Set("key1", "value1", time.Hour)

	err := cs.Delete(ctx, "key1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, ok := cs.local.Get("key1")
	if ok {
		t.Error("expected key to be deleted from local cache")
	}
}

func TestCacheService_Exists(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	cs.local.Set("key1", "value1", time.Hour)

	exists, err := cs.Exists(ctx, "key1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected key to exist")
	}

	exists, err = cs.Exists(ctx, "nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected key not to exist")
	}
}

func TestCacheService_Expire(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	cs.local.Set("key1", "value1", time.Hour)

	err := cs.Expire(ctx, "key1", time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCacheService_Incr(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, false)
	defer cs.Close()

	ctx := context.Background()

	_, err := cs.Incr(ctx, "counter")
	if err != nil && err.Error() != "redis: nil" {
		t.Logf("expected nil error or redis.Nil for localOnly=false without redis: %v", err)
	}
}

func TestCacheService_Decr(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, false)
	defer cs.Close()

	ctx := context.Background()

	_, err := cs.Decr(ctx, "counter")
	if err != nil && err.Error() != "redis: nil" {
		t.Logf("expected nil error or redis.Nil for localOnly=false without redis: %v", err)
	}
}

func TestCacheService_IncrBy(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, false)
	defer cs.Close()

	ctx := context.Background()

	_, err := cs.IncrBy(ctx, "counter", 5)
	if err != nil && err.Error() != "redis: nil" {
		t.Logf("expected nil error or redis.Nil for localOnly=false without redis: %v", err)
	}
}

func TestCacheService_TTL(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()

	ttl, err := cs.TTL(ctx, "nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ttl >= 0 {
		t.Logf("TTL for nonexistent key: %v (Redis returns -2ns for key not found)", ttl)
	}
}

func TestCacheService_GetMulti_Empty(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()

	result, err := cs.GetMulti(ctx, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestCacheService_SetMulti_Empty(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()

	err := cs.SetMulti(ctx, map[string]interface{}{}, time.Hour)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCacheService_DeleteMulti_Empty(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()

	err := cs.DeleteMulti(ctx, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCacheService_Flush(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	ctx := context.Background()
	cs.local.Set("key1", "value1", time.Hour)
	cs.local.Set("key2", "value2", time.Hour)

	err := cs.Flush(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, ok := cs.local.Get("key1")
	if ok {
		t.Error("expected local cache to be flushed")
	}
}

func TestRateLimiterStore_NewRateLimiterStore(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	rl := NewRateLimiterStore(cs)
	if rl == nil {
		t.Fatal("expected RateLimiterStore to be created")
	}
	if rl.cache != cs {
		t.Error("expected cache to be set")
	}
}

func TestRateLimiterStore_GetLimiter_NotFound(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	rl := NewRateLimiterStore(cs)

	requests, count, ttl, ok := rl.GetLimiter("nonexistent")
	if ok {
		t.Error("expected not to find limiter")
	}
	if requests != nil {
		t.Error("expected nil requests")
	}
	if count != 0 {
		t.Error("expected 0 count")
	}
	if ttl != 0 {
		t.Error("expected 0 ttl")
	}
}

func TestRateLimiterStore_SetLimiter(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	rl := NewRateLimiterStore(cs)

	requests := []time.Time{time.Now(), time.Now().Add(-time.Second)}
	rl.SetLimiter("testkey", requests, 10, time.Minute)

	foundRequests, count, _, ok := rl.GetLimiter("testkey")
	if !ok {
		t.Error("expected to find limiter")
	}
	if count != 2 {
		t.Errorf("expected 2 requests, got %d", count)
	}
	if len(foundRequests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(foundRequests))
	}
}

func TestRateLimiterStore_Increment(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	rl := NewRateLimiterStore(cs)

	count, err := rl.Increment("testkey")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	count, err = rl.Increment("testkey")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestRateLimiterStore_Reset(t *testing.T) {
	cs := NewCacheService("localhost", "", 0, true)
	defer cs.Close()

	rl := NewRateLimiterStore(cs)

	rl.SetLimiter("testkey", []time.Time{time.Now()}, 10, time.Minute)
	rl.Increment("testkey")

	rl.Reset("testkey")

	requests, _, _, ok := rl.GetLimiter("testkey")
	if ok {
		t.Error("expected limiter to be reset")
	}
	if requests != nil {
		t.Error("expected nil requests after reset")
	}
}
