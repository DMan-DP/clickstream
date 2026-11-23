package handler

import (
	"context"
	"log/slog"
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

var _ http.ResponseWriter = (*statusRecorder)(nil)

// statusRecorder wraps an http.ResponseWriter and records what was sent
// to the client: the response status and the number of body bytes.
// Calls are passed through to the wrapped writer.
type statusRecorder struct {
	http.ResponseWriter
	status      int  // status sent to the client, valid once wroteHeader is set
	wroteHeader bool // the status line has been sent, explicitly or implicitly
	bytes       int  // body bytes written by the wrapped writer
}

// Write writes data to the wrapped writer and adds the number of bytes
// actually written to r.bytes. If no status was sent yet, it records
// 200 OK, as net/http sends it implicitly on the first Write.
func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}

	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

// WriteHeader sends statusCode to the wrapped writer and records it if
// no status was sent yet. Later calls are passed through as well, so
// net/http still reports them as superfluous; the recorded status stays
// the first one, which is what the client receives.
func (r *statusRecorder) WriteHeader(statusCode int) {
	if !r.wroteHeader {
		r.status = statusCode
		r.wroteHeader = true
	}

	r.ResponseWriter.WriteHeader(statusCode)
}

// Unwrap returns the wrapped writer, so that http.ResponseController
// can reach its optional methods such as Flush and SetWriteDeadline.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// WithAccessLog returns a handler that runs next and then logs one Info
// record "request" with the method, path, response status, body bytes
// and duration in whole milliseconds. Requests to /livez and /readyz are
// not logged: kubelet probes them every few seconds on every pod. It
// must be the outermost middleware, so that it sees the status that
// actually reached the client and the full duration. If next panics,
// the request is not logged.
func WithAccessLog(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		path := r.URL.Path
		if path != "/livez" && path != "/readyz" {
			logger.InfoContext(r.Context(), "request",
				"method", r.Method,
				"path", path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	})
}
