package service

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// The expectations here come from the contracts of Add, FunnelRow, Reset,
// DropExpiredSessions and Flush and from the funnel decisions: steps are
// visit, pizza, contacts, submit; a session reaches each step at most
// once; skipped steps are filled in; a step is counted in the window of
// the event that took the session there; a session expires after the
// timeout without events, by the watermark.

var (
	sessionA = uuid.MustParse("6f1c2a3b-4d5e-4f60-8a7b-9c0d1e2f3a4b")
	sessionB = uuid.MustParse("0a1b2c3d-4e5f-4a6b-8c7d-8e9f0a1b2c3d")
)

// Event builders: one event of the page per funnel signal.

func pageView(s uuid.UUID, t time.Time) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypePageView, ElementID: "page", Time: t}
}

func pizzaChange(s uuid.UUID, t time.Time) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypeChange, ElementID: "pizza-select", Value: "pepperoni", Time: t}
}

// nameInput is an input of the name field; length is the sent Value.
func nameInput(s uuid.UUID, t time.Time, length int) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypeInput, ElementID: "customer-name", Value: strconv.Itoa(length), Time: t}
}

// addressInput is an input of the address field; length is the sent Value.
func addressInput(s uuid.UUID, t time.Time, length int) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypeInput, ElementID: "delivery-address", Value: strconv.Itoa(length), Time: t}
}

func formSubmit(s uuid.UUID, t time.Time) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypeSubmit, ElementID: "order-form", Time: t}
}

// focus is an event that means nothing for the funnel.
func focus(s uuid.UUID, t time.Time) domain.Event {
	return domain.Event{SessionID: s, Type: domain.EventTypeFocus, ElementID: "customer-name", Time: t}
}

