// Package httpx holds the HTTP middleware chain and the problem+json writer.
package httpx

import (
	"encoding/json"
	"net/http"
)

// Problem is an RFC 9457 problem detail. It is the only error body this API
// writes.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// WriteProblem writes an RFC 9457 error response. Handlers never write an
// error body any other way.
//
// detail is shown to the caller, so it must not carry internal state: no
// library error strings, no claim values, no upstream responses.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	p := Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	if r != nil {
		p.Instance = r.URL.Path
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
