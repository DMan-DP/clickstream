package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	maxBodyBytes = 64 << 10
	maxBatchSize = 100
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

	_ /*req*/, err := decodeBatch(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError

		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "Request entity too large")
			return
		}

		h.logger.Error("bad request", "err", err)
		writeError(w, http.StatusBadRequest, "Bad request")
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

type batchRequest struct {
	SessionID string         `json:"session_id"`
	Events    []eventRequest `json:"events"`
}

type eventRequest struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"type"`
	ElementID string    `json:"element_id"`
	Value     *string   `json:"value"`
	ClientTS  time.Time `json:"client_ts"`
}
