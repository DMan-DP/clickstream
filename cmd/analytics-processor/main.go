// Command analytics-processor reads user events from Kafka as a member of
// a consumer group.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/config"
	"github.com/DMan-DP/clickstream/internal/analytics/consumer"
	"github.com/DMan-DP/clickstream/internal/analytics/handler"
	"github.com/DMan-DP/clickstream/internal/analytics/heartbeat"
	"github.com/DMan-DP/clickstream/internal/analytics/repository"
	"github.com/DMan-DP/clickstream/internal/analytics/service"
	"github.com/IBM/sarama"
)

const (
	dbConnTimeout         = 10 * time.Second
	serverShutdownTimeout = 10 * time.Second
)

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

	threshold := heartbeatFactor * (cfg.FlushInterval + consumer.FlushTimeout)
	tracker := heartbeat.NewTracker(threshold)

	routes := handler.Routes{
		Livez:  handler.NewLiveness(logger.With("component", "liveness"), tracker),
		Readyz: handler.NewReadiness(logger.With("component", "readiness"), db),
	}

	server := &http.Server{
		Handler:           handler.NewRouter(routes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Listen before joining the consumer group: a busy port fails the start
	// here, before the instance joins the group and triggers a rebalance.
	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	// Closes the listener if run returns before Serve takes it over. After
	// Serve or Shutdown it is already closed, and the second Close only
	// returns an error, which is ignored.
	defer func() {
		_ = ln.Close()
	}()

	group, err := consumer.NewConsumerGroup(cfg.KafkaBrokers, cfg.KafkaGroupID)
	if err != nil {
		return err
	}

	stats := service.NewStats(cfg.WindowSize, cfg.SessionTimeout, db)
	consumerHandler := consumer.NewHandler(logger.With("component", "consumer"), stats, cfg.FlushInterval, tracker)

	// The server must answer probes while the consumer makes its final
	// flushes, so it is shut down only after the consumer group is closed.
	// A failure to serve stops the consumer too. Buffer 1: the goroutine
	// never blocks on send, even if run has not reached the read yet. The
	// channel is closed when the goroutine ends.
	serveErr := make(chan error, 1)
	logger.Info("server starting", "addr", ln.Addr().String())
	go func() {
		defer close(serveErr)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("server: %w", err)
			stop()
		}
	}()

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

		err := group.Consume(ctx, []string{cfg.KafkaTopic}, consumerHandler)
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

	logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()

	// Shutdown makes Serve return, so the read below does not
	// block for long: it gets the serve error, if any, or nil once the
	// goroutine has closed the channel.
	err = errors.Join(err, server.Shutdown(shutdownCtx), <-serveErr)
	if err != nil {
		return err
	}

	logger.Info("gracefully stopped")
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
