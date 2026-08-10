package internal

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
)

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

	m := NewModule(Config{Redis: mr1.Addr(), GRPCAddr: "127.0.0.1:0"})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(t.Context())

	defs := m.Settings()
	if len(defs) != 3 {
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
}
