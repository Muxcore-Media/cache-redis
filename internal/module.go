package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/cache-redis/internal/server"
	"github.com/Muxcore-Media/core/pkg/contracts"
)

type Module struct {
	cache   *cache.Cache
	srv     *server.Server
	grpcSrv *grpc.Server
	lis     net.Listener

	id       string
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
		cfg.GRPCAddr = ":9600"
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
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "Redis-backed distributed cache provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityCache},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "CacheProvider",
				Version:   "v0.4.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
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
	m.grpcSrv = grpc.NewServer()
	m.srv.RegisterWithGRPC(m.grpcSrv)

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
		m.cache.Close()
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
