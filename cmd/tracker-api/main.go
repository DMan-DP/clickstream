package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DMan-DP/clickstream/internal/tracker/config"
	"github.com/DMan-DP/clickstream/internal/tracker/handler"
	"golang.org/x/sync/errgroup"
)

func run() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.WithTimeout(handler.NewRouter(), 5*time.Second),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(signalCtx)

	log.Printf("Server starting on %s", cfg.HTTPAddr)

	g.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		stop()

		log.Println("Shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	log.Println("Server gracefully stopped")
	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
