package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/cache-redis/internal/server"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/module/v1"
)

func main() {
	meshAddr := flag.String("muxcore-mesh-addr", "localhost:9090", "gRPC address of the MuxCore mesh")
	moduleID := flag.String("muxcore-module-id", "cache-redis", "Module identifier")
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	httpAddr := flag.String("http-addr", ":9600", "Address for HTTP cache API")
	healthAddr := flag.String("health-addr", ":9601", "Address for HTTP health and metrics")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting cache-redis", "version", "0.1.0")

	addr := *redisAddr
	if env := os.Getenv("REDIS_ADDR"); env != "" {
		addr = env
	}
	password := *redisPassword
	if env := os.Getenv("REDIS_PASSWORD"); env != "" {
		password = env
	}
	db := *redisDB
	if env := os.Getenv("REDIS_DB"); env != "" {
		if n, err := strconv.Atoi(env); err == nil {
			db = n
		}
	}

	c, err := cache.New(addr, password, db)
	if err != nil {
		slog.Error("failed to connect to Redis", "addr", addr, "error", err)
		os.Exit(1)
	}
	defer c.Close()

	srv := server.New(c)

	healthLis, err := net.Listen("tcp", *healthAddr)
	if err != nil {
		slog.Warn("health listen failed", "addr", *healthAddr, "error", err)
	} else {
		go func() {
			slog.Info("HTTP health and metrics listening", "addr", *healthAddr)
			if err := http.Serve(healthLis, srv.Handler()); err != nil {
				slog.Error("health server error", "error", err)
			}
		}()
	}

	apiLis, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		slog.Error("failed to listen", "addr", *httpAddr, "error", err)
		os.Exit(1)
	}
	go func() {
		slog.Info("HTTP cache API listening", "addr", *httpAddr)
		if err := http.Serve(apiLis, srv.Handler()); err != nil {
			slog.Error("API server error", "error", err)
		}
	}()

	conn, err := grpc.NewClient(*meshAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Warn("core not reachable, running standalone", "addr", *meshAddr, "error", err)
	} else {
		defer conn.Close()
		slog.Info("connected to core mesh", "addr", *meshAddr)

		regClient := modulev1.NewModuleRegistrationClient(conn)
		resp, err := regClient.Register(context.Background(), &modulev1.RegisterRequest{
			ModuleId: *moduleID,
			ModuleInfo: &modulev1.ModuleInfo{
				Id:           *moduleID,
				Name:         "Cache Redis",
				Version:      "0.1.0",
				Description:  "Redis-backed distributed cache provider",
				Author:       "MuxCore",
				Roles:        []string{"infrastructure"},
				Capabilities: []string{contracts.CapabilityCache},
				HttpAddr:     *httpAddr,
			},
		})
		if err != nil {
			slog.Warn("registration failed, running standalone", "error", err)
		} else if !resp.Accepted {
			slog.Warn("registration rejected, running standalone", "reason", resp.Error)
		} else {
			slog.Info("module registered with core",
				"id", *moduleID,
				"mesh_addr", resp.MeshAddr,
				"node_id", resp.NodeId,
			)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	slog.Info("shutting down...")

	if conn != nil {
		regClient := modulev1.NewModuleRegistrationClient(conn)
		regClient.Unregister(context.Background(), &modulev1.UnregisterRequest{ModuleId: *moduleID})
	}

	slog.Info("shutdown complete")
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: cache-redis [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
}
