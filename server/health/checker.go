package health

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReadinessHandler checks dependencies (DB, Redis).
// Used by Kubernetes readinessProbe.
func ReadinessHandler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		// 1. Check DB
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		// 2. Check Redis (Optional / If Configured)
		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
	}
}

// LivenessHandler checks if the app process is running.
// Used by Kubernetes livenessProbe.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ALIVE"))
	}
}
