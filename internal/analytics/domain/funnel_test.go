package domain

import (
	"fmt"
	"testing"
)

func TestFunnelSignal(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  FunnelSignal
	}{
		// Visit: any page_view.
		{
			name:  "page view",
			event: Event{Type: EventTypePageView, ElementID: "page"},
			want:  FunnelSignalVisit,
		},
		{
			// The rule is any page_view, the element is not checked.
			name:  "page view with another element",
			event: Event{Type: EventTypePageView, ElementID: "order-form"},
			want:  FunnelSignalVisit,
		},

		// Pizza: change of every builder element, the value does not matter.
		{
			name:  "change on pizza select",
			event: Event{Type: EventTypeChange, ElementID: "pizza-select", Value: "pepperoni"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on size small",
			event: Event{Type: EventTypeChange, ElementID: "size-small", Value: "small"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on size medium",
			event: Event{Type: EventTypeChange, ElementID: "size-medium", Value: "medium"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on size large",
			event: Event{Type: EventTypeChange, ElementID: "size-large", Value: "large"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on topping cheese",
			event: Event{Type: EventTypeChange, ElementID: "topping-cheese", Value: "true"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on topping mushrooms",
			event: Event{Type: EventTypeChange, ElementID: "topping-mushrooms", Value: "true"},
			want:  FunnelSignalPizza,
		},
		{
			// Unchecking a topping is an action in the builder too.
			name:  "change on topping jalapeno unchecked",
			event: Event{Type: EventTypeChange, ElementID: "topping-jalapeno", Value: "false"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "change on quantity slider",
			event: Event{Type: EventTypeChange, ElementID: "quantity-slider", Value: "3"},
			want:  FunnelSignalPizza,
		},
		{
			// Unlike IsChoice, the funnel does not need a value.
			name:  "change on builder element with empty value",
			event: Event{Type: EventTypeChange, ElementID: "pizza-select", Value: ""},
			want:  FunnelSignalPizza,
		},

		// Pizza: click on a quantity button.
		{
			name:  "click on quantity minus",
			event: Event{Type: EventTypeClick, ElementID: "quantity-minus-button"},
			want:  FunnelSignalPizza,
		},
		{
			name:  "click on quantity plus",
			event: Event{Type: EventTypeClick, ElementID: "quantity-plus-button"},
			want:  FunnelSignalPizza,
		},

		// Contact fields: input and change, length above zero or zero.
		{
			name:  "input on name filled",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "5"},
			want:  FunnelSignalNameFilled,
		},
		{
			name:  "change on name filled",
			event: Event{Type: EventTypeChange, ElementID: "customer-name", Value: "5"},
			want:  FunnelSignalNameFilled,
		},
		{
			// The smallest filled length: catches ">= 1" written as "> 1".
			name:  "input on name length one",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "1"},
			want:  FunnelSignalNameFilled,
		},
		{
			name:  "input on name cleared",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "0"},
			want:  FunnelSignalNameCleared,
		},
		{
			name:  "change on name cleared",
			event: Event{Type: EventTypeChange, ElementID: "customer-name", Value: "0"},
			want:  FunnelSignalNameCleared,
		},
		{
			name:  "input on address filled",
			event: Event{Type: EventTypeInput, ElementID: "delivery-address", Value: "12"},
			want:  FunnelSignalAddressFilled,
		},
		{
			name:  "change on address filled",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: "12"},
			want:  FunnelSignalAddressFilled,
		},
		{
			name:  "input on address length one",
			event: Event{Type: EventTypeInput, ElementID: "delivery-address", Value: "1"},
			want:  FunnelSignalAddressFilled,
		},
		{
			name:  "input on address cleared",
			event: Event{Type: EventTypeInput, ElementID: "delivery-address", Value: "0"},
			want:  FunnelSignalAddressCleared,
		},
		{
			name:  "change on address cleared",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: "0"},
			want:  FunnelSignalAddressCleared,
		},

		// Contact fields: Value is not a non-negative number, the state is
		// unknown. Both fields, because each has its own branch.
		{
			name:  "name with empty value",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: ""},
			want:  FunnelSignalNone,
		},
		{
			name:  "name with text value",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "abc"},
			want:  FunnelSignalNone,
		},
		{
			name:  "name with negative value",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "-3"},
			want:  FunnelSignalNone,
		},
		{
			// A length is whole; a fraction is not a valid length.
			name:  "name with fractional value",
			event: Event{Type: EventTypeInput, ElementID: "customer-name", Value: "3.5"},
			want:  FunnelSignalNone,
		},
		{
			name:  "address with empty value",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: ""},
			want:  FunnelSignalNone,
		},
		{
			name:  "address with text value",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: "abc"},
			want:  FunnelSignalNone,
		},
		{
			name:  "address with negative value",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: "-3"},
			want:  FunnelSignalNone,
		},
		{
			name:  "address with fractional value",
			event: Event{Type: EventTypeChange, ElementID: "delivery-address", Value: "3.5"},
			want:  FunnelSignalNone,
		},

		// Submit: submit of order-form.
		{
			name:  "submit of order form",
			event: Event{Type: EventTypeSubmit, ElementID: "order-form"},
			want:  FunnelSignalSubmit,
		},

		// Only the type is wrong.
		{
			// The slider sends input while dragging; only change counts.
			name:  "input on quantity slider",
			event: Event{Type: EventTypeInput, ElementID: "quantity-slider", Value: "3"},
			want:  FunnelSignalNone,
		},
		{
			// Not sent by the page today, but the rule is change only.
			name:  "input on pizza select",
			event: Event{Type: EventTypeInput, ElementID: "pizza-select", Value: "pepperoni"},
			want:  FunnelSignalNone,
		},
		{
			name:  "click on size radio",
			event: Event{Type: EventTypeClick, ElementID: "size-large"},
			want:  FunnelSignalNone,
		},
		{
			name:  "change on quantity button",
			event: Event{Type: EventTypeChange, ElementID: "quantity-plus-button"},
			want:  FunnelSignalNone,
		},
		{
			name:  "focus on name",
			event: Event{Type: EventTypeFocus, ElementID: "customer-name", Value: "5"},
			want:  FunnelSignalNone,
		},
		{
			name:  "blur on address",
			event: Event{Type: EventTypeBlur, ElementID: "delivery-address", Value: "12"},
			want:  FunnelSignalNone,
		},
		{
			name:  "click on order form",
			event: Event{Type: EventTypeClick, ElementID: "order-form"},
			want:  FunnelSignalNone,
		},
		{
			name:  "unknown type on page",
			event: Event{Type: "scroll", ElementID: "page"},
			want:  FunnelSignalNone,
		},

		// Only the element is wrong.
		{
			name:  "input on phone",
			event: Event{Type: EventTypeInput, ElementID: "customer-phone", Value: "5"},
			want:  FunnelSignalNone,
		},
		{
			name:  "change on comment",
			event: Event{Type: EventTypeChange, ElementID: "order-comment", Value: "5"},
			want:  FunnelSignalNone,
		},
		{
			name:  "input on promo code",
			event: Event{Type: EventTypeInput, ElementID: "promo-code", Value: "5"},
			want:  FunnelSignalNone,
		},
		{
			name:  "change on unknown builder-like element",
			event: Event{Type: EventTypeChange, ElementID: "size-xl", Value: "xl"},
			want:  FunnelSignalNone,
		},
		{
			name:  "click on promo apply button",
			event: Event{Type: EventTypeClick, ElementID: "promo-apply-button"},
			want:  FunnelSignalNone,
		},
		{
			name:  "click on reset button",
			event: Event{Type: EventTypeClick, ElementID: "reset-button"},
			want:  FunnelSignalNone,
		},
		{
			// A click on the order button is not a submit: the form sends
			// its own submit event.
			name:  "click on submit order button",
			event: Event{Type: EventTypeClick, ElementID: "submit-order-button"},
			want:  FunnelSignalNone,
		},
		{
			name:  "submit of order button",
			event: Event{Type: EventTypeSubmit, ElementID: "submit-order-button"},
			want:  FunnelSignalNone,
		},
		{
			name:  "submit of another form",
			event: Event{Type: EventTypeSubmit, ElementID: "promo-form"},
			want:  FunnelSignalNone,
		},

		// Nothing is set.
		{
			name:  "zero event",
			event: Event{},
			want:  FunnelSignalNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.event.FunnelSignal(); got != tc.want {
				t.Errorf("FunnelSignal(type=%s element=%s value=%q) = %v, want %v",
					tc.event.Type, tc.event.ElementID, tc.event.Value, got, tc.want)
			}
		})
	}
}

