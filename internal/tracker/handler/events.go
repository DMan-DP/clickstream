package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	eventsService ingester
}

func NewEvents(eventsService ingester) *Events {
	return &Events{eventsService: eventsService}
}

func (*Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
