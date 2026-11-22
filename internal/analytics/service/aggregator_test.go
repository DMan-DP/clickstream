package service

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// testSessionTimeout is the funnel session timeout for tests that check
// other things: the default SESSION_TIMEOUT.
const testSessionTimeout = 30 * time.Minute

func TestAggregator(t *testing.T) {
	const size = 30 * time.Second

	// base is the start of a window; other times are built from it.
	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	moscow := time.FixedZone("UTC+3", 3*60*60)

	tests := []struct {
		name         string
		events       []domain.Event
		wantCounters []domain.CounterRow
		wantChoices  []domain.ChoiceRow
	}{
		{
			name: "no events",
		},

		// Windows.
		{
			name: "same event three times in one window",
			events: []domain.Event{
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(1 * time.Second)},
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(10 * time.Second)},
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(29 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 3},
			},
		},
		{
			name: "event exactly on boundary starts next window",
			events: []domain.Event{
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(size - time.Nanosecond)},
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(size)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
				{WindowStart: base.Add(size), Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
			},
		},
		{
			name: "windows far apart",
			events: []domain.Event{
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(5 * time.Second)},
				{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(5*time.Minute + 5*time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
				{WindowStart: base.Add(5 * time.Minute), Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
			},
		},
		{
			name: "time zone other than UTC",
			events: []domain.Event{
				// 15:00:10 UTC+3 is 12:00:10 UTC.
				{Type: domain.EventTypeChange, ElementID: "size-large", Value: "large", Time: time.Date(2026, 11, 21, 15, 0, 10, 0, moscow)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "size-large", Count: 1},
			},
			wantChoices: []domain.ChoiceRow{
				{WindowStart: base, ElementID: "size-large", Value: "large", Count: 1},
			},
		},

		// Counter keys.
		{
			name: "different types on one element",
			events: []domain.Event{
				{Type: domain.EventTypeFocus, ElementID: "customer-name", Time: base.Add(1 * time.Second)},
				{Type: domain.EventTypeInput, ElementID: "customer-name", Value: "5", Time: base.Add(2 * time.Second)},
				{Type: domain.EventTypeBlur, ElementID: "customer-name", Time: base.Add(3 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeFocus, ElementID: "customer-name", Count: 1},
				{WindowStart: base, Type: domain.EventTypeInput, ElementID: "customer-name", Count: 1},
				{WindowStart: base, Type: domain.EventTypeBlur, ElementID: "customer-name", Count: 1},
			},
		},
		{
			name: "same type on different elements",
			events: []domain.Event{
				{Type: domain.EventTypeClick, ElementID: "quantity-plus-button", Time: base.Add(1 * time.Second)},
				{Type: domain.EventTypeClick, ElementID: "quantity-minus-button", Time: base.Add(2 * time.Second)},
				{Type: domain.EventTypeClick, ElementID: "quantity-plus-button", Time: base.Add(3 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "quantity-plus-button", Count: 2},
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "quantity-minus-button", Count: 1},
			},
		},

		// Choices.
		{
			name: "choice is counted in both counters and choices",
			events: []domain.Event{
				{Type: domain.EventTypeChange, ElementID: "pizza-select", Value: "pepperoni", Time: base.Add(1 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "pizza-select", Count: 1},
			},
			wantChoices: []domain.ChoiceRow{
				{WindowStart: base, ElementID: "pizza-select", Value: "pepperoni", Count: 1},
			},
		},
		{
			name: "different values of one element in one window",
			events: []domain.Event{
				{Type: domain.EventTypeChange, ElementID: "pizza-select", Value: "pepperoni", Time: base.Add(1 * time.Second)},
				{Type: domain.EventTypeChange, ElementID: "pizza-select", Value: "margherita", Time: base.Add(2 * time.Second)},
				{Type: domain.EventTypeChange, ElementID: "pizza-select", Value: "pepperoni", Time: base.Add(3 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "pizza-select", Count: 3},
			},
			wantChoices: []domain.ChoiceRow{
				{WindowStart: base, ElementID: "pizza-select", Value: "pepperoni", Count: 2},
				{WindowStart: base, ElementID: "pizza-select", Value: "margherita", Count: 1},
			},
		},
		{
			name: "same value in different windows",
			events: []domain.Event{
				{Type: domain.EventTypeChange, ElementID: "topping-cheese", Value: "true", Time: base.Add(1 * time.Second)},
				{Type: domain.EventTypeChange, ElementID: "topping-cheese", Value: "true", Time: base.Add(size + 1*time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "topping-cheese", Count: 1},
				{WindowStart: base.Add(size), Type: domain.EventTypeChange, ElementID: "topping-cheese", Count: 1},
			},
			wantChoices: []domain.ChoiceRow{
				{WindowStart: base, ElementID: "topping-cheese", Value: "true", Count: 1},
				{WindowStart: base.Add(size), ElementID: "topping-cheese", Value: "true", Count: 1},
			},
		},
		{
			name: "non-choice events are only in counters",
			events: []domain.Event{
				// Wrong type: input, not change.
				{Type: domain.EventTypeInput, ElementID: "quantity-slider", Value: "3", Time: base.Add(1 * time.Second)},
				// Wrong element: a text field.
				{Type: domain.EventTypeChange, ElementID: "customer-name", Value: "5", Time: base.Add(2 * time.Second)},
				// Empty value.
				{Type: domain.EventTypeChange, ElementID: "pizza-select", Time: base.Add(3 * time.Second)},
			},
			wantCounters: []domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeInput, ElementID: "quantity-slider", Count: 1},
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "customer-name", Count: 1},
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "pizza-select", Count: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAggregator(size, testSessionTimeout)
			for _, e := range tc.events {
				a.Add(e)
			}

			s := a.Snapshot()
			if s.WindowSize != size {
				t.Errorf("WindowSize = %v, want %v", s.WindowSize, size)
			}

			checkSnapshot(t, s, tc.wantCounters, tc.wantChoices)
		})
	}
}

// TestAggregatorWindowSize checks that the Aggregator uses the window size
// passed to NewAggregator. The other tests use 30s, the default
// WINDOW_SIZE, so a hard-coded size would pass them.
func TestAggregatorWindowSize(t *testing.T) {
	const size = time.Minute
	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	a := NewAggregator(size, testSessionTimeout)
	// With 30s windows these two events would fall into different windows.
	a.Add(domain.Event{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(10 * time.Second)})
	a.Add(domain.Event{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(45 * time.Second)})

	s := a.Snapshot()
	if s.WindowSize != size {
		t.Errorf("WindowSize = %v, want %v", s.WindowSize, size)
	}
	checkSnapshot(t, s,
		[]domain.CounterRow{
			{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 2},
		},
		nil,
	)
}

// TestAggregatorSnapshotKeepsCounts checks that Snapshot does not reset
// the Aggregator: a second call returns the same rows.
func TestAggregatorSnapshotKeepsCounts(t *testing.T) {
	const size = 30 * time.Second
	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	a := NewAggregator(size, testSessionTimeout)
	a.Add(domain.Event{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(1 * time.Second)})
	a.Add(domain.Event{Type: domain.EventTypeChange, ElementID: "size-large", Value: "large", Time: base.Add(2 * time.Second)})

	wantCounters := []domain.CounterRow{
		{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
		{WindowStart: base, Type: domain.EventTypeChange, ElementID: "size-large", Count: 1},
	}
	wantChoices := []domain.ChoiceRow{
		{WindowStart: base, ElementID: "size-large", Value: "large", Count: 1},
	}

	checkSnapshot(t, a.Snapshot(), wantCounters, wantChoices)
	checkSnapshot(t, a.Snapshot(), wantCounters, wantChoices)
}

// TestAggregatorReset checks that Reset drops all counts and counting
// starts from zero after it.
func TestAggregatorReset(t *testing.T) {
	const size = 30 * time.Second
	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	click := domain.Event{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(1 * time.Second)}

	a := NewAggregator(size, testSessionTimeout)
	a.Add(click)
	a.Add(domain.Event{Type: domain.EventTypeChange, ElementID: "size-large", Value: "large", Time: base.Add(2 * time.Second)})

	a.Reset()
	checkSnapshot(t, a.Snapshot(), nil, nil)

	a.Add(click)
	checkSnapshot(t, a.Snapshot(),
		[]domain.CounterRow{
			{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
		},
		nil,
	)
}

// checkSnapshot compares the rows of s with the wanted rows, ignoring
// their order.
func checkSnapshot(t *testing.T, s domain.Snapshot, wantCounters []domain.CounterRow, wantChoices []domain.ChoiceRow) {
	t.Helper()

	slices.SortFunc(wantCounters, compareCounterRows)
	slices.SortFunc(s.Counters, compareCounterRows)
	if !slices.Equal(wantCounters, s.Counters) {
		t.Errorf("Counters =\n%+v\nwant\n%+v", s.Counters, wantCounters)
	}

	slices.SortFunc(wantChoices, compareChoiceRows)
	slices.SortFunc(s.Choices, compareChoiceRows)
	if !slices.Equal(wantChoices, s.Choices) {
		t.Errorf("Choices =\n%+v\nwant\n%+v", s.Choices, wantChoices)
	}
}

func compareCounterRows(a, b domain.CounterRow) int {
	return cmp.Or(
		a.WindowStart.Compare(b.WindowStart),
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.ElementID, b.ElementID),
	)
}

func compareChoiceRows(a, b domain.ChoiceRow) int {
	return cmp.Or(
		a.WindowStart.Compare(b.WindowStart),
		cmp.Compare(a.ElementID, b.ElementID),
		cmp.Compare(a.Value, b.Value),
	)
}
