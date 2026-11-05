module github.com/DMan-DP/clickstream

go 1.27.3

// Planned dependencies (go get when first imported):
//   github.com/IBM/sarama             Kafka producer and consumer group
//   github.com/jmoiron/sqlx           database/sql extensions
//   github.com/jackc/pgx/v5           PostgreSQL driver, import pgx/v5/stdlib for sqlx
//   github.com/google/uuid            event_id generation in tracker-api

require golang.org/x/sync v0.24.0
