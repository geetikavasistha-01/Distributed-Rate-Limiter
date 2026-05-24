# Distributed Rate Limiter

A high-performance, horizontally scalable API traffic control system written in Go.

## The Challenge of Traffic in Distributed Systems

In modern microservice architectures, protecting upstream services from cascading failures and abusive traffic patterns is a fundamental requirement. A single misbehaving client, a sudden traffic spike, or a malicious actor can exhaust compute resources and bring down entire clusters. 

The engineering challenge is not merely to limit requests, but to do so consistently across a fleet of stateless API servers. When multiple instances of an application receive concurrent requests from the same user, local memory is insufficient to enforce global limits. The system must coordinate state over a network, and it must do so in milliseconds to avoid introducing unacceptable latency into the critical path.

This Distributed Rate Limiter was engineered to solve that exact problem. By externalizing the state to Redis and utilizing atomic Lua scripts, it provides a centralized, race-condition-free source of truth for traffic quotas, allowing the API layer to scale infinitely without compromising limit enforcement.

## Architecture and Design

The system is designed with a strong emphasis on pluggability, atomicity, and observability.

### 1. Atomic Evaluation
Network round-trips are expensive. Traditional read-modify-write cycles over a network are prone to race conditions under heavy concurrent load. To mitigate this, the core algorithms are implemented as Lua scripts executed directly within the Redis engine. This guarantees atomicity for every request check, reducing the operation to a single, highly optimized network call.

### 2. Pluggable Algorithmic Engines
Traffic shaping requirements vary by use case. The system implements four distinct rate-limiting strategies behind a unified interface:

- Fixed Window: A lightweight counter resetting at fixed time boundaries. Ideal for simple quota enforcement.
- Sliding Window Log: Maintains a precise log of request timestamps. Provides exact limits without the boundary-burst vulnerabilities of fixed windows.
- Token Bucket: Allows for controlled bursts of traffic while enforcing a steady long-term rate.
- Leaky Bucket: Enforces a strict, smoothed output rate regardless of incoming burstiness, functioning as a traffic shaper.

### 3. In-Memory Hot-Key Tracking
To provide immediate visibility into potential abuse, the system features a concurrent, in-memory tracking mechanism. As traffic flows through the middleware, an asynchronous pipeline identifies and records the most frequently rate-limited keys, providing operations teams with real-time insight into malicious or runaway clients.

### 4. Deep Observability
Operating a distributed system blind is a recipe for disaster. The rate limiter exposes native Prometheus metrics, tracking total requests, limit rejections, and Redis connection errors. This telemetry enables robust alerting rules and comprehensive Grafana dashboards.

## Getting Started

### Prerequisites
- Go 1.21 or higher
- Docker and Docker Compose
- Make

### Local Development

1. Run the test suite (includes race detection and coverage):
   ```bash
   make test
   ```

2. Build the binary locally:
   ```bash
   make build
   ```

### Deployment

The system is fully containerized and includes a complete observability stack out of the box.

Bring up the entire stack (Redis, Nginx Load Balancer, 3 API Replicas, and Prometheus) using Docker Compose:

```bash
make docker-up
```

To tear down the environment:

```bash
make docker-down
```

## API Usage

The system exposes REST endpoints for traffic simulation, consumption, and configuration management.

Check a limit without consuming a token (Dry Run):
```bash
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"key": "user-123"}'
```

Consume a token:
```bash
curl -X POST http://localhost:8080/consume \
  -H "Content-Type: application/json" \
  -d '{"key": "user-123"}'
```

Check system health:
```bash
curl http://localhost:8080/health
```

## Configuration

The system is configured entirely via environment variables, adhering to twelve-factor app principles:

- SERVER_PORT: Port the HTTP server binds to (default: 8080)
- REDIS_URL: Redis instance address (default: localhost:6379)
- RATE_LIMIT_ALGORITHM: The algorithm engine to load (default: fixed_window)
- RATE_LIMIT_REQUESTS: Number of allowed requests per window (default: 10)
- RATE_LIMIT_WINDOW: The time window for the limit (default: 1m)
- LOG_LEVEL: The structured logging level (default: info)

## License

This project is licensed under the MIT License.
