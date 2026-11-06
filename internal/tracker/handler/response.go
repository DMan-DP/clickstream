package handler

import (
	"encoding/json"
	"net/http"
)

// writeError writes status and a JSON body {"error": msg}.
// msg is sent to the client as is, so it must not contain internal
// details; log the underlying error separately.
// It must be called before anything else is written to w.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
