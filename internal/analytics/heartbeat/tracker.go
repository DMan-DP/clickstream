// Package heartbeat tracks when each partition consumer last showed signs of
// life. It backs the liveness probe of analytics-processor: a partition that
// has not reported for longer than the threshold means its loop is stuck,
// and only a restart can help.
package heartbeat

import (
	"fmt"
	"sync"
	"time"
)

// Tracker stores the time of the last heartbeat of each partition assigned
// to this instance. It is safe for concurrent use: ConsumeClaim goroutines
// write to it, the HTTP probe handler reads it.
type Tracker struct {
	mu         sync.Mutex
	partitions map[int32]time.Time
	threshold  time.Duration
}

// NewTracker returns a Tracker that reports a partition as stale when its
// last heartbeat is older than threshold. The threshold must be larger than
// the flush interval plus the flush timeout, otherwise a slow but working
// flush fails the probe.
//
// NewTracker panics if threshold is not positive.
func NewTracker(threshold time.Duration) *Tracker {
	if threshold <= 0 {
		panic("heartbeat: non-positive threshold")
	}

	return &Tracker{
		partitions: make(map[int32]time.Time),
		threshold:  threshold,
	}
}

// Record stores now as the last heartbeat of the partition. Call it when
// consuming of the partition starts and on every flush tick.
func (t *Tracker) Record(partition int32, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partitions[partition] = now
}

// Remove forgets the partition. Call it when its ConsumeClaim returns,
// otherwise a revoked partition stays in the tracker and soon looks stale
// forever.
func (t *Tracker) Remove(partition int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.partitions, partition)
}

// Check returns an error naming a stale partition if the last heartbeat of
// any partition is more than threshold before now, and nil otherwise. A
// heartbeat exactly threshold old is still fresh. If several partitions are
// stale, one of them is reported. With no partitions assigned Check returns
// nil: having no work is not a failure.
func (t *Tracker) Check(now time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for partition, lastSeen := range t.partitions {
		d := now.Sub(lastSeen)
		if d > t.threshold {
			return fmt.Errorf("partition %d is stale: last seen %v ago (threshold: %v)",
				partition, d.Round(time.Millisecond), t.threshold)
		}
	}

	return nil
}
