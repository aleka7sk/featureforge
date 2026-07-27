package application

import "time"

// Clock supplies the current time to commands. Passed explicitly to every
// command constructor -- never called as a package-level time.Now (FF-010 §2).
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock, backed by the wall clock. It is the
// only place in this module time.Now may appear.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock is a deterministic test Clock. It advances only when told.
type FixedClock struct {
	now time.Time
}

// NewFixedClock returns a FixedClock starting at now.
func NewFixedClock(now time.Time) *FixedClock {
	return &FixedClock{now: now}
}

// Now returns the clock's current fixed time.
func (c *FixedClock) Now() time.Time { return c.now }

// Advance moves the clock forward by d and returns the new time.
func (c *FixedClock) Advance(d time.Duration) time.Time {
	c.now = c.now.Add(d)
	return c.now
}

// Set moves the clock to t and returns it.
func (c *FixedClock) Set(t time.Time) time.Time {
	c.now = t
	return c.now
}
