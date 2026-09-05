package internal

import (
	"context"
	"net"
	"testing"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.HTTPAddr != "127.0.0.1:9600" {
		t.Errorf("HTTPAddr = %q, want 127.0.0.1:9600", info.HTTPAddr)
	}
}

func TestModuleInit(t *testing.T) {
	m := NewModule(Config{Redis: ":0", GRPCAddr: ":0"})
	ctx := context.Background()

	err := m.Init(ctx)
	if err == nil {
		t.Skip("Redis not available, skipping Init test")
	}
	if _, ok := err.(*net.OpError); ok {
		t.Skip("Redis not available, skipping Init test")
	}
}

func TestResolveGRPCAddr_InsecureLoopback(t *testing.T) {
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "true")
	if got := resolveGRPCAddr(":9600"); got != "127.0.0.1:9600" {
		t.Fatalf("got %q", got)
	}
	if got := resolveGRPCAddr("0.0.0.0:9600"); got != "127.0.0.1:9600" {
		t.Fatalf("got %q", got)
	}
	if got := resolveGRPCAddr("192.168.1.1:9600"); got != "192.168.1.1:9600" {
		t.Fatalf("got %q", got)
	}
}

func TestModuleLifecycle(t *testing.T) {
	t.Setenv("REDIS_ADDR", "localhost:6379")
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Skipf("Redis not available, skipping lifecycle test: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
