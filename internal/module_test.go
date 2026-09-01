package internal

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/Muxcore-Media/cache-redis/internal/cache"
	cachev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/cache/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func testModule(t *testing.T) (*Module, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	m := NewModule(Config{
		Redis:    cache.Config{Addr: mr.Addr()},
		GRPCAddr: "127.0.0.1:0",
		HTTPAddr: "127.0.0.1:0",
	})
	return m, mr
}

func TestModuleInfo(t *testing.T) {
	Version = "0.1.5"
	m := NewModule(Config{})
	info := m.Info()
	if info.ID != "cache-redis" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Version != "0.1.5" {
		t.Errorf("Version = %q", info.Version)
	}
	if info.HTTPAddr == "" {
		t.Error("HTTPAddr must be set for metrics/health")
	}
}

func TestModuleLifecycle(t *testing.T) {
	m, _ := testModule(t)
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	resp, err := http.Get("http://" + m.httpLis.Addr().String() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestCacheServiceInProcess(t *testing.T) {
	m, mr := testModule(t)
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	conn, err := grpc.NewClient(m.lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := cachev1.NewCacheServiceClient(conn)

	if _, err := client.Set(ctx, &cachev1.SetCacheRequest{
		Key: "integration", Value: []byte("ok"), TtlSeconds: 30,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := client.Get(ctx, &cachev1.GetCacheRequest{Key: "integration"})
	if err != nil || !got.GetFound() || string(got.GetValue()) != "ok" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if mr.TTL("integration") <= 0 {
		t.Fatal("expected TTL")
	}

	metricsResp, err := http.Get("http://" + m.httpLis.Addr().String() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = metricsResp.Body.Close() }()
	body, _ := io.ReadAll(metricsResp.Body)
	if len(body) == 0 {
		t.Fatal("empty /metrics body")
	}
}

func TestSettingsRedisAddrReconnect(t *testing.T) {
	mr1, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr1.Close()
	mr2, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr2.Close()

	m := NewModule(Config{Redis: cache.Config{Addr: mr1.Addr()}, GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(t.Context()) }()

	defs := m.Settings()
	if len(defs) != 5 {
		t.Fatalf("settings=%d", len(defs))
	}
	if err := m.UpdateSetting("redis_addr", mr2.Addr()); err != nil {
		t.Fatal(err)
	}
	if got := m.Settings()[0].Value; got != mr2.Addr() {
		t.Fatalf("addr=%q want %q", got, mr2.Addr())
	}
	if err := m.Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("redis_db", "-1"); err == nil {
		t.Fatal("expected error")
	}
	if err := m.UpdateSetting("cache_key_prefix", "test:"); err != nil {
		t.Fatal(err)
	}
	if got := m.Settings()[4].Value; got != "test:" {
		t.Fatalf("prefix=%q", got)
	}
}
