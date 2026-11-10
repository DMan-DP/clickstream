// Package publisher writes accepted tracker events to the message broker.
package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DMan-DP/clickstream/internal/message"
	"github.com/IBM/sarama"
)

// Kafka publishes events to a Kafka topic through a synchronous producer.
// Each event becomes one message: the key is the session id, so all events
// of a session go to the same partition, and the value is the event as JSON.
type Kafka struct {
	producer sarama.SyncProducer
	topic    string
}

// NewKafka creates a publisher for the given brokers and topic. Every
// message must be acknowledged by all in-sync replicas. It returns an error
// if the brokers cannot be reached.
func NewKafka(brokers []string, topic string) (*Kafka, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Kafka{
		producer: producer,
		topic:    topic,
	}, nil
}

// Publish sends all messages in one call and returns when every message
// has been acknowledged or has failed. A non-nil error means the batch is
// not published, even if some messages were written. If ctx is already
// done, nothing is sent and the context error is returned. A send that
// has already started is not interrupted by ctx.
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

	err := k.producer.SendMessages(msgs)
	if err != nil {
		return fmt.Errorf("publish: send messages: %w", err)
	}

	return nil
}

// Close flushes pending messages and shuts down the producer. Call it after
// the last Publish.
func (k *Kafka) Close() error {
	return k.producer.Close()
}
