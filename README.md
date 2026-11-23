# clickstream

English | [Русский](README.ru.md)

User action analytics for a web page. The page sends clicks, input and field changes to a server, the server publishes them to Kafka, and a separate service reads the events, counts them in 30-second windows and stores the counters in PostgreSQL.

## How it works

1. The `frontend/` page is a pizza order form with buttons, text fields, a slider, checkboxes, radio buttons, a dropdown and a form submit.
2. The `frontend/tracker.js` script queues events and sends them in batches to `POST /events` every 5 seconds.
3. The `tracker-api` service validates a batch and publishes each event as a separate message to the Kafka topic `user-events`. The message key is `session_id`, so events of one session go to the same partition.
4. The `analytics-processor` service reads the topic in a consumer group, counts events in 30-second windows and writes the counters to the `event_stats` table. In the same windows it counts the sessions that reached each step of the order funnel (page visit, pizza chosen, contacts filled in, order sent) and writes them to `funnel_stats`.

Windows are based on the server receive time (UTC). The client time is not trusted, but it is kept in the message.

## Stack

- Go 1.27
- Kafka 4.1 (`apache/kafka` image, KRaft mode without ZooKeeper), [IBM/sarama](https://github.com/IBM/sarama) client
- PostgreSQL 17, [pgx](https://github.com/jackc/pgx) driver through `database/sql` and [sqlx](https://github.com/jmoiron/sqlx)
- [goose](https://github.com/pressly/goose) migrations
- Docker Compose
- Kubernetes in [minikube](https://minikube.sigs.k8s.io/), manifests assembled with kustomize (`kubectl apply -k`)
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
deploy/
  frontend/               nginx image with the page
  migrate/                image that applies the migrations
  k8s/                    Kubernetes manifests for minikube
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
   - `tracker-api` directly: `curl http://localhost:8080/livez` returns `200` (port TRACKER_API_PORT).
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

## Running in Kubernetes (minikube)

The same stack in a local cluster, namespace `clickstream`: PostgreSQL (StatefulSet, one replica), Kafka (StatefulSet, three brokers, see [Kafka replication](#kafka-replication)), the `kafka-topics` and `migrate` Jobs, `tracker-api` and `analytics-processor` with 3 replicas each, and `frontend` (nginx with the page). kafka-ui is not deployed.

You need Docker, [minikube](https://minikube.sigs.k8s.io/docs/start/) and kubectl.

1. Start the cluster:

   ```
   minikube start --driver=docker --cpus=4 --memory=4096
   ```

2. Build the images and copy them into minikube. The cluster does not see the local Docker images, and the manifests do not pull them from a registry (`imagePullPolicy: IfNotPresent`):

   ```
   docker compose build
   minikube image load clickstream/tracker-api:dev
   minikube image load clickstream/analytics-processor:dev
   minikube image load clickstream/migrate:dev
   minikube image load clickstream/frontend:dev
   ```

3. Copy the database settings and change the password (letters and digits only, it goes into `DATABASE_URL` as is). The file is not committed:

   ```
   cp deploy/k8s/postgres.secret.env.example deploy/k8s/postgres.secret.env
   ```

4. Apply the manifests and wait for the services:

   ```
   kubectl apply -k deploy/k8s
   kubectl -n clickstream wait --for=condition=available deployment --all --timeout=5m
   kubectl -n clickstream get pods
   ```

   Until Kafka is up and the migrations are applied, the `analytics-processor` pods may restart (`CrashLoopBackOff`), this is expected.

5. Open the page. The Services are not exposed outside the cluster, so forward a port (it stays in the foreground, stop it with Ctrl+C):

   ```
   kubectl -n clickstream port-forward svc/frontend 3000:80
   ```

   Demo page: http://localhost:3000. If the Docker Compose stack is running too, port 3000 is taken: stop it or forward another port, for example `3001:80`. `tracker-api` directly: `kubectl -n clickstream port-forward svc/tracker-api 8080:8080`, then `curl http://localhost:8080/livez`. A port-forward goes to one pod, not through the Service balancing.

Logs of all replicas of a service:

```
kubectl -n clickstream logs -f -l app.kubernetes.io/name=analytics-processor --prefix
```

Queries from [Queries](#queries) go through the PostgreSQL pod. Bash:

```
kubectl -n clickstream exec -i postgres-0 -- psql -U clickstream -d clickstream < queries/stats.sql
```

PowerShell:

```
Get-Content queries/stats.sql | kubectl -n clickstream exec -i postgres-0 -- psql -U clickstream -d clickstream
```

The settings of the services are in `deploy/k8s/tracker-api.env` and `deploy/k8s/analytics-processor.env`. After an edit run `kubectl apply -k deploy/k8s` again, the pods of that service are replaced by themselves.

After a change in the code, rebuild the image, load it into minikube and restart the pods (the tag stays `:dev`, so Kubernetes does not notice the new image by itself):

```
docker compose build tracker-api
minikube image load clickstream/tracker-api:dev
kubectl -n clickstream rollout restart deployment/tracker-api
```

New migrations: load `clickstream/migrate:dev` the same way, then start the Job again. A finished Job is not restarted by `apply`, and its pod template cannot be changed, so delete it first:

```
kubectl -n clickstream delete job migrate
kubectl apply -k deploy/k8s
```

Stopping:

- `minikube stop` stops the whole cluster, the data stays. `minikube start` brings it back.
- `kubectl -n clickstream scale deployment,statefulset --all --replicas=0` stops only this project, the data stays. To start it again, run `kubectl apply -k deploy/k8s`: the manifests set the replica counts back.
- `kubectl delete -k deploy/k8s` removes the namespace together with the PostgreSQL and Kafka volumes. Unlike `docker compose down`, the data is gone.

### Kafka replication

In the cluster Kafka runs as three brokers, Docker Compose keeps one for development. Every partition of `user-events` has 3 replicas on different brokers (replication factor 3) with `min.insync.replicas=2`, and `tracker-api` waits for all in-sync replicas (`acks=all`). So `202` means the event is stored on at least two brokers:

- one broker down: the remaining replicas take over as partition leaders, events are accepted;
- two brokers down: Kafka rejects writes with `NOT_ENOUGH_REPLICAS`, `tracker-api` answers `503` and the page keeps the events and retries.

To see it, check the partition leaders and in-sync replicas (`Isr`):

```
kubectl -n clickstream exec kafka-0 -- /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic user-events
```

In Git Bash prefix the command with `MSYS_NO_PATHCONV=1`, otherwise the path `/opt/...` is rewritten into a Windows path. Then take one broker down with `kubectl -n clickstream delete pod kafka-1` (it is recreated by itself), or two with `kubectl -n clickstream scale statefulset kafka --replicas=1` and back with `kubectl apply -k deploy/k8s`.

All three brokers run on the single minikube node, so this protects against a failed broker, not against a lost machine.

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

## Limitations

- No deduplication. Every event carries an `event_id`, but it is not checked yet, so a retried batch is counted twice.
- One PostgreSQL instance everywhere, and one Kafka broker in Docker Compose: losing either of them stops the pipeline. In Kubernetes Kafka has three brokers, but they all run on one minikube node.
- Windows are based on the server receive time. Events that the tracker sends late (after a network failure or the next time the page is opened) land in a later window.
- The repository has no integration test against a real PostgreSQL.
