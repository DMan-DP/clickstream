-- +goose Up
-- Window length becomes part of the key: WINDOW_SIZE is configurable, and
-- without it an additive upsert would mix windows of different length.
-- window_sec goes first so that range queries filter one length and then
-- scan window_start in order.
-- Existing rows were written with the 30-second default.
ALTER TABLE event_stats
    ADD COLUMN window_sec integer NOT NULL DEFAULT 30 CHECK (window_sec > 0);
ALTER TABLE event_stats
    ALTER COLUMN window_sec DROP DEFAULT;
ALTER TABLE event_stats
    DROP CONSTRAINT event_stats_pkey,
    ADD PRIMARY KEY (window_sec, window_start, event_type, element_id);

COMMENT ON TABLE event_stats IS 'Event counters per window (server time, UTC)';
COMMENT ON COLUMN event_stats.window_sec IS 'Window length in seconds';
COMMENT ON COLUMN event_stats.window_start IS 'Window start; window end = window_start + window_sec';

-- Popular choices: how many times an element was set to a value in a window.
-- Only change events of the choice whitelist (pizza, size, toppings,
-- quantity). Upserted the same way as event_stats.
CREATE TABLE value_stats (
    window_sec   integer     NOT NULL CHECK (window_sec > 0),
    window_start timestamptz NOT NULL,
    element_id   text        NOT NULL,
    value        text        NOT NULL,
    count        bigint      NOT NULL CHECK (count > 0),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (window_sec, window_start, element_id, value)
);

COMMENT ON TABLE value_stats IS 'Choice counters per window: element value changes (server time, UTC)';
COMMENT ON COLUMN value_stats.window_sec IS 'Window length in seconds';
COMMENT ON COLUMN value_stats.window_start IS 'Window start; window end = window_start + window_sec';
COMMENT ON COLUMN value_stats.value IS 'Chosen value as sent by the page, e.g. pepperoni, true, 3';

-- +goose Down
-- Fails if event_stats has rows that differ only by window_sec.
DROP TABLE value_stats;

ALTER TABLE event_stats
    DROP CONSTRAINT event_stats_pkey,
    ADD PRIMARY KEY (window_start, event_type, element_id);
ALTER TABLE event_stats
    DROP COLUMN window_sec;

COMMENT ON TABLE event_stats IS 'Event counters per 30-second window (server time, UTC)';
COMMENT ON COLUMN event_stats.window_start IS 'Window start; window end = window_start + 30 seconds';
