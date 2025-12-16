package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, labeled by status, method, and path.",
		},
		[]string{"status", "method", "path"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets, // Uses default buckets (.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)
		},
		[]string{"status", "method", "path"},
	)
)

// MetricsMiddleware records RED metrics (Rate, Errors, Duration) for every request.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(ww.Status())

		// Use chi's RouteContext to get the pattern (e.g. "/users/{id}") instead of raw path "/users/123"
		// to prevent cardinality explosion in Prometheus.
		path := r.URL.Path
		if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
			path = rctx.RoutePattern()
		}

		httpRequestsTotal.WithLabelValues(status, r.Method, path).Inc()
		httpRequestDuration.WithLabelValues(status, r.Method, path).Observe(duration)
	})
}
