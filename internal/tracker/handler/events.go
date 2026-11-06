package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
	"uuid"
)

const (
	maxBodyBytes = 64 << 10
	maxBatchSize = 100
)

var (
	errNoEvents      = errors.New("no events")
	errTooManyEvents = errors.New("too many events")
	errNoSessionID   = errors.New("session_id is missing or nil")
)

type ingester interface {
	Ingest()
}

type Events struct {
	logger        *slog.Logger
	eventsService ingester
}

func NewEvents(logger *slog.Logger, eventsService ingester) *Events {
	return &Events{
		logger:        logger,
		eventsService: eventsService,
	}
}

func (h *Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	req, err := decodeBatch(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "Request entity too large")
			h.logger.Warn("bad request", "err", err)
			return
		}

		h.logger.Warn("bad request", "err", err)
		writeError(w, http.StatusBadRequest, "Bad request")
		return
	}

	if err := validate(req); err != nil {
		h.logger.Warn("bad request", "err", err)
		writeError(w, http.StatusBadRequest, validationMessage(err))
		return
	}
}

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
	EventID   string    `json:"event_id"`
	EventType string    `json:"type"`
	ElementID string    `json:"element_id"`
	Value     *string   `json:"value"`
	ClientTS  time.Time `json:"client_ts"`
}
