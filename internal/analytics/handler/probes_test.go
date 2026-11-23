package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var discardLogger = slog.New(slog.DiscardHandler)

type fakeChecker struct {
	err    error
	called bool
	now    time.Time
}

func (f *fakeChecker) Check(now time.Time) error {
	f.called = true
	f.now = now
	return f.err
}

type fakePinger struct {
	err         error
	called      bool
	hasDeadline bool
	deadline    time.Time
	ctxErr      error
}

func (f *fakePinger) PingContext(ctx context.Context) error {
	f.called = true
	f.deadline, f.hasDeadline = ctx.Deadline()
	f.ctxErr = ctx.Err()
	return f.err
}

func TestLiveness(t *testing.T) {
	tests := []struct {
		name       string
		checkErr   error
		wantStatus int
	}{
		{name: "all partitions alive", checkErr: nil, wantStatus: http.StatusOK},
		{name: "stale partition", checkErr: errors.New("partition 2 is stale"), wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &fakeChecker{err: tt.checkErr}
			h := NewLiveness(discardLogger, checker)

			before := time.Now()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
			after := time.Now()

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
			if !checker.called {
				t.Fatal("Check was not called")
			}
			if checker.now.Before(before) || checker.now.After(after) {
				t.Errorf("Check got now = %v, want the current time between %v and %v", checker.now, before, after)
			}
		})
	}
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{name: "database reachable", pingErr: nil, wantStatus: http.StatusOK},
		{name: "database down", pingErr: errors.New("connection refused"), wantStatus: http.StatusServiceUnavailable},
		{name: "ping timed out", pingErr: context.DeadlineExceeded, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakePinger{err: tt.pingErr}
			h := NewReadiness(discardLogger, db)

			start := time.Now()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
			if !db.called {
				t.Fatal("PingContext was not called")
			}
			if !db.hasDeadline {
				t.Fatal("ping context has no deadline, want one bounded by pingTimeout")
			}
			// Contract: the ping is bounded by pingTimeout (1 s), so the
			// probe fails with a logged reason before the probe timeout.
			if limit := start.Add(time.Second); db.deadline.After(limit) {
				t.Errorf("ping deadline %v is later than start + 1s (%v)", db.deadline, limit)
			}
		})
	}
}

// The ping must use the request context: when the prober has gone, the
// ping is cancelled too.
func TestReadinessUsesRequestContext(t *testing.T) {
	db := &fakePinger{}
	h := NewReadiness(discardLogger, db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !errors.Is(db.ctxErr, context.Canceled) {
		t.Errorf("ping ctx.Err() = %v, want context.Canceled", db.ctxErr)
	}
}

func TestRouter(t *testing.T) {
	// Each route answers with its own status, so a swapped route shows up.
	routes := Routes{
		Livez:  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		Readyz: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }),
	}
	mux := NewRouter(routes)

	tests := []struct {
		method     string
		path       string
		wantStatus int
		wantAllow  bool
	}{
		{method: http.MethodGet, path: "/livez", wantStatus: http.StatusOK},
		{method: http.MethodHead, path: "/livez", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/readyz", wantStatus: http.StatusAccepted},
		{method: http.MethodHead, path: "/readyz", wantStatus: http.StatusAccepted},
		{method: http.MethodPost, path: "/livez", wantStatus: http.StatusMethodNotAllowed, wantAllow: true},
		{method: http.MethodPost, path: "/readyz", wantStatus: http.StatusMethodNotAllowed, wantAllow: true},
		{method: http.MethodGet, path: "/health", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if allow := rec.Header().Get("Allow"); tt.wantAllow && allow == "" {
				t.Error("Allow header is missing")
			}
		})
	}
}
