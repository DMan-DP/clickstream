package domain

import "time"

// ReportQuery selects the stored counters for a report: windows of length
// WindowSize whose start is in [From, To).
type ReportQuery struct {
	// WindowSize is the window length the counters were written with. Rows
	// of other lengths are not read: they cannot be added together.
	WindowSize time.Duration

	// From is the start of the period, inclusive.
	From time.Time

	// To is the end of the period, exclusive.
	To time.Time
}

// Report is the statistics for a period, read from the stored counters.
// Every count is a sum over all windows of the period.
type Report struct {
	// Series is the number of events per window and type, ordered by
	// window start and type. Windows without events are absent.
	Series []SeriesPoint

	// Totals is the number of events per type and element for the period,
	// ordered by type, then by events descending.
	Totals []ElementTotal

	// Choices is how many times each value of a choice group was picked
	// for the period, ordered by group, then by picks descending.
	Choices []ChoiceTotal

	// Funnel is the number of sessions that reached each step for the
	// period, one entry per step in funnel order, 0 for a step nobody
	// reached.
	Funnel []FunnelTotal
}

// SeriesPoint is the number of events of one type in one window.
type SeriesPoint struct {
	WindowStart time.Time
	Type        EventType
	Events      int64
}

// ElementTotal is the number of events of one type on one element.
type ElementTotal struct {
	Type      EventType
	ElementID string
	Events    int64
}

// ChoiceTotal is how many times a value was picked in a choice group.
// Groups are pizza, size, topping and quantity: sizes and toppings are
// separate page elements joined into one group. For a topping the value
// is its name; unchecking a topping is not a pick.
type ChoiceTotal struct {
	Choice string
	Value  string
	Picks  int64
}

// FunnelTotal is the number of sessions that reached a funnel step. Step
// is the name of the step as FunnelStep.String gives it: visit, pizza,
// contacts or submit.
type FunnelTotal struct {
	Step     string
	Sessions int64
}
