package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	_ "github.com/jackc/pgx/v5/stdlib" // Explicitly register pgx driver
)

// Config holds standard database configuration.
// It is the service's responsibility to load these values.
type Config struct {
	DSN             string        `envconfig:"DB_DSN" required:"true"`
	MaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"15m"`
}

// NewPostgres initializes a *sql.DB with OpenTelemetry instrumentation and connection pooling.
// It returns a standard *sql.DB compatible with any stdlib pattern (including Ent).
func NewPostgres(ctx context.Context, cfg Config, serviceName string) (*sql.DB, error) {
	// 1. Wrap the driver with OTel instrumentation
	// This ensures every SQL query automatically emits a tracing span.
	db, err := otelsql.Open("pgx", cfg.DSN,
		otelsql.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
		otelsql.WithDBName("postgres"),
	)
	if err != nil {
		return nil, fmt.Errorf("helix-fnd/database: failed to open connection: %w", err)
	}

	// 2. Configure Connection Pooling (Critical for production stability)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 3. Verify Connectivity with Timeout
	// Fail fast if the DB is unreachable during startup.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("helix-fnd/database: failed to ping database: %w", err)
	}

	return db, nil
}
