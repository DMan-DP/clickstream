package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	maxValueRunes     = 64
	maxElementIDRunes = 64
)

var (
	errInvalidEventID   = errors.New("event_id is not a valid UUID")
	errUnknownType      = errors.New("unknown event type")
	errEmptyElementID   = errors.New("element_id is missing")
	errTooLongElementID = errors.New("element_id is too long")
	errEmptyClientTS    = errors.New("client_ts is missing")
)

// allowedTypes is the whitelist of event types. Matching is case sensitive.
var allowedTypes = map[string]struct{}{
	"click":     {},
	"input":     {},
	"change":    {},
	"focus":     {},
	"blur":      {},
	"submit":    {},
	"page_view": {},
}

// Batch is a batch of events from one browser session. Request-level
// checks (event count, session_id) are done by the caller.
type Batch struct {
	SessionID uuid.UUID
	Events    []Event
}

// Event is a single user action as sent by the client.
type Event struct {
	// ID is the raw client-generated event_id. Ingest parses it and skips
	// the event if it is not a valid non-nil UUID.
	ID        string
	Type      string
	ElementID string
	// Value is optional. Values longer than 64 characters are cleared,
	// the event itself is kept.
	Value *string
	// ClientTS is the client clock, kept for reference only. Aggregation
	// uses the server receive time.
	ClientTS time.Time
}

// IngestResult reports how many events of a batch were accepted and how
// many were skipped by event-level validation.
type IngestResult struct {
	Accepted int
	Skipped  int
}

// Events accepts event batches from the tracker.
type Events struct {
	logger *slog.Logger
	now    func() time.Time
}

// NewEvents creates an events service.
func NewEvents(logger *slog.Logger) *Events {
	return &Events{logger: logger, now: time.Now}
}

// Ingest validates each event of the batch, skips invalid ones and
// accepts the rest. A non-nil error means the batch was not accepted
// as a whole and the client should retry it.
func (s *Events) Ingest(ctx context.Context, batch Batch) (IngestResult, error) {
	serverTS := s.now().UTC()
	var res IngestResult

	for _, e := range batch.Events {
		e.ElementID = strings.TrimSpace(e.ElementID)

		if err := validateEvent(e); err != nil {
			s.logger.WarnContext(ctx, "invalid event",
				"session_id", batch.SessionID,
				"event_id", e.ID,
				"err", err,
			)
			res.Skipped++
			continue
		}

		if valueTooLong(e.Value) {
			s.logger.WarnContext(ctx, "value cleared",
				"session_id", batch.SessionID,
				"event_id", e.ID,
				"value_runes", utf8.RuneCountInString(*e.Value),
				"limit", maxValueRunes,
			)
			e.Value = nil
		}

		s.logger.DebugContext(ctx, "accepted event",
			"session_id", batch.SessionID,
			"event_id", e.ID,
			"event_type", e.Type,
			"element_id", e.ElementID,
			"value", e.Value,
			"server_ts", serverTS,
		)
		res.Accepted++
	}

	return res, nil
}

func validateEvent(e Event) error {
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return fmt.Errorf("validate event: %w: %q: %v", errInvalidEventID, e.ID, err)
	}

	if id == uuid.Nil() {
		return fmt.Errorf("validate event: %w: %q", errInvalidEventID, e.ID)
	}

	if _, ok := allowedTypes[e.Type]; !ok {
		return fmt.Errorf("validate event: %w: %q", errUnknownType, e.Type)
	}

	if e.ElementID == "" {
		return fmt.Errorf("validate event: %w: %q", errEmptyElementID, e.ElementID)
	}

	if utf8.RuneCountInString(e.ElementID) > maxElementIDRunes {
		return fmt.Errorf("validate event: %w: got %d max %d", errTooLongElementID, utf8.RuneCountInString(e.ElementID), maxElementIDRunes)
	}

	if e.ClientTS.IsZero() {
		return fmt.Errorf("validate event: %w", errEmptyClientTS)
	}

	return nil
}

func valueTooLong(v *string) bool {
	return v != nil && utf8.RuneCountInString(*v) > maxValueRunes
}