// TestFunnelSignalString checks the String contract without fixing the
// exact names: every known signal has its own name that does not look like
// an unknown value, and unknown values are printed as FunnelSignal(n).
func TestFunnelSignalString(t *testing.T) {
	known := []FunnelSignal{
		FunnelSignalNone,
		FunnelSignalVisit,
		FunnelSignalPizza,
		FunnelSignalNameFilled,
		FunnelSignalNameCleared,
		FunnelSignalAddressFilled,
		FunnelSignalAddressCleared,
		FunnelSignalSubmit,
	}

	seen := make(map[string]FunnelSignal, len(known))
	for _, s := range known {
		name := s.String()
		if name == "" {
			t.Errorf("FunnelSignal(%d).String() is empty", int(s))
		}
		if unknown := fmt.Sprintf("FunnelSignal(%d)", int(s)); name == unknown {
			t.Errorf("FunnelSignal(%d).String() = %q, the form for unknown values", int(s), name)
		}
		if prev, ok := seen[name]; ok {
			t.Errorf("FunnelSignal(%d) and FunnelSignal(%d) have the same name %q", int(prev), int(s), name)
		}
		seen[name] = s
	}

	unknown := []FunnelSignal{-1, FunnelSignalSubmit + 1, 42}
	for _, s := range unknown {
		want := fmt.Sprintf("FunnelSignal(%d)", int(s))
		if got := s.String(); got != want {
			t.Errorf("FunnelSignal(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}

// TestFunnelStepString checks the String contract. The names of the
// counted steps are stored in the database, so they are fixed exactly.
// FunnelStepNone is never counted, so only its name being unique is
// checked. Unknown values are printed as FunnelStep(n).
func TestFunnelStepString(t *testing.T) {
	stored := []struct {
		step FunnelStep
		want string
	}{
		{FunnelStepVisit, "visit"},
		{FunnelStepPizza, "pizza"},
		{FunnelStepContacts, "contacts"},
		{FunnelStepSubmit, "submit"},
	}

	for _, tt := range stored {
		if got := tt.step.String(); got != tt.want {
			t.Errorf("FunnelStep(%d).String() = %q, want %q", int(tt.step), got, tt.want)
		}
	}

	none := FunnelStepNone.String()
	if none == "" {
		t.Errorf("FunnelStepNone.String() is empty")
	}
	if unknown := fmt.Sprintf("FunnelStep(%d)", int(FunnelStepNone)); none == unknown {
		t.Errorf("FunnelStepNone.String() = %q, the form for unknown values", none)
	}
	for _, tt := range stored {
		if none == tt.want {
			t.Errorf("FunnelStepNone.String() = %q, the same as FunnelStep(%d)", none, int(tt.step))
		}
	}

	unknown := []FunnelStep{-1, FunnelStepSubmit + 1, 42}
	for _, s := range unknown {
		want := fmt.Sprintf("FunnelStep(%d)", int(s))
		if got := s.String(); got != want {
			t.Errorf("FunnelStep(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}
