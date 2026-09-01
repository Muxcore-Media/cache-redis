package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/cache-redis/internal/server"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

type Module struct {
	cache   *cache.Cache
	srv     *server.Server
	grpcSrv *grpc.Server
	httpSrv *http.Server
	lis     net.Listener
	httpLis net.Listener

	id       string
	cfgMu    sync.RWMutex
	redisCfg cache.Config
	grpcAddr string
	httpAddr string
}

type Config struct {
	ID       string
	Redis    cache.Config
	GRPCAddr string
	HTTPAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "cache-redis"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9600"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:9601"
	}
	if cfg.Redis.Addr == "" {
		if envCfg, err := cache.ConfigFromEnv(); err != nil {
			cfg.Redis.Addr = "localhost:6379"
		} else {
			cfg.Redis = envCfg
		}
	}
	if v := os.Getenv("CACHE_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("CACHE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id:       cfg.ID,
		redisCfg: cfg.Redis,
		grpcAddr: cfg.GRPCAddr,
		httpAddr: cfg.HTTPAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Cache Redis",
		Version:      Version,
		Roles:        []string{"infrastructure"},
		Description:  "Redis-backed distributed cache provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityCache, "cache.redis", "settings"},
		HTTPAddr:     m.httpAddr,
		MinCoreVersion: MinCoreVersion,
	}
}

func (m *Module) Init(ctx context.Context) error {
	m.cfgMu.RLock()
	redisCfg := m.redisCfg
	m.cfgMu.RUnlock()

	c, err := cache.New(redisCfg)
	if err != nil {
		return fmt.Errorf("connect to Redis: %w", err)
	}
	m.cache = c
	m.srv = server.New(c)

	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis

	httpLis, err := net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen http %s: %w", m.httpAddr, err)
	}
	m.httpLis = httpLis

	slog.Info("cache-redis initialized",
		"redis", redisCfg.Addr,
		"grpc", m.grpcAddr,
		"http", m.httpAddr,
		"prefix", redisCfg.KeyPrefix,
	)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	m.srv.RegisterWithGRPC(m.grpcSrv)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	go func() {
		slog.Info("cache-redis gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("cache-redis gRPC serve error", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(m.srv.Metrics()))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := m.Health(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.httpSrv = &http.Server{Handler: mux}
	go func() {
		slog.Info("cache-redis HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("cache-redis HTTP serve error", "error", err)
		}
	}()

	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.cache != nil {
		_ = m.cache.Close()
	}
	slog.Info("cache-redis stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.cache == nil {
		return fmt.Errorf("not initialized")
	}
	return m.cache.Health(ctx)
}

func (m *Module) reconnect(newCfg cache.Config) error {
	c, err := cache.New(newCfg)
	if err != nil {
		return fmt.Errorf("reconnect Redis: %w", err)
	}
	old := m.srv.ReplaceCache(c)
	m.cache = c
	m.cfgMu.Lock()
	m.redisCfg = newCfg
	m.cfgMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func mergeRedisConfig(cur cache.Config, addr string, password *string, username *string, db *int, prefix *string) cache.Config {
	out := cur
	if addr != "" {
		out.Addr = addr
	}
	if password != nil {
		out.Password = *password
	}
	if username != nil {
		out.Username = *username
	}
	if db != nil {
		out.DB = *db
	}
	if prefix != nil {
		out.KeyPrefix = *prefix
	}
	return out
}

func redisConfigEqual(a, b cache.Config) bool {
	return a.Addr == b.Addr &&
		a.Username == b.Username &&
		a.Password == b.Password &&
		a.DB == b.DB &&
		a.KeyPrefix == b.KeyPrefix
}

func parseDB(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid redis_db %q (integer >= 0)", value)
	}
	return n, nil
}
