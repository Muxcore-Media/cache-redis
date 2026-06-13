# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Cache proto definition (`muxcore/cache/v1/cache.proto`) with gRPC service spec
- Generated Go code from cache proto (messages + gRPC stubs)
- Full test suite: 7 cache unit tests (miniredis), 11 server gRPC tests
- Redis client resilience: `MaxRetries=3`, backoff config, `PoolTimeout`
- CompareAndSwap: atomic Lua script (supports nil oldValue = assert-not-exists)
- Distributed locking with Redis SETNX + safe Lua-based unlock
- Pub/sub support via Redis channels (server-streaming gRPC)
- Prometheus-format metrics counters (get/set/delete/incr)
- Production deployment: Dockerfile, docker-compose, systemd unit
- Contract declaration: `CacheProvider` with `MinCoreVersion: 0.4.0`

### Fixed

- CompareAndSwap was a no-op `SetArgs("XX")` — now proper atomic CAS
- Unlock error was silently discarded — now logged to slog
- docker-compose and systemd referenced `your-module` instead of `cache-redis`
- Dockerfile had invalid `--health-check` flag — removed
- Makefile binary name and Docker org were placeholders

### Changed

- Module info now declares `Contracts` and `MinCoreVersion`
- go.mod tidied, miniredis/v2 added for tests
- Redis connection upgraded: retry, backoff, pool timeout

## [0.1.0] - 2026-06-13

### Added

- Initial project scaffold from muxcore-module-starter
