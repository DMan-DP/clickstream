package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
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

// TestWithAccessLog checks the contract of WithAccessLog and statusRecorder:
// next's response reaches the client unchanged, one Info record "request"
// is logged with the status the client actually got, the body bytes and
// the duration in whole milliseconds, and probe paths are not logged.
// Each case runs in a synctest bubble, so durations are exact.
func TestWithAccessLog(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		next   http.HandlerFunc

		wantCode int    // status the client receives
		wantBody string // body the client receives
		// wantLog is the expected record without the "time" key;
		// nil means nothing is logged.
		wantLog map[string]any
	}{
		{
			name:     "no write is an implicit 200",
			method:   http.MethodPost,
			path:     "/events",
			next:     func(w http.ResponseWriter, r *http.Request) {},
			wantCode: http.StatusOK,
			wantLog:  record(http.MethodPost, "/events", 200, 0, 0),
		},
		{
			name:   "write only is an implicit 200 with the byte count",
			method: http.MethodGet,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hello"))
			},
			wantCode: http.StatusOK,
			wantBody: "hello",
			wantLog:  record(http.MethodGet, "/events", 200, 5, 0),
		},
		{
			name:   "explicit status and body",
			method: http.MethodPost,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error":"x"}`))
			},
			wantCode: http.StatusServiceUnavailable,
			wantBody: `{"error":"x"}`,
			wantLog:  record(http.MethodPost, "/events", 503, 13, 0),
		},
		{
			name:   "several writes are summed",
			method: http.MethodPost,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte("ab"))
				w.Write([]byte("cde"))
			},
			wantCode: http.StatusAccepted,
			wantBody: "abcde",
			wantLog:  record(http.MethodPost, "/events", 202, 5, 0),
		},
		{
			name:   "second WriteHeader does not change the status",
			method: http.MethodPost,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.WriteHeader(http.StatusOK)
			},
			wantCode: http.StatusServiceUnavailable,
			wantLog:  record(http.MethodPost, "/events", 503, 0, 0),
		},
		{
			name:   "WriteHeader after Write does not change the status",
			method: http.MethodPost,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("ok"))
				w.WriteHeader(http.StatusNotFound)
			},
			wantCode: http.StatusOK,
			wantBody: "ok",
			wantLog:  record(http.MethodPost, "/events", 200, 2, 0),
		},
		{
			name:   "duration in whole milliseconds",
			method: http.MethodPost,
			path:   "/events",
			next: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(1500*time.Millisecond + 700*time.Microsecond)
				w.WriteHeader(http.StatusAccepted)
			},
			wantCode: http.StatusAccepted,
			wantLog:  record(http.MethodPost, "/events", 202, 0, 1500),
		},
		{
			name:   "unknown path is logged",
			method: http.MethodGet,
			path:   "/unknown",
			next: func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			},
			wantCode: http.StatusNotFound,
			wantBody: "404 page not found\n",
			wantLog:  record(http.MethodGet, "/unknown", 404, 19, 0),
		},
		{
			name:   "livez is not logged",
			method: http.MethodGet,
			path:   "/livez",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "failing readyz is not logged either",
			method: http.MethodGet,
			path:   "/readyz",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name:   "path that only starts like a probe is logged",
			method: http.MethodGet,
			path:   "/livez/extra",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantCode: http.StatusNotFound,
			wantLog:  record(http.MethodGet, "/livez/extra", 404, 0, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var buf bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&buf, nil))

				req := httptest.NewRequest(tt.method, tt.path, nil)
				rec := httptest.NewRecorder()

				WithAccessLog(tt.next, logger).ServeHTTP(rec, req)

				if rec.Code != tt.wantCode {
					t.Errorf("client status = %d, want %d", rec.Code, tt.wantCode)
				}
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("client body = %q, want %q", got, tt.wantBody)
				}

				records := decodeRecords(t, &buf)
				if tt.wantLog == nil {
					if len(records) != 0 {
						t.Fatalf("logged %d records, want none: %v", len(records), records)
					}
					return
				}
				if len(records) != 1 {
					t.Fatalf("logged %d records, want 1: %v", len(records), records)
				}
				got := records[0]
				if _, ok := got["time"]; !ok {
					t.Errorf("record has no time: %v", got)
				}
				delete(got, "time")
				if diff := cmp.Diff(tt.wantLog, got); diff != "" {
					t.Errorf("record mismatch (-want +got):\n%s", diff)
				}
			})
		})
	}
}

// TestWithAccessLogPassesThrough checks that the wrapper is transparent
// for what next does besides status and body: headers set by next reach
// the client, and http.ResponseController reaches the wrapped writer
// through Unwrap.
func TestWithAccessLogPassesThrough(t *testing.T) {
	var flushErr error
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		flushErr = http.NewResponseController(w).Flush()
	})

	req := httptest.NewRequest(http.MethodPost, "/events", nil)
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	WithAccessLog(next, logger).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if flushErr != nil {
		t.Errorf("Flush through the wrapper: %v", flushErr)
	}
	if !rec.Flushed {
		t.Errorf("wrapped writer was not flushed")
	}
}

// record builds the expected access log record. Numbers are float64,
// as encoding/json decodes them into map[string]any.
func record(method, path string, status, bytes, durationMS int) map[string]any {
	return map[string]any{
		"level":       "INFO",
		"msg":         "request",
		"method":      method,
		"path":        path,
		"status":      float64(status),
		"bytes":       float64(bytes),
		"duration_ms": float64(durationMS),
	}
}

// decodeRecords decodes the JSON lines written by slog.JSONHandler.
func decodeRecords(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	var records []map[string]any
	dec := json.NewDecoder(r)
	for {
		var m map[string]any
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			return records
		}
		if err != nil {
			t.Fatalf("decode log: %v", err)
		}
		records = append(records, m)
	}
}
