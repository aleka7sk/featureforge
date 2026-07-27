package peos

import (
	"errors"
	"fmt"
)

// Serialization errors (FF-009 §8).
var (
	ErrStoredPayloadInvalid     = errors.New("peos: stored payload is invalid")
	ErrPayloadDigestMismatch    = errors.New("peos: payload digest mismatch")
	ErrContentIntegrityMismatch = errors.New("peos: content integrity mismatch")
)

// wrapPEOS wraps a PEOS SDK error with additional context while preserving
// errors.Is matchability against every PEOS sentinel the SDK itself wraps
// (FF-009 §8 wrapping rule 2).
func wrapPEOS(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("peos: %s: %w", context, err)
}
