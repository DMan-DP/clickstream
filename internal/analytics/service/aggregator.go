package service

import (
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// counterKey identifies one event counter: a window, an event type
// and a page element.
type counterKey struct {
	windowStart time.Time
	eventType   domain.EventType
	elementID   string
}

// choiceKey identifies one selected value: a window, a choice element
// and the value.
type choiceKey struct {
	windowStart time.Time
	elementID   string
	value       string
}

// Aggregator counts events and selected values by window between
// writes. It is not safe for concurrent use: each partition has its
// own Aggregator in the goroutine of its ConsumeClaim.
type Aggregator struct {
	windowSize time.Duration
	counters   map[counterKey]int64
	choices    map[choiceKey]int64
}

// NewAggregator returns an empty Aggregator with windows of windowSize.
// windowSize must be positive.
func NewAggregator(windowSize time.Duration) *Aggregator {
	a := &Aggregator{windowSize: windowSize}
	a.Reset()
	return a
}

// Add counts e in the window of its Time. A choice event, see IsChoice,
// is also counted in the selected values.
func (a *Aggregator) Add(e domain.Event) {
	windowStart := domain.WindowStart(e.Time, a.windowSize)

	ck := counterKey{
		windowStart: windowStart,
		eventType:   e.Type,
		elementID:   e.ElementID,
	}

	a.counters[ck]++

	if e.IsChoice() {
		chk := choiceKey{
			windowStart: windowStart,
			elementID:   e.ElementID,
			value:       e.Value,
		}

		a.choices[chk]++
	}
}

// Snapshot returns the counts accumulated since the last Reset. It does
// not change the Aggregator: call Reset only after the snapshot is
// saved, so a failed write keeps the counts for the next attempt. Rows
// are in no particular order.
func (a *Aggregator) Snapshot() domain.Snapshot {
	counters := make([]domain.CounterRow, 0, len(a.counters))
	for ck, count := range a.counters {
		row := domain.CounterRow{
			WindowStart: ck.windowStart,
			Type:        ck.eventType,
			ElementID:   ck.elementID,
			Count:       count,
		}
		counters = append(counters, row)
	}

	choices := make([]domain.ChoiceRow, 0, len(a.choices))
	for chk, count := range a.choices {
		row := domain.ChoiceRow{
			WindowStart: chk.windowStart,
			ElementID:   chk.elementID,
			Value:       chk.value,
			Count:       count,
		}
		choices = append(choices, row)
	}

	return domain.Snapshot{
		WindowSize: a.windowSize,
		Counters:   counters,
		Choices:    choices,
	}
}

// Reset drops all counts. Call it after the Snapshot is saved and
// before the next Add: counts added in between are lost.
func (a *Aggregator) Reset() {
	a.counters = make(map[counterKey]int64)
	a.choices = make(map[choiceKey]int64)
}
