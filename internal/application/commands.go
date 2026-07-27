package application

import (
	"strings"
	"time"
)

// Every command in this package follows the same shape (FF-010 §3):
// a struct of caller-supplied inputs, validated before any repository or
// recorder call; exactly one UnitOfWork.Do per command; a typed result
// naming the keys created; and identical re-execution with identical
// content is a no-op (idempotency flows from the repository contracts'
// own idempotent Put/Append semantics -- no command re-implements it).

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &fieldError{field: field, reason: "must not be empty"}
	}
	return nil
}

type fieldError struct {
	field  string
	reason string
}

func (e *fieldError) Error() string {
	return "application: " + e.field + " " + e.reason
}

func (e *fieldError) Unwrap() error { return ErrInvalidCommand }

// zeroTime reports whether t was never set by the caller.
func zeroTime(t time.Time) bool { return t.IsZero() }
