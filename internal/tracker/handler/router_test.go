package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const headerStubKey = "X-Stub"

func TestRouter(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantHandler string
		wantAllow   string
	}{
		{
			name:       "GET livez ok",
			method:     http.MethodGet,
			path:       "/livez",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HEAD livez ok",
			method:     http.MethodHead,
			path:       "/livez",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET livez trailing slash 404",
			method:     http.MethodGet,
			path:       "/livez/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "POST livez 405",
			method:     http.MethodPost,
			path:       "/livez",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  "GET, HEAD",
		},
		{
			name:        "GET readyz",
			method:      http.MethodGet,
			path:        "/readyz",
			wantStatus:  http.StatusNoContent,
			wantHandler: "readyz",
		},
		{
			name:        "HEAD readyz",
			method:      http.MethodHead,
			path:        "/readyz",
			wantStatus:  http.StatusNoContent,
			wantHandler: "readyz",
		},
		{
			name:       "POST readyz 405",
			method:     http.MethodPost,
			path:       "/readyz",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  "GET, HEAD",
		},
		{
			name:       "GET nope 404",
			method:     http.MethodGet,
			path:       "/nope",
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "POST events 204",
			method:      http.MethodPost,
			path:        "/events",
			wantStatus:  http.StatusNoContent,
			wantHandler: "events",
		},
		{
			name:       "GET events 405",
			method:     http.MethodGet,
			path:       "/events",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  "POST",
		},
	}

	routes := Routes{
		Events: createStubHandler("events"),
		Readyz: createStubHandler("readyz"),
	}
	mux := NewRouter(routes)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantAllow != "" {
				allow := rec.Header().Get("Allow")
				if allow != tc.wantAllow {
					t.Errorf("allow methods: got %q, want %q", allow, tc.wantAllow)
				}
			}

			stubHandler := rec.Result().Header.Get(headerStubKey)
			if tc.wantHandler != stubHandler {
				t.Errorf("stub header: got %q, want %q", stubHandler, tc.wantHandler)
			}
		})

	}
}

func createStubHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerStubKey, name)
		w.WriteHeader(http.StatusNoContent)
	})
}
