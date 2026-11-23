package handler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestReadiness(t *testing.T) {
	var shuttingDown atomic.Bool
	h := NewReadiness(&shuttingDown)

	probe := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec
	}

	// The zero value of the flag means the service is ready.
	rec := probe()
	if rec.Code != http.StatusOK {
		t.Errorf("before shutdown: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("before shutdown: body = %q, want empty", rec.Body.String())
	}

	// The handler must read the flag on every request, not once in the
	// constructor.
	shuttingDown.Store(true)
	rec = probe()
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("after shutdown started: status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("after shutdown started: body = %q, want empty", rec.Body.String())
	}
}
