package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type IdempotencyMode string

const (
	ModeHeaderOnly IdempotencyMode = "HEADER_ONLY"
)

type IdempotencyConfig struct {
	Mode        IdempotencyMode
	HeaderKey   string
	Expiry      time.Duration
	RedisClient *redis.Client
	Logger      *slog.Logger
}

// StoredResponse adalah apa yang kita simpan di Redis.
// Kita butuh Status Code, Header, dan Body untuk me-replay response.
type StoredResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

// responseCapturer membajak ResponseWriter untuk menyalin output.
type responseCapturer struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (w *responseCapturer) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseCapturer) Write(b []byte) (int, error) {
	w.body.Write(b)                  // Salin ke buffer kita
	return w.ResponseWriter.Write(b) // Tulis ke client asli
}

func IdempotencyMiddleware(cfg IdempotencyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip Safe Methods (GET, HEAD, OPTIONS) - Idempotency usually for mutations
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Get Key
			key := r.Header.Get(cfg.HeaderKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			redisKey := fmt.Sprintf("idempotency:%s", key)
			ctx := r.Context()

			// Check State
			// Kita gunakan "SETNX" untuk mengklaim kunci.
			// TTL pendek (30s) untuk lock pemrosesan (mencegah deadlock jika pod crash saat proses).
			processingTTL := 30 * time.Second
			acquired, err := cfg.RedisClient.SetNX(ctx, redisKey, "PROCESSING", processingTTL).Result()
			if err != nil {
				cfg.Logger.Error("idempotency: redis error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !acquired {
				// Kunci ada. Cek isinya.
				val, err := cfg.RedisClient.Get(ctx, redisKey).Result()
				if err != nil {
					// Redis error atau key expired tepat saat kita cek
					next.ServeHTTP(w, r)
					return
				}

				if val == "PROCESSING" {
					// Request SEDANG diproses oleh thread/pod lain.
					// Di sini valid mengembalikan 409 Conflict atau 429 Too Many Requests
					// untuk memberitahu klien "Sabar, lagi dikerjakan".
					http.Error(w, "Request is currently being processed", http.StatusConflict)
					return
				}

				// Jika bukan PROCESSING, asumsikan itu adalah JSON dari StoredResponse
				var stored StoredResponse
				if jsonErr := json.Unmarshal([]byte(val), &stored); jsonErr == nil {
					// HIT! Kembalikan response yang tersimpan.
					cfg.Logger.Info("Idempotency Hit", "key", key)

					for k, v := range stored.Headers {
						for _, val := range v {
							w.Header().Add(k, val)
						}
					}
					// Tambahkan header penanda
					w.Header().Set("X-Idempotency-Hit", "true")
					w.WriteHeader(stored.Status)
					w.Write(stored.Body)
					return
				}

				// Jika data rusak/tidak valid, kita anggap miss dan proses ulang (Fail Open)
				cfg.Logger.Warn("Idempotency cache corrupted, reprocessing", "key", key)
			}

			// Execute Handler & Capture Response
			capturer := &responseCapturer{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(capturer, r)

			// Simpan Response hanya jika sukses (2xx) atau sesuai kebutuhan bisnis.
			// Biasanya kita simpan hasil final apapun (termasuk 400/500) agar konsisten.

			// Jangan simpan jika status code 0 (koneksi putus/panic sebelum writeheader)
			if capturer.statusCode == 0 {
				// Hapus lock agar bisa di-retry
				cfg.RedisClient.Del(ctx, redisKey)
				return
			}

			respToStore := StoredResponse{
				Status:  capturer.statusCode,
				Headers: capturer.Header(),
				Body:    capturer.body.Bytes(),
			}

			data, _ := json.Marshal(respToStore)

			// Ganti status "PROCESSING" dengan data response sebenarnya
			// Gunakan TTL penuh (misal 24 jam)
			cfg.RedisClient.Set(ctx, redisKey, data, cfg.Expiry)
		})
	}
}
