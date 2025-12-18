package health

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"log/slog"

	"github.com/redis/go-redis/v9"
)

// CachedHealthChecker prevents thundering herds by caching health status.
// During a crash loop or massive scale-up, thousands of probes shouldn't DDoS the DB.
type CachedHealthChecker struct {
	db        *sql.DB
	rdb       *redis.Client
	mu        sync.RWMutex
	healthy   bool
	lastCheck time.Time
	interval  time.Duration
}

func NewCachedHealthChecker(db *sql.DB, rdb *redis.Client) *CachedHealthChecker {
	checker := &CachedHealthChecker{
		db:       db,
		rdb:      rdb,
		interval: 5 * time.Second, // Only check DB once every 5 seconds
		healthy:  true,            // Optimistic start
	}
	// Start background poller immediately
	go checker.poll()
	return checker
}

func (c *CachedHealthChecker) poll() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for range ticker.C {
		c.check()
	}
}

func (c *CachedHealthChecker) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dbErr := c.db.PingContext(ctx)

	var redisErr error
	if c.rdb != nil {
		redisErr = c.rdb.Ping(ctx).Err()
	}

	isHealthy := dbErr == nil && redisErr == nil

	if !isHealthy {
		slog.Error("Health check failed", "db_error", dbErr, "redis_error", redisErr)
	}

	c.mu.Lock()
	c.healthy = isHealthy
	c.lastCheck = time.Now()
	c.mu.Unlock()
}

// ReadinessHandler now reads from memory. 0ms latency.
func (c *CachedHealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	healthy := c.healthy
	c.mu.RUnlock()

	if !healthy {
		http.Error(w, "Service Unavailable (Cached Health)", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}

// LivenessHandler checks if the app process is running.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ALIVE"))
	}
}
