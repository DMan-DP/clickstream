package service

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

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
			a := NewAggregator(size)
			for _, e := range tc.events {
				a.Add(e)
			}

			s := a.Snapshot()
			if s.WindowSize != size {
				t.Errorf("WindowSize = %v, want %v", s.WindowSize, size)
			}

			slices.SortFunc(tc.wantCounters, compareCounterRows)
			slices.SortFunc(s.Counters, compareCounterRows)
			if !slices.Equal(tc.wantCounters, s.Counters) {
				t.Errorf("Counters =\n%+v\nwant\n%+v", s.Counters, tc.wantCounters)
			}

			slices.SortFunc(tc.wantChoices, compareChoiceRows)
			slices.SortFunc(s.Choices, compareChoiceRows)
			if !slices.Equal(tc.wantChoices, s.Choices) {
				t.Errorf("Choices =\n%+v\nwant\n%+v", s.Choices, tc.wantChoices)
			}

			if len(a.counters) != 0 || a.counters == nil {
				t.Errorf("after Snapshot counters = %+v, want empty non-nil map", a.counters)
			}

			if len(a.choices) != 0 || a.choices == nil {
				t.Errorf("after Snapshot choices = %+v, want empty non-nil map", a.choices)
			}
		})
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
