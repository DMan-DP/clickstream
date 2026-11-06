package handler

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDecodeBatch(t *testing.T) {
	fixtureEventsCount := 25
	fixtureSessionID := "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29"

	// first, middle with value, last
	fixtureEvents := []struct {
		idx  int
		want eventRequest
	}{
		{
			idx: 0,
			want: eventRequest{
				EventID:   "05d4eec3-a4ce-465e-91fc-562da38b0c45",
				EventType: "page_view",
				ElementID: "page",
				Value:     nil,
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 0, 0, time.UTC),
			},
		},
		{
			idx: 3,
			want: eventRequest{
				EventID:   "4ae9b1e7-e0c0-4ec7-9c64-03748264ed4d",
				EventType: "change",
				ElementID: "pizza-select",
				Value:     new("pepperoni"),
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 3, 640_000_000, time.UTC),
			},
		},
		{
			idx: 24,
			want: eventRequest{
				EventID:   "06c328fa-970b-49a7-bd43-73ded3f0e901",
				EventType: "submit",
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
			t.Fatalf("decodeBatch() got %d events, want %d", len(req.Events), fixtureEventsCount)
		}

		if req.SessionID != fixtureSessionID {
			t.Errorf("decodeBatch() SessionID got %q, want %q", req.SessionID, fixtureSessionID)
		}

		for _, e := range fixtureEvents {
			got := req.Events[e.idx]
			if got.EventID != e.want.EventID {
				t.Errorf("decodeBatch() Events[%d].EventID got %q, want %q", e.idx, got.EventID, e.want.EventID)
			}

			if got.EventType != e.want.EventType {
				t.Errorf("decodeBatch() Events[%d].EventType got %q, want %q", e.idx, got.EventType, e.want.EventType)
			}

			if got.ElementID != e.want.ElementID {
				t.Errorf("decodeBatch() Events[%d].ElementID got %q, want %q", e.idx, got.ElementID, e.want.ElementID)
			}

			if !equalStringPtr(got.Value, e.want.Value) {
				t.Errorf("decodeBatch() Events[%d].Value got %v, want %v", e.idx, formatStringPtr(got.Value), formatStringPtr(e.want.Value))
			}

			if !got.ClientTS.Equal(e.want.ClientTS) {
				t.Errorf("decodeBatch() Events[%d].ClientTS got %v, want %v", e.idx, got.ClientTS, e.want.ClientTS)
			}
		}
	})

	eventsCount := 1

	tests := []struct {
		name      string
		in        string
		wantValue *string
		wantErr   bool
	}{
		{
			name:      "value absent",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: nil,
			wantErr:   false,
		},
		{
			name:      "value empty string",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "promo-code", "value": "", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: new(""),
			wantErr:   false,
		},
		{
			name:      "value present",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "pizza-select", "value": "pepperoni", "client_ts": "2026-11-06T10:15:00.000Z"}]}`,
			wantValue: new("pepperoni"),
			wantErr:   false,
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
			name:      "trailing newline",
			in:        `{"session_id": "3ed98b41-6ae1-430e-8a3d-9945cbbe1c29", "events": [{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "click", "element_id": "size-large", "client_ts": "2026-11-06T10:15:00.000Z"}]}` + "\n",
			wantValue: nil,
			wantErr:   false,
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := decodeBatch(strings.NewReader(tc.in))
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("decodeBatch() err = %v, wantErr %t", err, tc.wantErr)
				}

				t.Logf("decodeBatch() got err: %v", err)
				return
			}

			if tc.wantErr {
				t.Fatalf("decodeBatch() expected err, got nil")
			}

			if len(req.Events) != eventsCount {
				t.Fatalf("decodeBatch() got %d events, want %d", len(req.Events), eventsCount)
			}

			value := req.Events[0].Value
			if !equalStringPtr(value, tc.wantValue) {
				t.Fatalf("decodeBatch() event got value: %v, want: %v", formatStringPtr(value), formatStringPtr(tc.wantValue))
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

func TestEventsServeHTTP(t *testing.T) {

}
