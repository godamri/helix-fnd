package response

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	Meta    Meta        `json:"meta"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Meta struct {
	TraceID string `json:"trace_id"`
}

func JSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	traceID := getTraceID(r)
	env := Envelope{
		Success: true,
		Data:    data,
		Meta:    Meta{TraceID: traceID},
	}
	write(w, status, env)
}

func ErrorJSON(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	traceID := getTraceID(r)
	env := Envelope{
		Success: false,
		Error: &Error{
			Code:    code,
			Message: message,
		},
		Meta: Meta{TraceID: traceID},
	}
	write(w, status, env)
}

func write(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// If we fail here, it's catastrophic (e.g., broken pipe). Log and give up.
		// We can't write to w anymore.
	}
}

func getTraceID(r *http.Request) string {
	tid := r.Header.Get("X-Trace-Id")
	if tid == "" {
		tid = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	return tid
}
