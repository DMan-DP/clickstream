package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/google/go-cmp/cmp"

	"github.com/DMan-DP/clickstream/internal/httpx"
	"github.com/DMan-DP/clickstream/internal/tracker/service"
)

var fixtureSessionID = uuid.MustParse("3ed98b41-6ae1-430e-8a3d-9945cbbe1c29")

// Limits of the request contract: up to 100 events and a body up to
// 64 KiB. Tests use these and not maxBatchSize / maxBodyBytes, so a wrong
// constant in the handler fails the tests.
const (
	contractMaxBatch = 100
	contractMaxBody  = 64 << 10
)

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
			if diff := cmp.Diff(e.want, req.Events[e.idx]); diff != "" {
				t.Errorf("decodeBatch() Events[%d] mismatch (-want +got):\n%s", e.idx, diff)
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

			if diff := cmp.Diff(tc.wantValue, req.Events[0].Value); diff != "" {
				t.Fatalf("decodeBatch() Events[0].Value mismatch (-want +got):\n%s", diff)
			}

			if req.SessionID != tc.wantSessionID {
				t.Errorf("decodeBatch() SessionID = %v, want %v", req.SessionID, tc.wantSessionID)
			}
		})
	}
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
			events:    slices.Repeat(event, contractMaxBatch),
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
			events:    slices.Repeat(event, contractMaxBatch+1),
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

// stubIngester returns a preset result and remembers the ctx and the batch
// it got.
type stubIngester struct {
	result service.IngestResult
	err    error
	called bool
	ctx    context.Context
	got    service.Batch
}

func (s *stubIngester) Ingest(ctx context.Context, batch service.Batch) (service.IngestResult, error) {
	s.called = true
	s.ctx = ctx
	s.got = batch
	return s.result, s.err
}

// padTo appends spaces to body up to n bytes. JSON allows whitespace
// after the object, so the body stays valid.
func padTo(body string, n int) string {
	return body + strings.Repeat(" ", n-len(body))
}

// batchJSON returns a request body with n valid events.
func batchJSON(sessionID string, n int) string {
	events := make([]string, 0, n)
	for i := range n {
		events = append(events, fmt.Sprintf(
			`{"event_id":"00000000-0000-4000-8000-%012d","type":"click","element_id":"btn","client_ts":"2026-11-06T10:15:00Z"}`,
			i+1,
		))
	}

	return fmt.Sprintf(`{"session_id":%q,"events":[%s]}`, sessionID, strings.Join(events, ","))
}

