package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"log/slog"

	"github.com/go-chi/chi/v5"
)

// Checker handles the health check endpoints.
type Checker struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewChecker creates a new health checker instance.
func NewChecker(db *sql.DB, logger *slog.Logger) *Checker {
	return &Checker{
		db:     db,
		logger: logger,
	}
}

// RegisterRoutes registers the health check routes on the router.
func (c *Checker) RegisterRoutes(r chi.Router) {
	r.Get("/health", c.HandleHealth)   // Liveness
	r.Get("/ready", c.HandleReadiness) // Readiness
}

// HandleHealth provides a simple liveness check (Kubernetes Liveness Probe).
// Just returns 200 OK if the binary is running.
func (c *Checker) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// HandleReadiness checks if the service is ready to accept traffic (Kubernetes Readiness Probe).
// Performs a real-time check against the database.
func (c *Checker) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	// If DB is slow (>200ms), we consider ourselves down to prevent traffic blackholes.
	// We want the Load Balancer to cut us off immediately if DB is struggling.
	ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
	defer cancel()

	status := "UP"
	statusCode := http.StatusOK

	if err := c.db.PingContext(ctx); err != nil {
		c.logger.Error("readiness check failed: database unreachable or slow", "error", err)
		status = "DOWN"
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]string{
		"status": status,
		"db":     status,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.logger.Error("failed to write health response", "error", err)
	}
}
