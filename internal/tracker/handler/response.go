package handler

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

// writeError writes status and a JSON body {"error": msg}.
// msg is sent to the client as is, so it must not contain internal
// details; log the underlying error separately.
// It must be called before anything else is written to w.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// writeJSON writes status and data encoded as JSON with the
// application/json content type. It must be called before anything else
// is written to w. An encoding error is ignored: the status line is
// already sent at that point, so the client cannot be told about it.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
