# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [Unreleased]

## [0.1.0] — 2026-08-09

### Added

- Redis-backed cache sidecar (`cache` / `cache.redis`): Get, Set, Delete, Exists, Incr, CompareAndSwap, Lock/Unlock, Publish/Subscribe.
- CI Redis service container + `TestLiveRedisRoundTrip` (skips when `REDIS_ADDR` unset).
- COMPATIBILITY for Redis 6+ and core ≥ 0.5.0.
