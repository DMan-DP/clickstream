package handler

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"
)

var fixtureSessionID = uuid.MustParse("3ed98b41-6ae1-430e-8a3d-9945cbbe1c29")

func TestDecodeBatch(t *testing.T) {
	fixtureEventsCount := 25

	// first, middle with value, last
	fixtureEvents := []struct {
		idx  int
		want eventRequest
	}{
		{
			idx: 0,
			want: eventRequest{
				ID:        "05d4eec3-a4ce-465e-91fc-562da38b0c45",
				Type:      "page_view",
				ElementID: "page",
				Value:     nil,
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 0, 0, time.UTC),
			},
		},
		{
			idx: 3,
			want: eventRequest{
				ID:        "4ae9b1e7-e0c0-4ec7-9c64-03748264ed4d",
				Type:      "change",
				ElementID: "pizza-select",
				Value:     new("pepperoni"),
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 3, 640_000_000, time.UTC),
			},
		},
		{
			idx: 24,
			want: eventRequest{
				ID:        "06c328fa-970b-49a7-bd43-73ded3f0e901",
				Type:      "submit",
				ElementID: "order-form",
				Value:     nil,
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 14, 525_000_000, time.UTC),
			},
		},
	}

	t.Run("real batch", func(t *testing.T) {
		file, err := os.Open("testdata/batch.json")
		if err != nil {
			t.Fatalf("open file: %v", err)
		}
		defer file.Close()

		req, err := decodeBatch(file)
		if err != nil {
			t.Fatalf("decodeBatch(testdata/batch.json) err = %v, want nil", err)
		}

		if len(req.Events) != fixtureEventsCount {
			t.Fatalf("decodeBatch() len(Events) = %d, want %d", len(req.Events), fixtureEventsCount)
		}

		if req.SessionID != fixtureSessionID {
			t.Errorf("decodeBatch() SessionID = %v, want %v", req.SessionID, fixtureSessionID)
		}

		for _, e := range fixtureEvents {
			got := req.Events[e.idx]
			if got.ID != e.want.ID {
				t.Errorf("decodeBatch() Events[%d].ID = %q, want %q", e.idx, got.ID, e.want.ID)
			}

			if got.Type != e.want.Type {
				t.Errorf("decodeBatch() Events[%d].Type = %q, want %q", e.idx, got.Type, e.want.Type)
			}

			if got.ElementID != e.want.ElementID {
				t.Errorf("decodeBatch() Events[%d].ElementID = %q, want %q", e.idx, got.ElementID, e.want.ElementID)
			}

			if !equalStringPtr(got.Value, e.want.Value) {
				t.Errorf("decodeBatch() Events[%d].Value = %v, want %v", e.idx, formatStringPtr(got.Value), formatStringPtr(e.want.Value))
			}

			if !got.ClientTS.Equal(e.want.ClientTS) {
				t.Errorf("decodeBatch() Events[%d].ClientTS = %v, want %v", e.idx, got.ClientTS, e.want.ClientTS)
			}
		}
	})

	eventsCount := 1

	tests := []struct {
		name          string
		in            string
		wantSessionID uuid.UUID
		wantValue     *string
		wantErr       bool
	}{
		{
			name:          "value absent",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     nil,
			wantErr:       false,
		},
		{
			name:          "value empty string",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "promo-code", "value": "", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     new(""),
			wantErr:       false,
		},
		{
			name:          "value present",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "pizza-select", "value": "pepperoni", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     new("pepperoni"),
			wantErr:       false,
		},
		{
			name:      "bad client ts",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "yesterday"}]}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "client ts number",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": 1793960100000}]}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "broken json",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:          "trailing newline",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}` + "\n",
			wantSessionID: fixtureSessionID,
			wantValue:     nil,
			wantErr:       false,
		},
		{
			name:      "trailing garbage",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}abc`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "two objects",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": []}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "empty body",
			in:        ``,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:          "unknown top level field ignored",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "device": "mobile", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     nil,
			wantErr:       false,
		},
		{
			name:          "unknown event field ignored",
			in:            `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "pizza-select", "value": "pepperoni", "page_x": 120, "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     new("pepperoni"),
			wantErr:       false,
		},
		{
			name:      "session id garbage",
			in:        `{"session_id": "abc", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "session id empty string",
			in:        `{"session_id": "", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:      "session id number",
			in:        `{"session_id": 123, "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: nil,
			wantErr:   true,
		},
		{
			name:          "session id uppercase",
			in:            `{"session_id": "3ED98B41-6AE1-430E-8A3D-9945CBBE1C29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: fixtureSessionID,
			wantValue:     nil,
			wantErr:       false,
		},
		{
			name:          "session id null passes to validate",
			in:            `{"session_id": null, "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: uuid.Nil(),
			wantValue:     nil,
			wantErr:       false,
		},
		{
			name:          "session id absent passes to validate",
			in:            `{"events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantSessionID: uuid.Nil(),
			wantValue:     nil,
			wantErr:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := decodeBatch(strings.NewReader(tc.in))
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("decodeBatch() err = %v, wantErr %t", err, tc.wantErr)
				}

				t.Logf("decodeBatch() err = %v", err)
				return
			}

			if tc.wantErr {
				t.Fatalf("decodeBatch() err = nil, want error")
			}

			if len(req.Events) != eventsCount {
				t.Fatalf("decodeBatch() len(Events) = %d, want %d", len(req.Events), eventsCount)
			}

			value := req.Events[0].Value
			if !equalStringPtr(value, tc.wantValue) {
				t.Fatalf("decodeBatch() Events[0].Value = %v, want %v", formatStringPtr(value), formatStringPtr(tc.wantValue))
			}

			if req.SessionID != tc.wantSessionID {
				t.Errorf("decodeBatch() SessionID = %v, want %v", req.SessionID, tc.wantSessionID)
			}
		})
	}
}

