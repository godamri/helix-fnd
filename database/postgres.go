package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Config holds standard database configuration.
type Config struct {
	DBDSN             string        `envconfig:"DB_DSN" required:"true"`
	DBMaxConns        int32         `envconfig:"DB_MAX_OPEN_CONNS" default:"50"`
	DBMinConns        int32         `envconfig:"DB_MIN_IDLE_CONNS" default:"10"`
	DBMaxConnIdle     time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"30m"`
	DBMaxConnLife     time.Duration `envconfig:"DB_CONN_MAX_IDLE_TIME" default:"15m"`
	DBConnectTimeout  time.Duration `envconfig:"DB_CONN_TIMEOUT" default:"15m"`
	HealthCheckPeriod time.Duration `envconfig:"DB_HEALTHCHECK_PERIOD" default:"1m"`
}

// NewPostgres initializes a *pgxpool.Pool.
func NewPostgres(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("database: failed to parse DBDSN: %w", err)
	}

	// Tuning Pool
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLife
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdle
	poolConfig.ConnConfig.ConnectTimeout = cfg.DBConnectTimeout
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod

	poolConfig.ConnConfig.Tracer = &otelPgxTracer{
		tracer: otel.Tracer("helix-fnd/database"),
	}

	poolConfig.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("database: failed to create pool: %w", err)
	}

	// Fail-fast verification
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: failed to ping postgres: %w", err)
	}

	return pool, nil
}

// otelPgxTracer implements pgx.QueryTracer to provide direct OpenTelemetry integration.
type otelPgxTracer struct {
	tracer trace.Tracer
}

// TraceQueryStart is called at the beginning of Query, QueryRow, and Exec calls.
func (t *otelPgxTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if !trace.SpanFromContext(ctx).IsRecording() {
		return ctx
	}

	// High-cardinality names (like the SQL itself) are bad for some APM backends.
	ctx, _ = t.tracer.Start(ctx, "db.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.statement", data.SQL), // Prepared statement SQL (usually parameter placeholders $1, $2)
			// attribute.Int("db.args_count", len(data.Args)), // Optional: debug info
		),
	)
	return ctx
}

// TraceQueryEnd is called after a query is executed.
func (t *otelPgxTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	defer span.End()

	if data.Err != nil {
		// Record error cleanly
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	} else {
		// Record strict command tag if available (e.g., "INSERT 0 1")
		span.SetAttributes(attribute.String("db.command_tag", data.CommandTag.String()))
	}
}
