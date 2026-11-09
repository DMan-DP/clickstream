package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DMan-DP/clickstream/internal/tracker/config"
	"github.com/DMan-DP/clickstream/internal/tracker/handler"
	"github.com/DMan-DP/clickstream/internal/tracker/service"
	"golang.org/x/sync/errgroup"
)

func run(logger *slog.Logger, cfg *config.Config) error {
	eventsService := service.NewEvents(logger.With("component", "event-service"))
	eventsHandler := handler.NewEvents(logger.With("component", "event-handler"), eventsService)

	routes := handler.Routes{
		Events: eventsHandler,
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.WithTimeout(handler.NewRouter(routes), 5*time.Second),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(signalCtx)

	logger.Info("server starting", "addr", cfg.HTTPAddr)

	g.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		stop()

		logger.Info("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	logger.Info("server gracefully stopped")
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
