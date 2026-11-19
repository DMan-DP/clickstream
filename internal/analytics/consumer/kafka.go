// Package consumer reads user events from Kafka as a member of a consumer
// group.
package consumer

import (
	"log/slog"

	"github.com/IBM/sarama"
)

// Handler processes the messages of the partitions assigned to this group
// member. Sarama calls ConsumeClaim in a separate goroutine for each
// partition, so the methods may run concurrently.
type Handler struct {
}

// The compiler checks that Handler implements the sarama interface.
var _ sarama.ConsumerGroupHandler = (*Handler)(nil)

// NewConsumerGroup creates a consumer group client connected to the brokers.
// The caller owns the group and must close it.
func NewConsumerGroup(brokers []string, groupID string) (sarama.ConsumerGroup, error) {
	panic("TODO")
}

// NewHandler creates a Handler that writes its logs to logger.
func NewHandler(logger *slog.Logger) *Handler {
	panic("TODO")
}

// Setup runs at the start of a new session, after a rebalance and before
// any ConsumeClaim goroutine starts.
func (h *Handler) Setup(sess sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup runs at the end of a session, after all ConsumeClaim goroutines
// have returned.
func (h *Handler) Cleanup(sess sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim reads the messages of one partition until the claim's
// channel is closed or the session ends.
func (h *Handler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	panic("TODO")
}
