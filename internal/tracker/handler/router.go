package handler

import (
	"net/http"
)

type Routes struct {
	Events http.Handler
	Readyz http.Handler
}

// NewRouter returns the routes of tracker-api: GET /livez (Liveness),
// GET /readyz (r.Readyz) and POST /events (r.Events). Routing is done by
// http.ServeMux, so HEAD /livez and HEAD /readyz are served by the same
// handlers, a known path with another method gets 405 with an Allow
// header, and any other path gets 404.
func NewRouter(r Routes) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", Liveness)
	mux.Handle("GET /readyz", r.Readyz)
	mux.Handle("POST /events", r.Events)
	return mux
}
