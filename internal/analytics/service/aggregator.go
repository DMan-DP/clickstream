package service

import (
	"time"
	"uuid"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
)

// counterKey identifies one event counter: a window, an event type
// and a page element.
type counterKey struct {
	windowStart time.Time
	eventType   domain.EventType
	elementID   string
}

// choiceKey identifies one selected value: a window, a choice element
// and the value.
type choiceKey struct {
	windowStart time.Time
	elementID   string
	value       string
}

// stepKey identifies one funnel counter: a window and a funnel step.
type stepKey struct {
	windowStart time.Time
	step        domain.FunnelStep
}

// sessionState is what the aggregator keeps about one session between
// its events: only what is needed to handle the next event. step is the
// furthest funnel step reached; nameFilled and addressFilled are the
// current state of the contact fields and may go back to false, while
// step never goes back. lastSeen is the Time of the latest event of the
// session, compared with the watermark to expire the session.
type sessionState struct {
	step          domain.FunnelStep
	nameFilled    bool
	addressFilled bool
	lastSeen      time.Time
}

// expired reports whether the session has had no events for longer
// than timeout, measured by the watermark, not by the wall clock.
func (s sessionState) expired(watermark time.Time, timeout time.Duration) bool {
	return watermark.Sub(s.lastSeen) > timeout
}

// Aggregator counts events and selected values by window between
// writes. It is not safe for concurrent use: each partition has its
// own Aggregator in the goroutine of its ConsumeClaim.
type Aggregator struct {
	windowSize     time.Duration
	counters       map[counterKey]int64
	choices        map[choiceKey]int64
	funnel         map[stepKey]int64
	sessions       map[uuid.UUID]sessionState
	sessionTimeout time.Duration

	// watermark is the latest Time among the events seen so far: the clock
	// of the partition. Sessions expire by it, not by time.Now, so a replay
	// of old events gives the same result. Zero until the first event.
	watermark time.Time
}

// NewAggregator returns an empty Aggregator with windows of windowSize.
// A session with no events for longer than sessionTimeout, by the
// watermark, starts the funnel again. Both durations must be positive.
func NewAggregator(windowSize time.Duration, sessionTimeout time.Duration) *Aggregator {
	a := &Aggregator{
		windowSize:     windowSize,
		sessionTimeout: sessionTimeout,
		sessions:       make(map[uuid.UUID]sessionState),
	}
	a.Reset()
	return a
}

// Add counts e in the window of its Time. A choice event, see IsChoice,
// is also counted in the selected values. Add also moves the funnel
// state of the session of e: every step the session reaches for the
// first time is counted once, in the window of e. A session with no
// events for longer than the session timeout, by the watermark, starts
// the funnel again. Every event, even one that means nothing for the
// funnel, keeps its session alive.
func (a *Aggregator) Add(e domain.Event) {
	windowStart := domain.WindowStart(e.Time, a.windowSize)
	a.countEvent(windowStart, e)
	a.moveFunnel(windowStart, e)
}

// countEvent counts e in the event counters of windowStart and, for a
// choice event, in the selected values.
func (a *Aggregator) countEvent(windowStart time.Time, e domain.Event) {
	ck := counterKey{
		windowStart: windowStart,
		eventType:   e.Type,
		elementID:   e.ElementID,
	}
	a.counters[ck]++

	if e.IsChoice() {
		chk := choiceKey{
			windowStart: windowStart,
			elementID:   e.ElementID,
			value:       e.Value,
		}
		a.choices[chk]++
	}
}

// moveFunnel moves the watermark and the funnel state of the session of e,
// see Add for the rules. Every step the session reaches with e, the
// skipped ones included, is counted once in windowStart.
func (a *Aggregator) moveFunnel(windowStart time.Time, e domain.Event) {
	signal := e.FunnelSignal()
	a.watermark = maxTime(a.watermark, e.Time)
	state := a.sessions[e.SessionID]

	if state.expired(a.watermark, a.sessionTimeout) {
		state = sessionState{}
	}

	oldStep := state.step
	newStep := oldStep

	switch signal {
	case domain.FunnelSignalVisit:
		newStep = max(newStep, domain.FunnelStepVisit)
	case domain.FunnelSignalPizza:
		newStep = max(newStep, domain.FunnelStepPizza)
	case domain.FunnelSignalNameFilled, domain.FunnelSignalNameCleared:
		state.nameFilled = signal == domain.FunnelSignalNameFilled
	case domain.FunnelSignalAddressFilled, domain.FunnelSignalAddressCleared:
		state.addressFilled = signal == domain.FunnelSignalAddressFilled
	case domain.FunnelSignalSubmit:
		if oldStep >= domain.FunnelStepContacts &&
			state.addressFilled && state.nameFilled {
			newStep = domain.FunnelStepSubmit
		}
	}

	if state.nameFilled && state.addressFilled {
		newStep = max(newStep, domain.FunnelStepContacts)
	}

	for step := oldStep + 1; step <= newStep; step++ {
		key := stepKey{
			windowStart: windowStart,
			step:        step,
		}
		a.funnel[key]++
	}

	state.step = newStep
	state.lastSeen = maxTime(state.lastSeen, e.Time)

	a.sessions[e.SessionID] = state
}

// Snapshot returns the counts accumulated since the last Reset. It does
// not change the Aggregator: call Reset only after the snapshot is
// saved, so a failed write keeps the counts for the next attempt. Rows
// are in no particular order.
func (a *Aggregator) Snapshot() domain.Snapshot {
	counters := make([]domain.CounterRow, 0, len(a.counters))
	for ck, count := range a.counters {
		row := domain.CounterRow{
			WindowStart: ck.windowStart,
			Type:        ck.eventType,
			ElementID:   ck.elementID,
			Count:       count,
		}
		counters = append(counters, row)
	}

	choices := make([]domain.ChoiceRow, 0, len(a.choices))
	for chk, count := range a.choices {
		row := domain.ChoiceRow{
			WindowStart: chk.windowStart,
			ElementID:   chk.elementID,
			Value:       chk.value,
			Count:       count,
		}
		choices = append(choices, row)
	}

	funnel := make([]domain.FunnelRow, 0, len(a.funnel))
	for fk, sessions := range a.funnel {
		row := domain.FunnelRow{
			WindowStart: fk.windowStart,
			Step:        fk.step,
			Sessions:    sessions,
		}
		funnel = append(funnel, row)
	}

	return domain.Snapshot{
		WindowSize: a.windowSize,
		Counters:   counters,
		Choices:    choices,
		Funnel:     funnel,
	}
}

// Reset drops all counts, the funnel counters included. The session
// states and the watermark are kept: they live between writes. Call it
// after the Snapshot is saved and before the next Add: counts added in
// between are lost.
func (a *Aggregator) Reset() {
	a.counters = make(map[counterKey]int64)
	a.choices = make(map[choiceKey]int64)
	a.funnel = make(map[stepKey]int64)
}

// DropExpiredSessions removes the sessions that have expired by the
// current watermark, see expired. It only frees memory: Add starts an
// expired session again anyway, so no count changes. Call it on every
// Flush, whether the write succeeds or not: the sessions are not part
// of the Snapshot.
func (a *Aggregator) DropExpiredSessions() {
	for id, session := range a.sessions {
		if session.expired(a.watermark, a.sessionTimeout) {
			delete(a.sessions, id)
		}
	}
}

// maxTime returns the later of t1 and t2.
func maxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}
