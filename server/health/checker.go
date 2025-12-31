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

type Checker struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewChecker(db *sql.DB, logger *slog.Logger) *Checker {
	return &Checker{
		db:     db,
		logger: logger,
	}
}

func (c *Checker) RegisterRoutes(r chi.Router) {
	r.Get("/health", c.HandleHealth)   // Liveness
	r.Get("/ready", c.HandleReadiness) // Readiness
}

func (c *Checker) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (c *Checker) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
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
