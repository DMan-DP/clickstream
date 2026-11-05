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
frontend/                 demo page and tracker
docker-compose.yml        Kafka, kafka-ui, PostgreSQL, topic creation and migrations
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

You need Docker and Go.

1. Copy the settings and change the password if you like:

   ```
   cp .env.example .env
   ```

2. Start the infrastructure:

   ```
   docker compose up -d
   ```

   Compose creates the `user-events` topic with 3 partitions and applies the migrations. The Kafka UI is at http://localhost:8090, PostgreSQL listens on port 5432.

3. Run `tracker-api` on the host. The environment variables are described in `.env.example`.

   Bash:

   ```
   KAFKA_BROKERS=localhost:9092 KAFKA_TOPIC=user-events go run ./cmd/tracker-api
   ```

   PowerShell:

   ```
   $env:KAFKA_BROKERS="localhost:9092"; $env:KAFKA_TOPIC="user-events"; go run ./cmd/tracker-api
   ```

   Check: `curl http://localhost:8080/health` returns `200`.

Tests:

```
go test ./...
```

## Status

Work in progress.

- [x] Infrastructure in Docker Compose: Kafka, kafka-ui, PostgreSQL, topic, migrations
- [x] Demo page and event tracker
- [x] `tracker-api` skeleton: configuration, health check, graceful shutdown
- [ ] Receiving events with `POST /events` and serving the page
- [ ] Publishing to Kafka
- [ ] `analytics-processor`: reading from Kafka, window aggregation, writing to PostgreSQL
- [ ] Docker images for the services
