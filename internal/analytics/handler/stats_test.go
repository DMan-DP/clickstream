package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
	"github.com/DMan-DP/clickstream/internal/httpx"
)

type fakeReportReader struct {
	report      domain.Report
	err         error
	called      bool
	query       domain.ReportQuery
	hasDeadline bool
	deadline    time.Time
}

func (f *fakeReportReader) ReadReport(ctx context.Context, q domain.ReportQuery) (domain.Report, error) {
	f.called = true
	f.query = q
	f.deadline, f.hasDeadline = ctx.Deadline()
	return f.report, f.err
}

// A fixed clock in a non-UTC zone: the handler must report the period in UTC.
var statsNow = time.Date(2026, 11, 23, 15, 4, 5, 0, time.FixedZone("MSK", 3*60*60))

func newTestStats(reader reportReader) *Stats {
	h := NewStats(discardLogger, reader, 30*time.Second)
	h.now = func() time.Time { return statsNow }
	return h
}

func serveStats(h *Stats, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestStatsQueryParams(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantWindow time.Duration
		wantPeriod time.Duration
	}{
		{name: "defaults", target: "/stats", wantWindow: 30 * time.Second, wantPeriod: time.Hour},
		{name: "empty values are defaults", target: "/stats?period=&window_sec=", wantWindow: 30 * time.Second, wantPeriod: time.Hour},
		{name: "custom period", target: "/stats?period=15m", wantWindow: 30 * time.Second, wantPeriod: 15 * time.Minute},
		{name: "custom window", target: "/stats?window_sec=60", wantWindow: time.Minute, wantPeriod: time.Hour},
		{name: "both", target: "/stats?period=2h30m&window_sec=10", wantWindow: 10 * time.Second, wantPeriod: 150 * time.Minute},
		{name: "max period", target: "/stats?period=168h", wantWindow: 30 * time.Second, wantPeriod: 168 * time.Hour},
		{name: "min window", target: "/stats?window_sec=1", wantWindow: time.Second, wantPeriod: time.Hour},
		{name: "max window", target: "/stats?window_sec=86400", wantWindow: 24 * time.Hour, wantPeriod: time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReportReader{}
			rec := serveStats(newTestStats(reader), tt.target)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
			}
			if !reader.called {
				t.Fatal("ReadReport was not called")
			}

			q := reader.query
			if q.WindowSize != tt.wantWindow {
				t.Errorf("WindowSize = %v, want %v", q.WindowSize, tt.wantWindow)
			}
			if !q.To.Equal(statsNow) {
				t.Errorf("To = %v, want %v", q.To, statsNow)
			}
			if q.To.Location() != time.UTC {
				t.Errorf("To location = %v, want UTC", q.To.Location())
			}
			if got := q.To.Sub(q.From); got != tt.wantPeriod {
				t.Errorf("To - From = %v, want %v", got, tt.wantPeriod)
			}
		})
	}
}

func TestStatsBadRequest(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantParam string
	}{
		{name: "period not a duration", target: "/stats?period=abc", wantParam: "period"},
		{name: "period without unit", target: "/stats?period=60", wantParam: "period"},
		{name: "period zero", target: "/stats?period=0s", wantParam: "period"},
		{name: "period negative", target: "/stats?period=-5m", wantParam: "period"},
		{name: "period above max", target: "/stats?period=168h1s", wantParam: "period"},
		{name: "window not a number", target: "/stats?window_sec=abc", wantParam: "window_sec"},
		{name: "window fractional", target: "/stats?window_sec=1.5", wantParam: "window_sec"},
		{name: "window with unit", target: "/stats?window_sec=30s", wantParam: "window_sec"},
		{name: "window zero", target: "/stats?window_sec=0", wantParam: "window_sec"},
		{name: "window negative", target: "/stats?window_sec=-30", wantParam: "window_sec"},
		{name: "window above max", target: "/stats?window_sec=86401", wantParam: "window_sec"},
		{name: "window overflows", target: "/stats?window_sec=99999999999999999999", wantParam: "window_sec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReportReader{}
			rec := serveStats(newTestStats(reader), tt.target)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if reader.called {
				t.Error("ReadReport was called for a bad request")
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var body httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v, body %s", err, rec.Body)
			}
			if !strings.Contains(body.Error, tt.wantParam) {
				t.Errorf("error = %q, want it to name %q", body.Error, tt.wantParam)
			}
		})
	}
}

func TestStatsReaderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   bool
	}{
		{name: "client gone", err: fmt.Errorf("query: %w", context.Canceled), wantStatus: httpx.StatusClientClosedRequest, wantBody: false},
		{name: "timeout", err: fmt.Errorf("query: %w", context.DeadlineExceeded), wantStatus: http.StatusServiceUnavailable, wantBody: true},
		{name: "other error", err: errors.New("relation does not exist"), wantStatus: http.StatusInternalServerError, wantBody: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReportReader{err: tt.err}
			rec := serveStats(newTestStats(reader), "/stats")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if gotBody := rec.Body.Len() > 0; gotBody != tt.wantBody {
				t.Errorf("has body = %v, want %v, body %s", gotBody, tt.wantBody, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "relation") {
				t.Errorf("body leaks the internal error: %s", rec.Body)
			}
		})
	}
}

func TestStatsReaderDeadline(t *testing.T) {
	reader := &fakeReportReader{}
	before := time.Now()
	serveStats(newTestStats(reader), "/stats")

	if !reader.hasDeadline {
		t.Fatal("ReadReport got a context without a deadline")
	}
	if limit := before.Add(reportTimeout); reader.deadline.After(limit.Add(time.Second)) {
		t.Errorf("deadline = %v, want at most about %v", reader.deadline, limit)
	}
}

func TestStatsReaderUsesRequestContext(t *testing.T) {
	h := newTestStats(readerFunc(func(ctx context.Context, q domain.ReportQuery) (domain.Report, error) {
		return domain.Report{}, ctx.Err()
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stats", nil).WithContext(ctx))

	if rec.Code != httpx.StatusClientClosedRequest {
		t.Errorf("status = %d, want %d: the request context must reach ReadReport", rec.Code, httpx.StatusClientClosedRequest)
	}
}

type readerFunc func(ctx context.Context, q domain.ReportQuery) (domain.Report, error)

func (f readerFunc) ReadReport(ctx context.Context, q domain.ReportQuery) (domain.Report, error) {
	return f(ctx, q)
}

func TestStatsResponse(t *testing.T) {
	windowStart := time.Date(2026, 11, 23, 15, 0, 30, 0, time.FixedZone("MSK", 3*60*60))
	reader := &fakeReportReader{report: domain.Report{
		Series: []domain.SeriesPoint{
			{WindowStart: windowStart, Type: domain.EventType("click"), Events: 12},
		},
		Totals: []domain.ElementTotal{
			{Type: domain.EventType("change"), ElementID: "pizza-select", Events: 7},
		},
		Choices: []domain.ChoiceTotal{
			{Choice: "pizza", Value: "pepperoni", Picks: 4},
		},
		Funnel: []domain.FunnelTotal{
			{Step: "visit", Sessions: 5},
			{Step: "pizza", Sessions: 4},
		},
	}}
	rec := serveStats(newTestStats(reader), "/stats?period=2h&window_sec=60")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	want := `{
		"window_sec": 60,
		"from": "2026-11-23T10:04:05Z",
		"to": "2026-11-23T12:04:05Z",
		"series": [{"window_start": "2026-11-23T12:00:30Z", "event_type": "click", "events": 12}],
		"totals": [{"event_type": "change", "element_id": "pizza-select", "events": 7}],
		"choices": [{"choice": "pizza", "value": "pepperoni", "picks": 4}],
		"funnel": [{"step": "visit", "sessions": 5}, {"step": "pizza", "sessions": 4}]
	}`
	assertJSONEqual(t, rec.Body.Bytes(), want)
}

func TestStatsEmptyReport(t *testing.T) {
	rec := serveStats(newTestStats(&fakeReportReader{}), "/stats")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	for _, key := range []string{"series", "totals", "choices", "funnel"} {
		if got := string(body[key]); got != "[]" {
			t.Errorf("%s = %s, want []", key, got)
		}
	}
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()

	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("body is not JSON: %v, body %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}

	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("body =\n%s\nwant\n%s", gb, wb)
	}
}