func TestAggregatorFunnel(t *testing.T) {
	const size = 30 * time.Second

	// base is the start of a window; other times are built from it.
	base := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return base.Add(d) }

	const (
		visit    = domain.FunnelStepVisit
		pizza    = domain.FunnelStepPizza
		contacts = domain.FunnelStepContacts
		submit   = domain.FunnelStepSubmit
	)
	row := func(windowStart time.Time, step domain.FunnelStep, sessions int64) domain.FunnelRow {
		return domain.FunnelRow{WindowStart: windowStart, Step: step, Sessions: sessions}
	}

	tests := []struct {
		name   string
		events []domain.Event
		want   []domain.FunnelRow
	}{
		{
			name: "no events",
		},
		{
			name: "events without funnel meaning count no step",
			events: []domain.Event{
				focus(sessionA, at(1*time.Second)),
				{SessionID: sessionA, Type: domain.EventTypeClick, ElementID: "submit-order-button", Time: at(2 * time.Second)},
				{SessionID: sessionA, Type: domain.EventTypeBlur, ElementID: "customer-name", Time: at(3 * time.Second)},
			},
		},

		// One step at a time.
		{
			name:   "page_view gives visit",
			events: []domain.Event{pageView(sessionA, at(1*time.Second))},
			want:   []domain.FunnelRow{row(base, visit, 1)},
		},
		{
			name: "repeated page_view is counted once, also in another window",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pageView(sessionA, at(2*time.Second)),
				pageView(sessionA, at(size+1*time.Second)),
			},
			want: []domain.FunnelRow{row(base, visit, 1)},
		},
		{
			name: "sessions are counted separately",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pageView(sessionB, at(2*time.Second)),
			},
			want: []domain.FunnelRow{row(base, visit, 2)},
		},
		{
			name: "pizza after visit",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pizzaChange(sessionA, at(2*time.Second)),
				pizzaChange(sessionA, at(3*time.Second)),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1)},
		},

		// Skipped steps.
		{
			name:   "pizza without page_view fills in visit",
			events: []domain.Event{pizzaChange(sessionA, at(1*time.Second))},
			want:   []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1)},
		},
		{
			name: "contacts without page_view and pizza fill in both, in the window of contacts",
			events: []domain.Event{
				nameInput(sessionA, at(10*time.Second), 5),
				addressInput(sessionA, at(size+5*time.Second), 12),
			},
			want: []domain.FunnelRow{
				row(base.Add(size), visit, 1),
				row(base.Add(size), pizza, 1),
				row(base.Add(size), contacts, 1),
			},
		},

		// Contacts.
		{
			name: "name alone does not give contacts",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pizzaChange(sessionA, at(2*time.Second)),
				nameInput(sessionA, at(3*time.Second), 5),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1)},
		},
		{
			name: "address alone does not give contacts",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pizzaChange(sessionA, at(2*time.Second)),
				addressInput(sessionA, at(3*time.Second), 12),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1)},
		},
		{
			name: "fields of different sessions do not add up",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionB, at(2*time.Second), 12),
			},
		},
		{
			name: "contacts is counted in the window of the second field",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pizzaChange(sessionA, at(2*time.Second)),
				nameInput(sessionA, at(3*time.Second), 5),
				addressInput(sessionA, at(size+3*time.Second), 12),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1),
				row(base, pizza, 1),
				row(base.Add(size), contacts, 1),
			},
		},
		{
			name: "cleared name blocks contacts",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				nameInput(sessionA, at(2*time.Second), 0),
				addressInput(sessionA, at(3*time.Second), 12),
			},
		},
		{
			name: "contacts is counted once when a field is cleared and filled again",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				nameInput(sessionA, at(3*time.Second), 0),
				nameInput(sessionA, at(size+1*time.Second), 6),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1)},
		},
		{
			name: "contact field with a broken Value keeps the field state",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				{SessionID: sessionA, Type: domain.EventTypeInput, ElementID: "customer-name", Value: "abc", Time: at(2 * time.Second)},
				addressInput(sessionA, at(3*time.Second), 12),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1)},
		},

		// Submit.
		{
			name: "submit after contacts",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				formSubmit(sessionA, at(3*time.Second)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1), row(base, submit, 1),
			},
		},
		{
			name: "submit without contacts is not counted",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				pizzaChange(sessionA, at(2*time.Second)),
				nameInput(sessionA, at(3*time.Second), 5),
				formSubmit(sessionA, at(4*time.Second)),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1)},
		},
		{
			name: "submit before contacts is not counted later",
			events: []domain.Event{
				pageView(sessionA, at(1*time.Second)),
				formSubmit(sessionA, at(2*time.Second)),
				nameInput(sessionA, at(3*time.Second), 5),
				addressInput(sessionA, at(4*time.Second), 12),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1)},
		},
		{
			name: "submit with the name cleared after contacts is not counted",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				nameInput(sessionA, at(3*time.Second), 0),
				formSubmit(sessionA, at(4*time.Second)),
				// Filled again: the next attempt is the counted one.
				nameInput(sessionA, at(size+1*time.Second), 6),
				formSubmit(sessionA, at(size+2*time.Second)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1),
				row(base.Add(size), submit, 1),
			},
		},
		{
			name: "submit with the address cleared after contacts is not counted",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				addressInput(sessionA, at(3*time.Second), 0),
				formSubmit(sessionA, at(4*time.Second)),
			},
			want: []domain.FunnelRow{row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1)},
		},
		{
			name: "repeated submit is counted once",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				formSubmit(sessionA, at(3*time.Second)),
				formSubmit(sessionA, at(size+3*time.Second)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1), row(base, submit, 1),
			},
		},
		{
			name: "filling a field again after submit does not take the session back",
			events: []domain.Event{
				nameInput(sessionA, at(1*time.Second), 5),
				addressInput(sessionA, at(2*time.Second), 12),
				formSubmit(sessionA, at(3*time.Second)),
				nameInput(sessionA, at(4*time.Second), 0),
				nameInput(sessionA, at(5*time.Second), 7),
				formSubmit(sessionA, at(size+1*time.Second)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1), row(base, pizza, 1), row(base, contacts, 1), row(base, submit, 1),
			},
		},
		{
			name: "session over two windows: each step in the window of its event",
			events: []domain.Event{
				pageView(sessionA, at(5*time.Second)),
				nameInput(sessionA, at(20*time.Second), 5),
				addressInput(sessionA, at(40*time.Second), 12),
				formSubmit(sessionA, at(50*time.Second)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1),
				row(base.Add(size), pizza, 1),
				row(base.Add(size), contacts, 1),
				row(base.Add(size), submit, 1),
			},
		},

		// Session timeout, testSessionTimeout = 30 min.
		{
			name: "session starts again after the timeout",
			events: []domain.Event{
				pageView(sessionA, at(0)),
				pizzaChange(sessionA, at(31*time.Minute)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1),
				row(base.Add(31*time.Minute), visit, 1),
				row(base.Add(31*time.Minute), pizza, 1),
			},
		},
		{
			name: "pause of exactly the timeout keeps the session",
			events: []domain.Event{
				pageView(sessionA, at(0)),
				pageView(sessionA, at(30*time.Minute)),
			},
			want: []domain.FunnelRow{row(base, visit, 1)},
		},
		{
			name: "event without funnel meaning keeps the session alive",
			events: []domain.Event{
				pageView(sessionA, at(0)),
				focus(sessionA, at(20*time.Minute)),
				pageView(sessionA, at(40*time.Minute)),
			},
			want: []domain.FunnelRow{row(base, visit, 1)},
		},
		{
			name: "expired session forgets the contact fields",
			events: []domain.Event{
				nameInput(sessionA, at(0), 5),
				addressInput(sessionA, at(31*time.Minute), 12),
			},
		},
		{
			name: "timeout is measured by the watermark, not by the session's own events",
			events: []domain.Event{
				pageView(sessionA, at(0)),
				// Moves the watermark to 40 min.
				pageView(sessionB, at(40*time.Minute)),
				// 20 min after its last event, but 40 min by the watermark.
				pageView(sessionA, at(20*time.Minute)),
			},
			want: []domain.FunnelRow{
				row(base, visit, 1),
				row(base.Add(40*time.Minute), visit, 1),
				row(base.Add(20*time.Minute), visit, 1),
			},
		},
		{
			name: "late event does not move the last seen time back",
			events: []domain.Event{
				pageView(sessionA, at(20*time.Minute)),
				pageView(sessionA, at(0)),
				// 25 min after the latest event of the session.
				pageView(sessionA, at(45*time.Minute)),
			},
			want: []domain.FunnelRow{row(base.Add(20*time.Minute), visit, 1)},
		},
		{
			name: "late event is counted in its own window",
			events: []domain.Event{
				pageView(sessionA, at(size+5*time.Second)),
				pizzaChange(sessionA, at(5*time.Second)),
			},
			want: []domain.FunnelRow{row(base.Add(size), visit, 1), row(base, pizza, 1)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAggregator(size, testSessionTimeout)
			for _, e := range tc.events {
				a.Add(e)
			}
			checkFunnel(t, a.Snapshot().Funnel, tc.want)
		})
	}
}

