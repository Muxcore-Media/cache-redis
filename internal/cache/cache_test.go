package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setup(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	c, err := New(s.Addr(), "", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, s
}

func TestGetSet(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	data, err := c.Get(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Fatal("expected nil for missing key")
	}

	if err := c.Set(ctx, "foo", []byte("bar"), 0); err != nil {
		t.Fatal(err)
	}
	data, err = c.Get(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "bar" {
		t.Fatalf("expected 'bar', got %q", data)
	}
}

func TestGetSetTTL(t *testing.T) {
	c, s := setup(t)
	ctx := context.Background()

	if err := c.Set(ctx, "exp", []byte("gone"), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	s.FastForward(200 * time.Millisecond)

	data, err := c.Get(ctx, "exp")
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Fatal("expected nil for expired key")
	}
}

func TestDelete(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"), 0)
	c.Set(ctx, "b", []byte("2"), 0)

	if err := c.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	data, _ := c.Get(ctx, "a")
	if data != nil {
		t.Fatal("expected nil after delete")
	}
	data, _ = c.Get(ctx, "b")
	if string(data) != "2" {
		t.Fatalf("expected '2', got %q", data)
	}

	if err := c.Delete(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestExists(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	ok, err := c.Exists(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected false for missing key")
	}

	c.Set(ctx, "present", []byte("x"), 0)
	ok, err = c.Exists(ctx, "present")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true for present key")
	}
}

func TestIncr(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	n, err := c.Incr(ctx, "counter", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	n, err = c.Incr(ctx, "counter", 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6, got %d", n)
	}

	n, err = c.Incr(ctx, "counter", -3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestCompareAndSwap(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	// nil oldValue means key must not exist
	swapped, err := c.CompareAndSwap(ctx, "cas", nil, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if !swapped {
		t.Fatal("expected swap to succeed on new key")
	}

	// same call should fail (key now exists)
	swapped, err = c.CompareAndSwap(ctx, "cas", nil, []byte("again"))
	if err != nil {
		t.Fatal(err)
	}
	if swapped {
		t.Fatal("expected swap to fail when key already exists")
	}

	// wrong old value
	swapped, err = c.CompareAndSwap(ctx, "cas", []byte("wrong"), []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if swapped {
		t.Fatal("expected swap to fail with wrong old value")
	}

	// correct old value
	swapped, err = c.CompareAndSwap(ctx, "cas", []byte("first"), []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if !swapped {
		t.Fatal("expected swap to succeed with correct old value")
	}

	data, _ := c.Get(ctx, "cas")
	if string(data) != "second" {
		t.Fatalf("expected 'second', got %q", data)
	}
}

func TestLockUnlock(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	lk, err := c.Lock(ctx, "mylock", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// second lock should fail
	_, err = c.Lock(ctx, "mylock", 10*time.Second)
	if err == nil {
		t.Fatal("expected error on double lock")
	}

	if err := lk.Unlock(ctx); err != nil {
		t.Fatal(err)
	}

	// now should be able to lock again
	lk2, err := c.Lock(ctx, "mylock", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lk2.Unlock(ctx)

	// unlock without holding should fail
	err = lk.Unlock(ctx)
	if err != ErrLockNotHeld {
		t.Fatalf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestPublishSubscribe(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	ch, err := c.Subscribe(ctx, "events")
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Publish(ctx, "events", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ch:
		if string(msg) != "hello" {
			t.Fatalf("expected 'hello', got %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestHealth(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()

	if err := c.Health(ctx); err != nil {
		t.Fatal(err)
	}

	c.Close()
	if err := c.Health(ctx); err == nil {
		t.Fatal("expected error after close")
	}
}
