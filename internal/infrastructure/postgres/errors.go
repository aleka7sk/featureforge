package postgres

import (
	"errors"
	"fmt"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATE codes this adapter interprets. Everything else is passed through.
const (
	sqlstateForeignKeyViolation = "23503"
	sqlstateUniqueViolation     = "23505"
	sqlstateSerializationFailue = "40001"
	sqlstateDeadlockDetected    = "40P01"
)

// mapError translates a driver error into the application error taxonomy so
// that callers of either adapter can match the same sentinels with errors.Is
// (FF-009 §8).
//
// It is called in exactly one place -- UnitOfWork.Do, on the way out -- and
// repository methods deliberately return raw driver errors. Mapping inside a
// repository would replace the *pgconn.PgError with a wrapped sentinel, and
// Do would then be unable to tell a retryable serialization failure from a
// permanent one, silently turning every contended write into an error instead
// of a retry.
//
// An error that is not a PgError -- most importantly an application sentinel
// a command returned directly from inside the Do callback -- is returned
// unchanged, so errors.Is still matches the original cause.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.Code {
	case sqlstateForeignKeyViolation:
		return fmt.Errorf("%w: %s: %s", application.ErrReferencedValueMissing, pgErr.ConstraintName, pgErr.Detail)
	case sqlstateUniqueViolation:
		if pgErr.ConstraintName == "revision_order_artifact_sequence_key" {
			return fmt.Errorf("%w: %s", application.ErrRevisionSequenceConflict, pgErr.Detail)
		}
		return fmt.Errorf("%w: %s: %s", application.ErrImmutableValueConflict, pgErr.ConstraintName, pgErr.Detail)
	case sqlstateSerializationFailue, sqlstateDeadlockDetected:
		// Reached only when UnitOfWork.Do has exhausted its retries; a
		// retryable conflict never surfaces to a caller.
		return fmt.Errorf("%w: %s", application.ErrTransactionAborted, pgErr.Message)
	default:
		return err
	}
}

// isRetryable reports whether err is a serialization failure or deadlock,
// which UnitOfWork.Do resolves by re-running the whole callback rather than
// by surfacing to the caller.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == sqlstateSerializationFailue || pgErr.Code == sqlstateDeadlockDetected
}
