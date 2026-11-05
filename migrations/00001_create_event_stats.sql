-- +goose Up
-- Event counters per 30-second window, by server receive time.
-- One window arrives in parts (partitions, consumer instances, late events),
-- so rows are upserted with count = event_stats.count + EXCLUDED.count.
CREATE TABLE event_stats (
    window_start timestamptz NOT NULL,
    event_type   text        NOT NULL,
    element_id   text        NOT NULL,
    count        bigint      NOT NULL CHECK (count > 0),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (window_start, event_type, element_id)
);

COMMENT ON TABLE event_stats IS 'Event counters per 30-second window (server time, UTC)';
COMMENT ON COLUMN event_stats.window_start IS 'Window start; window end = window_start + 30 seconds';

-- +goose Down
DROP TABLE event_stats;
