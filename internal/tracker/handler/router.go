package handler

import (
	"net/http"
)

type Routes struct {
	Events http.Handler
}

// NewRouter returns the routes of tracker-api: GET /health (Health) and
// POST /events (r.Events). Routing is done by http.ServeMux, so HEAD
// /health is served by Health, a known path with another method gets 405
// with an Allow header, and any other path gets 404.
func NewRouter(r Routes) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", Health)
	mux.Handle("POST /events", r.Events)
	return mux
}
