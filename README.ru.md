# clickstream

[English](README.md) | Русский

Аналитика действий пользователя на веб-странице. Страница отправляет клики, ввод и изменения полей на сервер, сервер публикует их в Kafka, отдельный сервис читает события, считает их по 30-секундным окнам и сохраняет счетчики в PostgreSQL.

## Как это работает

1. Страница `frontend/` с формой заказа пиццы: кнопки, текстовые поля, слайдер, чекбоксы, радиокнопки, выпадающий список, отправка формы.
2. Скрипт `frontend/tracker.js` собирает события в очередь и раз в 5 секунд отправляет их пачкой на `POST /events`.
3. Сервис `tracker-api` проверяет пачку и публикует каждое событие отдельным сообщением в топик Kafka `user-events`. Ключ сообщения `session_id`, поэтому события одной сессии попадают в одну партицию.
4. Сервис `analytics-processor` читает топик в consumer group, считает события по окнам в 30 секунд и записывает счетчики в таблицу `event_stats`. По тем же окнам он считает сессии, дошедшие до каждого шага воронки заказа (заход на страницу, выбор пиццы, заполненные контакты, отправка заказа), и записывает их в `funnel_stats`.

Окна считаются по времени приема на сервере (UTC). Времени клиента сервис не доверяет, но сохраняет его в сообщении.

## Стек

