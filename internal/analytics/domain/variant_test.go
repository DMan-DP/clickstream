package domain

import "testing"

func TestIsChoice(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		// All three conditions hold: one case per element kind.
		{
			name:  "change on select",
			event: Event{Type: EventTypeChange, ElementID: "pizza-select", Value: "pepperoni"},
			want:  true,
		},
		{
			name:  "change on radio",
			event: Event{Type: EventTypeChange, ElementID: "size-large", Value: "large"},
			want:  true,
		},
		{
			name:  "change on checkbox checked",
			event: Event{Type: EventTypeChange, ElementID: "topping-cheese", Value: "true"},
			want:  true,
		},
		{
			name:  "change on checkbox unchecked",
			event: Event{Type: EventTypeChange, ElementID: "topping-jalapeno", Value: "false"},
			want:  true,
		},
		{
			name:  "change on slider",
			event: Event{Type: EventTypeChange, ElementID: "quantity-slider", Value: "3"},
			want:  true,
		},

		// Only the type is wrong.
		{
			name:  "input on slider",
			event: Event{Type: EventTypeInput, ElementID: "quantity-slider", Value: "3"},
			want:  false,
		},
		{
			name:  "click on radio",
			event: Event{Type: EventTypeClick, ElementID: "size-small", Value: "small"},
			want:  false,
		},
		{
			name:  "focus on select",
			event: Event{Type: EventTypeFocus, ElementID: "pizza-select", Value: "margherita"},
			want:  false,
		},

		// Only the element is wrong.
		{
			name:  "change on text field",
			event: Event{Type: EventTypeChange, ElementID: "customer-name", Value: "5"},
			want:  false,
		},
		{
			name:  "change on unknown element",
			event: Event{Type: EventTypeChange, ElementID: "unknown-element", Value: "1"},
			want:  false,
		},
		{
			name:  "element id differs in case",
			event: Event{Type: EventTypeChange, ElementID: "Pizza-Select", Value: "pepperoni"},
			want:  false,
		},
		{
			name:  "empty element id",
			event: Event{Type: EventTypeChange, ElementID: "", Value: "pepperoni"},
			want:  false,
		},

		// Only the value is wrong.
		{
			name:  "change on select with empty value",
			event: Event{Type: EventTypeChange, ElementID: "pizza-select", Value: ""},
			want:  false,
		},
		{
			name:  "change on checkbox with empty value",
			event: Event{Type: EventTypeChange, ElementID: "topping-cheese", Value: ""},
			want:  false,
		},

		// Several conditions are wrong.
		{
			name:  "click on quantity button",
			event: Event{Type: EventTypeClick, ElementID: "quantity-plus-button"},
			want:  false,
		},
		{
			name:  "page view",
			event: Event{Type: EventTypePageView, ElementID: "page"},
			want:  false,
		},
		{
			name:  "zero event",
			event: Event{},
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if isChoice := tc.event.IsChoice(); isChoice != tc.want {
				t.Errorf("IsChoice(%q) = %v, want %v", tc.event, isChoice, tc.want)
			}
		})
	}
}
