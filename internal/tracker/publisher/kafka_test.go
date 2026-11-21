package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"github.com/DMan-DP/clickstream/internal/message"
	"github.com/DMan-DP/clickstream/internal/tracker/service"
	"github.com/IBM/sarama"
	"github.com/google/go-cmp/cmp"
)

// TestKafkaTimeouts checks the send budget on the config that NewProducer
// uses, not on the constants, so a config field set to another value is
// caught too.
func TestKafkaTimeouts(t *testing.T) {
	c := producerConfig()
	attempt := c.Net.DialTimeout + c.Net.WriteTimeout + c.Net.ReadTimeout
	retries := time.Duration(c.Producer.Retry.Max)
	worstCase := (retries+1)*attempt + retries*(c.Producer.Retry.Backoff+c.Metadata.Timeout)

	t.Run("worst case", func(t *testing.T) {
		if worstCase > sendBudget {
			t.Errorf("worst case %v exceeds sendBudget %v; lower the sarama timeouts or retries", worstCase, sendBudget)
		}
	})

	t.Run("broker timeout", func(t *testing.T) {
		if c.Producer.Timeout >= c.Net.ReadTimeout {
			t.Errorf("Producer.Timeout %v must be less than Net.ReadTimeout %v, otherwise the client gives up before the broker answers", c.Producer.Timeout, c.Net.ReadTimeout)
		}
	})

	t.Run("metadata timeout", func(t *testing.T) {
		if c.Metadata.Timeout < attempt {
			t.Errorf("Metadata.Timeout %v is shorter than one attempt %v", c.Metadata.Timeout, attempt)
		}
	})
}

// TestProducerConfigAcks checks that every message must be acknowledged
// by all in-sync replicas, as NewProducer promises.
func TestProducerConfigAcks(t *testing.T) {
	if got := producerConfig().Producer.RequiredAcks; got != sarama.WaitForAll {
		t.Errorf("RequiredAcks = %v, want %v (WaitForAll)", got, sarama.WaitForAll)
	}
}

// stubProducer is a sarama.SyncProducer for tests. SendMessages records the
// messages, signals on started and blocks until the test calls releaseOnce,
// then returns sendErr. Close only counts calls and never releases a pending
// send, so a test can check that Kafka.Close waits for background sends.
// Methods that are not overridden panic through the nil embedded interface.
type stubProducer struct {
	sarama.SyncProducer

	started     chan struct{}
	release     chan struct{}
	releaseOnce func()

	mu       sync.Mutex
	messages []*sarama.ProducerMessage

	sendErr error

	closeCalls atomic.Int32
}

func newStubProducer(sendErr error, startedCap int) *stubProducer {
	releaseChan := make(chan struct{})
	return &stubProducer{
		started:     make(chan struct{}, startedCap),
		release:     releaseChan,
		releaseOnce: sync.OnceFunc(func() { close(releaseChan) }),
		sendErr:     sendErr,
	}
}

func (p *stubProducer) SendMessages(msgs []*sarama.ProducerMessage) error {
	p.mu.Lock()
	p.messages = append(p.messages, msgs...)
	p.mu.Unlock()
	p.started <- struct{}{}
	<-p.release
	return p.sendErr
}

func (p *stubProducer) Close() error {
	p.closeCalls.Add(1)
	return nil
}

const testTopic = "test-events"

// testEvents returns a batch of three events from two sessions, so the
// message keys differ within one batch. The last event has a value.
func testEvents() []message.Event {
	sessionA := uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
	sessionB := uuid.MustParse("1b4e28ba-2fa1-41d2-883f-0016d3cca427")
	serverTS := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	return []message.Event{
		{
			EventID:   uuid.MustParse("05d4eec3-a4ce-465e-91fc-562da38b0c45"),
			SessionID: sessionA,
			Type:      "click",
			ElementID: "order-button",
			Value:     nil,
			ClientTS:  time.Date(2026, 11, 9, 11, 59, 58, 0, time.UTC),
			ServerTS:  serverTS,
		},
		{
			EventID:   uuid.MustParse("6fa459ea-ee8a-4ca4-894e-db77e160355e"),
			SessionID: sessionB,
			Type:      "click",
			ElementID: "menu-link",
			Value:     nil,
			ClientTS:  time.Date(2026, 11, 9, 11, 59, 59, 0, time.UTC),
			ServerTS:  serverTS,
		},
		{
			EventID:   uuid.MustParse("9b2c8f4e-3d1a-4e7b-a5c6-2f8e1d0b7a93"),
			SessionID: sessionA,
			Type:      "select",
			ElementID: "pizza-select",
			Value:     new("pepperoni"),
			ClientTS:  time.Date(2026, 11, 9, 11, 59, 59, 0, time.UTC),
			ServerTS:  serverTS,
		},
	}
}

