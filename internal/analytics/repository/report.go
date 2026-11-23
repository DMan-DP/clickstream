package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// Report queries, the same as in queries/stats.sql. Parameters of every
// query: $1 window length in seconds, $2 start of the period (inclusive),
// $3 end of the period (exclusive). A row holds the counter of one window
// and one key; the upsert has already merged the parts of a window into
// it. Every query sums the rows it groups together: windows of the
// period, elements of an event type, or both.

const reportSeriesQuery = `
			SELECT window_start, event_type, sum(count)::bigint AS events
			FROM event_stats
			WHERE window_sec = $1 AND window_start >= $2 AND window_start < $3
			GROUP BY window_start, event_type
			ORDER BY window_start, event_type
			`

const reportTotalsQuery = `
			SELECT event_type, element_id, sum(count)::bigint AS events
			FROM event_stats
			WHERE window_sec = $1 AND window_start >= $2 AND window_start < $3
			GROUP BY event_type, element_id
			ORDER BY event_type, events DESC, element_id
			`

// Sizes and toppings are separate page elements, so they are grouped. A
// topping change to false is an uncheck, not a pick.
const reportChoicesQuery = `
			SELECT CASE
			           WHEN element_id LIKE 'size-%' THEN 'size'
			           WHEN element_id LIKE 'topping-%' THEN 'topping'
			           WHEN element_id = 'pizza-select' THEN 'pizza'
			           WHEN element_id = 'quantity-slider' THEN 'quantity'
			           ELSE element_id
			       END AS choice,
			       CASE
			           WHEN element_id LIKE 'topping-%' THEN substr(element_id, 9)
			           ELSE value
			       END AS value,
			       sum(count)::bigint AS picks
			FROM value_stats
			WHERE window_sec = $1 AND window_start >= $2 AND window_start < $3
			  AND NOT (element_id LIKE 'topping-%' AND value = 'false')
			GROUP BY 1, 2
			ORDER BY choice, picks DESC, value
			`

// Steps come from a fixed list, so a step nobody reached gives 0 and the
// order is the funnel order.
const reportFunnelQuery = `
			WITH steps (step, pos) AS (
			    VALUES ('visit', 1), ('pizza', 2), ('contacts', 3), ('submit', 4)
			)
			SELECT s.step, coalesce(sum(f.sessions), 0)::bigint AS sessions
			FROM steps s
			LEFT JOIN funnel_stats f
			       ON f.step = s.step
			      AND f.window_sec = $1 AND f.window_start >= $2 AND f.window_start < $3
			GROUP BY s.step, s.pos
			ORDER BY s.pos
			`

// Rows of the report queries. Column names are the aliases in the
// queries; the rows are converted to domain types, so the schema does not
// leak out of the repository.
type seriesRow struct {
	WindowStart time.Time `db:"window_start"`
	EventType   string    `db:"event_type"`
	Events      int64     `db:"events"`
}

type totalRow struct {
	EventType string `db:"event_type"`
	ElementID string `db:"element_id"`
	Events    int64  `db:"events"`
}

type choiceRow struct {
	Choice string `db:"choice"`
	Value  string `db:"value"`
	Picks  int64  `db:"picks"`
}

type funnelRow struct {
	Step     string `db:"step"`
	Sessions int64  `db:"sessions"`
}

// ReadReport reads the counters with window length q.WindowSize whose
// window starts in [q.From, q.To) and sums them into a report, see
// domain.Report. The four queries run in one read-only REPEATABLE READ
// transaction, so they all see the same snapshot of the database: a
// window flushed between two queries cannot make the totals disagree with
// the series.
func (r *Postgres) ReadReport(ctx context.Context, q domain.ReportQuery) (domain.Report, error) {
	windowSec := int64(q.WindowSize.Seconds())

	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.Report{}, fmt.Errorf("db begin tx: %w", err)
	}
	defer tx.Rollback()

	var series []seriesRow
	if err := tx.SelectContext(ctx, &series, reportSeriesQuery, windowSec, q.From, q.To); err != nil {
		return domain.Report{}, fmt.Errorf("db read series: %w", err)
	}

	var totals []totalRow
	if err := tx.SelectContext(ctx, &totals, reportTotalsQuery, windowSec, q.From, q.To); err != nil {
		return domain.Report{}, fmt.Errorf("db read totals: %w", err)
	}

	var choices []choiceRow
	if err := tx.SelectContext(ctx, &choices, reportChoicesQuery, windowSec, q.From, q.To); err != nil {
		return domain.Report{}, fmt.Errorf("db read choices: %w", err)
	}

	var funnel []funnelRow
	if err := tx.SelectContext(ctx, &funnel, reportFunnelQuery, windowSec, q.From, q.To); err != nil {
		return domain.Report{}, fmt.Errorf("db read funnel: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Report{}, fmt.Errorf("db commit: %w", err)
	}

	report := domain.Report{
		Series:  make([]domain.SeriesPoint, 0, len(series)),
		Totals:  make([]domain.ElementTotal, 0, len(totals)),
		Choices: make([]domain.ChoiceTotal, 0, len(choices)),
		Funnel:  make([]domain.FunnelTotal, 0, len(funnel)),
	}

	for _, row := range series {
		report.Series = append(report.Series, domain.SeriesPoint{
			WindowStart: row.WindowStart,
			Type:        domain.EventType(row.EventType),
			Events:      row.Events,
		})
	}

	for _, row := range totals {
		report.Totals = append(report.Totals, domain.ElementTotal{
			Type:      domain.EventType(row.EventType),
			ElementID: row.ElementID,
			Events:    row.Events,
		})
	}

	for _, row := range choices {
		report.Choices = append(report.Choices, domain.ChoiceTotal{
			Choice: row.Choice,
			Value:  row.Value,
			Picks:  row.Picks,
		})
	}

	for _, row := range funnel {
		report.Funnel = append(report.Funnel, domain.FunnelTotal{
			Step:     row.Step,
			Sessions: row.Sessions,
		})
	}

	return report, nil
}
