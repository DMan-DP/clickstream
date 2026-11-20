package domain

// choiceElements are the page elements whose values are counted as popular
// choices. Text fields are not here: their value is only the input length.
var choiceElements = map[string]struct{}{
	// select: the pizza name.
	"pizza-select": {},

	// radio: the size, one element per option.
	"size-small":  {},
	"size-medium": {},
	"size-large":  {},

	// checkbox: "true" or "false".
	"topping-cheese":    {},
	"topping-mushrooms": {},
	"topping-jalapeno":  {},

	// range: the quantity, also set by the -/+ buttons.
	"quantity-slider": {},
}

// IsChoice reports whether e is a choice of a value on the page: a change
// event on an element from choiceElements with a non-empty value.
func (e Event) IsChoice() bool {
	if e.Type != EventTypeChange || e.Value == "" {
		return false
	}
	_, ok := choiceElements[e.ElementID]
	return ok
}
