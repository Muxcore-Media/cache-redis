package server_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/cache-redis/internal/server"
	cachev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/cache/v1"
)

func startServer(t *testing.T, c *cache.Cache) (cachev1.CacheServiceClient, func()) {
	t.Helper()
	srv := server.New(c)
	grpcSrv := grpc.NewServer()
	srv.RegisterWithGRPC(grpcSrv)

	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = grpcSrv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return cachev1.NewCacheServiceClient(conn), cleanup
}

func testRedis(t *testing.T) (*cache.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := cache.New(cache.Config{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, mr
}

func TestCacheService_GetSetTTL(t *testing.T) {
	c, mr := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	miss, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: "k"})
	if err != nil || miss.GetFound() {
		t.Fatalf("miss: found=%v err=%v", miss.GetFound(), err)
	}

	if _, err := client.Set(ctx, &cachev1.SetCacheRequest{
		Key: "k", Value: []byte("v"), TtlSeconds: 60,
	}); err != nil {
		t.Fatal(err)
	}
	hit, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: "k"})
	if err != nil || !hit.GetFound() || string(hit.GetValue()) != "v" {
		t.Fatalf("hit: %+v err=%v", hit, err)
	}
	if mr.TTL("k") <= 0 {
		t.Fatal("expected TTL on key")
	}
}

func TestCacheService_ExistsIncr(t *testing.T) {
	c, _ := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ex, err := client.Exists(ctx, &cachev1.ExistsCacheRequest{Key: "n"})
	if err != nil || ex.GetExists() {
		t.Fatalf("exists: %+v err=%v", ex, err)
	}
	inc, err := client.Incr(ctx, &cachev1.IncrCacheRequest{Key: "n", Delta: 3})
	if err != nil || inc.GetValue() != 3 {
		t.Fatalf("incr: %+v err=%v", inc, err)
	}
	ex, err = client.Exists(ctx, &cachev1.ExistsCacheRequest{Key: "n"})
	if err != nil || !ex.GetExists() {
		t.Fatalf("exists after incr: %+v err=%v", ex, err)
	}
}

func TestCacheService_CompareAndSwap(t *testing.T) {
	c, _ := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	if _, err := client.Set(ctx, &cachev1.SetCacheRequest{Key: "cas", Value: []byte("old")}); err != nil {
		t.Fatal(err)
	}
	ok, err := client.CompareAndSwap(ctx, &cachev1.CompareAndSwapCacheRequest{
		Key: "cas", OldValue: []byte("old"), NewValue: []byte("new"),
	})
	if err != nil || !ok.GetSwapped() {
		t.Fatalf("cas: %+v err=%v", ok, err)
	}
	got, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: "cas"})
	if err != nil || string(got.GetValue()) != "new" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

func TestCacheService_LockUnlock(t *testing.T) {
	c, _ := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	lk, err := client.Lock(ctx, &cachev1.LockCacheRequest{Key: "lock-me"})
	if err != nil || !lk.GetAcquired() || lk.GetToken() == "" {
		t.Fatalf("lock: %+v err=%v", lk, err)
	}
	un, err := client.Unlock(ctx, &cachev1.UnlockCacheRequest{Token: lk.GetToken()})
	if err != nil || un.GetStatus() != "ok" {
		t.Fatalf("unlock: %+v err=%v", un, err)
	}
}

func TestCacheService_LockUnlockSecondReplica(t *testing.T) {
	c, _ := testRedis(t)
	srvA := server.New(c)
	srvB := server.New(c)
	ctx := context.Background()

	lk, err := srvA.Lock(ctx, &cachev1.LockCacheRequest{Key: "shared"})
	if err != nil || !lk.GetAcquired() {
		t.Fatalf("lock A: %+v err=%v", lk, err)
	}
	un, err := srvB.Unlock(ctx, &cachev1.UnlockCacheRequest{Token: lk.GetToken()})
	if err != nil || un.GetStatus() != "ok" {
		t.Fatalf("unlock B: %+v err=%v", un, err)
	}
}

func TestCacheService_PublishSubscribe(t *testing.T) {
	c, mr := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	stream, err := client.Subscribe(ctx, &cachev1.SubscribeCacheRequest{Channel: "ch"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := client.Publish(ctx, &cachev1.PublishCacheRequest{
		Channel: "ch", Message: []byte("hello"),
	}); err != nil {
		t.Fatal(err)
	}
	msg, err := stream.Recv()
	if err != nil || string(msg.GetMessage()) != "hello" {
		t.Fatalf("recv: %+v err=%v publish=%v", msg, err, mr)
	}
}

func TestCacheService_DeleteCount(t *testing.T) {
	c, _ := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	if _, err := client.Set(ctx, &cachev1.SetCacheRequest{Key: "del1", Value: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Delete(ctx, &cachev1.DeleteCacheRequest{Keys: []string{"del1", "missing"}})
	if err != nil || resp.GetDeleted() != 1 {
		t.Fatalf("delete: deleted=%d err=%v", resp.GetDeleted(), err)
	}
}

func TestCacheService_EmptyKeyInvalidArgument(t *testing.T) {
	c, _ := testRedis(t)
	client, cleanup := startServer(t, c)
	t.Cleanup(cleanup)
	ctx := context.Background()

	cases := []struct {
		name string
		run  func() error
	}{
		{"Get", func() error {
			_, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: ""})
			return err
		}},
		{"Set", func() error {
			_, err := client.Set(ctx, &cachev1.SetCacheRequest{Key: "", Value: []byte("x")})
			return err
		}},
		{"Exists", func() error {
			_, err := client.Exists(ctx, &cachev1.ExistsCacheRequest{Key: ""})
			return err
		}},
		{"Incr", func() error {
			_, err := client.Incr(ctx, &cachev1.IncrCacheRequest{Key: ""})
			return err
		}},
		{"CompareAndSwap", func() error {
			_, err := client.CompareAndSwap(ctx, &cachev1.CompareAndSwapCacheRequest{Key: ""})
			return err
		}},
		{"Lock", func() error {
			_, err := client.Lock(ctx, &cachev1.LockCacheRequest{Key: ""})
			return err
		}},
		{"Publish", func() error {
			_, err := client.Publish(ctx, &cachev1.PublishCacheRequest{Channel: ""})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status.Code(tc.run()) != codes.InvalidArgument {
				t.Fatalf("%s: expected InvalidArgument", tc.name)
			}
		})
	}
}

func TestCacheService_EmptyChannelSubscribe(t *testing.T) {
	c, _ := testRedis(t)
	srv := server.New(c)
	err := srv.Subscribe(&cachev1.SubscribeCacheRequest{Channel: ""}, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Subscribe empty channel: %v", err)
	}
}

func TestServer_Metrics(t *testing.T) {
	c, _ := testRedis(t)
	srv := server.New(c)
	ctx := context.Background()
	if _, err := srv.Get(ctx, &cachev1.GetCacheRequest{Key: "m"}); err == nil {
		// miss is ok
	}
	body := srv.Metrics()
	if body == "" || !strings.Contains(body, "cache_get_total") {
		t.Fatalf("metrics=%q", body)
	}
}
