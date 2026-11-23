// Package handler serves the HTTP probes of analytics-processor.
package handler

import (
	"log/slog"
	"net/http"
	"time"
)

// livenessChecker reports whether the consumer loops are alive. It returns
// an error when at least one partition has not reported for too long.
type livenessChecker interface {
	Check(now time.Time) error
}

// Liveness is the liveness probe. It checks only the consumer loops, not
// Kafka or PostgreSQL: a restart does not fix an unavailable dependency.
type Liveness struct {
	logger  *slog.Logger
	checker livenessChecker
}

func NewLiveness(logger *slog.Logger, checker livenessChecker) *Liveness {
	return &Liveness{
		logger:  logger,
		checker: checker,
	}
}

// ServeHTTP answers 200 when every assigned partition has reported
// recently, and 503 otherwise. No partitions assigned is healthy. The
// response has no body; the reason of a failure is logged.
func (h *Liveness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.checker.Check(time.Now()); err != nil {
		h.logger.WarnContext(r.Context(), "liveness check failed", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}
