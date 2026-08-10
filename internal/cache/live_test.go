package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveRedisRoundTrip runs against a real Redis when REDIS_ADDR is set
// (CI starts redis:7-alpine as a service container).
func TestLiveRedisRoundTrip(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set (miniredis covers unit tests)")
	}
	password := os.Getenv("REDIS_PASSWORD")
	c, err := New(addr, password, 0)
	if err != nil {
		// Self-hosted runners may lack Docker service containers; unit tests use miniredis.
		t.Skipf("Redis at %s unreachable (%v); miniredis covers unit tests", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "muxcore-cache-redis-live-" + time.Now().Format("150405.000")
	if err := c.Set(ctx, key, []byte("live"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "live" {
		t.Fatalf("got %q", got)
	}
	ok, err := c.Exists(ctx, key)
	if err != nil || !ok {
		t.Fatalf("Exists: ok=%v err=%v", ok, err)
	}
	if err := c.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, err = c.Exists(ctx, key)
	if err != nil || ok {
		t.Fatalf("after Delete Exists: ok=%v err=%v", ok, err)
	}
}
