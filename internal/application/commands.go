package application

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aleka7sk/featureforge/internal/canonicaltime"
)

// Every command in this package follows the same shape (FF-010 §3):
// a struct of caller-supplied inputs, static validation before repository or
// recorder work, exactly one UnitOfWork.Do, and a typed result naming the
// persisted act. Exact replay is recognized from a validated stored semantic
// act before any write; repository Put/Append equality remains only the final
// create-path safety net.

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &fieldError{field: field, reason: "must not be empty"}
	}
	return nil
}

func requireOneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return &fieldError{field: field, reason: "must name a supported value"}
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

var governedIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func requireIdentity(field, value string) error {
	if !governedIdentity.MatchString(value) {
		return &fieldError{field: field, reason: "must match the governed identity grammar"}
	}
	return nil
}

func requireAcceptanceMemberIdentity(field, value string) error {
	return requireIdentity(field, value)
}

func invalidCommand(err error) error {
	if err == nil || errors.Is(err, ErrInvalidCommand) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrInvalidCommand, err)
}

func integrityError(reason string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrStoredStateIntegrity, reason)
	}
	return fmt.Errorf("%w: %s: %v", ErrStoredStateIntegrity, reason, err)
}

func normalizeTime(value time.Time) time.Time { return canonicaltime.Normalize(value) }

func optionalTime(value, candidate time.Time) time.Time {
	if value.IsZero() {
		return candidate
	}
	return normalizeTime(value)
}

func canonicalTimeEqual(left, right time.Time) bool {
	return canonicaltime.Equal(left, right)
}

func requireServerTime(value time.Time, act string) error {
	if value.IsZero() {
		return fmt.Errorf("application: clock returned a zero time for %s", act)
	}
	return nil
}
