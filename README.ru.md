# clickstream

[English](README.md) | Русский

Аналитика действий пользователя на веб-странице. Страница отправляет клики, ввод и изменения полей на сервер, сервер публикует их в Kafka, отдельный сервис читает события, считает их по 30-секундным окнам и сохраняет счетчики в PostgreSQL.

## Как это работает

1. Страница `frontend/` с формой заказа пиццы: кнопки, текстовые поля, слайдер, чекбоксы, радиокнопки, выпадающий список, отправка формы.
2. Скрипт `frontend/tracker.js` собирает события в очередь и раз в 5 секунд отправляет их пачкой на `POST /events`.
3. Сервис `tracker-api` проверяет пачку и публикует каждое событие отдельным сообщением в топик Kafka `user-events`. Ключ сообщения `session_id`, поэтому события одной сессии попадают в одну партицию.
4. Сервис `analytics-processor` читает топик в consumer group, считает события по окнам в 30 секунд и записывает счетчики в таблицу `event_stats`.

Окна считаются по времени приема на сервере (UTC). Времени клиента сервис не доверяет, но сохраняет его в сообщении.

## Стек

- Go 1.27
- Kafka 4.1 (образ `apache/kafka`, режим KRaft без ZooKeeper), клиент [IBM/sarama](https://github.com/IBM/sarama)
- PostgreSQL 17, драйвер [pgx](https://github.com/jackc/pgx) через `database/sql` и [sqlx](https://github.com/jmoiron/sqlx)
- Миграции [goose](https://github.com/pressly/goose)
- Docker Compose
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
frontend/                 демо-страница и трекер
nginx/                    конфиг nginx: раздает frontend/, проксирует /events в tracker-api
docker-compose.yml        Kafka, kafka-ui, PostgreSQL, создание топика, миграции, nginx
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

Нужны Docker и Go.

1. Скопировать настройки и при желании поменять пароль:

   ```
   cp .env.example .env
   ```

2. Поднять инфраструктуру:

   ```
   docker compose up -d
   ```

   Compose создаст топик `user-events` с 3 партициями и применит миграции. Интерфейс Kafka откроется на http://localhost:8090, PostgreSQL слушает порт 5432.

   Демо-страница откроется на http://localhost:3000. Ее раздает nginx, он же проксирует `POST /events` в `tracker-api` на хосте (`TRACKER_API_URL` в `.env`), поэтому у страницы и API один origin и CORS не нужен. Правки в `frontend/` видны после перезагрузки страницы, перезапуск не нужен.

3. Запустить `tracker-api` на хосте. Переменные окружения описаны в `.env.example`.

   Bash:

   ```
   KAFKA_BROKERS=localhost:9092 KAFKA_TOPIC=user-events go run ./cmd/tracker-api
   ```

   PowerShell:

   ```
   $env:KAFKA_BROKERS="localhost:9092"; $env:KAFKA_TOPIC="user-events"; go run ./cmd/tracker-api
   ```

   Проверка: `curl http://localhost:8080/health` возвращает `200`. Затем открыть http://localhost:3000: запросы `POST /events` в DevTools получают `202`. Пока `tracker-api` не запущен, nginx отвечает `502`, а трекер хранит события и повторяет отправку.

Тесты:

```
go test ./...
```

## Состояние

Проект в разработке.

- [x] Инфраструктура в Docker Compose: Kafka, kafka-ui, PostgreSQL, топик, миграции
- [x] Демо-страница и трекер событий, раздает nginx с проксированием `/events` в `tracker-api`
- [x] Каркас `tracker-api`: конфигурация, health check, корректная остановка
- [ ] Прием событий `POST /events`
- [ ] Публикация в Kafka
- [ ] `analytics-processor`: чтение из Kafka, агрегация по окнам, запись в PostgreSQL
- [ ] Docker-образы сервисов