// TestAggregatorSessionTimeout checks that the Aggregator uses the timeout
// passed to NewAggregator. The other tests use 30 min, the default
// SESSION_TIMEOUT, so a hard-coded timeout would pass them.
func TestAggregatorSessionTimeout(t *testing.T) {
	base := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	a := NewAggregator(30*time.Second, 10*time.Minute)
	a.Add(pageView(sessionA, base))
	// With a 30 min timeout the session would still be alive.
	a.Add(pageView(sessionA, base.Add(15*time.Minute)))

	checkFunnel(t, a.Snapshot().Funnel, []domain.FunnelRow{
		{WindowStart: base, Step: domain.FunnelStepVisit, Sessions: 1},
		{WindowStart: base.Add(15 * time.Minute), Step: domain.FunnelStepVisit, Sessions: 1},
	})
}

// TestAggregatorFunnelSnapshotAndReset checks that Snapshot does not reset
// the funnel counters and that Reset drops them but keeps the sessions.
func TestAggregatorFunnelSnapshotAndReset(t *testing.T) {
	base := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	a := NewAggregator(30*time.Second, testSessionTimeout)
	a.Add(pageView(sessionA, base.Add(1*time.Second)))

	want := []domain.FunnelRow{{WindowStart: base, Step: domain.FunnelStepVisit, Sessions: 1}}
	checkFunnel(t, a.Snapshot().Funnel, want)
	checkFunnel(t, a.Snapshot().Funnel, want)

	a.Reset()
	checkFunnel(t, a.Snapshot().Funnel, nil)

	// The session is kept: visit is not counted again, the next step is.
	a.Add(pageView(sessionA, base.Add(2*time.Second)))
	a.Add(pizzaChange(sessionA, base.Add(3*time.Second)))
	checkFunnel(t, a.Snapshot().Funnel, []domain.FunnelRow{
		{WindowStart: base, Step: domain.FunnelStepPizza, Sessions: 1},
	})
}

