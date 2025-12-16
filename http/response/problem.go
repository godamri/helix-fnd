package response

import (
	"encoding/json"
	"net/http"
)

// RFC 7807: Problem Details for HTTP APIs
// Standardizes error responses for better client parsing.
type Problem struct {
	Type     string `json:"type"`               // URI reference identifying the problem type
	Title    string `json:"title"`              // Short, human-readable summary
	Status   int    `json:"status"`             // HTTP status code
	Detail   string `json:"detail,omitempty"`   // Human-readable explanation specific to this occurrence
	Instance string `json:"instance,omitempty"` // URI reference identifying the specific occurrence

	// Extension members
	TraceID string      `json:"trace_id,omitempty"`
	Errors  interface{} `json:"errors,omitempty"` // For validation errors (field-level)
}

func (p *Problem) Render(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// ErrorProblem sends an RFC 7807 response.
func ErrorProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string, errors interface{}) {
	traceID := getTraceID(r)
	prob := &Problem{
		Type:     "about:blank", // Default type. Can be customized per error code if docs exist.
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		TraceID:  traceID,
		Errors:   errors,
	}
	prob.Render(w)
}
