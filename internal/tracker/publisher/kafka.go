// Package publisher writes accepted tracker events to the message broker.
package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DMan-DP/clickstream/internal/message"
	"github.com/DMan-DP/clickstream/internal/tracker/service"
	"github.com/IBM/sarama"
)

// Kafka publishes events to a Kafka topic through a synchronous producer.
// Each event becomes one message: the key is the session id, so all events
// of a session go to the same partition, and the value is the event as JSON.
type Kafka struct {
	producer sarama.SyncProducer
	topic    string

	// slots limits concurrent sends to maxInFlight. A send takes a slot
	// before it starts and releases it when SendMessages returns.
	slots chan struct{}

	// mu guards closed. Publish checks closed and registers the send in wg
	// under the read lock, so Close cannot miss a send that has started.
	mu     sync.RWMutex
	closed bool

	// wg tracks sends running in the background. Close waits for them
	// before it closes the producer.
	wg sync.WaitGroup
}

// Kafka send budget. SyncProducer.SendMessages does not take a context, so
// the only way to bound it is the sarama timeouts below. Their worst case
// must not exceed sendBudget, the Kafka part of the 5s request budget set by
// handler.WithTimeout, which in turn is below the server WriteTimeout and
// the Shutdown timeout in main. TestKafkaTimeouts checks this; change the
// values together.
//
//	attempt    = dialTimeout + writeTimeout + readTimeout
//	worst case = (maxRetry + 1) * attempt + maxRetry * (retryBackoff + metadataTimeout)
//
// producerTimeout is how long the broker waits for in-sync replicas; it must
// be less than readTimeout, otherwise the client gives up before the broker
// answers. metadataTimeout bounds a whole metadata refresh (dial included),
// so it is not shorter than one attempt; a too short value fails
// NewSyncProducer while the broker is still starting.
const (
	sendBudget           = 4 * time.Second
	dialTimeout          = 150 * time.Millisecond
	writeTimeout         = 100 * time.Millisecond
	readTimeout          = 450 * time.Millisecond
	maxRetry             = 2
	retryBackoff         = 250 * time.Millisecond
	producerTimeout      = 350 * time.Millisecond
	metadataTimeout      = 700 * time.Millisecond
	metadataRetryMax     = 1
	metadataRetryBackoff = 100 * time.Millisecond
)

// maxInFlight limits concurrent sends, including those still running in the
// background after Publish returned. By Little's law, sends in flight =
// batches per second * send time: 1000/s * 150ms (p95 under load, not the
// mean) = 150. While Kafka is down every send holds a slot for about the
// worst case above, and under load longer: sarama queues retries per
// partition, so a new send may wait behind earlier ones. A request is still
// answered within its deadline, because Publish stops waiting when ctx is
// done. When all slots are busy, new batches wait for a free slot until ctx
// is done and are not sent at all.
//
// maxInFlight is also how many sends Close may wait for. Measured with Kafka
// down and all slots busy, Close took 14-19s; with 20 sends in flight it
// returned at once. stop_grace_period in docker-compose.yml must cover
// Shutdown plus this.
const maxInFlight = 150

// producerConfig returns the sarama config for NewProducer. The timeouts
// are tied to sendBudget, see TestKafkaTimeouts.
func producerConfig() *sarama.Config {
	c := sarama.NewConfig()
	c.Version = sarama.V4_1_0_0
	c.Producer.RequiredAcks = sarama.WaitForAll
	// Idempotent producer is off on purpose: sarama requires
	// Net.MaxOpenRequests = 1 for it, which limits one instance to a single
	// in-flight request per broker. Duplicates from producer retries are
	// acceptable (at-least-once) and are removed by deduplication on the
	// consumer side.
	c.Producer.Idempotent = false
	c.Producer.Return.Successes = true
	// Errors are returned by default; set explicitly because SendMessages
	// relies on both Successes and Errors to report the result of every message.
	c.Producer.Return.Errors = true
	c.Net.DialTimeout = dialTimeout
	c.Net.WriteTimeout = writeTimeout
	c.Net.ReadTimeout = readTimeout
	c.Producer.Retry.Max = maxRetry
	c.Producer.Retry.Backoff = retryBackoff
	c.Producer.Timeout = producerTimeout
	c.Metadata.Timeout = metadataTimeout
	c.Metadata.Retry.Max = metadataRetryMax
	c.Metadata.Retry.Backoff = metadataRetryBackoff
	return c
}

