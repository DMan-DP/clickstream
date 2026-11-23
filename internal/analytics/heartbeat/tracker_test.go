package heartbeat_test

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/heartbeat"
)

const threshold = 10 * time.Second

// now is the moment of every Check. A fixed time keeps the tests free of
// sleeps and of the real clock.
var now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// beat is one Record call: the partition reported age before now.
type beat struct {
	partition int32
	age       time.Duration
}

func TestTrackerCheck(t *testing.T) {
	tests := []struct {
		name    string
		beats   []beat
		removed []int32
		// stale lists the partitions an error may name. Empty means Check
		// must return nil.
		stale []int32
	}{
		{
			name: "no partitions",
		},
		{
			name:  "just recorded",
			beats: []beat{{partition: 42, age: 0}},
		},
		{
			name:  "younger than threshold",
			beats: []beat{{partition: 42, age: threshold - time.Second}},
		},
		{
			name:  "exactly threshold old is fresh",
			beats: []beat{{partition: 42, age: threshold}},
		},
		{
			name:  "one nanosecond over threshold is stale",
			beats: []beat{{partition: 42, age: threshold + time.Nanosecond}},
			stale: []int32{42},
		},
		{
			name:  "long silence is stale",
			beats: []beat{{partition: 42, age: time.Hour}},
			stale: []int32{42},
		},
		{
			name: "one stale among fresh",
			beats: []beat{
				{partition: 1, age: time.Second},
				{partition: 42, age: 2 * threshold},
				{partition: 2, age: threshold},
			},
			stale: []int32{42},
		},
		{
			name: "several stale, any of them is reported",
			beats: []beat{
				{partition: 42, age: 2 * threshold},
				{partition: 1, age: time.Second},
				{partition: 77, age: 3 * threshold},
			},
			stale: []int32{42, 77},
		},
		{
			name: "new record refreshes a stale partition",
			beats: []beat{
				{partition: 42, age: time.Hour},
				{partition: 42, age: time.Second},
			},
		},
		{
			name: "older record after a fresh one wins",
			beats: []beat{
				{partition: 42, age: time.Second},
				{partition: 42, age: time.Hour},
			},
			stale: []int32{42},
		},
		{
			name:    "removed stale partition is forgotten",
			beats:   []beat{{partition: 42, age: time.Hour}},
			removed: []int32{42},
		},
		{
			name: "remove forgets only that partition",
			beats: []beat{
				{partition: 42, age: time.Hour},
				{partition: 77, age: time.Hour},
			},
			removed: []int32{42},
			stale:   []int32{77},
		},
		{
			name:    "remove of unknown partition is harmless",
			beats:   []beat{{partition: 42, age: time.Second}},
			removed: []int32{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := heartbeat.NewTracker(threshold)
			for _, b := range tt.beats {
				tr.Record(b.partition, now.Add(-b.age))
			}
			for _, p := range tt.removed {
				tr.Remove(p)
			}

			// Map iteration order is random, so a bug that depends on which
			// partition comes first shows up only sometimes. Repeat Check to
			// make it show up almost surely.
			for range checkRepeats {
				err := tr.Check(now)

				if len(tt.stale) == 0 {
					if err != nil {
						t.Fatalf("Check() = %v, want nil", err)
					}
					continue
				}
				if err == nil {
					t.Fatalf("Check() = nil, want error naming one of %v", tt.stale)
				}
				if !namesOneOf(err.Error(), tt.stale) {
					t.Fatalf("Check() = %q, want it to name one of %v", err, tt.stale)
				}
			}
		})
	}
}

// checkRepeats is how many times each table case calls Check.
const checkRepeats = 50

// The table above removes after all records, so it cannot express "record
// again after remove". This case does.
func TestTrackerRecordAfterRemove(t *testing.T) {
	tr := heartbeat.NewTracker(threshold)
	tr.Record(42, now.Add(-time.Second))
	tr.Remove(42)
	tr.Record(42, now.Add(-time.Hour))

	err := tr.Check(now)
	if err == nil || !namesOneOf(err.Error(), []int32{42}) {
		t.Fatalf("Check() = %v, want error naming partition 42", err)
	}
}

// Check uses the time passed in, not the clock: the same state is fresh or
// stale depending only on now.
func TestTrackerCheckUsesGivenTime(t *testing.T) {
	tr := heartbeat.NewTracker(threshold)
	tr.Record(42, now)

	if err := tr.Check(now.Add(threshold)); err != nil {
		t.Fatalf("Check(now+threshold) = %v, want nil", err)
	}
	if err := tr.Check(now.Add(threshold + time.Millisecond)); err == nil {
		t.Fatal("Check(now+threshold+1ms) = nil, want error")
	}
}

// Tracker is used from several ConsumeClaim goroutines and the probe handler
// at once. The test itself checks little; it exists for go test -race.
func TestTrackerConcurrentUse(t *testing.T) {
	tr := heartbeat.NewTracker(threshold)
	var wg sync.WaitGroup

	for p := range int32(8) {
		wg.Go(func() {
			for i := range 1000 {
				tr.Record(p, now.Add(time.Duration(i)*time.Millisecond))
				if i%10 == 0 {
					tr.Remove(p)
				}
			}
		})
	}
	wg.Go(func() {
		for range 1000 {
			_ = tr.Check(now)
		}
	})
	wg.Wait()

	if err := tr.Check(now); err != nil {
		t.Fatalf("Check() after concurrent use = %v, want nil", err)
	}
}

func TestNewTrackerPanicsOnNonPositiveThreshold(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Nanosecond, -threshold} {
		t.Run(d.String(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("NewTracker(%v) did not panic", d)
				}
			}()
			heartbeat.NewTracker(d)
		})
	}
}

// The smallest positive threshold is valid: the panic is only for zero and
// below.
func TestNewTrackerAcceptsSmallestThreshold(t *testing.T) {
	tr := heartbeat.NewTracker(time.Nanosecond)
	tr.Record(42, now)
	if err := tr.Check(now.Add(time.Nanosecond)); err != nil {
		t.Fatalf("Check(now+1ns) = %v, want nil", err)
	}
}

// namesOneOf reports whether msg contains the decimal number of one of the
// partitions. The tests use numbers like 42 and 77 that cannot appear in
// the durations of the message by accident.
func namesOneOf(msg string, partitions []int32) bool {
	for _, p := range partitions {
		if strings.Contains(msg, strconv.Itoa(int(p))) {
			return true
		}
	}
	return false
}
