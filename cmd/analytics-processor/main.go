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
	"github.com/DMan-DP/clickstream/internal/analytics/heartbeat"
	"github.com/DMan-DP/clickstream/internal/analytics/repository"
	"github.com/DMan-DP/clickstream/internal/analytics/service"
	"github.com/IBM/sarama"
)

const dbConnTimeout = 10 * time.Second

// heartbeatFactor sets the liveness threshold as a multiple of the longest
// normal gap between two heartbeats of a partition: the flush interval plus
// the flush timeout. With 3 a partition has to miss about three ticks in a
// row before the probe fails, so one slow flush does not restart the pod.
const heartbeatFactor = 3

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

	threshold := heartbeatFactor * (cfg.FlushInterval + consumer.FlushTimeout)
	tracker := heartbeat.NewTracker(threshold)
	stats := service.NewStats(cfg.WindowSize, cfg.SessionTimeout, db)
	handler := consumer.NewHandler(logger.With("component", "consumer"), stats, cfg.FlushInterval, tracker)

	var wg sync.WaitGroup
	wg.Go(func() {
		for err := range group.Errors() {
			logger.Error("consumer group error", "err", err)
		}
	})

	var consumeErr error

	logger.Info("consumer starting", "group_id", cfg.KafkaGroupID, "topic", cfg.KafkaTopic, "liveness_threshold_sec", threshold.Seconds())
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
