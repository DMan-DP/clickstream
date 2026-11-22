// Command analytics-processor reads user events from Kafka as a member of
// a consumer group.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/config"
	"github.com/DMan-DP/clickstream/internal/analytics/consumer"
	"github.com/DMan-DP/clickstream/internal/analytics/repository"
	"github.com/DMan-DP/clickstream/internal/analytics/service"
	"github.com/IBM/sarama"
)

const dbConnTimeout = 10 * time.Second

func run(logger *slog.Logger, cfg *config.Config) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbCtx, cancel := context.WithTimeout(ctx, dbConnTimeout)
	db, err := repository.NewPostgres(dbCtx, cfg.DSN, cfg.DBMaxConns)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, db.Close())
	}()
	logger.Info("db connected", "max_conns", cfg.DBMaxConns)

	group, err := consumer.NewConsumerGroup(cfg.KafkaBrokers, cfg.KafkaGroupID)
	if err != nil {
		return err
	}

	stats := service.NewStats(cfg.WindowSize, db)
	handler := consumer.NewHandler(logger.With("component", "consumer"), stats, cfg.FlushInterval)

	var wg sync.WaitGroup
	wg.Go(func() {
		for err := range group.Errors() {
			logger.Error("consumer group error", "err", err)
		}
	})

	var consumeErr error

	logger.Info("consumer starting", "group_id", cfg.KafkaGroupID, "topic", cfg.KafkaTopic)
	for {
		if ctx.Err() != nil {
			break
		}

		err := group.Consume(ctx, []string{cfg.KafkaTopic}, handler)
		if errors.Is(err, sarama.ErrClosedConsumerGroup) ||
			errors.Is(err, context.Canceled) {
			break
		}

		if err != nil {
			consumeErr = err
			break
		}
	}

	stop()

	logger.Info("shutting down consumer")
	err = errors.Join(consumeErr, group.Close())

	wg.Wait()
	if err != nil {
		return err
	}

	logger.Info("consumer gracefully stopped")
	return nil
}

func main() {
	var lvl slog.LevelVar
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &lvl})
	logger := slog.New(h)
	slog.SetDefault(logger)

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	lvl.Set(cfg.LogLevel)

	err = run(logger, cfg)
	if err != nil {
		logger.Error("run failed", "err", err)
		os.Exit(1)
	}
}
