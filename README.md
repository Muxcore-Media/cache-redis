# Cache Redis

[![CI](https://github.com/Muxcore-Media/cache-redis/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/cache-redis/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Redis-backed distributed cache provider for MuxCore.**

A MuxCore sidecar module that provides ephemeral state storage, distributed locking, and pub/sub messaging via Redis. Without this module, core has no distributed cache — modules that depend on `CacheProvider` will fail at discovery.

---

## How It Works

```
Module request ──→ cache-redis (gRPC) ──→ Redis
                     │
                     ▼
              Get/Set/Delete/Incr/CAS/
              Lock/Unlock/Publish/Subscribe
```

### Key operations

- **Get/Set/Delete/Exists** — Standard key-value operations with optional TTL on Set.
- **Incr** — Atomic integer increment/decrement.
- **CompareAndSwap** — Atomic compare-and-swap via Lua script. A nil oldValue asserts the key does not exist.
- **Lock/Unlock** — Distributed locks with Redis SETNX and Lua-based safe release.
- **Publish/Subscribe** — Redis pub/sub channels for real-time messaging.
- **Metrics** — Prometheus-format counters for get, set, delete, and incr operations, exposed via gRPC.

---

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--redis-addr` | `localhost:6379` | Redis server address |
| `--grpc-addr` | `:9600` | gRPC listen address |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | `` | Redis AUTH password |
| `REDIS_DB` | `0` | Redis database number |
| `CACHE_GRPC_ADDR` | `:9600` | gRPC listen address |

---

## Quick Start

```bash
# Build
make build

# Run against local core (dev mode)
export MUXCORE_INSECURE_DISABLE_TLS=true
./cache-redis --muxcore-mesh-addr localhost:9090
```

---

## Deployment

### Docker

```bash
make docker
docker run -d --restart=unless-stopped \
  -e REDIS_ADDR=redis:6379 \
  -e MUXCORE_GRPC_ADDR=core:9090 \
  ghcr.io/muxcore-media/cache-redis:latest
```

### docker-compose

```bash
docker compose -f deploy/docker-compose.yml up
```

---

## Development

```bash
make dev      # run in dev mode
make test     # run tests
make lint     # golangci-lint
make fmt      # format code
```

### Integration Tests

```bash
# Start core and Redis in dev mode, then:
REDIS_ADDR=localhost:6379 MUXCORE_GRPC_ADDR=localhost:9090 go test -tags=integration -race -count=1 ./test/
```

---

## Implementation

- Registers with capability: `"cache"`
- Implements `contracts.CacheProvider`
- Uses `go-redis/v9` client with connection pooling
- Exposes gRPC `CacheService` for inter-module access

---

## License

GPL-3.0
