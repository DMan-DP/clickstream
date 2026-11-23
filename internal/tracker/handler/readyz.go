package handler

import (
	"net/http"
)

// shutdownFlag reports whether the shutdown has started. *atomic.Bool
// satisfies it, and the handler can only read the flag through it.
type shutdownFlag interface {
	Load() bool
}

// Readiness is the readiness probe. It does not check Kafka: the producer
// fails at start without brokers, and later outages are handled per
// request with 503.
type Readiness struct {
	shuttingDown shutdownFlag
}

func NewReadiness(shuttingDown shutdownFlag) *Readiness {
	return &Readiness{
		shuttingDown: shuttingDown,
	}
}

// ServeHTTP answers 200 while the service accepts events and 503 once the
// shutdown has started, so a load balancer stops sending new requests
// during the shutdown delay. The response has no body.
func (h *Readiness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.shuttingDown.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}
