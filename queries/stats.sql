-- Queries over the counters written by analytics-processor.
--
-- Run with psql (see README), optional variables:
--   window_sec  window length in seconds, must match WINDOW_SIZE (default 30)
--   period      how far back from now, a Postgres interval (default 1 day)
--   top         how many values to show per choice group (default 3)
--
-- Counters are additive: one window may be written in several parts
-- (ticker in the middle of a window, several partitions), so every query
-- sums count instead of reading a single row.

\if :{?window_sec}
\else
    \set window_sec 30
\endif
\if :{?period}
\else
    \set period '1 day'
\endif
\if :{?top}
\else
    \set top 3
\endif

\echo
\echo '1. Events per window and type'
SELECT window_start,
       window_start + make_interval(secs => window_sec) AS window_end,
       event_type,
       sum(count) AS events
FROM event_stats
WHERE window_sec = :window_sec
  AND window_start >= now() - :'period'::interval
GROUP BY window_start, window_sec, event_type
ORDER BY window_start, event_type;

\echo '2. Events per window, type and element'
SELECT window_start,
       event_type,
       element_id,
       sum(count) AS events
FROM event_stats
WHERE window_sec = :window_sec
  AND window_start >= now() - :'period'::interval
GROUP BY window_start, event_type, element_id
ORDER BY window_start, event_type, events DESC, element_id;

\echo '3. Totals for the period by type and element'
SELECT event_type,
       element_id,
       sum(count) AS events
FROM event_stats
WHERE window_sec = :window_sec
  AND window_start >= now() - :'period'::interval
GROUP BY event_type, element_id
ORDER BY event_type, events DESC, element_id;

-- Sizes and toppings are separate page elements, so they are grouped here.
-- A topping change to false means the box was unchecked: not a choice.
-- Default values (margherita, medium, quantity 1) produce no events and are
-- undercounted: this shows how often a value was picked, not final orders.
\echo '4. Top choices for the period'
WITH choices AS (
    SELECT CASE
               WHEN element_id LIKE 'size-%' THEN 'size'
               WHEN element_id LIKE 'topping-%' THEN 'topping'
               WHEN element_id = 'pizza-select' THEN 'pizza'
               WHEN element_id = 'quantity-slider' THEN 'quantity'
               ELSE element_id
           END AS choice,
           CASE
               WHEN element_id LIKE 'topping-%' THEN substr(element_id, 9)
               ELSE value
           END AS value,
           sum(count) AS picks
    FROM value_stats
    WHERE window_sec = :window_sec
      AND window_start >= now() - :'period'::interval
      AND NOT (element_id LIKE 'topping-%' AND value = 'false')
    GROUP BY 1, 2
),
ranked AS (
    SELECT choice,
           value,
           picks,
           round(100.0 * picks / sum(picks) OVER (PARTITION BY choice), 1) AS share_pct,
           rank() OVER (PARTITION BY choice ORDER BY picks DESC) AS place
    FROM choices
)
SELECT choice, place, value, picks, share_pct
FROM ranked
WHERE place <= :top
ORDER BY choice, place, value;

-- A session is counted on a step in the window of the event that took it
-- there, so a session spread over two windows has visit in one and the
-- rest in the other. One window alone is not a funnel: read the period.
-- Steps come from a fixed list, so a step nobody reached shows 0.
\echo '5. Order funnel for the period'
WITH steps (step, pos) AS (
    VALUES ('visit', 1), ('pizza', 2), ('contacts', 3), ('submit', 4)
),
totals AS (
    SELECT s.step,
           s.pos,
           coalesce(sum(f.sessions), 0) AS sessions
    FROM steps s
    LEFT JOIN funnel_stats f
           ON f.step = s.step
          AND f.window_sec = :window_sec
          AND f.window_start >= now() - :'period'::interval
    GROUP BY s.step, s.pos
)
SELECT step,
       sessions,
       lag(sessions) OVER w - sessions AS dropped,
       round(100.0 * sessions / nullif(lag(sessions) OVER w, 0), 1) AS from_prev_pct,
       round(100.0 * sessions / nullif(first_value(sessions) OVER w, 0), 1) AS from_visit_pct
FROM totals
WINDOW w AS (ORDER BY pos)
ORDER BY pos;
