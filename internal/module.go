package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/cache-redis/internal/grpctls"
	"github.com/Muxcore-Media/cache-redis/internal/server"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

type Module struct {
	cache   *cache.Cache
	srv     *server.Server
	grpcSrv *grpc.Server
	lis     net.Listener

	id       string
	cfgMu    sync.RWMutex
	redis    string
	password string
	db       int
	grpcAddr string
}

type Config struct {
	ID       string
	Redis    string
	Password string
	DB       int
	GRPCAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "cache-redis"
	}
	if cfg.Redis == "" {
		cfg.Redis = "localhost:6379"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9600"
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DB = n
		}
	}
	if v := os.Getenv("CACHE_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	cfg.GRPCAddr = resolveGRPCAddr(cfg.GRPCAddr)
	return &Module{
		id:       cfg.ID,
		redis:    cfg.Redis,
		password: cfg.Password,
		db:       cfg.DB,
		grpcAddr: cfg.GRPCAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Cache Redis",
		Version:      "0.1.5",
		Roles:        []string{"infrastructure"},
		Description:  "Redis-backed distributed cache provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityCache, "cache.redis", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	c, err := cache.New(m.redis, m.password, m.db)
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

	slog.Info("cache-redis initialized", "redis", m.redis, "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	var grpcOpts []grpc.ServerOption
	tlsCfg, err := grpctls.ServerConfig()
	if err != nil {
		return fmt.Errorf("gRPC TLS: %w", err)
	}
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("cache-redis gRPC TLS enabled", "addr", m.grpcAddr)
	} else {
		slog.Warn("cache-redis gRPC listening without TLS (dev only)",
			"addr", m.grpcAddr,
			"hint", "unset MUXCORE_INSECURE_DISABLE_TLS for production",
		)
	}
	m.grpcSrv = grpc.NewServer(grpcOpts...)
	m.srv.RegisterWithGRPC(m.grpcSrv)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	go func() {
		slog.Info("cache-redis gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("cache-redis gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.cache != nil {
		_ = m.cache.Close()
	}
	slog.Info("cache-redis stopped")
	return nil
}

// resolveGRPCAddr prefers loopback when plaintext is explicitly enabled and the
// bind address would otherwise listen on all interfaces.
func resolveGRPCAddr(addr string) string {
	if !grpctls.InsecureAllowed() {
		return addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}
	if host == "" || host == "0.0.0.0" {
		return "127.0.0.1:" + port
	}
	return addr
}

func (m *Module) Health(ctx context.Context) error {
	if m.cache == nil {
		return fmt.Errorf("not initialized")
	}
	return m.cache.Health(ctx)
}
