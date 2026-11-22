package domain

import (
	"fmt"
	"strconv"
)

// FunnelSignal is what a single event tells about the order funnel. It is
// not a funnel step: the aggregator keeps the session state and decides
// from the signals which steps the session has reached. The zero value,
// FunnelSignalNone, means the event does not matter for the funnel.
type FunnelSignal int

// Funnel signals. Text fields send the input length as Value, so a contact
// field is filled when the length is above zero.
//
//   - FunnelSignalVisit: page_view.
//   - FunnelSignalPizza: change of the pizza, size, topping or quantity
//     element, or click on a quantity button.
//   - FunnelSignalNameFilled, FunnelSignalNameCleared: input or change of
//     customer-name with the length above zero or zero.
//   - FunnelSignalAddressFilled, FunnelSignalAddressCleared: the same for
//     delivery-address.
//   - FunnelSignalSubmit: submit of order-form. It is an attempt: the form
//     may have failed the check in the browser.
const (
	FunnelSignalNone FunnelSignal = iota
	FunnelSignalVisit
	FunnelSignalPizza
	FunnelSignalNameFilled
	FunnelSignalNameCleared
	FunnelSignalAddressFilled
	FunnelSignalAddressCleared
	FunnelSignalSubmit
)

// FunnelStep is how far a session has gone in the order funnel. Steps are
// ordered: reaching a step means all steps before it are reached too, so
// the furthest step alone describes the session, and "step k is reached"
// is step >= k. The zero value, FunnelStepNone, means no step is reached
// yet; it is session state only and is never counted.
type FunnelStep int

// Funnel steps in funnel order. The order of the constants matters: the
// aggregator compares steps with < and >=.
//
//   - FunnelStepVisit: the session is seen on the page.
//   - FunnelStepPizza: the session has built a pizza.
//   - FunnelStepContacts: the name and the address are both filled.
//   - FunnelStepSubmit: the order form is submitted after FunnelStepContacts.
const (
	FunnelStepNone FunnelStep = iota
	FunnelStepVisit
	FunnelStepPizza
	FunnelStepContacts
	FunnelStepSubmit
)

// pizzaElements are the elements of the pizza builder: a change of any of
// them gives FunnelSignalPizza. The list is the same as choiceElements
// today but answers another question, so a new popular choice must not
// change the funnel by accident.
var pizzaElements = map[string]struct{}{
	"pizza-select":      {},
	"size-small":        {},
	"size-medium":       {},
	"size-large":        {},
	"topping-cheese":    {},
	"topping-mushrooms": {},
	"topping-jalapeno":  {},
	"quantity-slider":   {},
}

// FunnelSignal reports what e tells about the order funnel, see the
// FunnelSignal constants for the rules. A contact field event whose Value
// is not a non-negative whole number gives FunnelSignalNone: the field
// state is unknown, so the session state must not change.
func (e Event) FunnelSignal() FunnelSignal {
	switch e.Type {
	case EventTypePageView:
		return FunnelSignalVisit
	case EventTypeChange, EventTypeInput:
		if e.Type == EventTypeChange {
			if _, ok := pizzaElements[e.ElementID]; ok {
				return FunnelSignalPizza
			}
		}

		switch e.ElementID {
		case "customer-name":
			v, err := strconv.Atoi(e.Value)
			if err != nil || v < 0 {
				return FunnelSignalNone
			}
			if v > 0 {
				return FunnelSignalNameFilled
			}
			return FunnelSignalNameCleared
		case "delivery-address":
			v, err := strconv.Atoi(e.Value)
			if err != nil || v < 0 {
				return FunnelSignalNone
			}
			if v > 0 {
				return FunnelSignalAddressFilled
			}
			return FunnelSignalAddressCleared
		}
	case EventTypeClick:
		if e.ElementID == "quantity-minus-button" || e.ElementID == "quantity-plus-button" {
			return FunnelSignalPizza
		}
	case EventTypeSubmit:
		if e.ElementID == "order-form" {
			return FunnelSignalSubmit
		}
	}

	return FunnelSignalNone
}

// String returns the signal name for logs and test output. Unknown values
// are printed as FunnelSignal(n).
func (s FunnelSignal) String() string {
	switch s {
	case FunnelSignalNone:
		return "none"
	case FunnelSignalVisit:
		return "visit"
	case FunnelSignalPizza:
		return "pizza"
	case FunnelSignalNameFilled:
		return "name filled"
	case FunnelSignalNameCleared:
		return "name cleared"
	case FunnelSignalAddressFilled:
		return "address filled"
	case FunnelSignalAddressCleared:
		return "address cleared"
	case FunnelSignalSubmit:
		return "submit"
	default:
		return fmt.Sprintf("FunnelSignal(%d)", int(s))
	}
}

// String returns the step name. The names are stored in the database as
// funnel_stats.step, so they must never change: a rename splits the
// history of a step in two. Unknown values are printed as FunnelStep(n).
func (s FunnelStep) String() string {
	switch s {
	case FunnelStepNone:
		return "none"
	case FunnelStepVisit:
		return "visit"
	case FunnelStepPizza:
		return "pizza"
	case FunnelStepContacts:
		return "contacts"
	case FunnelStepSubmit:
		return "submit"
	default:
		return fmt.Sprintf("FunnelStep(%d)", int(s))
	}
}