func TestKafkaPublish(t *testing.T) {
	tests := []struct {
		name    string
		sendErr error
		// wantErr is checked with errors.Is. nil means Publish succeeds.
		// The error text is not part of the contract and is not checked.
		wantErr error
	}{
		{
			name:    "all acknowledged",
			sendErr: nil,
			wantErr: nil,
		},
		{
			// SendMessages reports failed messages as sarama.ProducerErrors.
			name: "some messages failed",
			sendErr: sarama.ProducerErrors{
				{Err: sarama.ErrNotEnoughReplicas},
				{Err: sarama.ErrRequestTimedOut},
			},
			wantErr: service.ErrUnavailable,
		},
		{
			name:    "send failed",
			sendErr: errors.New("broker is down"),
			wantErr: service.ErrUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStubProducer(tc.sendErr, 1)
			stub.releaseOnce()
			k := NewKafka(stub, testTopic)
			t.Cleanup(func() { k.Close() })

			events := testEvents()
			err := k.Publish(context.Background(), events)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Publish() error = %v, want %v", err, tc.wantErr)
			}

			// The batch reaches the producer whatever the result of the send.
			stub.mu.Lock()
			msgs := slices.Clone(stub.messages)
			stub.mu.Unlock()

			if len(msgs) != len(events) {
				t.Fatalf("producer got %d messages, want %d", len(msgs), len(events))
			}

			// One message per event. The order of messages is not part of
			// the contract, so each message is matched by its event id.
			want := make(map[uuid.UUID]message.Event, len(events))
			for _, e := range events {
				want[e.EventID] = e
			}

			for i, msg := range msgs {
				if msg.Topic != testTopic {
					t.Errorf("message %d topic = %q, want %q", i, msg.Topic, testTopic)
				}

				value, err := msg.Value.Encode()
				if err != nil {
					t.Fatalf("message %d: encode value: %v", i, err)
				}
				var got message.Event
				if err := json.Unmarshal(value, &got); err != nil {
					t.Fatalf("message %d: decode value: %v", i, err)
				}

				e, ok := want[got.EventID]
				if !ok {
					t.Errorf("message %d: event %s is unknown or sent twice", i, got.EventID)
					continue
				}
				delete(want, got.EventID)

				if diff := cmp.Diff(e, got); diff != "" {
					t.Errorf("message %d value mismatch (-want +got):\n%s", i, diff)
				}

				key, err := msg.Key.Encode()
				if err != nil {
					t.Fatalf("message %d: encode key: %v", i, err)
				}
				if want := e.SessionID.String(); string(key) != want {
					t.Errorf("message %d key = %q, want %q", i, key, want)
				}
			}
		})
	}
}

// The tests below run in a synctest bubble: synctest.Wait returns once every
// other goroutine is durably blocked, and timeouts use the fake clock, so the
// tests neither sleep nor depend on goroutine scheduling.

func TestKafkaCloseWaitsForSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub := newStubProducer(nil, 1)
		// Unblocks the send if the test fails before releasing it.
		t.Cleanup(stub.releaseOnce)
		k := NewKafka(stub, testTopic)

		published := make(chan error, 1)
		go func() { published <- k.Publish(context.Background(), testEvents()) }()
		<-stub.started

		closed := make(chan error, 1)
		go func() { closed <- k.Close() }()
		synctest.Wait()

		select {
		case err := <-closed:
			t.Fatalf("Close() returned %v while a send is running", err)
		default:
		}
		if n := stub.closeCalls.Load(); n != 0 {
			t.Fatalf("producer closed %d times while a send is running, want 0", n)
		}

		stub.releaseOnce()

		if err := <-published; err != nil {
			t.Errorf("Publish() error = %v, want nil", err)
		}
		if err := <-closed; err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
		if n := stub.closeCalls.Load(); n != 1 {
			t.Errorf("producer closed %d times, want 1", n)
		}
	})
}

