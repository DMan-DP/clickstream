package handler

import (
	"net/http"
)

type Routes struct {
	Events http.Handler
}

func NewRouter(r Routes) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", Health)
	mux.Handle("POST /events", r.Events)
	return mux
}