func TestEventsServeHTTP(t *testing.T) {
	validBody := batchJSON(fixtureSessionID.String(), 3)

	tests := []struct {
		name         string
		body         string
		ingestResult service.IngestResult
		ingestErr    error

		wantStatus int
		wantCalled bool
		// Exactly one of the three is checked: a 202 body, an error body
		// or no body at all. wantResponse is the whole JSON object as
		// decoded into a map, so the field names are checked too.
		wantResponse map[string]any
		// wantError is the exact error text; empty means any non-empty
		// text. The texts are fixed only for request-level checks, see
		// validationMessage.
		wantError     string
		wantEmptyBody bool
	}{
		// Success.
		{
			name:         "valid batch",
			body:         validBody,
			ingestResult: service.IngestResult{Accepted: 2, Skipped: 1},
			wantStatus:   http.StatusAccepted,
			wantCalled:   true,
			wantResponse: map[string]any{"accepted": 2.0, "skipped": 1.0},
		},
		{
			name:         "all events skipped",
			body:         validBody,
			ingestResult: service.IngestResult{Accepted: 0, Skipped: 3},
			wantStatus:   http.StatusAccepted,
			wantCalled:   true,
			wantResponse: map[string]any{"accepted": 0.0, "skipped": 3.0},
		},
		{
			name:         "max batch size",
			body:         batchJSON(fixtureSessionID.String(), contractMaxBatch),
			ingestResult: service.IngestResult{Accepted: contractMaxBatch},
			wantStatus:   http.StatusAccepted,
			wantCalled:   true,
			wantResponse: map[string]any{"accepted": float64(contractMaxBatch), "skipped": 0.0},
		},
		{
			name:         "body exactly at limit",
			body:         padTo(validBody, contractMaxBody),
			ingestResult: service.IngestResult{Accepted: 3},
			wantStatus:   http.StatusAccepted,
			wantCalled:   true,
			wantResponse: map[string]any{"accepted": 3.0, "skipped": 0.0},
		},

		// Request level: the service is not called.
		{
			name:       "malformed JSON",
			body:       `{"session_id":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "trailing data after object",
			body:       validBody + `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Also checks that decodeBatch keeps *http.MaxBytesError in the chain.
			name:       "body one byte over limit",
			body:       padTo(validBody, contractMaxBody+1),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "session_id malformed",
			body:       batchJSON("not-a-uuid", 1),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "session_id nil",
			body:       batchJSON("00000000-0000-0000-0000-000000000000", 1),
			wantStatus: http.StatusBadRequest,
			wantError:  "session_id is missing or invalid",
		},
		{
			name:       "session_id missing",
			body:       `{"events":[{"event_id":"00000000-0000-4000-8000-000000000001","type":"click","element_id":"btn","client_ts":"2026-11-06T10:15:00Z"}]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "session_id is missing or invalid",
		},
		{
			name:       "no events",
			body:       batchJSON(fixtureSessionID.String(), 0),
			wantStatus: http.StatusBadRequest,
			wantError:  "No events",
		},
		{
			name:       "events missing",
			body:       fmt.Sprintf(`{"session_id":%q}`, fixtureSessionID.String()),
			wantStatus: http.StatusBadRequest,
			wantError:  "No events",
		},
		{
			name:       "too many events",
			body:       batchJSON(fixtureSessionID.String(), contractMaxBatch+1),
			wantStatus: http.StatusBadRequest,
			wantError:  fmt.Sprintf("Too many events, max %d", contractMaxBatch),
		},

		// Service errors.
		{
			name:       "ingest deadline exceeded",
			body:       validBody,
			ingestErr:  context.DeadlineExceeded,
			wantStatus: http.StatusServiceUnavailable,
			wantCalled: true,
		},
		{
			// errors.Is must see through wrapping.
			name:       "ingest deadline exceeded wrapped",
			body:       validBody,
			ingestErr:  fmt.Errorf("publish batch: %w", context.DeadlineExceeded),
			wantStatus: http.StatusServiceUnavailable,
			wantCalled: true,
		},
		{
			name:       "ingest unexpected error",
			body:       validBody,
			ingestErr:  errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			// The client is gone: only the status is written, without a body,
			// so access logs see 499 instead of an implicit 200.
			name:          "ingest client gone",
			body:          validBody,
			ingestErr:     fmt.Errorf("publish batch: %w", context.Canceled),
			wantStatus:    httpx.StatusClientClosedRequest,
			wantCalled:    true,
			wantEmptyBody: true,
		},
		{
			name:       "ingest unavailable",
			body:       validBody,
			ingestErr:  fmt.Errorf("publish batch: %w", service.ErrUnavailable),
			wantStatus: http.StatusServiceUnavailable,
			wantCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubIngester{result: tc.ingestResult, err: tc.ingestErr}
			h := NewEvents(slog.New(slog.DiscardHandler), stub)

			req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}

			if stub.called != tc.wantCalled {
				t.Errorf("Ingest called = %v, want %v", stub.called, tc.wantCalled)
			}

			if tc.wantEmptyBody {
				if rec.Body.Len() != 0 {
					t.Errorf("body = %q, want empty", rec.Body.String())
				}
				return
			}

			if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var got map[string]any
			if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if tc.wantResponse != nil {
				if diff := cmp.Diff(tc.wantResponse, got); diff != "" {
					t.Errorf("body mismatch (-want +got):\n%s", diff)
				}
				return
			}

			// An error body is {"error": msg} and nothing else.
			msg, ok := got["error"].(string)
			if !ok || len(got) != 1 {
				t.Fatalf(`body = %v, want {"error": <text>}`, got)
			}
			if tc.wantError == "" && msg == "" {
				t.Errorf("error text is empty")
			}
			if tc.wantError != "" && msg != tc.wantError {
				t.Errorf("error = %q, want %q", msg, tc.wantError)
			}
		})
	}
}

// TestEventsServeHTTPPassesContext checks that the service gets the request
// context, so the deadline of WithTimeout reaches the publisher.
func TestEventsServeHTTPPassesContext(t *testing.T) {
	type ctxKey struct{}

	stub := &stubIngester{result: service.IngestResult{Accepted: 1}}
	h := NewEvents(slog.New(slog.DiscardHandler), stub)

	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(batchJSON(fixtureSessionID.String(), 1)))
	req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "request"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	// The service may get the request ctx or a ctx derived from it, so the
	// test checks that a value of the request ctx reaches Ingest.
	if stub.ctx == nil || stub.ctx.Value(ctxKey{}) != "request" {
		t.Errorf("Ingest got a ctx that is not derived from the request ctx")
	}
}

// TestEventsServeHTTPPassesBatch checks that the request is passed to the
// service field by field.
func TestEventsServeHTTPPassesBatch(t *testing.T) {
	body := fmt.Sprintf(`{
		"session_id": %q,
		"events": [
			{"event_id": "05d4eec3-a4ce-465e-91fc-562da38b0c45", "type": "change", "element_id": "pizza-select", "value": "pepperoni", "client_ts": "2026-11-06T10:15:03.640Z"},
			{"event_id": "06c328fa-970b-49a7-bd43-73ded3f0e901", "type": "submit", "element_id": "order-form", "client_ts": "2026-11-06T10:15:14.525Z"}
		]
	}`, fixtureSessionID.String())

	want := service.Batch{
		SessionID: fixtureSessionID,
		Events: []service.Event{
			{
				ID:        "05d4eec3-a4ce-465e-91fc-562da38b0c45",
				Type:      "change",
				ElementID: "pizza-select",
				Value:     new("pepperoni"),
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 3, 640_000_000, time.UTC),
			},
			{
				ID:        "06c328fa-970b-49a7-bd43-73ded3f0e901",
				Type:      "submit",
				ElementID: "order-form",
				Value:     nil,
				ClientTS:  time.Date(2026, 11, 6, 10, 15, 14, 525_000_000, time.UTC),
			},
		},
	}

	stub := &stubIngester{result: service.IngestResult{Accepted: 2}}
	h := NewEvents(slog.New(slog.DiscardHandler), stub)

	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if diff := cmp.Diff(want, stub.got); diff != "" {
		t.Errorf("batch passed to service mismatch (-want +got):\n%s", diff)
	}
}