func TestKafkaPublishDeadlineWhileSending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub := newStubProducer(nil, 1)
		k := NewKafka(stub, testTopic)
		// Cleanups run in reverse order: release the send, then close.
		t.Cleanup(func() { k.Close() })
		t.Cleanup(stub.releaseOnce)

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		err := k.Publish(ctx, testEvents())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Publish() error = %v, want %v", err, context.DeadlineExceeded)
		}

		select {
		case <-stub.started:
		default:
			t.Error("send did not start")
		}
	})
}

// TestKafkaPublishNoFreeSlot checks the limit through behaviour, not
// through the slots channel: maxInFlight sends run at the same time, and
// the next Publish waits until ctx is done without starting a send.
func TestKafkaPublishNoFreeSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub := newStubProducer(nil, maxInFlight+1)
		k := NewKafka(stub, testTopic)
		// Cleanups run in reverse order: release the sends, then close.
		t.Cleanup(func() { k.Close() })
		t.Cleanup(stub.releaseOnce)

		for range maxInFlight {
			go k.Publish(context.Background(), testEvents())
		}
		synctest.Wait()
		if n := len(stub.started); n != maxInFlight {
			t.Fatalf("%d sends running, want %d", n, maxInFlight)
		}

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		err := k.Publish(ctx, testEvents())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Publish() error = %v, want %v", err, context.DeadlineExceeded)
		}

		synctest.Wait()
		if n := len(stub.started); n != maxInFlight {
			t.Errorf("%d sends started, want %d: a send started without a free slot", n, maxInFlight)
		}
	})
}

// TestKafkaPublishReleasesSlot checks that a finished send gives its slot
// back: more than maxInFlight sends in a row all succeed.
func TestKafkaPublishReleasesSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub := newStubProducer(nil, maxInFlight+1)
		stub.releaseOnce()
		k := NewKafka(stub, testTopic)
		t.Cleanup(func() { k.Close() })

		for i := range maxInFlight + 1 {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			err := k.Publish(ctx, testEvents())
			cancel()
			if err != nil {
				t.Fatalf("Publish() #%d error = %v, want nil", i+1, err)
			}
		}
	})
}

func TestKafkaPublishAfterClose(t *testing.T) {
	stub := newStubProducer(nil, 1)
	stub.releaseOnce()
	k := NewKafka(stub, testTopic)

	if err := k.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	err := k.Publish(context.Background(), testEvents())
	if !errors.Is(err, service.ErrUnavailable) {
		t.Fatalf("Publish() error = %v, want %v", err, service.ErrUnavailable)
	}

	select {
	case <-stub.started:
		t.Error("send started after Close")
	default:
	}
	// The slot taken before the closed check must be given back.
	if n := len(k.slots); n != 0 {
		t.Errorf("%d send slots taken after Publish returned, want 0", n)
	}
}

// TestKafkaCloseWhileClosing checks that a Close made while another Close
// is waiting for a send returns nil at once, before the producer is closed.
func TestKafkaCloseWhileClosing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub := newStubProducer(nil, 1)
		// Unblocks the send if the test fails before releasing it.
		t.Cleanup(stub.releaseOnce)
		k := NewKafka(stub, testTopic)

		published := make(chan error, 1)
		go func() { published <- k.Publish(context.Background(), testEvents()) }()
		<-stub.started

		first := make(chan error, 1)
		go func() { first <- k.Close() }()
		synctest.Wait()

		second := make(chan error, 1)
		go func() { second <- k.Close() }()
		synctest.Wait()

		select {
		case err := <-second:
			if err != nil {
				t.Errorf("second Close() error = %v, want nil", err)
			}
		default:
			t.Fatal("second Close() waits for the first one, want it to return at once")
		}
		if n := stub.closeCalls.Load(); n != 0 {
			t.Fatalf("producer closed %d times while a send is running, want 0", n)
		}

		stub.releaseOnce()

		if err := <-published; err != nil {
			t.Errorf("Publish() error = %v, want nil", err)
		}
		if err := <-first; err != nil {
			t.Errorf("first Close() error = %v, want nil", err)
		}
		if n := stub.closeCalls.Load(); n != 1 {
			t.Errorf("producer closed %d times, want 1", n)
		}
	})
}

func TestKafkaCloseTwice(t *testing.T) {
	stub := newStubProducer(nil, 1)
	k := NewKafka(stub, testTopic)

	if err := k.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}
	if err := k.Close(); err != nil {
		t.Errorf("second Close() error = %v, want nil", err)
	}
	if n := stub.closeCalls.Load(); n != 1 {
		t.Errorf("producer closed %d times, want 1", n)
	}
}
