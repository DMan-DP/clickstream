// Package httpx holds the HTTP response conventions shared by the
// services: JSON bodies, the error body format and the status for a
// client that has gone.
package httpx

import (
	"encoding/json"
	"net/http"
)

// StatusClientClosedRequest is the nginx status for a request whose client
// closed the connection before the response was written. net/http has no
// constant for it.
const StatusClientClosedRequest = 499

// ErrorResponse is the body of every error response: {"error": msg}.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteError writes status and a JSON body {"error": msg}.
// msg is sent to the client as is, so it must not contain internal
// details; log the underlying error separately.
// It must be called before anything else is written to w.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: msg})
}

// WriteJSON writes status and data encoded as JSON with the
// application/json content type. It must be called before anything else
// is written to w. An encoding error is ignored: the status line is
// already sent at that point, so the client cannot be told about it.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
