package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"

	"github.com/Muxcore-Media/cache-redis/internal"
	"github.com/Muxcore-Media/cache-redis/internal/cache"
)

var version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		os.Exit(runHealthCheck())
	}
	internal.Version = version
	mod := internal.NewModule(internal.Config{})
	insecure := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	if err := modulesdk.Run(modulesdk.Config{
		Module:   mod,
		Insecure: insecure,
	}); err != nil {
		slog.Error("module exited", "error", err)
		os.Exit(1)
	}
}

func runHealthCheck() int {
	if addr := strings.TrimSpace(os.Getenv("CACHE_HTTP_ADDR")); addr != "" {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			if strings.HasPrefix(addr, ":") {
				host, port = "127.0.0.1", strings.TrimPrefix(addr, ":")
			} else {
				fmt.Fprintf(os.Stderr, "health-check: bad CACHE_HTTP_ADDR: %v\n", err)
				return 1
			}
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		url := fmt.Sprintf("http://%s/health", net.JoinHostPort(host, port))
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "health-check: %v\n", err)
			return 1
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "health-check: status %d\n", resp.StatusCode)
			return 1
		}
		return 0
	}

	cfg, err := cache.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "health-check: %v\n", err)
		return 1
	}
	c, err := cache.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health-check: redis: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "health-check: %v\n", err)
		return 1
	}
	return 0
}
