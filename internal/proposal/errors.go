package proposal

import "errors"

var (
	// ErrInvalidSourceReference reports a source string outside FF-024's
	// closed, canonical source-reference grammar.
	ErrInvalidSourceReference = errors.New("proposal: source reference is invalid")
	// ErrInvalidContextPack reports a zero, incomplete, or contradictory
	// context pack.
	ErrInvalidContextPack = errors.New("proposal: context pack is invalid")
	// ErrInvalidProposal reports malformed proposal content, identity, or
	// binding data.
	ErrInvalidProposal = errors.New("proposal: proposal is invalid")
)
