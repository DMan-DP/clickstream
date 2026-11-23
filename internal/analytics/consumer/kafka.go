// Package consumer reads user events from Kafka as a member of a consumer
// group.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
	"github.com/DMan-DP/clickstream/internal/analytics/service"
	"github.com/DMan-DP/clickstream/internal/message"
	"github.com/IBM/sarama"
)

// FlushTimeout limits one save of the counts of a partition: on every flush
// tick and the last one when ConsumeClaim returns. The last save runs after
// the session context is cancelled, so it needs its own deadline: well below
// the rebalance timeout (60 s), during which the other members wait, and
// below the 10 s that docker stop gives before it kills the process. The
// liveness threshold in main is derived from it.
const FlushTimeout = 5 * time.Second

// PartitionTracker records that the loop of a partition is alive. The
// liveness probe reads these records and fails when a partition has been
// silent for too long.
type PartitionTracker interface {
	Record(partition int32, now time.Time)
	Remove(partition int32)
}

// Handler processes the messages of the partitions assigned to this group
// member. Sarama calls ConsumeClaim in a separate goroutine for each
// partition, so the methods may run concurrently.
type Handler struct {
	logger        *slog.Logger
	stats         *service.Stats
	flushInterval time.Duration
	tracker       PartitionTracker
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

// NewHandler creates a Handler that writes its logs to logger, counts the
// events of each partition with stats and saves the counts every
// flushInterval. On every flush tick it reports the partition to tracker,
// which must not be nil. flushInterval must be positive.
func NewHandler(logger *slog.Logger, stats *service.Stats, flushInterval time.Duration, tracker PartitionTracker) *Handler {
	return &Handler{
		logger:        logger,
		stats:         stats,
		flushInterval: flushInterval,
		tracker:       tracker,
	}
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
// channel is closed or the session ends. Events are counted in memory and
// saved every flushInterval; after a successful save the last read
// message is marked, so its offset is committed only when the counts are
// in the database. A failed save keeps the counts for the next tick. On
// return the counts are saved one last time with finalFlushTimeout.
func (h *Handler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	pStats := h.stats.NewPartition()
	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	h.tracker.Record(claim.Partition(), time.Now())

	var lastMsg *sarama.ConsumerMessage

	flush := func(ctx context.Context) {
		err := pStats.Flush(ctx)
		if err != nil {
			h.logger.ErrorContext(ctx, "flush stats", "partition", claim.Partition(), "err", err)
			return
		}

		if lastMsg != nil {
			sess.MarkMessage(lastMsg, "")
			lastMsg = nil
		}
	}

	defer func() {
		h.tracker.Remove(claim.Partition())
		ctx, cancel := context.WithTimeout(context.WithoutCancel(sess.Context()), FlushTimeout)
		defer cancel()
		flush(ctx)
	}()

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			var event message.Event
			err := json.Unmarshal(msg.Value, &event)
			lastMsg = msg
			if err != nil {
				h.logger.ErrorContext(sess.Context(), "unmarshal event", "partition", msg.Partition, "offset", msg.Offset, "err", err, "value", string(msg.Value))
				continue
			}

			pStats.Add(toDomainEvent(event))
			h.logger.DebugContext(sess.Context(), "event received", "partition", msg.Partition, "offset", msg.Offset, "event", event)
		case <-ticker.C:
			h.tracker.Record(claim.Partition(), time.Now())
			ctx, cancel := context.WithTimeout(sess.Context(), FlushTimeout)
			flush(ctx)
			cancel()
		case <-sess.Context().Done():
			return nil
		}
	}
}

// toDomainEvent converts a Kafka message event into the event of the
// aggregation. A missing Value becomes an empty string; the time of the
// event is ServerTS.
func toDomainEvent(e message.Event) domain.Event {
	value := ""
	if e.Value != nil {
		value = *e.Value
	}

	return domain.Event{
		SessionID: e.SessionID,
		EventID:   e.EventID,
		Type:      domain.EventType(e.Type),
		ElementID: e.ElementID,
		Value:     value,
		Time:      e.ServerTS,
	}
}
