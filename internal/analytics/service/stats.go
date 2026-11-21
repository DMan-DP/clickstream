package service

import (
	"context"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// statsStore saves aggregated counts.
type statsStore interface {
	// Save adds the counts of snapshot to the stored counts in one
	// transaction. A nil error means all rows are committed. A non-nil
	// error means nothing is written, so the caller may retry with the
	// same snapshot. Saving the same snapshot twice after success counts
	// it twice.
	Save(ctx context.Context, snapshot domain.Snapshot) error
}

// Stats counts events by window and saves the counts to a store. One
// Stats serves the whole process; each partition gets its own
// PartitionStats.
type Stats struct {
	windowSize time.Duration
	store      statsStore
}

// NewStats returns a Stats with windows of windowSize that saves the
// counts to store. windowSize must be positive.
func NewStats(windowSize time.Duration, store statsStore) *Stats {
	return &Stats{
		windowSize: windowSize,
		store:      store,
	}
}

// PartitionStats counts the events of one partition and saves them on
// Flush. It is not safe for concurrent use: Add and Flush must be called
// from one goroutine.
type PartitionStats struct {
	aggregator *Aggregator
	store      statsStore
}

// NewPartition returns an empty PartitionStats for one partition. Call
// it at the start of ConsumeClaim; the result belongs to that goroutine.
func (s *Stats) NewPartition() *PartitionStats {
	return &PartitionStats{
		aggregator: NewAggregator(s.windowSize),
		store:      s.store,
	}
}

// Add counts e in memory. The count is saved by the next successful
// Flush.
func (p *PartitionStats) Add(e domain.Event) {
	p.aggregator.Add(e)
}

// Flush saves the counts added since the last successful Flush, passing
// ctx to the store. If there is nothing to save, it returns nil without
// calling the store. A nil error means the counts are committed, so the
// caller may mark the read messages as processed. On error the counts
// are kept and the next Flush saves them, together with any new ones.
func (p *PartitionStats) Flush(ctx context.Context) error {
	s := p.aggregator.Snapshot()
	if len(s.Counters) == 0 && len(s.Choices) == 0 {
		return nil
	}

	if err := p.store.Save(ctx, s); err != nil {
		return err
	}

	p.aggregator.Reset()
	return nil
}
