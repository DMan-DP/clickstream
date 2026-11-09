module github.com/DMan-DP/clickstream

go 1.27.3

// Planned dependencies (go get when first imported):
//   github.com/IBM/sarama             Kafka producer and consumer group
//   github.com/jmoiron/sqlx           database/sql extensions
//   github.com/jackc/pgx/v5           PostgreSQL driver, import pgx/v5/stdlib for sqlx

require (
	github.com/google/go-cmp v0.7.0
	golang.org/x/sync v0.24.0
)
