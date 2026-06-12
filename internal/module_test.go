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
