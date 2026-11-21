package domain

import (
	"testing"
	"time"
)

func TestWindowStart(t *testing.T) {
	moscow := time.FixedZone("UTC+3", 3*60*60)
	newYork := time.FixedZone("UTC-5", -5*60*60)

	tests := []struct {
		name string
		t    time.Time
		size time.Duration
		want time.Time
	}{
		{
			name: "middle of window",
			t:    time.Date(2026, 11, 20, 10, 15, 17, 0, time.UTC),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "exactly on boundary starts new window",
			t:    time.Date(2026, 11, 20, 10, 15, 30, 0, time.UTC),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 10, 15, 30, 0, time.UTC),
		},
		{
			name: "one nanosecond before boundary",
			t:    time.Date(2026, 11, 20, 10, 15, 29, 999999999, time.UTC),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "midnight",
			t:    time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "positive offset zone",
			t:    time.Date(2026, 11, 20, 13, 15, 17, 0, moscow),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "negative offset zone crosses date",
			t:    time.Date(2026, 11, 19, 23, 59, 45, 0, newYork),
			size: 30 * time.Second,
			want: time.Date(2026, 11, 20, 4, 59, 30, 0, time.UTC),
		},
		{
			name: "one minute window",
			t:    time.Date(2026, 11, 20, 10, 15, 59, 0, time.UTC),
			size: time.Minute,
			want: time.Date(2026, 11, 20, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "five minute window",
			t:    time.Date(2026, 11, 20, 10, 17, 17, 0, time.UTC),
			size: 5 * time.Minute,
			want: time.Date(2026, 11, 20, 10, 15, 0, 0, time.UTC),
		},
		{
			// 7s does not divide a day, so windows counted from the zero
			// time and from the Unix epoch differ. From the zero time to
			// 10:15:17 there are 63930766517 s; 63930766517 mod 7 = 2,
			// so the window starts 2 s earlier.
			name: "window not dividing a day counts from zero time",
			t:    time.Date(2026, 11, 20, 10, 15, 17, 0, time.UTC),
			size: 7 * time.Second,
			want: time.Date(2026, 11, 20, 10, 15, 15, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window := WindowStart(tc.t, tc.size)
			if !window.Equal(tc.want) {
				t.Errorf("WindowStart(%v, %v) = %v, want %v", tc.t, tc.size, window, tc.want)
			}

			if window.Location() != time.UTC {
				t.Errorf("WindowStart() location = %v, want %v", window.Location(), time.UTC)
			}
		})
	}
}
