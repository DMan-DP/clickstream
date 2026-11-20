// Package consumer reads user events from Kafka as a member of a consumer
// group.
package consumer

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/DMan-DP/clickstream/internal/message"
	"github.com/IBM/sarama"
)

// Handler processes the messages of the partitions assigned to this group
// member. Sarama calls ConsumeClaim in a separate goroutine for each
// partition, so the methods may run concurrently.
type Handler struct {
	logger *slog.Logger
}

// The compiler checks that Handler implements the sarama interface.
var _ sarama.ConsumerGroupHandler = (*Handler)(nil)

// consumerConfig returns the sarama config for NewConsumerGroup.
func consumerConfig() *sarama.Config {
	c := sarama.NewConfig()
	c.Version = sarama.V4_1_0_0
	// Auto-commit sends only offsets marked with MarkMessage, so a message is
	// committed only after the handler has processed it. Set explicitly
	// because the at-least-once guarantee relies on it.
	c.Consumer.Offsets.AutoCommit.Enable = true
	c.Consumer.Offsets.Initial = sarama.OffsetOldest
	c.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategySticky()}
	c.Consumer.Return.Errors = true
	return c
}

// NewConsumerGroup creates a consumer group client connected to the brokers.
// The caller owns the group and must close it.
func NewConsumerGroup(brokers []string, groupID string) (sarama.ConsumerGroup, error) {
	client, err := sarama.NewConsumerGroup(brokers, groupID, consumerConfig())
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer group: %w", err)
	}

	return client, nil
}

// NewHandler creates a Handler that writes its logs to logger.
func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}

// Setup runs at the start of a new session, after a rebalance and before
// any ConsumeClaim goroutine starts.
func (h *Handler) Setup(sess sarama.ConsumerGroupSession) error {
	h.logger.InfoContext(sess.Context(), "session started", "member_id", sess.MemberID(), "generation_id", sess.GenerationID(), "claims", sess.Claims())
	return nil
}

// Cleanup runs at the end of a session, after all ConsumeClaim goroutines
// have returned.
func (h *Handler) Cleanup(sess sarama.ConsumerGroupSession) error {
	h.logger.InfoContext(sess.Context(), "session ended", "member_id", sess.MemberID(), "generation_id", sess.GenerationID(), "claims", sess.Claims())
	return nil
}

// ConsumeClaim reads the messages of one partition until the claim's
// channel is closed or the session ends.
func (h *Handler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			var event message.Event
			err := json.Unmarshal(msg.Value, &event)
			if err != nil {
				h.logger.ErrorContext(sess.Context(), "unmarshal event", "partition", msg.Partition, "offset", msg.Offset, "err", err, "value", string(msg.Value))
			} else {
				h.logger.DebugContext(sess.Context(), "event received", "partition", msg.Partition, "offset", msg.Offset, "event", event)
			}
			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}
