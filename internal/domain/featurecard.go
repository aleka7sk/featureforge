package domain

import (
	"strings"
	"time"
)

// FeatureCard is the operational entry point for one capability (FF-002 §2).
// It carries no derived engineering state: no current revision, no readiness,
// no lifecycle state, and no collection of requirements, decisions, or claims.
//
// CapabilityArtifactID is the one permitted link into engineering state,
// stored as a plain product-owned string — never a PEOS type. It starts
// unset and is completed exactly once via WithCapabilityArtifactID; see
// internal/application's FeatureCardRepository.LinkCapability for how that
// one-time completion is persisted without violating create-only Put
// semantics (documented in the M.3 implementation report).
type FeatureCard struct {
	id                    FeatureCardID
	projectID             ProjectID
	title                 string
	description           string
	createdAt             time.Time
	capabilityArtifactID  string
	hasCapabilityArtifact bool
}

// NewFeatureCard validates its arguments and returns a FeatureCard with no
// capability link. createdAt is supplied explicitly by the caller.
func NewFeatureCard(id FeatureCardID, projectID ProjectID, title, description string, createdAt time.Time) (FeatureCard, error) {
	if id.IsZero() {
		return FeatureCard{}, ErrFeatureIDRequired
	}
	if projectID.IsZero() {
		return FeatureCard{}, ErrFeatureProjectRequired
	}
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return FeatureCard{}, ErrInvalidFeatureTitle
	}
	return FeatureCard{
		id:          id,
		projectID:   projectID,
		title:       trimmed,
		description: strings.TrimSpace(description),
		createdAt:   createdAt,
	}, nil
}

// WithCapabilityArtifactID returns a copy of c with its capability link set.
// It never mutates c. artifactID must be a non-empty identity string; the
// caller (internal/application) is responsible for enforcing that a card is
// linked to at most one capability across its lifetime.
func (c FeatureCard) WithCapabilityArtifactID(artifactID string) (FeatureCard, error) {
	if err := validateIdentity("capability artifact id", artifactID); err != nil {
		return FeatureCard{}, err
	}
	c.capabilityArtifactID = artifactID
	c.hasCapabilityArtifact = true
	return c, nil
}

// ID returns the feature card's identity.
func (c FeatureCard) ID() FeatureCardID { return c.id }

// ProjectID returns the identity of the project this card belongs to.
func (c FeatureCard) ProjectID() ProjectID { return c.projectID }

// Title returns the feature card's title.
func (c FeatureCard) Title() string { return c.title }

// Description returns the feature card's description or intent.
func (c FeatureCard) Description() string { return c.description }

// CreatedAt returns when the feature card was created.
func (c FeatureCard) CreatedAt() time.Time { return c.createdAt }

// CapabilityArtifactID returns the linked capability artifact identity, if any.
func (c FeatureCard) CapabilityArtifactID() (string, bool) {
	return c.capabilityArtifactID, c.hasCapabilityArtifact
}

// SameEstablishment reports whether c and other carry the same stable base
// FeatureCard value. The separately persisted, monotonic capability link is
// deliberately excluded: materializing that link on a read must not turn an
// otherwise identical create-only Put into a conflict.
func (c FeatureCard) SameEstablishment(other FeatureCard) bool {
	return c.id == other.id &&
		c.projectID == other.projectID &&
		c.title == other.title &&
		c.description == other.description &&
		c.createdAt.Equal(other.createdAt)
}

// IsZero reports whether c is the zero value.
func (c FeatureCard) IsZero() bool { return c.id.IsZero() }
