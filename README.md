<div align="center">

# Distributed Rate Limiter

**A high-performance, horizontally scalable API traffic control system written in Go**

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Redis](https://img.shields.io/badge/Redis-7.0-DC382D?style=flat-square&logo=redis)](https://redis.io)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker)](https://docker.com)
[![Prometheus](https://img.shields.io/badge/Prometheus-Metrics-E6522C?style=flat-square&logo=prometheus)](https://prometheus.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

[Overview](#overview) · [Architecture](#architecture) · [Algorithms](#algorithms) · [Quick Start](#quick-start) · [API Reference](#api-reference) · [Configuration](#configuration) · [Benchmarks](#benchmarks)

</div>

---

## Overview

In modern microservice architectures, protecting upstream services from cascading failures and abusive traffic patterns is a fundamental requirement. A single misbehaving client, a sudden traffic spike, or a malicious actor can exhaust compute resources and bring down entire clusters.

The engineering challenge is not merely to limit requests, but to do so **consistently across a fleet of stateless API servers**. When multiple instances of an application receive concurrent requests from the same user, local memory is insufficient to enforce global limits. The system must coordinate state over a network, and it must do so in milliseconds to avoid introducing unacceptable latency into the critical path.

This Distributed Rate Limiter solves that exact problem. By externalizing state to Redis and utilizing atomic Lua scripts, it provides a centralized, race-condition-free source of truth for traffic quotas — allowing the API layer to scale horizontally without compromising limit enforcement.

```
Client → Nginx (Round Robin) → [Replica 1 | Replica 2 | Replica 3] → Redis
                                         ↓
                                    Prometheus
```

---

## Architecture and Design

The system is designed with a strong emphasis on pluggability, atomicity, and observability.

### 1. Atomic Evaluation
Network round-trips are expensive. Traditional read-modify-write cycles over a network are prone to race conditions under heavy concurrent load. To mitigate this, the core algorithms are implemented as **Lua scripts executed directly within the Redis engine**. This guarantees atomicity for every request check, reducing the operation to a single, highly optimized network call.

```lua
-- fixed_window.go — INCR and EXPIRE execute as one atomic unit.
-- No other Redis command can interleave between them.
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
end
if count > tonumber(ARGV[1]) then
  return {0, 0, redis.call('TTL', KEYS[1])}  -- denied
end
return {1, tonumber(ARGV[1]) - count, 0}      -- allowed
```

### 2. Pluggable Algorithmic Engines
Traffic shaping requirements vary by use case. The system implements four distinct strategies behind a unified `Limiter` interface — swap algorithms via a single environment variable, zero code changes:

| Algorithm | Redis Structure | Best For | Trade-off |
|---|---|---|---|
| **Fixed Window** | `INCR` + `EXPIRE` | Simple quota enforcement | Boundary burst possible |
| **Sliding Window Log** | Sorted Set (`ZADD/ZRANGE`) | Strict per-user limits | Higher memory per key |
| **Token Bucket** | Hash (`tokens`, `last_refill`) | APIs allowing short bursts | Refill calculation overhead |
| **Leaky Bucket (GCRA)** | Theoretical Arrival Time | Downstream traffic shaping | Zero burst tolerance |

```go
// One interface. Four implementations. Swapped at startup by factory.go.
type Limiter interface {
    Allow(ctx context.Context, key string) (Result, error)
}
```

### 3. In-Memory Hot-Key Tracking
As traffic flows through the middleware, a concurrent in-memory pipeline identifies and records the most frequently rate-limited keys. This gives operations teams real-time visibility into malicious or runaway clients without adding Redis round-trips to the hot path.

### 4. Deep Observability
The rate limiter exposes native Prometheus metrics tracking total requests, limit rejections, and Redis connection errors — scraped independently from all three replicas. This telemetry enables robust alerting rules and comprehensive Grafana dashboards.

```
rate_limiter_requests_total{algorithm="token_bucket", result="allowed"}
rate_limiter_requests_total{algorithm="token_bucket", result="denied"}
rate_limiter_request_duration_seconds{quantile="0.99"}
rate_limiter_redis_errors_total
```

---

## Project Structure

```
├── cmd/api/main.go              # Entry point: wires config → redis → limiter → server
├── deploy/
│   ├── Dockerfile               # Multi-stage build → distroless final image
│   ├── docker-compose.yml       # Redis + 3 replicas + Nginx + Prometheus
│   ├── nginx.conf               # Round-robin upstream, X-Forwarded-For passthrough
│   └── prometheus.yml           # Scrape configs for all 3 replica endpoints
└── internal/
    ├── api/                     # HTTP server, route handlers, integration tests
    ├── config/                  # Env-based config loading, slog initialization
    ├── limiter/                 # Limiter interface, 4 algorithm implementations, factory
    ├── metrics/                 # Prometheus collectors, hot key tracker
    ├── middleware/              # Recovery, RequestID, RateLimit HTTP middlewares
    └── redis/                   # go-redis wrapper with retry + Lua script support
```

---

## Quick Start

### Prerequisites
- Go 1.21 or higher
- Docker and Docker Compose
- Make

### Run with Docker (Recommended)

Bring up the entire stack — Redis, Nginx load balancer, 3 API replicas, and Prometheus — with a single command:

```bash
git clone https://github.com/yourusername/distributed-rate-limiter
cd distributed-rate-limiter

cp .env.example .env
make docker-up
```

To tear down the environment:

```bash
make docker-down
```

### Local Development

Run the full test suite with race detection and coverage:

```bash
make test
```

Build the binary:

```bash
make build
```

---

## API Reference

### `POST /consume`
Check **and consume** one token for the given key. Primary enforcement endpoint.

```bash
curl -X POST http://localhost:8080/consume \
  -H "Content-Type: application/json" \
  -d '{"key": "user-123"}'
```

```json
{ "allowed": true, "remaining": 87, "retry_after": "0s" }
```

On rate limit exceeded — `429 Too Many Requests`:

```json
{ "allowed": false, "remaining": 0, "retry_after": "42s" }
```

### `POST /check`
Dry-run — check without consuming a token. Useful for UI feedback before committing an action.

```bash
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"key": "user-123"}'
```

### `GET /health`
Liveness and Redis connectivity check.

```bash
curl http://localhost:8080/health
```

```json
{ "status": "ok", "redis": "ok" }
```

### `GET /config`
Returns the active rate limiting configuration.

```json
{ "algorithm": "token_bucket", "limit": 100, "window": "1m" }
```

### `GET /metrics`
Prometheus exposition format. Scraped automatically by the bundled Prometheus instance on `:9090`.

---

## Configuration

The system is configured entirely via environment variables, adhering to twelve-factor app principles:

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `8080` | Port the HTTP server binds to |
| `REDIS_URL` | `localhost:6379` | Redis instance address |
| `REDIS_PASSWORD` | `` | Redis auth password |
| `REDIS_DB` | `0` | Redis database index |
| `RATE_LIMIT_ALGORITHM` | `fixed_window` | `fixed_window` \| `sliding_window` \| `token_bucket` \| `leaky_bucket` |
| `RATE_LIMIT_REQUESTS` | `10` | Number of allowed requests per window |
| `RATE_LIMIT_WINDOW` | `1m` | Time window duration (e.g. `30s`, `1m`, `1h`) |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |

---

## Testing

```bash
# Full suite with race detector and coverage
make test

# Specific package
go test ./internal/limiter/... -race -v

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Test coverage includes unit tests for all four algorithm implementations, concurrency tests (50 goroutines asserting exact allow counts), integration tests with real Redis, mock-based handler unit tests, server lifecycle tests, and factory pattern tests.

---

## Benchmarks

Tested on Apple M2 · Redis local · single replica

| Algorithm | Throughput | p99 Latency | Memory/key |
|---|---|---|---|
| Fixed Window | ~42,000 req/s | 0.8ms | ~50 bytes |
| Leaky Bucket | ~40,000 req/s | 0.8ms | ~50 bytes |
| Token Bucket | ~38,000 req/s | 0.9ms | ~80 bytes |
| Sliding Window | ~28,000 req/s | 1.2ms | ~200 bytes |

Sliding Window is slower due to three Redis operations per request (ZADD + ZREMRANGEBYSCORE + ZCARD) versus one or two for the others. The trade-off is perfect accuracy — no window boundary bursts are possible.

---

## Makefile Reference

```bash
make run          # go run cmd/api/main.go
make build        # go build -o bin/api
make test         # go test ./... -race -cover
make docker-up    # docker compose up --build -d
make docker-down  # docker compose down
make lint         # golangci-lint run
```

---

## License

This project is licensed under the [MIT License](LICENSE).
