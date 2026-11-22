package domain

import "time"

// CounterRow is the number of events of one type on one element in one window.
type CounterRow struct {
	// WindowStart is the start of the window, see WindowStart.
	WindowStart time.Time

	// Type is the kind of user action.
	Type EventType

	// ElementID is the id of the page element, or "page" for page_view.
	ElementID string

	// Count is the number of events with this key in the window.
	Count int64
}

// ChoiceRow is the number of times one value was selected
// on one element in one window.
type ChoiceRow struct {
	// WindowStart is the start of the window, see WindowStart.
	WindowStart time.Time

	// ElementID is the id of the choice element, see IsChoice.
	ElementID string

	// Value is the selected value.
	Value string

	// Count is the number of times the value was selected in the window.
	Count int64
}

// FunnelRow is the number of sessions that reached one funnel step
// for the first time in one window.
type FunnelRow struct {
	// WindowStart is the start of the window, see WindowStart.
	WindowStart time.Time

	// Step is the funnel step, never FunnelStepNone.
	Step FunnelStep

	// Sessions is the number of sessions that reached the step in the
	// window. A session is counted once per step, in the window of the
	// event that took it there.
	Sessions int64
}

// Snapshot is the state of the aggregator since the previous write.
// A window may be partial: the rest of it comes in the next snapshot,
// so rows with the same key must be added up on write.
type Snapshot struct {
	// WindowSize is the length of every window in the snapshot.
	WindowSize time.Duration

	// Counters are the event counters, in no particular order.
	Counters []CounterRow

	// Choices are the selected values, in no particular order.
	Choices []ChoiceRow

	// Funnel are the funnel step counters, in no particular order.
	Funnel []FunnelRow
}
