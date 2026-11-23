# clickstream

English | [Русский](README.ru.md)

User action analytics for a web page. The page sends clicks, input and field changes to a server, the server publishes them to Kafka, and a separate service reads the events, counts them in 30-second windows and stores the counters in PostgreSQL.

## How it works

1. The `frontend/` page is a pizza order form with buttons, text fields, a slider, checkboxes, radio buttons, a dropdown and a form submit.
2. The `frontend/tracker.js` script queues events and sends them in batches to `POST /events` every 5 seconds.
3. The `tracker-api` service validates a batch and publishes each event as a separate message to the Kafka topic `user-events`. The message key is `session_id`, so events of one session go to the same partition.
4. The `analytics-processor` service reads the topic in a consumer group, counts events in 30-second windows and writes the counters to the `event_stats` table.

Windows are based on the server receive time (UTC). The client time is not trusted, but it is kept in the message.

## Stack

- Go 1.27
- Kafka 4.1 (`apache/kafka` image, KRaft mode without ZooKeeper), [IBM/sarama](https://github.com/IBM/sarama) client
- PostgreSQL 17, [pgx](https://github.com/jackc/pgx) driver through `database/sql` and [sqlx](https://github.com/jmoiron/sqlx)
- [goose](https://github.com/pressly/goose) migrations
- Docker Compose
- Frontend in plain HTML, CSS and JavaScript, no frameworks

## Project layout

```
cmd/
  tracker-api/            receives events over HTTP and publishes them to Kafka
  analytics-processor/    reads from Kafka and aggregates events in windows
internal/
  tracker/                tracker-api code: config, handler, service, publisher
  analytics/              analytics-processor code: domain, service, repository, consumer
migrations/               PostgreSQL schema
queries/                  SQL queries over the stored counters
frontend/                 demo page and tracker
nginx/                    nginx config: serves frontend/, proxies /events to tracker-api
Dockerfile                service image, the SERVICE argument selects the binary
docker-compose.yml        Kafka, kafka-ui, PostgreSQL, topic, migrations, both services, nginx
```

## Event format

Request `POST /events`:

```json
{
  "session_id": "5f0c2a1e-8d3b-4c7a-9e21-6b4f0d9a7c13",
  "events": [
    {
      "event_id": "0b6e2f4a-1c9d-4e8b-a7f3-2d5c8e1b9a60",
      "type": "change",
      "element_id": "size",
      "value": "large",
      "client_ts": "2026-11-05T12:00:03.120Z"
    }
  ]
}
```

- `type`: `click`, `input`, `change`, `focus`, `blur`, `submit`, `page_view`. Events of an unknown type are skipped, the rest of the batch is accepted.
- `element_id` is required. Surrounding whitespace is trimmed, the result must be 1 to 64 characters, otherwise the event is skipped.
- `value` is optional: the value of a slider, dropdown, checkbox or radio button. For text fields only the input length is sent, the text itself never leaves the page.
- A batch holds up to 100 events, the request body is limited to 64 KB.

A Kafka message has the same fields plus `session_id` and `server_ts`.

## Delivery guarantees

At-least-once along the whole path: an event is not lost, but it may arrive more than once.

- The tracker keeps unconfirmed events in `localStorage` and retries on network errors and on `5xx` and `429` responses. Events not sent before the page was closed are sent the next time it is opened.
- `tracker-api` answers `202 Accepted` only after Kafka has confirmed the write.
- `analytics-processor` writes the counters to PostgreSQL first and commits the Kafka offset after that.

Every event gets an `event_id` on the client, so a retry can be told apart from a new event. Deduplication is not implemented yet.

## Running

You need Docker. Go is needed only for the tests and for running a service on the host.

1. Copy the settings and change the password if you like:

   ```
   cp .env.example .env
   ```

2. Build the images and start everything:

   ```
   docker compose up -d --build
   ```

   Compose creates the `user-events` topic with 3 partitions, applies the migrations, then starts `tracker-api` and `analytics-processor`, and starts nginx once `tracker-api` is healthy.

   - Demo page: http://localhost:3000. It is served by nginx, which also proxies `POST /events` to `tracker-api`, so the page and the API share one origin and no CORS is needed. Changes in `frontend/` show up after a page reload.
   - `tracker-api` directly: `curl http://localhost:8080/health` returns `200` (port TRACKER_API_PORT).
   - Kafka UI: http://localhost:8090.
   - PostgreSQL: port 5433 (POSTGRES_PORT), the counters reach the tables FLUSH_INTERVAL after the events, see [Queries](#queries).

   Service logs:

   ```
   docker compose logs -f analytics-processor
   ```

3. Stopping: `docker compose stop` keeps the containers, `docker compose down` removes them, `docker compose down -v` also removes the Kafka and PostgreSQL data.

After a change in the Go code, rebuild the image: `docker compose up -d --build tracker-api` (or `analytics-processor`).

### A service on the host, under the debugger

Stop the service container first, otherwise port 8080 is taken, and a second `analytics-processor` joins the same group and takes over some partitions:

```
docker compose stop tracker-api
```

For `tracker-api`, also point nginx at the host: set `TRACKER_API_URL=http://host.docker.internal:8080` in `.env` and recreate only nginx (`--no-deps`, so compose does not start the `tracker-api` container again):

```
docker compose up -d --no-deps frontend
```

Then start the service. VS Code: the `tracker-api` and `analytics-processor` launch configurations (F5) take the variables from `.env`. Or from the terminal, setting the variables from `.env.example` by hand, for example for `tracker-api`:

Bash:

```
KAFKA_BROKERS=localhost:9092 KAFKA_TOPIC=user-events go run ./cmd/tracker-api
```

PowerShell:

```
$env:KAFKA_BROKERS="localhost:9092"; $env:KAFKA_TOPIC="user-events"; go run ./cmd/tracker-api
```

While `tracker-api` is not running, nginx answers `502` and the tracker keeps the events and retries. To move everything back into containers: set `TRACKER_API_URL=http://tracker-api:8080` in `.env` again and run `docker compose up -d`.

Tests:

```
go test ./...
```

## Queries

[queries/stats.sql](queries/stats.sql) reads the counters written by `analytics-processor`: events per window and type, the same split by element, totals for the period, the top choices (pizza, size, toppings, quantity) and the order funnel conversion.

Bash:

```
docker compose exec -T postgres psql -U clickstream -d clickstream < queries/stats.sql
```

PowerShell:

```
Get-Content queries/stats.sql | docker compose exec -T postgres psql -U clickstream -d clickstream
```

Optional psql variables: `window_sec` (window length in seconds, must match `WINDOW_SIZE`, default `30`), `period` (how far back from now, default `1 day`), `top` (values per choice group, default `3`):

```
docker compose exec -T postgres psql -U clickstream -d clickstream -v "period=1 hour" -v top=1 < queries/stats.sql
```

Counters are additive and one window may be stored in several parts, so the queries sum `count`. Default choices (margherita, medium, quantity 1) produce no events, so the top shows how often a value was picked, not what was ordered.

## Status

Work in progress.

- [x] Infrastructure in Docker Compose: Kafka, kafka-ui, PostgreSQL, topic, migrations
- [x] Demo page and event tracker, served by nginx with `/events` proxied to `tracker-api`
- [x] `tracker-api` skeleton: configuration, health check, graceful shutdown
- [x] Receiving events with `POST /events`
- [x] Publishing to Kafka
- [x] `analytics-processor`: reading from Kafka, window aggregation, writing to PostgreSQL
- [x] Docker images for the services, the whole stack starts with one command
