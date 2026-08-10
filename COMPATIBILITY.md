# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.5         | v0.5.0+     | Current |
| v0.1.1         | v0.5.0+     | Compatible |
| v0.1.0         | v0.4.0+     | Superseded |

## Contracts

| Contract | Capability | Status |
|----------|-----------|--------|
| CacheService (core proto) | `cache` | Current |
| — | `cache.redis` | Current (provider hint) |

Requires a reachable Redis 6+ instance (`REDIS_ADDR`). Unit tests use **miniredis** (in-process local Redis). Self-hosted CI starts `redis:7-alpine` as a service container when Docker is available and runs `TestLiveRedisRoundTrip`; if Redis is unreachable the live test **skips** (miniredis still gates CI).

Prefer this module over `cache-local` when multiple hosts or process restarts must share cache state.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
