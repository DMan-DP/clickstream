package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
	"uuid"
)

// validEvent returns an event that passes validateEvent. Each case
// changes one field of it to check a single rule.
func validEvent() Event {
	return Event{
		ID:        "05d4eec3-a4ce-465e-91fc-562da38b0c45",
		Type:      "click",
		ElementID: "order-button",
		Value:     nil,
		ClientTS:  time.Date(2026, 11, 6, 10, 15, 0, 0, time.UTC),
	}
}

func TestValidateEvent(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(e *Event)
		wantErr error
	}{
		{
			name:    "valid",
			mutate:  func(*Event) {},
			wantErr: nil,
		},
		{
			name:    "valid with value",
			mutate:  func(e *Event) { e.Value = new("pepperoni") },
			wantErr: nil,
		},
		{
			// Value length is not checked here, Ingest clears long values.
			name:    "valid with value longer than 64 runes",
			mutate:  func(e *Event) { e.Value = new(strings.Repeat("я", 65)) },
			wantErr: nil,
		},

		// event_id: all forms accepted by uuid.Parse, any case.
		{
			name:    "event_id upper case",
			mutate:  func(e *Event) { e.ID = "05D4EEC3-A4CE-465E-91FC-562DA38B0C45" },
			wantErr: nil,
		},
		{
			name:    "event_id in braces",
			mutate:  func(e *Event) { e.ID = "{05d4eec3-a4ce-465e-91fc-562da38b0c45}" },
			wantErr: nil,
		},
		{
			name:    "event_id with urn prefix",
			mutate:  func(e *Event) { e.ID = "urn:uuid:05d4eec3-a4ce-465e-91fc-562da38b0c45" },
			wantErr: nil,
		},
		{
			name:    "event_id without hyphens",
			mutate:  func(e *Event) { e.ID = "05d4eec3a4ce465e91fc562da38b0c45" },
			wantErr: nil,
		},
		{
			name:    "event_id empty",
			mutate:  func(e *Event) { e.ID = "" },
			wantErr: errInvalidEventID,
		},
		{
			name:    "event_id malformed",
			mutate:  func(e *Event) { e.ID = "not-a-uuid" },
			wantErr: errInvalidEventID,
		},
		{
			name:    "event_id with trailing garbage",
			mutate:  func(e *Event) { e.ID = "05d4eec3-a4ce-465e-91fc-562da38b0c45x" },
			wantErr: errInvalidEventID,
		},
		{
			name:    "event_id nil uuid",
			mutate:  func(e *Event) { e.ID = "00000000-0000-0000-0000-000000000000" },
			wantErr: errInvalidEventID,
		},

		// type: whitelist, case sensitive.
		{
			name:    "type click",
			mutate:  func(e *Event) { e.Type = "click" },
			wantErr: nil,
		},
		{
			name:    "type input",
			mutate:  func(e *Event) { e.Type = "input" },
			wantErr: nil,
		},
		{
			name:    "type change",
			mutate:  func(e *Event) { e.Type = "change" },
			wantErr: nil,
		},
		{
			name:    "type focus",
			mutate:  func(e *Event) { e.Type = "focus" },
			wantErr: nil,
		},
		{
			name:    "type blur",
			mutate:  func(e *Event) { e.Type = "blur" },
			wantErr: nil,
		},
		{
			name:    "type submit",
			mutate:  func(e *Event) { e.Type = "submit" },
			wantErr: nil,
		},
		{
			name: "type page_view",
			mutate: func(e *Event) {
				e.Type = "page_view"
				e.ElementID = "page"
			},
			wantErr: nil,
		},
		{
			name:    "type unknown",
			mutate:  func(e *Event) { e.Type = "scroll" },
			wantErr: errUnknownType,
		},
		{
			name:    "type wrong case",
			mutate:  func(e *Event) { e.Type = "Click" },
			wantErr: errUnknownType,
		},
		{
			name:    "type empty",
			mutate:  func(e *Event) { e.Type = "" },
			wantErr: errUnknownType,
		},

		// element_id: required, whitespace only counts as empty.
		{
			name:    "element_id empty",
			mutate:  func(e *Event) { e.ElementID = "" },
			wantErr: errEmptyElementID,
		},
		{
			name:    "element_id spaces only",
			mutate:  func(e *Event) { e.ElementID = "   " },
			wantErr: errEmptyElementID,
		},
		{
			name:    "element_id tabs and newlines only",
			mutate:  func(e *Event) { e.ElementID = "\t\n" },
			wantErr: errEmptyElementID,
		},

		// client_ts: required; absent and null decode to the zero time.
		// The range is not checked, any non-zero time in any zone is valid.
		{
			name:    "client_ts zero",
			mutate:  func(e *Event) { e.ClientTS = time.Time{} },
			wantErr: errEmptyClientTS,
		},
		{
			name:    "client_ts explicit zero UTC",
			mutate:  func(e *Event) { e.ClientTS = time.Time{}.UTC() },
			wantErr: errEmptyClientTS,
		},
		{
			name: "client_ts non-UTC zone",
			mutate: func(e *Event) {
				e.ClientTS = time.Date(2026, 11, 6, 13, 15, 0, 0, time.FixedZone("UTC+3", 3*60*60))
			},
			wantErr: nil,
		},

		// Check order: event_id, then type, then element_id, then client_ts.
		{
			name: "invalid event_id and unknown type",
			mutate: func(e *Event) {
				e.ID = "not-a-uuid"
				e.Type = "scroll"
			},
			wantErr: errInvalidEventID,
		},
		{
			name: "unknown type and empty element_id",
			mutate: func(e *Event) {
				e.Type = "scroll"
				e.ElementID = ""
			},
			wantErr: errUnknownType,
		},
		{
			name: "empty element_id and zero client_ts",
			mutate: func(e *Event) {
				e.ElementID = ""
				e.ClientTS = time.Time{}
			},
			wantErr: errEmptyElementID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := validEvent()
			tc.mutate(&e)

			err := validateEvent(e)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("validateEvent() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValueTooLong(t *testing.T) {
	// The limit is 64 runes, not bytes: Cyrillic letters take 2 bytes
	// in UTF-8, most emoji take 4.
	tests := []struct {
		name  string
		value *string
		want  bool
	}{
		{
			name:  "nil",
			value: nil,
			want:  false,
		},
		{
			name:  "empty",
			value: new(""),
			want:  false,
		},
		{
			name:  "short",
			value: new("pepperoni"),
			want:  false,
		},
		{
			name:  "64 latin",
			value: new(strings.Repeat("a", 64)),
			want:  false,
		},
		{
			name:  "65 latin",
			value: new(strings.Repeat("a", 65)),
			want:  true,
		},
		{
			// 128 bytes, a byte count would call it too long.
			name:  "64 cyrillic",
			value: new(strings.Repeat("я", 64)),
			want:  false,
		},
		{
			name:  "65 cyrillic",
			value: new(strings.Repeat("я", 65)),
			want:  true,
		},
		{
			// 66 bytes but 33 runes.
			name:  "33 cyrillic",
			value: new(strings.Repeat("я", 33)),
			want:  false,
		},
		{
			// 256 bytes but 64 runes.
			name:  "64 emoji",
			value: new(strings.Repeat("😀", 64)),
			want:  false,
		},
		{
			name:  "65 emoji",
			value: new(strings.Repeat("😀", 65)),
			want:  true,
		},
		{
			name:  "64 mixed",
			value: new(strings.Repeat("aя😀", 21) + "a"),
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := valueTooLong(tc.value); got != tc.want {
				t.Errorf("valueTooLong() = %v, want %v", got, tc.want)
			}
		})
	}
}

// eventWith returns validEvent changed by mutate.
func eventWith(mutate func(e *Event)) Event {
	e := validEvent()
	mutate(&e)
	return e
}

func TestIngest(t *testing.T) {
	sessionID := uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
	fixedNow := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	invalidID := eventWith(func(e *Event) { e.ID = "not-a-uuid" })
	unknownType := eventWith(func(e *Event) { e.Type = "scroll" })
	emptyElement := eventWith(func(e *Event) { e.ElementID = "" })

	tests := []struct {
		name   string
		events []Event
		want   IngestResult
	}{
		{
			// The handler rejects empty batches, the service must not fail on them.
			name:   "empty batch",
			events: nil,
			want:   IngestResult{Accepted: 0, Skipped: 0},
		},
		{
			name:   "single valid",
			events: []Event{validEvent()},
			want:   IngestResult{Accepted: 1, Skipped: 0},
		},
		{
			name: "all valid",
			events: []Event{
				eventWith(func(e *Event) { e.ID = "11111111-1111-4111-8111-111111111111"; e.Type = "click" }),
				eventWith(func(e *Event) { e.ID = "22222222-2222-4222-8222-222222222222"; e.Type = "focus" }),
				eventWith(func(e *Event) {
					e.ID = "33333333-3333-4333-8333-333333333333"
					e.Type = "page_view"
					e.ElementID = "page"
				}),
			},
			want: IngestResult{Accepted: 3, Skipped: 0},
		},
		{
			name:   "all invalid",
			events: []Event{invalidID, unknownType, emptyElement},
			want:   IngestResult{Accepted: 0, Skipped: 3},
		},
		{
			name: "nil event_id skipped",
			events: []Event{
				eventWith(func(e *Event) { e.ID = "00000000-0000-0000-0000-000000000000" }),
			},
			want: IngestResult{Accepted: 0, Skipped: 1},
		},
		{
			name:   "mixed",
			events: []Event{validEvent(), invalidID, validEvent(), unknownType},
			want:   IngestResult{Accepted: 2, Skipped: 2},
		},
		{
			// A long value is cleared, the event itself is accepted.
			name: "long value accepted",
			events: []Event{
				eventWith(func(e *Event) { e.Value = new(strings.Repeat("я", 65)) }),
			},
			want: IngestResult{Accepted: 1, Skipped: 0},
		},
		{
			// Deduplication is deferred, duplicates within a batch are accepted.
			name:   "duplicate event_id accepted",
			events: []Event{validEvent(), validEvent()},
			want:   IngestResult{Accepted: 2, Skipped: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewEvents(slog.New(slog.DiscardHandler))
			s.now = func() time.Time { return fixedNow }

			batch := Batch{SessionID: sessionID, Events: tc.events}

			got, err := s.Ingest(context.Background(), batch)
			if err != nil {
				t.Fatalf("Ingest() error = %v, want nil", err)
			}

			if got != tc.want {
				t.Errorf("Ingest() = %+v, want %+v", got, tc.want)
			}

			if sum := got.Accepted + got.Skipped; sum != len(tc.events) {
				t.Errorf("Accepted + Skipped = %d, want len(events) = %d", sum, len(tc.events))
			}
		})
	}
}
