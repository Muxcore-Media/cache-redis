package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	cachev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/cache/v1"
)

func setup(t *testing.T) (cachev1.CacheServiceClient, func()) {
	t.Helper()
	s := miniredis.RunT(t)
	c, err := cache.New(s.Addr(), "", 0)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}

	svr := New(c)
	grpcSrv := grpc.NewServer()
	svr.RegisterWithGRPC(grpcSrv)

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go grpcSrv.Serve(lis)

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("tcp", addr)
	}
	conn, err := grpc.Dial(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := cachev1.NewCacheServiceClient(conn)
	cleanup := func() {
		conn.Close()
		grpcSrv.Stop()
		c.Close()
	}
	return client, cleanup
}

func TestServerGetSet(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Found {
		t.Fatal("expected not found")
	}

	_, err = client.Set(ctx, &cachev1.SetCacheRequest{
		Key:   "foo",
		Value: []byte("bar"),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err = client.Get(ctx, &cachev1.GetCacheRequest{Key: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Found {
		t.Fatal("expected found")
	}
	if string(resp.Value) != "bar" {
		t.Fatalf("expected 'bar', got %q", resp.Value)
	}
}

func TestServerDelete(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	client.Set(ctx, &cachev1.SetCacheRequest{Key: "a", Value: []byte("1")})
	client.Set(ctx, &cachev1.SetCacheRequest{Key: "b", Value: []byte("2")})

	del, err := client.Delete(ctx, &cachev1.DeleteCacheRequest{Keys: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if del.Deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", del.Deleted)
	}
}

func TestServerExists(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.Exists(ctx, &cachev1.ExistsCacheRequest{Key: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Exists {
		t.Fatal("expected false")
	}

	client.Set(ctx, &cachev1.SetCacheRequest{Key: "x", Value: []byte("1")})
	resp, err = client.Exists(ctx, &cachev1.ExistsCacheRequest{Key: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Exists {
		t.Fatal("expected true")
	}
}

func TestServerIncr(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.Incr(ctx, &cachev1.IncrCacheRequest{Key: "c", Delta: 1})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Value != 1 {
		t.Fatalf("expected 1, got %d", resp.Value)
	}

	resp, err = client.Incr(ctx, &cachev1.IncrCacheRequest{Key: "c", Delta: 5})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Value != 6 {
		t.Fatalf("expected 6, got %d", resp.Value)
	}
}

func TestServerCompareAndSwap(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.CompareAndSwap(ctx, &cachev1.CompareAndSwapCacheRequest{
		Key:      "cas",
		NewValue: []byte("first"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Swapped {
		t.Fatal("expected swap to succeed")
	}

	resp, err = client.CompareAndSwap(ctx, &cachev1.CompareAndSwapCacheRequest{
		Key:      "cas",
		OldValue: []byte("wrong"),
		NewValue: []byte("new"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Swapped {
		t.Fatal("expected swap to fail with wrong old value")
	}

	resp, err = client.CompareAndSwap(ctx, &cachev1.CompareAndSwapCacheRequest{
		Key:      "cas",
		OldValue: []byte("first"),
		NewValue: []byte("second"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Swapped {
		t.Fatal("expected swap to succeed")
	}
}

func TestServerLockUnlock(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	lresp, err := client.Lock(ctx, &cachev1.LockCacheRequest{Key: "lock", TtlSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !lresp.Acquired {
		t.Fatal("expected lock acquired")
	}

	lresp2, err := client.Lock(ctx, &cachev1.LockCacheRequest{Key: "lock", TtlSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if lresp2.Acquired {
		t.Fatal("expected second lock to fail")
	}

	uresp, err := client.Unlock(ctx, &cachev1.UnlockCacheRequest{Token: lresp.Token})
	if err != nil {
		t.Fatal(err)
	}
	if uresp.Status != "ok" {
		t.Fatalf("expected ok, got %s", uresp.Status)
	}
}

func TestServerPublishSubscribe(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	stream, err := client.Subscribe(ctx, &cachev1.SubscribeCacheRequest{Channel: "ch"})
	if err != nil {
		t.Fatal(err)
	}

	// Give the subscription time to register with miniredis
	time.Sleep(200 * time.Millisecond)

	_, err = client.Publish(ctx, &cachev1.PublishCacheRequest{Channel: "ch", Message: []byte("hi")})
	if err != nil {
		t.Fatal(err)
	}

	msg, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg.Message) != "hi" {
		t.Fatalf("expected 'hi', got %q", msg.Message)
	}
}

func TestServerGetInvalidArgument(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	_, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestServerSetInvalidArgument(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	_, err := client.Set(ctx, &cachev1.SetCacheRequest{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestServerLockUnlockInvalidToken(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	uresp, err := client.Unlock(ctx, &cachev1.UnlockCacheRequest{Token: "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	if uresp.Status != "lock not found" {
		t.Fatalf("expected 'lock not found', got %s", uresp.Status)
	}
}

func TestServerMetrics(t *testing.T) {
	c := &cache.Cache{}
	s := New(c)
	m := s.Metrics()
	if m == "" {
		t.Fatal("expected non-empty metrics")
	}
}
