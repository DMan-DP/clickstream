package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// pingTimeout bounds the database ping of the readiness probe. It must be
// shorter than the probe timeout (timeoutSeconds in Kubernetes), otherwise
// the probe fails on its own timeout and the log has no reason.
const pingTimeout = 1 * time.Second

// pinger checks that the database is reachable.
type pinger interface {
	PingContext(ctx context.Context) error
}

// Readiness is the readiness probe: the instance is ready when it can
// write window counts to PostgreSQL.
type Readiness struct {
	logger *slog.Logger
	db     pinger
}

func NewReadiness(logger *slog.Logger, db pinger) *Readiness {
	return &Readiness{
		logger: logger,
		db:     db,
	}
}

// ServeHTTP answers 200 when the database answers a ping within
// pingTimeout, and 503 otherwise. The response has no body; the reason of
// a failure is logged.
func (h *Readiness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		h.logger.WarnContext(ctx, "readiness check failed", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}