func TestAggregatorDropExpiredSessions(t *testing.T) {
	const size = 30 * time.Second
	base := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	sessionC := uuid.MustParse("9d8c7b6a-5f4e-4d3c-8b2a-1f0e9d8c7b6a")

	// addEvents leaves the watermark at 31 min: A is expired (31 min
	// without events), C is exactly at the timeout, B is fresh.
	addEvents := func(a *Aggregator) {
		a.Add(pageView(sessionA, base))
		a.Add(pageView(sessionC, base.Add(1*time.Minute)))
		a.Add(pageView(sessionB, base.Add(31*time.Minute)))
	}

	t.Run("removes expired sessions only", func(t *testing.T) {
		a := NewAggregator(size, testSessionTimeout)
		addEvents(a)

		a.DropExpiredSessions()

		if _, ok := a.sessions[sessionA]; ok {
			t.Errorf("expired session A is kept")
		}
		if _, ok := a.sessions[sessionC]; !ok {
			t.Errorf("session C at exactly the timeout is removed")
		}
		if _, ok := a.sessions[sessionB]; !ok {
			t.Errorf("fresh session B is removed")
		}
	})

	t.Run("does not change counts", func(t *testing.T) {
		a := NewAggregator(size, testSessionTimeout)
		addEvents(a)
		a.Add(pizzaChange(sessionB, base.Add(31*time.Minute)))
		before := a.Snapshot()

		a.DropExpiredSessions()

		after := a.Snapshot()
		checkSnapshot(t, after, before.Counters, before.Choices)
		checkFunnel(t, after.Funnel, before.Funnel)
	})

	t.Run("gives the same funnel as without it", func(t *testing.T) {
		dropped := NewAggregator(size, testSessionTimeout)
		kept := NewAggregator(size, testSessionTimeout)
		addEvents(dropped)
		addEvents(kept)

		dropped.DropExpiredSessions()

		// A comes back: an expired session starts again either way.
		// B goes on: a live session is not counted again.
		for _, e := range []domain.Event{
			pageView(sessionA, base.Add(32*time.Minute)),
			pageView(sessionB, base.Add(32*time.Minute)),
		} {
			dropped.Add(e)
			kept.Add(e)
		}

		want := kept.Snapshot().Funnel
		checkFunnel(t, dropped.Snapshot().Funnel, want)
		// Sanity check of the scenario itself: A got visit a second time.
		checkFunnel(t, want, []domain.FunnelRow{
			{WindowStart: base, Step: domain.FunnelStepVisit, Sessions: 1},
			{WindowStart: base.Add(1 * time.Minute), Step: domain.FunnelStepVisit, Sessions: 1},
			{WindowStart: base.Add(31 * time.Minute), Step: domain.FunnelStepVisit, Sessions: 1},
			{WindowStart: base.Add(32 * time.Minute), Step: domain.FunnelStepVisit, Sessions: 1},
		})
	})
}

func TestPartitionStatsFlushFunnel(t *testing.T) {
	const size = 30 * time.Second
	base := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	errSave := errors.New("database is down")

	t.Run("saves funnel rows and keeps sessions after the write", func(t *testing.T) {
		store := &stubStore{}
		p := NewStats(size, testSessionTimeout, store).NewPartition()

		p.Add(pageView(sessionA, base.Add(1*time.Second)))
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("first Flush() = %v, want nil", err)
		}

		p.Add(pageView(sessionA, base.Add(2*time.Second)))
		p.Add(pizzaChange(sessionA, base.Add(3*time.Second)))
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("second Flush() = %v, want nil", err)
		}

		if len(store.saved) != 2 {
			t.Fatalf("Save calls = %d, want 2", len(store.saved))
		}
		checkFunnel(t, store.saved[0].Funnel, []domain.FunnelRow{
			{WindowStart: base, Step: domain.FunnelStepVisit, Sessions: 1},
		})
		// visit is not saved again: the session lives between writes.
		checkFunnel(t, store.saved[1].Funnel, []domain.FunnelRow{
			{WindowStart: base, Step: domain.FunnelStepPizza, Sessions: 1},
		})
	})

	t.Run("drops expired sessions even if the save fails", func(t *testing.T) {
		store := &stubStore{errs: []error{errSave}}
		p := NewStats(size, testSessionTimeout, store).NewPartition()

		p.Add(pageView(sessionA, base))
		p.Add(pageView(sessionB, base.Add(31*time.Minute)))
		if err := p.Flush(context.Background()); !errors.Is(err, errSave) {
			t.Fatalf("Flush() = %v, want %v", err, errSave)
		}

		if _, ok := p.aggregator.sessions[sessionA]; ok {
			t.Errorf("expired session A is kept after a failed Flush")
		}
		if _, ok := p.aggregator.sessions[sessionB]; !ok {
			t.Errorf("fresh session B is removed")
		}
	})

	t.Run("drops expired sessions after a successful save", func(t *testing.T) {
		store := &stubStore{}
		p := NewStats(size, testSessionTimeout, store).NewPartition()

		p.Add(pageView(sessionA, base))
		p.Add(pageView(sessionB, base.Add(31*time.Minute)))
		if err := p.Flush(context.Background()); err != nil {
			t.Fatalf("Flush() = %v, want nil", err)
		}

		if _, ok := p.aggregator.sessions[sessionA]; ok {
			t.Errorf("expired session A is kept after a successful Flush")
		}
	})
}

// checkFunnel compares funnel rows with the wanted rows, ignoring their
// order.
func checkFunnel(t *testing.T, got, want []domain.FunnelRow) {
	t.Helper()

	got = slices.Clone(got)
	want = slices.Clone(want)
	slices.SortFunc(got, compareFunnelRows)
	slices.SortFunc(want, compareFunnelRows)
	if !slices.Equal(got, want) {
		t.Errorf("Funnel =\n%+v\nwant\n%+v", got, want)
	}
}

func compareFunnelRows(a, b domain.FunnelRow) int {
	return cmp.Or(
		a.WindowStart.Compare(b.WindowStart),
		cmp.Compare(a.Step, b.Step),
	)
}
