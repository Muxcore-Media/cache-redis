package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func testCache(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := New(mr.Addr(), "", 0)
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
