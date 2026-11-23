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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DMan-DP/clickstream/internal/tracker/config"
	"github.com/DMan-DP/clickstream/internal/tracker/handler"
	"github.com/DMan-DP/clickstream/internal/tracker/publisher"
	"github.com/DMan-DP/clickstream/internal/tracker/service"
	"golang.org/x/sync/errgroup"
)

const serverShutdownTimeout = 10 * time.Second

func run(logger *slog.Logger, cfg *config.Config) error {
	// Listen before connecting to Kafka: a busy port fails the start at
	// once, without waiting for the brokers, and Serve below never fails on
	// a busy port, so the shutdown goroutine does not sleep ShutdownDelay
	// for a server that never started.
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

	producer, err := publisher.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return err
	}

	kafka := publisher.NewKafka(producer, cfg.KafkaTopic)
	defer func() {
		if err := kafka.Close(); err != nil {
			logger.Error("close kafka", "err", err)
		}
	}()

	eventsService := service.NewEvents(logger.With("component", "event-service"), kafka)
	eventsHandler := handler.NewEvents(logger.With("component", "event-handler"), eventsService)

	// shuttingDown is set on SIGTERM; /readyz reads it and answers 503.
	var shuttingDown atomic.Bool

	routes := handler.Routes{
		Events: eventsHandler,
		Readyz: handler.NewReadiness(&shuttingDown),
	}

	timeoutHandler := handler.WithTimeout(handler.NewRouter(routes), 5*time.Second)
	accessLogHandler := handler.WithAccessLog(timeoutHandler, logger.With("component", "http"))
	server := &http.Server{
		Handler:           accessLogHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(signalCtx)

	logger.Info("server starting", "addr", ln.Addr().String())

	g.Go(func() error {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		stop()

		// The load balancer learns about the shutdown with a delay: until
		// then it may still send requests here. /readyz answers 503, and
		// the server keeps accepting events for ShutdownDelay.
		shuttingDown.Store(true)
		logger.Info("shutdown delay", "delay_sec", cfg.ShutdownDelay.Seconds())
		time.Sleep(cfg.ShutdownDelay)

		logger.Info("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("server: %w", err)
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
