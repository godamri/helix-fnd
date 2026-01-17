package response

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/godamri/helix-fnd/pkg/contextx"
)

type Envelope struct {
	Data  interface{}    `json:"data,omitempty"`
	Meta  interface{}    `json:"meta,omitempty"`
	Error *ErrorResponse `json:"error,omitempty"`
}

type ErrorResponse struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	DocURL    string      `json:"doc_url,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

type Meta struct {
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
	TotalItems int    `json:"total_items,omitempty"`
	HasNext    bool   `json:"has_next,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
}

func JSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	env := Envelope{
		Data: data,
	}

	if data == nil && status != http.StatusNoContent {
		env.Data = map[string]string{}
	}

	write(w, r, status, env)
}

func JSONWithMeta(w http.ResponseWriter, r *http.Request, status int, data interface{}, meta interface{}) {
	env := Envelope{
		Data: data,
		Meta: meta,
	}
	write(w, r, status, env)
}

func ErrorJSON(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	ErrorJSONWithDetails(w, r, status, code, message, nil)
}

func ErrorJSONWithDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	reqID := contextx.GetRequestID(r.Context())

	env := Envelope{
		Error: &ErrorResponse{
			Code:      code,
			Message:   message,
			RequestID: reqID,
			Details:   details,
			// DocURL can be dynamically constructed if needed, e.g., os.Getenv("DOC_BASE_URL") + code
		},
	}
	write(w, r, status, env)
}

func write(w http.ResponseWriter, r *http.Request, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.ErrorContext(r.Context(), "http_response_write_failed",
			"status", status,
			"error", err,
		)
	}
}
