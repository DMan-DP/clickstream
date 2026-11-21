package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/DMan-DP/clickstream/internal/tracker/service"
)

const (
	maxBodyBytes = 64 << 10
	maxBatchSize = 100
)

// statusClientClosedRequest is the nginx status for a request whose client
// closed the connection before the response was written. net/http has no
// constant for it.
const statusClientClosedRequest = 499

var (
	errNoEvents      = errors.New("no events")
	errTooManyEvents = errors.New("too many events")
	errNoSessionID   = errors.New("session_id is missing or nil")
)

type ingester interface {
	Ingest(ctx context.Context, batch service.Batch) (service.IngestResult, error)
}

type Events struct {
	logger  *slog.Logger
	service ingester
}

func NewEvents(logger *slog.Logger, eventsService ingester) *Events {
	return &Events{
		logger:  logger,
		service: eventsService,
	}
}

// ServeHTTP accepts a batch of events as JSON and passes it to the
// service with the request context. The status is the contract for the
// client; an error body is {"error": msg} with a short text for people,
// without internal details.
//   - 202 with {"accepted": n, "skipped": m}, also when every event of
//     the batch is skipped;
//   - 400 for a malformed body, or for a failed request-level check with
//     the text of validationMessage;
//   - 413 for a body larger than maxBodyBytes (64 KiB);
//   - 503 when the service is unavailable or the request deadline is
//     exceeded, so the client may retry the batch;
//   - 499 without a body when the client has gone (context.Canceled);
//   - 500 for any other service error.
func (h *Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	ctx := r.Context()

	req, err := decodeBatch(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			h.logger.WarnContext(ctx, "bad request", "err", err)
			writeError(w, http.StatusRequestEntityTooLarge, "Request entity too large")
			return
		}

		h.logger.WarnContext(ctx, "bad request", "err", err)
		writeError(w, http.StatusBadRequest, "Bad request")
		return
	}

	if err := validate(req); err != nil {
		h.logger.WarnContext(ctx, "bad request", "err", err)
		writeError(w, http.StatusBadRequest, validationMessage(err))
		return
	}

	batch := toServiceBatch(req)
	result, err := h.service.Ingest(ctx, batch)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			h.logger.InfoContext(ctx, "ingest canceled",
				"events_count", len(req.Events),
				"session_id", req.SessionID,
				"err", err,
			)
			w.WriteHeader(statusClientClosedRequest)
			return
		case errors.Is(err, context.DeadlineExceeded):
			h.logger.ErrorContext(ctx, "ingest timeout",
				"events_count", len(req.Events),
				"session_id", req.SessionID,
				"err", err,
			)
			writeError(w, http.StatusServiceUnavailable, "Service unavailable")
			return
		case errors.Is(err, service.ErrUnavailable):
			h.logger.ErrorContext(ctx, "ingest unavailable",
				"events_count", len(req.Events),
				"session_id", req.SessionID,
				"err", err,
			)
			writeError(w, http.StatusServiceUnavailable, "Service unavailable")
			return
		default:
			h.logger.ErrorContext(ctx, "ingest failed",
				"events_count", len(req.Events),
				"session_id", req.SessionID,
				"err", err,
			)
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	}

	res := batchResponse{
		Accepted: result.Accepted,
		Skipped:  result.Skipped,
	}
	writeJSON(w, http.StatusAccepted, res)
	h.logger.InfoContext(ctx, "batch accepted",
		"session_id", req.SessionID,
		"accepted", result.Accepted,
		"skipped", result.Skipped,
	)
}

// decodeBatch reads exactly one JSON object from r into a batchRequest.
// Unknown fields are ignored; anything but whitespace after the object is
// an error. client_ts must be an RFC 3339 string. A missing or null
// session_id gives uuid.Nil without an error: validate rejects it. Errors
// wrap the cause, so *http.MaxBytesError is found with errors.As.
func decodeBatch(r io.Reader) (batchRequest, error) {
	var req batchRequest
	dec := json.NewDecoder(r)

	if err := dec.Decode(&req); err != nil {
		return batchRequest{}, fmt.Errorf("decode batch: %w", err)
	}

	var raw json.RawMessage
	err := dec.Decode(&raw)

	if err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}

		return batchRequest{}, fmt.Errorf("decode batch: %w", err)
	}

	return batchRequest{}, errors.New("decode batch: body must contain a single JSON object")
}

// validate runs the request-level checks of req and returns the first
// failure, wrapping a sentinel error for validationMessage. The checks
// run in this order: at least one and at most maxBatchSize events
// (errNoEvents, errTooManyEvents), then a non-nil session_id
// (errNoSessionID).
func validate(req batchRequest) error {
	eventsCount := len(req.Events)
	if eventsCount == 0 {
		return fmt.Errorf("validate batch: %w", errNoEvents)
	}

	if eventsCount > maxBatchSize {
		return fmt.Errorf("validate batch: %w: got %d, max %d", errTooManyEvents, eventsCount, maxBatchSize)
	}

	if req.SessionID == uuid.Nil() {
		return fmt.Errorf("validate batch: %w", errNoSessionID)
	}

	return nil
}

// validationMessage returns the text for the client for an error of
// validate: "No events", "Too many events, max N" or "session_id is
// missing or invalid". The sentinel errors are found with errors.Is, so
// they may be wrapped. Any other error gives "Bad request".
func validationMessage(err error) string {
	switch {
	case errors.Is(err, errNoEvents):
		return "No events"
	case errors.Is(err, errTooManyEvents):
		return fmt.Sprintf("Too many events, max %d", maxBatchSize)
	case errors.Is(err, errNoSessionID):
		return "session_id is missing or invalid"
	default:
		return "Bad request"
	}
}

type batchRequest struct {
	SessionID uuid.UUID      `json:"session_id"`
	Events    []eventRequest `json:"events"`
}

type eventRequest struct {
	ID        string    `json:"event_id"`
	Type      string    `json:"type"`
	ElementID string    `json:"element_id"`
	Value     *string   `json:"value"`
	ClientTS  time.Time `json:"client_ts"`
}

type batchResponse struct {
	Accepted int `json:"accepted"`
	Skipped  int `json:"skipped"`
}

func toServiceBatch(req batchRequest) service.Batch {
	batch := service.Batch{
		SessionID: req.SessionID,
		Events:    make([]service.Event, 0, len(req.Events)),
	}

	for _, reqE := range req.Events {
		batchE := service.Event{
			ID:        reqE.ID,
			Type:      reqE.Type,
			ElementID: reqE.ElementID,
			Value:     reqE.Value,
			ClientTS:  reqE.ClientTS,
		}
		batch.Events = append(batch.Events, batchE)
	}

	return batch
}
