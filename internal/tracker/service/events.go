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

	"github.com/DMan-DP/clickstream/internal/message"
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

// publisher sends accepted events to the message broker.
type publisher interface {
	// Publish sends all messages as one batch. A nil error means every
	// message was confirmed by the broker. A non-nil error means the batch
	// is treated as not published, even if some messages were written:
	// the caller retries the whole batch, so duplicates are possible.
	// Implementations may not be able to abort a send already in progress
	// when ctx is done.
	Publish(ctx context.Context, messages []message.Event) error
}

// Events accepts event batches from the tracker.
type Events struct {
	logger *slog.Logger
	pub    publisher
	now    func() time.Time
}

// NewEvents creates an events service.
func NewEvents(logger *slog.Logger, pub publisher) *Events {
	return &Events{logger: logger, pub: pub, now: time.Now}
}

// Ingest validates each event of the batch, skips invalid ones and
// accepts the rest. A non-nil error means the batch was not accepted
// as a whole and the client should retry it.
func (s *Events) Ingest(ctx context.Context, batch Batch) (IngestResult, error) {
	serverTS := s.now().UTC()
	var res IngestResult
	msgs := make([]message.Event, 0, len(batch.Events))

	for _, e := range batch.Events {
		msg, err := eventToMessage(e, batch.SessionID, serverTS)
		if err != nil {
			s.logger.WarnContext(ctx, "invalid event",
				"session_id", batch.SessionID,
				"event_id", e.ID,
				"err", err,
			)
			res.Skipped++
			continue
		}

		if valueTooLong(msg.Value) {
			s.logger.WarnContext(ctx, "value cleared",
				"session_id", msg.SessionID,
				"event_id", msg.EventID,
				"value_runes", utf8.RuneCountInString(*msg.Value),
				"limit", maxValueRunes,
			)
			msg.Value = nil
		}

		s.logger.DebugContext(ctx, "accepted event",
			"session_id", msg.SessionID,
			"event_id", msg.EventID,
			"event_type", msg.Type,
			"element_id", msg.ElementID,
			"value", msg.Value,
			"server_ts", serverTS,
		)
		msgs = append(msgs, msg)
		res.Accepted++
	}

	if len(msgs) > 0 {
		err := s.pub.Publish(ctx, msgs)
		if err != nil {
			return IngestResult{}, err
		}
	}

	return res, nil
}

func eventToMessage(e Event, sessionID uuid.UUID, serverTS time.Time) (message.Event, error) {
	e.ElementID = strings.TrimSpace(e.ElementID)

	id, err := uuid.Parse(e.ID)
	if err != nil {
		return message.Event{}, fmt.Errorf("event to message: %w: %q: %v", errInvalidEventID, e.ID, err)
	}

	if id == uuid.Nil() {
		return message.Event{}, fmt.Errorf("event to message: %w: %q", errInvalidEventID, e.ID)
	}

	if _, ok := allowedTypes[e.Type]; !ok {
		return message.Event{}, fmt.Errorf("event to message: %w: %q", errUnknownType, e.Type)
	}

	if e.ElementID == "" {
		return message.Event{}, fmt.Errorf("event to message: %w: %q", errEmptyElementID, e.ElementID)
	}

	if utf8.RuneCountInString(e.ElementID) > maxElementIDRunes {
		return message.Event{}, fmt.Errorf("event to message: %w: got %d max %d", errTooLongElementID, utf8.RuneCountInString(e.ElementID), maxElementIDRunes)
	}

	if e.ClientTS.IsZero() {
		return message.Event{}, fmt.Errorf("event to message: %w", errEmptyClientTS)
	}

	return message.Event{
		EventID:   id,
		SessionID: sessionID,
		Type:      e.Type,
		ElementID: e.ElementID,
		Value:     e.Value,
		ClientTS:  e.ClientTS,
		ServerTS:  serverTS,
	}, nil
}

func valueTooLong(v *string) bool {
	return v != nil && utf8.RuneCountInString(*v) > maxValueRunes
}
