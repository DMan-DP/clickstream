package handler

import (
	"net/http"
)

// Health reports that the HTTP server is up. It always answers 200 OK
// and does not check Kafka: it is a liveness probe, not readiness.
func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
