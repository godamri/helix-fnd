Helix Foundation (FND)
======================

> **Mission-Critical Go Microservices Framework** *High-Reliability Architecture. Semantic-Audit Ready. Zero-Compromise Consistency.*

Helix FND is a **robust defensive framework** engineered to build microservices that prioritize stability, data integrity, and error prevention above all else.

Built upon the principles of **Observability-First** and **Defense-in-Depth**, this framework provides the necessary guardrails to ensure operations are accurately accounted for, errors are transparently reported, and state mutations are fully traceable in complex distributed systems.

Core Capabilities
-----------------

### 1\. Transport & Resilience Layer

The first line of defense for ingress traffic, designed to withstand chaotic environments and traffic spikes.

-   **Hybrid Server Lifecycle:** Seamlessly orchestrates HTTP (Chi) and gRPC (Google) servers within a unified, signal-aware `Runner`.

-   **Zero-Trust mTLS:** Native support for Mutual TLS (Client Certificate Auth) to enforce strict service-to-service identity verification.

-   **GCRA Rate Limiter:** High-precision implementation of the *Generic Cell Rate Algorithm* using Lua scripts in Redis.

    -   *Circuit Breaker Fallback:* In the event of a Redis outage, the limiter automatically degrades to an *in-memory token bucket* strategy (Fail-Safe) to prevent system lockout while maintaining protection.

-   **Idempotency Gate:** A dedicated middleware ensuring strict exactly-once processing for critical operations using atomic Redis locks (`SetNX`), preventing duplicate execution and race conditions in distributed environments.

### 2\. Deep Observability

Comprehensive telemetry integration for complete system visibility, from the edge to the database.

-   **Integrated Telemetry:** Custom `slog` handler wrapped with `OTelHandler`.

    -   **Context injection:** Automatically injects `trace_id` and `span_id` into every log entry.

    -   **Auto-Error Recording:** Automatically marks the active OpenTelemetry Span as `Error` and records the exception stack trace whenever a log level of `ERROR` or higher is detected.

-   **RED Metrics:** Middleware that automatically captures Request Rate, Error Rate, and Duration via Prometheus, implemented with cardinality protection to prevent metric explosion.

-   **Distributed Tracing:** Full propagation of Trace Context across HTTP, gRPC, and Kafka headers, ensuring no request is lost in the void.

### 3\. High-Integrity Audit

An immutable audit trail designed for strict compliance and system accountability.

-   **Configurable Buffer Strategies:**

    -   **High Availability:** Drops log events if the buffer is full (Non-blocking / Best-effort).

    -   **High Integrity:** Blocks the main thread if the buffer is full (Guarantees *no-audit-loss* at the cost of latency).

-   **Multi-Output Support:** Native integration with Kafka (via `franz-go`) using Snappy batch compression for high throughput, with automatic fallback to `io.Writer` (stdout/file).

### 4\. Event Driven Architecture

Engineered for strict ordering, high throughput, and eventual consistency.

-   **Synchronous Kafka Producer:** Utilizes `ProduceSync` to guarantee message persistence, making it ideal for the **Transactional Outbox** pattern where event loss is unacceptable.

-   **Resilient Consumer:**

    -   **Strict Ordering:** Ensures messages are processed in the exact order they were received.

    -   **Dead Letter Queue (DLQ):** Automatically quarantines "poison pill" messages after `MaxRetries` is exceeded, preventing consumer lag accumulation.

    -   **Exponential Backoff:** Intelligent retry mechanism for transient failures.

-   **Trace Propagation:** Context tracing is automatically injected into and extracted from Kafka Record Headers.

### 5\. Security & Identity

-   **JWKS Caching Client:** High-performance JWT validation featuring background key refresh, stale-cache tolerance, and `singleflight` protection to prevent "thundering herd" attacks on identity providers.

-   **Bcrypt Wrapper:** Standardized password hashing with enforceable cost parameters.

### 6\. Data Persistence

-   **PostgreSQL (pgxpool):** Production-ready connection pool tuning with native OpenTelemetry instrumentation at the driver level.

-   **Redis (go-redis):** Automatic tracing hooks for every command and pipeline execution.

Quick Start
-----------

### Installation

```
go get github.com/godamri/helix-fnd

```

### Bootstrapping a Service

```
package main

import (
	"context"
	"log/slog"
	
	"github.com/godamri/helix-fnd/app"
	"github.com/godamri/helix-fnd/server"
	"github.com/godamri/helix-fnd/log"
)

type Config struct {
    Server server.Config
    Log    log.Config
}

func main() {
    // 1. Initialize Logger with OTel support
    logger := log.New(log.Config{Level: "info", Format: "json"})

    // 2. Lifecycle Runner
    runner := app.NewRunner(logger)

    runner.Run(func(ctx context.Context) error {
        // Load Config with strict validation
        var cfg Config
        loader := app.NewConfigLoader()
        if err := loader.Load(ctx, &cfg, "MYAPP"); err != nil {
            return err
        }

        // Start Server (HTTP + gRPC)
        // Dependencies are injected here
        srv := server.New(cfg.Server, logger, myRouter, myGrpcServer)
        return srv.Start(ctx)
    })
}

```

Configuration Standards
-----------------------

All configurations are loaded via `envconfig` with strict type enforcement and validation.

| Category | Env Variable | Default | Description |
| --- |  --- |  --- |  --- |
| **DB** | `DB_DSN` | \- | PostgreSQL Connection String (DSN) |
| **DB** | `DB_MAX_OPEN_CONNS` | `50` | Database connection pool size |
| **Redis** | `REDIS_ADDR` | \- | Redis Host:Port |
| **App** | `LOG_LEVEL` | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| **Audit** | `AUDIT_BLOCK_ON_FULL` | `false` | Set to `true` for critical paths where audit loss is unacceptable |

Architecture Decisions
----------------------

1.  **Fail-Fast Philosophy:** All critical dependencies (Redis, Postgres, Kafka) are pinged immediately upon startup. If any service is unreachable, the application panics instantly. We do not allow "zombie states" where the app is running but non-functional.

2.  **Context is King:** Every critical function (Logging, Database, HTTP Requests) requires `context.Context` to ensure proper tracing propagation and cancellation signals.

3.  **No Magic:** There is no hidden global state. All dependencies must be explicitly injected via constructors.

> *"No incomplete logic survives."*