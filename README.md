# Cache Redis

[![CI](https://github.com/Muxcore-Media/cache-redis/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/cache-redis/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Redis-backed distributed cache provider.**

A MuxCore sidecar module that implements the `cache` capability over Redis (`Get` / `Set` / `Delete` with optional TTL).

---

## How It Works

```
Module request ──→ cache-redis (gRPC) ──→ Redis
```

Requires a reachable Redis instance. Values are opaque bytes; TTL is honored on `Set`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | `` | Redis password (optional) |
| `REDIS_DB` | `0` | Redis database index |
| `CACHE_GRPC_ADDR` | `:9600` | gRPC listen address |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
export REDIS_ADDR=localhost:6379
./cache-redis --muxcore-mesh-addr localhost:9090
```

---

## Capability

`cache` — Distributed cache (Redis)

## License

GPL-3.0
