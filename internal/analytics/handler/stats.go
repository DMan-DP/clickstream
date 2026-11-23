package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
	"github.com/DMan-DP/clickstream/internal/httpx"
)

// Limits of the query parameters of GET /stats. maxPeriod bounds the rows
// one request may make the database scan. maxWindowSec is far above any
// sane WINDOW_SIZE and keeps window_sec * time.Second from overflowing.
const (
	defaultPeriod = time.Hour
	maxPeriod     = 7 * 24 * time.Hour
	maxWindowSec  = 24 * 60 * 60
)

// reportTimeout bounds the database queries of one report. It is below
// the WriteTimeout of the server, so a slow query ends with 503 and a
// log line instead of a dropped connection.
const reportTimeout = 5 * time.Second

// reportReader reads the stored counters for a period.
type reportReader interface {
	ReadReport(ctx context.Context, q domain.ReportQuery) (domain.Report, error)
}

// Stats serves the statistics for a period, read from PostgreSQL.
type Stats struct {
	logger     *slog.Logger
	db         reportReader
	windowSize time.Duration
	now        func() time.Time
}

// NewStats returns the stats handler. windowSize is the default window
// length of a report, WINDOW_SIZE of this instance.
func NewStats(logger *slog.Logger, db reportReader, windowSize time.Duration) *Stats {
	return &Stats{
		logger:     logger,
		db:         db,
		windowSize: windowSize,
		now:        time.Now,
	}
}

// ServeHTTP answers GET /stats?period=1h&window_sec=30 with the report for
// the period that ends now. Both parameters are optional; an empty value
// is the same as a missing one.
//   - period: a Go duration (15m, 2h), more than 0 and at most maxPeriod
//     (168h), default defaultPeriod (1h);
//   - window_sec: the window length in seconds, from 1 to maxWindowSec,
//     default the window size of this instance. Counters written with
//     another window length are not included.
//
// Statuses, an error body is {"error": msg}:
//   - 200 with the report as JSON, empty lists as [];
//   - 400 for a bad parameter, the message names it;
//   - 503 when the database does not answer within reportTimeout;
//   - 499 without a body when the client has gone;
//   - 500 for any other database error.
func (h *Stats) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q, err := parseReportQuery(r.URL.Query(), h.windowSize, h.now())
	if err != nil {
		h.logger.WarnContext(r.Context(), "bad request", "err", err)
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), reportTimeout)
	defer cancel()

	report, err := h.db.ReadReport(ctx, q)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			h.logger.InfoContext(ctx, "report canceled", "err", err)
			w.WriteHeader(httpx.StatusClientClosedRequest)
		case errors.Is(err, context.DeadlineExceeded):
			h.logger.ErrorContext(ctx, "report timeout", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "Service unavailable")
		default:
			h.logger.ErrorContext(ctx, "report failed", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toReportResponse(q, report))
}

// parseReportQuery reads period and window_sec from params, see
// Stats.ServeHTTP, and returns the query for the period that ends at now.
// The text of an error is meant for the client.
func parseReportQuery(params url.Values, defaultWindow time.Duration, now time.Time) (domain.ReportQuery, error) {
	period := defaultPeriod
	if s := params.Get("period"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 || d > maxPeriod {
			return domain.ReportQuery{}, fmt.Errorf("period must be a duration above 0 and at most %dh, e.g. 15m or 2h", int(maxPeriod.Hours()))
		}
		period = d
	}

	window := defaultWindow
	if s := params.Get("window_sec"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 || n > maxWindowSec {
			return domain.ReportQuery{}, fmt.Errorf("window_sec must be an integer from 1 to %d", maxWindowSec)
		}
		window = time.Duration(n) * time.Second
	}

	to := now.UTC()
	return domain.ReportQuery{
		WindowSize: window,
		From:       to.Add(-period),
		To:         to,
	}, nil
}

type reportResponse struct {
	WindowSec int64          `json:"window_sec"`
	From      time.Time      `json:"from"`
	To        time.Time      `json:"to"`
	Series    []seriesPoint  `json:"series"`
	Totals    []elementTotal `json:"totals"`
	Choices   []choiceTotal  `json:"choices"`
	Funnel    []funnelTotal  `json:"funnel"`
}

type seriesPoint struct {
	WindowStart time.Time `json:"window_start"`
	EventType   string    `json:"event_type"`
	Events      int64     `json:"events"`
}

type elementTotal struct {
	EventType string `json:"event_type"`
	ElementID string `json:"element_id"`
	Events    int64  `json:"events"`
}

type choiceTotal struct {
	Choice string `json:"choice"`
	Value  string `json:"value"`
	Picks  int64  `json:"picks"`
}

type funnelTotal struct {
	Step     string `json:"step"`
	Sessions int64  `json:"sessions"`
}

// toReportResponse converts report to the JSON response. Lists are never
// nil, so an empty one is encoded as [] and not null.
func toReportResponse(q domain.ReportQuery, report domain.Report) reportResponse {
	res := reportResponse{
		WindowSec: int64(q.WindowSize / time.Second),
		From:      q.From,
		To:        q.To,
		Series:    make([]seriesPoint, 0, len(report.Series)),
		Totals:    make([]elementTotal, 0, len(report.Totals)),
		Choices:   make([]choiceTotal, 0, len(report.Choices)),
		Funnel:    make([]funnelTotal, 0, len(report.Funnel)),
	}

	for _, p := range report.Series {
		res.Series = append(res.Series, seriesPoint{
			WindowStart: p.WindowStart.UTC(),
			EventType:   string(p.Type),
			Events:      p.Events,
		})
	}

	for _, t := range report.Totals {
		res.Totals = append(res.Totals, elementTotal{
			EventType: string(t.Type),
			ElementID: t.ElementID,
			Events:    t.Events,
		})
	}

	for _, c := range report.Choices {
		res.Choices = append(res.Choices, choiceTotal{
			Choice: c.Choice,
			Value:  c.Value,
			Picks:  c.Picks,
		})
	}

	for _, f := range report.Funnel {
		res.Funnel = append(res.Funnel, funnelTotal{
			Step:     f.Step,
			Sessions: f.Sessions,
		})
	}

	return res
}
