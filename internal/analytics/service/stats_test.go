package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// stubStore records every snapshot passed to Save and the ctx of the last
// call. The n-th call returns errs[n-1]; after errs run out, Save returns
// nil.
type stubStore struct {
	errs  []error
	saved []domain.Snapshot
	ctx   context.Context
}

func (s *stubStore) Save(ctx context.Context, snapshot domain.Snapshot) error {
	s.ctx = ctx
	s.saved = append(s.saved, snapshot)
	if n := len(s.saved); n <= len(s.errs) {
		return s.errs[n-1]
	}
	return nil
}

func TestPartitionStatsFlush(t *testing.T) {
	const size = 30 * time.Second
	base := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	click := domain.Event{Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: base.Add(1 * time.Second)}
	choice := domain.Event{Type: domain.EventTypeChange, ElementID: "size-large", Value: "large", Time: base.Add(2 * time.Second)}

	errSave := errors.New("database is down")

	t.Run("nothing added", func(t *testing.T) {
		store := &stubStore{}
		p := NewStats(size, store).NewPartition()

		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("Flush() = %v, want nil", err)
		}
		if len(store.saved) != 0 {
			t.Errorf("Save calls = %d, want 0", len(store.saved))
		}
	})

	t.Run("saves added counts once", func(t *testing.T) {
		store := &stubStore{}
		p := NewStats(size, store).NewPartition()
		p.Add(click)
		p.Add(choice)

		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("first Flush() = %v, want nil", err)
		}
		if len(store.saved) != 1 {
			t.Fatalf("Save calls = %d, want 1", len(store.saved))
		}
		if got := store.saved[0].WindowSize; got != size {
			t.Errorf("WindowSize = %v, want %v", got, size)
		}
		checkSnapshot(t, store.saved[0],
			[]domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
				{WindowStart: base, Type: domain.EventTypeChange, ElementID: "size-large", Count: 1},
			},
			[]domain.ChoiceRow{
				{WindowStart: base, ElementID: "size-large", Value: "large", Count: 1},
			},
		)

		// Nothing was added after the successful Flush.
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("second Flush() = %v, want nil", err)
		}
		if len(store.saved) != 1 {
			t.Errorf("Save calls after second Flush = %d, want 1", len(store.saved))
		}
	})

	t.Run("failed save is retried with new counts", func(t *testing.T) {
		store := &stubStore{errs: []error{errSave}}
		p := NewStats(size, store).NewPartition()

		p.Add(click)
		if err := p.Flush(context.Background()); !errors.Is(err, errSave) {
			t.Fatalf("first Flush() = %v, want %v", err, errSave)
		}

		p.Add(click)
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("second Flush() = %v, want nil", err)
		}
		if len(store.saved) != 2 {
			t.Fatalf("Save calls = %d, want 2", len(store.saved))
		}
		// The second Save gets the counts of both Flush calls.
		checkSnapshot(t, store.saved[1],
			[]domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 2},
			},
			nil,
		)
	})

	t.Run("failed save is retried without new counts", func(t *testing.T) {
		store := &stubStore{errs: []error{errSave}}
		p := NewStats(size, store).NewPartition()

		p.Add(click)
		if err := p.Flush(context.Background()); !errors.Is(err, errSave) {
			t.Fatalf("first Flush() = %v, want %v", err, errSave)
		}

		// Nothing is added between the two Flush calls.
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("second Flush() = %v, want nil", err)
		}
		if len(store.saved) != 2 {
			t.Fatalf("Save calls = %d, want 2", len(store.saved))
		}
		checkSnapshot(t, store.saved[1],
			[]domain.CounterRow{
				{WindowStart: base, Type: domain.EventTypeClick, ElementID: "submit-order-button", Count: 1},
			},
			nil,
		)
	})

	t.Run("passes ctx to store", func(t *testing.T) {
		store := &stubStore{}
		p := NewStats(size, store).NewPartition()
		p.Add(click)

		// Save may get ctx itself or a ctx derived from it, so the test
		// checks that a value of ctx reaches Save, not that ctx is equal.
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "flush")
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush() = %v, want nil", err)
		}
		if store.ctx == nil || store.ctx.Value(ctxKey{}) != "flush" {
			t.Errorf("Save got a ctx that is not derived from the Flush ctx")
		}
	})

	t.Run("partitions count separately", func(t *testing.T) {
		store := &stubStore{}
		stats := NewStats(size, store)
		p1 := stats.NewPartition()
		p2 := stats.NewPartition()
		p1.Add(click)

		if err := p2.Flush(context.Background()); err != nil {
			t.Fatalf("p2.Flush() = %v, want nil", err)
		}
		if len(store.saved) != 0 {
			t.Errorf("Save calls after p2.Flush = %d, want 0", len(store.saved))
		}
	})
}