- Go 1.27
- Kafka 4.1 (образ `apache/kafka`, режим KRaft без ZooKeeper), клиент [IBM/sarama](https://github.com/IBM/sarama)
- PostgreSQL 17, драйвер [pgx](https://github.com/jackc/pgx) через `database/sql` и [sqlx](https://github.com/jmoiron/sqlx)
- Миграции [goose](https://github.com/pressly/goose)
- Docker Compose
- Kubernetes в [minikube](https://minikube.sigs.k8s.io/), манифесты собирает kustomize (`kubectl apply -k`)
- Фронтенд на HTML, CSS и JavaScript без фреймворков

## Структура

```
cmd/
  tracker-api/            прием событий по HTTP и публикация в Kafka
  analytics-processor/    чтение из Kafka и агрегация по окнам
internal/
  tracker/                код tracker-api: config, handler, service, publisher
  analytics/              код analytics-processor: domain, service, repository, consumer
migrations/               схема PostgreSQL
queries/                  SQL-запросы к сохраненным счетчикам
frontend/                 демо-страница и трекер
nginx/                    конфиг nginx: раздает frontend/, проксирует /events в tracker-api
deploy/
  frontend/               образ nginx со страницей
  migrate/                образ, который применяет миграции
  k8s/                    манифесты Kubernetes для minikube
Dockerfile                образ сервиса, бинарник выбирает аргумент SERVICE
docker-compose.yml        Kafka, kafka-ui, PostgreSQL, топик, миграции, оба сервиса, nginx
```

## Формат событий

Запрос `POST /events`:

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

- `type`: `click`, `input`, `change`, `focus`, `blur`, `submit`, `page_view`. События неизвестного типа пропускаются, остальная пачка принимается.
- `element_id` обязателен. Пробелы по краям обрезаются, результат должен быть от 1 до 64 символов, иначе событие пропускается.
- `value` необязателен: значение слайдера, списка, чекбокса или радиокнопки. Для текстовых полей передается только длина ввода, сам текст не уходит со страницы.
- В пачке до 100 событий, тело запроса до 64 КБ.

Сообщение в Kafka содержит те же поля плюс `session_id` и `server_ts`.

## Гарантии доставки

At-least-once на всем пути: событие не теряется, но может прийти повторно.

- Трекер хранит неподтвержденные события в `localStorage` и повторяет отправку при сетевой ошибке и ответах `5xx` и `429`. События, не отправленные до закрытия страницы, уходят при следующем открытии.
- `tracker-api` отвечает `202 Accepted` только после подтверждения записи от Kafka.
- `analytics-processor` сначала записывает счетчики в PostgreSQL и только потом фиксирует смещение в Kafka.

Каждое событие получает `event_id` на клиенте, поэтому повтор можно отличить от нового события. Дедупликация пока не реализована.

## Запуск

Нужен Docker. Go нужен только для тестов и запуска сервисов на хосте.

1. Скопировать настройки и при желании поменять пароль:

   ```
   cp .env.example .env
   ```

2. Собрать образы и поднять все:

   ```
   docker compose up -d --build
   ```

   Compose создаст топик `user-events` с 3 партициями, применит миграции, затем запустит `tracker-api` и `analytics-processor`, а после готовности `tracker-api` запустит nginx.

   - Демо-страница: http://localhost:3000. Ее раздает nginx, он же проксирует `POST /events` в `tracker-api`, поэтому у страницы и API один origin и CORS не нужен. Правки в `frontend/` видны после перезагрузки страницы.
   - `tracker-api` напрямую: `curl http://localhost:8080/livez` возвращает `200` (порт TRACKER_API_PORT).
   - Интерфейс Kafka: http://localhost:8090.
   - PostgreSQL: порт 5433 (POSTGRES_PORT), счетчики появляются в таблицах через FLUSH_INTERVAL после событий, см. [Запросы](#запросы).

   Логи сервиса:

   ```
   docker compose logs -f analytics-processor
   ```

3. Остановка: `docker compose stop` сохраняет контейнеры, `docker compose down` удаляет их, `docker compose down -v` удаляет еще и данные Kafka и PostgreSQL.

После правки Go-кода пересобрать образ: `docker compose up -d --build tracker-api` (или `analytics-processor`).

### Сервис на хосте, под отладчиком

Остановить контейнер сервиса, иначе порт 8080 будет занят, а второй `analytics-processor` вступит в ту же группу и заберет часть партиций:

```
docker compose stop tracker-api
```

Для `tracker-api` еще переключить nginx на хост: в `.env` задать `TRACKER_API_URL=http://host.docker.internal:8080` и пересоздать только nginx (`--no-deps`, чтобы compose не запустил контейнер `tracker-api` обратно):

```
docker compose up -d --no-deps frontend
```

Затем запустить сервис. VS Code: конфигурации `tracker-api` и `analytics-processor` (F5) берут переменные из `.env`. Или из терминала, задав переменные из `.env.example` вручную, например для `tracker-api`:

Bash:

```
KAFKA_BROKERS=localhost:9092 KAFKA_TOPIC=user-events go run ./cmd/tracker-api
```

PowerShell:

```
$env:KAFKA_BROKERS="localhost:9092"; $env:KAFKA_TOPIC="user-events"; go run ./cmd/tracker-api
```

Пока `tracker-api` не запущен, nginx отвечает `502`, а трекер хранит события и повторяет отправку. Вернуть все в контейнеры: вернуть `TRACKER_API_URL=http://tracker-api:8080` в `.env` и выполнить `docker compose up -d`.

Тесты:

```
go test ./...
```

## Запуск в Kubernetes (minikube)

Тот же стек в локальном кластере, namespace `clickstream`: PostgreSQL и Kafka (StatefulSet, по одной реплике), Job `kafka-topics` и `migrate`, `tracker-api` и `analytics-processor` по 3 реплики и `frontend` (nginx со страницей). kafka-ui в кластер не входит.

Нужны Docker, [minikube](https://minikube.sigs.k8s.io/docs/start/) и kubectl.

1. Запустить кластер:

   ```
   minikube start --driver=docker --cpus=4 --memory=4096
   ```

2. Собрать образы и скопировать их в minikube. Кластер не видит локальные образы Docker, а манифесты не тянут их из реестра (`imagePullPolicy: IfNotPresent`):

   ```
   docker compose build
   minikube image load clickstream/tracker-api:dev
   minikube image load clickstream/analytics-processor:dev
   minikube image load clickstream/migrate:dev
   minikube image load clickstream/frontend:dev
   ```

3. Скопировать настройки базы и поменять пароль (только буквы и цифры, он попадает в `DATABASE_URL` как есть). Файл не коммитится:

   ```
   cp deploy/k8s/postgres.secret.env.example deploy/k8s/postgres.secret.env
   ```

4. Применить манифесты и дождаться сервисов:

   ```
   kubectl apply -k deploy/k8s
   kubectl -n clickstream wait --for=condition=available deployment --all --timeout=5m
   kubectl -n clickstream get pods
   ```

   Пока Kafka не поднялась и миграции не применены, поды `analytics-processor` могут перезапускаться (`CrashLoopBackOff`), это ожидаемо.

5. Открыть страницу. Сервисы не выставлены наружу кластера, поэтому нужен проброс порта (он занимает терминал, остановить Ctrl+C):

   ```
   kubectl -n clickstream port-forward svc/frontend 3000:80
   ```

   Демо-страница: http://localhost:3000. Если запущен и стек Docker Compose, порт 3000 занят: остановить его или пробросить другой порт, например `3001:80`. `tracker-api` напрямую: `kubectl -n clickstream port-forward svc/tracker-api 8080:8080`, затем `curl http://localhost:8080/livez`. Проброс идет в один под, а не через балансировку Service.

Логи всех реплик сервиса:

```
kubectl -n clickstream logs -f -l app.kubernetes.io/name=analytics-processor --prefix
```

Запросы из раздела [Запросы](#запросы) выполняются через под PostgreSQL. Bash:

```
kubectl -n clickstream exec -i postgres-0 -- psql -U clickstream -d clickstream < queries/stats.sql
```

PowerShell:

```
Get-Content queries/stats.sql | kubectl -n clickstream exec -i postgres-0 -- psql -U clickstream -d clickstream
```

Настройки сервисов лежат в `deploy/k8s/tracker-api.env` и `deploy/k8s/analytics-processor.env`. После правки снова выполнить `kubectl apply -k deploy/k8s`, поды этого сервиса заменятся сами.

После правки кода пересобрать образ, загрузить его в minikube и перезапустить поды (тег остается `:dev`, поэтому сам Kubernetes новый образ не заметит):

```
docker compose build tracker-api
minikube image load clickstream/tracker-api:dev
kubectl -n clickstream rollout restart deployment/tracker-api
```

Новые миграции: так же загрузить `clickstream/migrate:dev`, затем запустить Job заново. Завершенный Job `apply` не перезапускает, а шаблон его пода менять нельзя, поэтому сначала удалить его:

```
kubectl -n clickstream delete job migrate
kubectl apply -k deploy/k8s
```

Остановка:

- `minikube stop` останавливает весь кластер, данные сохраняются. `minikube start` поднимает его обратно.
- `kubectl -n clickstream scale deployment,statefulset --all --replicas=0` останавливает только этот проект, данные сохраняются. Запустить обратно: `kubectl apply -k deploy/k8s`, манифесты вернут число реплик.
- `kubectl delete -k deploy/k8s` удаляет namespace вместе с томами PostgreSQL и Kafka. В отличие от `docker compose down`, данные пропадают.

## Запросы

[queries/stats.sql](queries/stats.sql) читает счетчики, которые записал `analytics-processor`: события по окнам и типам, то же с разбивкой по элементам, итоги за период, топ вариантов выбора (пицца, размер, топпинги, количество) и конверсия воронки заказа.

Bash:

```
docker compose exec -T postgres psql -U clickstream -d clickstream < queries/stats.sql
```

PowerShell:

```
Get-Content queries/stats.sql | docker compose exec -T postgres psql -U clickstream -d clickstream
```

Необязательные переменные psql: `window_sec` (длина окна в секундах, должна совпадать с `WINDOW_SIZE`, по умолчанию `30`), `period` (на сколько назад от текущего момента, по умолчанию `1 day`), `top` (сколько значений показать в каждой группе, по умолчанию `3`):

```
docker compose exec -T postgres psql -U clickstream -d clickstream -v "period=1 hour" -v top=1 < queries/stats.sql
```

Счетчики складываются, одно окно может лежать в базе частями, поэтому запросы суммируют `count`. Значения по умолчанию (маргарита, средний размер, количество 1) событий не дают, поэтому топ показывает, как часто значение выбирали, а не что заказали.

## Ограничения

- Дедупликации нет. У каждого события есть `event_id`, но он пока не проверяется, поэтому повторно отправленная пачка посчитается дважды.
- Один брокер Kafka и один экземпляр PostgreSQL, и в Docker Compose, и в Kubernetes. У топика replication factor 1, поэтому потеря любого из них останавливает конвейер.
- Окна считаются по времени приема на сервере. События, которые трекер отправил с опозданием (после сбоя сети или при следующем открытии страницы), попадают в более позднее окно.
- У repository нет интеграционного теста на настоящем PostgreSQL.
