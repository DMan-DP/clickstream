package domain

import "time"

// WindowStart returns the start of the window that contains t: t truncated
// to a multiple of size since the zero time, in UTC. Windows are half-open,
// [start, start+size), so t exactly on a boundary starts a new window.
// size must be positive.
func WindowStart(t time.Time, size time.Duration) time.Time {
	return t.UTC().Truncate(size)
}
