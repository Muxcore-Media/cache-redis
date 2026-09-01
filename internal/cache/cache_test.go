package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func testCache(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := New(Config{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, mr
}

func TestCompareAndSwapMatch(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("old"), 0); err != nil {
		t.Fatal(err)
	}
	ok, err := c.CompareAndSwap(ctx, "k", []byte("old"), []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected swap success")
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
}

func TestCompareAndSwapMismatch(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("old"), 0); err != nil {
		t.Fatal(err)
	}
	ok, err := c.CompareAndSwap(ctx, "k", []byte("other"), []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected swap failure")
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("got %q", got)
	}
}

func TestCompareAndSwapNilOldMissingKey(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	ok, err := c.CompareAndSwap(ctx, "missing", nil, []byte("created"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected create via nil oldValue")
	}
	got, err := c.Get(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "created" {
		t.Fatalf("got %q", got)
	}
}

func TestCompareAndSwapNilOldExistingKey(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("present"), 0); err != nil {
		t.Fatal(err)
	}
	ok, err := c.CompareAndSwap(ctx, "k", nil, []byte("nope"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected failure when key exists and oldValue is nil")
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "present" {
		t.Fatalf("got %q", got)
	}
}

func TestCompareAndSwapEmptyOldVsMissing(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	ok, err := c.CompareAndSwap(ctx, "k", []byte{}, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("empty oldValue must not match missing key (nil does)")
	}
}

func TestCompareAndSwapPreservesTTL(t *testing.T) {
	c, mr := testCache(t)
	ctx := context.Background()
	if err := c.Set(ctx, "ttl-key", []byte("v1"), time.Minute); err != nil {
		t.Fatal(err)
	}
	ok, err := c.CompareAndSwap(ctx, "ttl-key", []byte("v1"), []byte("v2"))
	if err != nil || !ok {
		t.Fatalf("cas: ok=%v err=%v", ok, err)
	}
	ttl := mr.TTL("ttl-key")
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("ttl=%v want ~1m", ttl)
	}
	got, err := c.Get(ctx, "ttl-key")
	if err != nil || string(got) != "v2" {
		t.Fatalf("value=%q err=%v", got, err)
	}
}

func TestCompareAndSwapBinaryPayload(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	oldVal := []byte{'a', 0, 'b', 0xff}
	newVal := []byte{0, 'x', 0, 'y'}
	if err := c.Set(ctx, "bin", oldVal, 0); err != nil {
		t.Fatal(err)
	}
	ok, err := c.CompareAndSwap(ctx, "bin", oldVal, newVal)
	if err != nil || !ok {
		t.Fatalf("cas: ok=%v err=%v", ok, err)
	}
	got, err := c.Get(ctx, "bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(newVal) {
		t.Fatalf("len=%d want %d", len(got), len(newVal))
	}
	for i := range got {
		if got[i] != newVal[i] {
			t.Fatalf("byte %d: got %d want %d", i, got[i], newVal[i])
		}
	}
}

func TestLockUnlockDistributedToken(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	token, err := c.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected token")
	}
	if err := c.UnlockByToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := c.UnlockByToken(ctx, token); err != ErrLockNotHeld {
		t.Fatalf("second unlock: %v", err)
	}
}

func TestKeyPrefix(t *testing.T) {
	c, mr := testCache(t)
	c.prefix = "mux:"
	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("mux:k") {
		t.Fatal("expected prefixed redis key")
	}
}

func TestDeleteCount(t *testing.T) {
	c, _ := testCache(t)
	ctx := context.Background()
	if err := c.Set(ctx, "a", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	n, err := c.Delete(ctx, "a", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted=%d want 1", n)
	}
}

func TestSubscribeCancel(t *testing.T) {
	c, mr := testCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Subscribe(ctx, "events")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	mr.Publish("events", "one")
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscribe channel not closed after cancel")
	}
}
