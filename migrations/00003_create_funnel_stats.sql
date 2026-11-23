-- +goose Up
-- Order funnel: how many sessions reached a step for the first time in a
-- window. A session is counted in the window of the event that took it to
-- the step, so one window alone is not a funnel; conversion is read over a
-- period. Upserted the same way as event_stats.
-- step is stored by name, not by number: the number is the position of the
-- constant in code and would silently change meaning when a step is added.
-- The CHECK keeps the names in sync with the code; none is session state
-- only and is never written.
CREATE TABLE funnel_stats (
    window_sec   integer     NOT NULL CHECK (window_sec > 0),
    window_start timestamptz NOT NULL,
    step         text        NOT NULL
        CHECK (step IN ('visit', 'pizza', 'contacts', 'submit')),
    sessions     bigint      NOT NULL CHECK (sessions > 0),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (window_sec, window_start, step)
);

COMMENT ON TABLE funnel_stats IS 'Sessions that reached an order funnel step, per window (server time, UTC)';
COMMENT ON COLUMN funnel_stats.window_sec IS 'Window length in seconds';
COMMENT ON COLUMN funnel_stats.window_start IS 'Window start; window end = window_start + window_sec';
COMMENT ON COLUMN funnel_stats.step IS 'Funnel step: visit, pizza, contacts, submit';
COMMENT ON COLUMN funnel_stats.sessions IS 'Sessions that reached the step for the first time in this window';

-- +goose Down
DROP TABLE funnel_stats;
