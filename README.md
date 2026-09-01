# Cache Redis

[![CI](https://git.zem.systems/muxcore/cache-redis/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/cache-redis/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Redis-backed distributed cache provider.**

A MuxCore sidecar module that implements the `cache` / `cache.redis` capabilities over Redis.

gRPC surface: `Get`, `Set` (optional TTL), `Delete`, `Exists`, `Incr`, `CompareAndSwap`, `Lock` / `Unlock`, `Publish` / `Subscribe`.

Lock tokens are opaque and self-contained — any cache-redis replica can `Unlock` without process-local state. Default lock TTL is **30 seconds** when `ttl_seconds` is omitted or zero.

---

## How It Works

```
Module request ──→ cache-redis (gRPC :9600) ──→ Redis
                      │
                      └── HTTP :9601 (/metrics, /health)
```

Requires a reachable Redis instance. Values are opaque bytes; TTL is honored on `Set`, preserved on `CompareAndSwap`, and applied on `Lock`.

**Tests:** unit tests use [miniredis](https://github.com/alicebob/miniredis) (no Docker). CI starts `redis:7-alpine` and runs `TestLiveRedisRoundTrip` against `REDIS_ADDR=localhost:6379`.

Laptop demos that do not need a shared Redis should prefer [`cache-local`](https://github.com/Muxcore-Media/cache-local) (spool `default`). Opt into this module with spool tag `cache-redis`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis host:port |
| `REDIS_URL` | — | `redis://` or `rediss://` URL (overrides addr/password/db/TLS) |
| `REDIS_USERNAME` | `` | Redis ACL username |
| `REDIS_PASSWORD` | `` | Redis password (optional) |
| `REDIS_DB` | `0` | Redis database index |
| `REDIS_TLS` | `false` | Enable TLS (`rediss://` in `REDIS_URL` also enables TLS) |
| `REDIS_TLS_CA_FILE` | — | CA bundle for server verification |
| `REDIS_TLS_CERT_FILE` / `REDIS_TLS_KEY_FILE` | — | Mutual TLS client cert |
| `REDIS_TLS_SERVER_NAME` | — | TLS SNI / hostname verify override |
| `REDIS_TLS_INSECURE` | `false` | Skip TLS verify (dev only) |
| `CACHE_KEY_PREFIX` | `` | Prefix all keys/channels (use on shared Redis) |
| `CACHE_GRPC_ADDR` | `:9600` | gRPC listen address |
| `CACHE_HTTP_ADDR` | `127.0.0.1:9601` | HTTP listen for `/metrics` and `/health` |
| `MUXCORE_GRPC_ADDR` | — | Core mesh address (or `--muxcore-mesh-addr`) |
| `MUXCORE_MODULE_ID` | `cache-redis` | Module ID override (or `--muxcore-module-id`) |

`Info().HTTPAddr` reports the HTTP metrics/health listener (`CACHE_HTTP_ADDR`), not the gRPC port.

---

## Quick Start

```bash
make build

export REDIS_ADDR=localhost:6379
export CACHE_KEY_PREFIX=muxcore:
./cache-redis --muxcore-mesh-addr localhost:9090
# metrics: curl http://127.0.0.1:9601/metrics
# health:  curl http://127.0.0.1:9601/health
```

Core must be reachable (dev: `MUXCORE_INSECURE_DISABLE_TLS=true ./muxcored` in `../core`).

Docker Compose (`deploy/docker-compose.yml`) includes Redis and sets `REDIS_ADDR=redis:6379`.

---

## Capability

`cache` — Distributed cache (Redis)

## License

GPL-3.0
