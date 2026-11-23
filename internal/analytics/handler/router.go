package handler

import (
	"net/http"
)

type Routes struct {
	Livez  http.Handler
	Readyz http.Handler
}

// NewRouter returns the probe routes of analytics-processor: GET /livez
// (r.Livez) and GET /readyz (r.Readyz). Routing is done by http.ServeMux,
// so HEAD is served by the same handlers, a known path with another method
// gets 405 with an Allow header, and any other path gets 404.
func NewRouter(r Routes) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /livez", r.Livez)
	mux.Handle("GET /readyz", r.Readyz)
	return mux
}
