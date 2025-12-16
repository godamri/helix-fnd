Helix Platform Foundation
==================

**Standard Infrastructure Library for Helix Microservices**

`helix-fnd` is a shared library providing a standardized infrastructure foundation for all microservices within the Helix ecosystem. Its primary goal is to eliminate boilerplate code, enforce engineering standards (logging, tracing, config), and ensure every service is "production-ready" from day one.

📦 Key Features
---------------

-   **Application Bootstrap (`app`)**: Standardized graceful startup/shutdown via `Runner`.

-   **Configuration (`config`)**: Environment variable loading & validation using `envconfig` + `validator`.

-   **Database (`database`)**: `sql.DB` wrapper with automated *Connection Pooling* and *OpenTelemetry Tracing*.

-   **Server (`server`)**: HTTP & gRPC servers pre-configured with standard middleware (Recovery, Logging, Auth, OTel).

-   **Logging (`log`)**: Structured JSON logger (`slog`) with automatic Trace ID injection.

-   **Messaging (`messaging`)**: Kafka Producer wrapper with OTel context propagation.

-   **Crypto (`crypto`)**: Helpers for Password Hashing (Bcrypt) and JWKS Caching Client.

-   **Caching (`cache`)**: Redis client wrapper with fail-fast connectivity checks.

🚀 Installation
---------------

```
go get [github.com/godamri/helix-fnd@v0.1.0](https://github.com/godamri/helix-fnd@v0.1.0)

```

📖 Usage Guide
--------------

### 1\. Bootstrap Service (`main.go`)

Use `app.Runner` to manage the application lifecycle.

```
package main

import (
	"context"
	"log/slog"
	
	"[github.com/godamri/helix-fnd/app](https://github.com/godamri/helix-fnd/app)"
	"[github.com/godamri/helix-fnd/log](https://github.com/godamri/helix-fnd/log)"
)

func main() {
	// Init Logger
	logger := log.New(log.Config{Level: "info", Format: "json"})
	slog.SetDefault(logger)

	// Init Runner
	runner := app.NewRunner(logger)

	runner.Run(func(ctx context.Context) error {
		logger.Info("Service started!")
		
		// ... Init DB, Server, Workers here ...
		
		return nil // Block here if needed, or return nil for cleanup
	})
}

```

### 2\. Database Connection

Automatically integrated with OpenTelemetry.

```
import "[github.com/godamri/helix-fnd/database](https://github.com/godamri/helix-fnd/database)"

func initDB(ctx context.Context) (*sql.DB, error) {
	cfg := database.Config{
		DSN: "postgres://user:pass@localhost:5432/db?sslmode=disable",
	}
	return database.NewPostgres(ctx, cfg, "my-service-name")
}

```

### 3\. HTTP Server

Automatically attaches middleware: Panic Recovery, OTel Tracing, Logging, Security Headers.

```
import (
	"[github.com/go-chi/chi/v5](https://github.com/go-chi/chi/v5)"
	"[github.com/godamri/helix-fnd/server](https://github.com/godamri/helix-fnd/server)"
)

func initServer(logger *slog.Logger) {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello Helix!"))
	})

	// Setup Server Config (Port, Timeout, etc.)
	cfg := server.Config{Port: "8080"}
	
	// Server Wrapper
	srv := server.New(cfg, logger, r, nil) // nil because gRPC is disabled
	
	// Start (Blocking)
	srv.Start(context.Background())
}

```

### 4\. Authentication (JWKS)

Validate JWT Tokens by dynamically fetching Public Keys from the Auth Service.

```
import (
	"[github.com/godamri/helix-fnd/crypto](https://github.com/godamri/helix-fnd/crypto)"
	"[github.com/godamri/helix-fnd/server/middleware](https://github.com/godamri/helix-fnd/server/middleware)"
)

func setupAuth(logger *slog.Logger) {
	// Init JWKS Client
	jwks, _ := crypto.NewJWKSCachingClient(
		"http://auth-service/.well-known/jwks.json",
		"helix-auth-service",
		5*time.Minute,
		logger,
	)
	
	// Create Middleware Factory
	authFactory := middleware.NewAuthMiddlewareFactory(jwks)
	
	// Use in Router
	r.Use(authFactory.HTTPMiddleware)
}

```

🛡️ Resilience Philosophy
-------------------------

This Foundation is designed with the **"Fail Fast, Recover Safe"** principle:

1.  **Startup**: If config is invalid or DB is unreachable on start -> **Panic/Exit** (Fail Fast). Ensures failed deployments are detected immediately.

2.  **Runtime**: If a panic occurs in a handler -> **Recover & Log** (Recover Safe). Prevents a single bad request from crashing the entire pod.

🤝 Contribution
---------------

1.  Clone repo.

2.  Create a feature branch.

3.  Ensure `go mod tidy` runs and linters pass.

4.  Submit PR.

*Built for Helix Platform Ecosystem.*