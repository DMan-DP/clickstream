package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

// TestWithTimeout checks that next gets a ctx derived from the request ctx
// that is cancelled exactly after d, and that next writes the response.
// The test runs in a synctest bubble, so d passes on the fake clock.
func TestWithTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const d = 5 * time.Second
		type ctxKey struct{}

		var (
			elapsed time.Duration
			ctxErr  error
			value   any
		)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Waits for the ctx, but not forever: without a deadline the
			// test fails instead of hanging.
			select {
			case <-r.Context().Done():
			case <-time.After(2 * d):
			}
			elapsed = time.Since(start)
			ctxErr = r.Context().Err()
			value = r.Context().Value(ctxKey{})
			w.WriteHeader(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodPost, "/events", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "request"))
		rec := httptest.NewRecorder()

		WithTimeout(next, d).ServeHTTP(rec, req)

		if elapsed != d {
			t.Errorf("ctx done after %v, want %v", elapsed, d)
		}
		if !errors.Is(ctxErr, context.DeadlineExceeded) {
			t.Errorf("ctx error = %v, want %v", ctxErr, context.DeadlineExceeded)
		}
		if value != "request" {
			t.Errorf("next got a ctx that is not derived from the request ctx")
		}
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d written by next", rec.Code, http.StatusNoContent)
		}
	})
}