func equalStringPtr(a, b *string) bool {
	if a == b {
		return true
	}
	return a != nil && b != nil && *a == *b
}

func formatStringPtr(s *string) string {
	if s == nil {
		return "nil"
	}

	return strconv.Quote(*s)
}

func TestBatchRequestValidate(t *testing.T) {
	event := []eventRequest{{}}
	tests := []struct {
		name      string
		sessionID uuid.UUID
		events    []eventRequest
		wantErr   error
	}{
		{
			name:      "1 event",
			sessionID: fixtureSessionID,
			events:    slices.Repeat(event, 1),
			wantErr:   nil,
		},
		{
			name:      "max events",
			sessionID: fixtureSessionID,
			events:    slices.Repeat(event, maxBatchSize),
			wantErr:   nil,
		},
		{
			name:      "nil events",
			sessionID: fixtureSessionID,
			events:    nil,
			wantErr:   errNoEvents,
		},
		{
			name:      "empty events",
			sessionID: fixtureSessionID,
			events:    []eventRequest{},
			wantErr:   errNoEvents,
		},
		{
			name:      "too many events",
			sessionID: fixtureSessionID,
			events:    slices.Repeat(event, maxBatchSize+1),
			wantErr:   errTooManyEvents,
		},
		{
			name:      "nil session id",
			sessionID: uuid.Nil(),
			events:    slices.Repeat(event, 1),
			wantErr:   errNoSessionID,
		},
		{
			name:    "session id not set",
			events:  slices.Repeat(event, 1),
			wantErr: errNoSessionID,
		},
		{
			name:      "no events checked before session id",
			sessionID: uuid.Nil(),
			events:    nil,
			wantErr:   errNoEvents,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := batchRequest{
				SessionID: tc.sessionID,
				Events:    tc.events,
			}

			err := validate(req)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("validate() err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidationMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "no events",
			err:  fmt.Errorf("validate batch: %w", errNoEvents),
			want: "No events",
		},
		{
			name: "too many events",
			err:  fmt.Errorf("validate batch: %w: got %d, max %d", errTooManyEvents, maxBatchSize+1, maxBatchSize),
			want: "Too many events, max 100",
		},
		{
			name: "no session id",
			err:  fmt.Errorf("validate batch: %w", errNoSessionID),
			want: "session_id is missing or invalid",
		},
		{
			name: "unwrapped sentinel",
			err:  errNoEvents,
			want: "No events",
		},
		{
			name: "unknown error",
			err:  errors.New("something else"),
			want: "Bad request",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := validationMessage(tc.err)
			if msg != tc.want {
				t.Errorf("validationMessage(%v) = %q, want %q", tc.err, msg, tc.want)
			}
		})
	}
}

func TestEventsServeHTTP(t *testing.T) {
	t.Skip("not implemented yet")
}
