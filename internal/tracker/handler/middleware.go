package handler

import (
	"context"
	"net/http"
	"time"
)

// WithTimeout returns a handler that runs next with a request context
// cancelled after d. Only code that watches r.Context() is interrupted;
// next itself is still responsible for writing the response.
func WithTimeout(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