// NewProducer creates a synchronous Kafka producer configured for Kafka:
// all in-sync replicas must acknowledge each message, and retries and
// network timeouts keep one send within sendBudget. Pass the producer to
// NewKafka, which takes ownership of it.
func NewProducer(brokers []string) (sarama.SyncProducer, error) {
	producer, err := sarama.NewSyncProducer(brokers, producerConfig())
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}
	return producer, nil
}

// NewKafka creates a publisher that sends events to topic through producer.
// NewKafka takes ownership of producer: Close waits for background sends
// and then closes it, so the caller must not close producer itself.
func NewKafka(producer sarama.SyncProducer, topic string) *Kafka {
	return &Kafka{
		producer: producer,
		topic:    topic,
		slots:    make(chan struct{}, maxInFlight),
	}
}

// Publish sends all messages as one batch and waits for the result or for
// ctx to be done, whichever comes first. It is safe for concurrent use.
//
// A nil error means every message was acknowledged by all in-sync replicas.
//
// A non-nil error means the batch is treated as not published:
//   - if ctx is done (deadline or cancel), an error wrapping ctx.Err() is returned.
//     The send may still be running and may succeed later, so the batch
//     can end up in Kafka anyway;
//   - if Kafka fails to accept the batch, or Publish is called after Close,
//     the error wraps service.ErrUnavailable. Some messages may already be
//     written. Permanent Kafka errors, such as a too large message, are
//     reported the same way: event validation keeps messages small, so
//     they are not expected and are not told apart;
//   - any other error, such as a failed encoding, is internal and a retry
//     will not help.
//
// After a context error or service.ErrUnavailable the caller may retry the
// whole batch, so duplicates are possible.
//
// At most maxInFlight sends run at the same time. When all of them are busy,
// Publish waits for a free slot until ctx is done. A send that has started
// is not interrupted by ctx: it runs in the background until it completes or
// the sarama timeouts end it, and Close waits for it.
func (k *Kafka) Publish(ctx context.Context, messages []message.Event) error {
	msgs := make([]*sarama.ProducerMessage, 0, len(messages))
	for _, m := range messages {
		bytes, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("publish: marshal event %s: %w", m.EventID, err)
		}

		msg := &sarama.ProducerMessage{
			Topic: k.topic,
			Key:   sarama.StringEncoder(m.SessionID.String()),
			Value: sarama.ByteEncoder(bytes),
		}

		msgs = append(msgs, msg)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	select {
	case k.slots <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("publish: wait for send slot: %w", ctx.Err())
	}

	k.mu.RLock()
	if k.closed {
		k.mu.RUnlock()
		<-k.slots
		return fmt.Errorf("publish: %w: publisher is closed", service.ErrUnavailable)
	}

	result := make(chan error, 1)
	k.wg.Go(func() {
		defer func() { <-k.slots }()
		result <- k.producer.SendMessages(msgs)
	})

	k.mu.RUnlock()

	select {
	case err := <-result:
		if err != nil {
			failed, cause := len(msgs), err
			var errs sarama.ProducerErrors
			if errors.As(err, &errs) && len(errs) > 0 {
				failed, cause = len(errs), errs[0].Err
			}

			return fmt.Errorf("publish: %w: %d of %d messages failed: %v", service.ErrUnavailable, failed, len(msgs), cause)
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("publish: wait for send result: %w", ctx.Err())
	}
}

// Close stops accepting new batches, waits for the sends still running in
// the background and shuts down the producer. Close has no timeout of its
// own: while Kafka is down it may take as long as the slowest pending send,
// see maxInFlight. Publish calls made after
// Close return an error wrapping service.ErrUnavailable. Repeated calls do
// nothing and return nil. A call made while another Close is still waiting
// also returns nil at once, before the producer is closed: main calls Close
// once, so callers are not made to wait for each other as with sync.Once.
func (k *Kafka) Close() error {
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil
	}

	k.closed = true
	k.mu.Unlock()
	k.wg.Wait()

	err := k.producer.Close()
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
