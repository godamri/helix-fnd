package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// Config holds standard database configuration.
type Config struct {
	DSN             string        `envconfig:"DB_DSN" required:"true"`
	MaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"15m"`
}

// NewPostgres initializes a *sql.DB with OpenTelemetry instrumentation and connection pooling.
// NOTE: The caller MUST register the driver (e.g. _ "github.com/jackc/pgx/v5/stdlib")
// and pass the driverName (e.g. "pgx") explicitly.
func NewPostgres(ctx context.Context, cfg Config, driverName string, serviceName string) (*sql.DB, error) {
	// Wrap the driver with OTel instrumentation
	db, err := otelsql.Open(driverName, cfg.DSN,
		otelsql.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
		otelsql.WithDBName("postgres"),
	)
	if err != nil {
		return nil, fmt.Errorf("helix-fnd/database: failed to open connection: %w", err)
	}

	// Configure Connection Pooling
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify Connectivity with Timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("helix-fnd/database: failed to ping database: %w", err)
	}

	return db, nil
}
