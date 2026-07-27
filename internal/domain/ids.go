package domain

import (
	"fmt"
	"regexp"
)

// identityPattern is the FF-010 §1 identity validation rule, applied to every
// product-owned identity string.
var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

const maxIdentityBytes = 128

func validateIdentity(kind, value string) error {
	if len(value) == 0 {
		return fmt.Errorf("domain: %s must not be empty", kind)
	}
	if len(value) > maxIdentityBytes {
		return fmt.Errorf("domain: %s exceeds %d bytes: %q", kind, maxIdentityBytes, value)
	}
	if !identityPattern.MatchString(value) {
		return fmt.Errorf("domain: %s has invalid format: %q", kind, value)
	}
	return nil
}

// ProjectID is the product-owned identity of a Project.
type ProjectID struct {
	value string
}

// NewProjectID validates and returns a ProjectID.
func NewProjectID(value string) (ProjectID, error) {
	if err := validateIdentity("project id", value); err != nil {
		return ProjectID{}, fmt.Errorf("%w: %w", ErrProjectIDRequired, err)
	}
	return ProjectID{value: value}, nil
}

// String returns the underlying identity string.
func (id ProjectID) String() string { return id.value }

// IsZero reports whether id is the zero value.
func (id ProjectID) IsZero() bool { return id.value == "" }

// FeatureCardID is the product-owned identity of a FeatureCard.
type FeatureCardID struct {
	value string
}

// NewFeatureCardID validates and returns a FeatureCardID.
func NewFeatureCardID(value string) (FeatureCardID, error) {
	if err := validateIdentity("feature card id", value); err != nil {
		return FeatureCardID{}, fmt.Errorf("%w: %w", ErrFeatureIDRequired, err)
	}
	return FeatureCardID{value: value}, nil
}

// String returns the underlying identity string.
func (id FeatureCardID) String() string { return id.value }

// IsZero reports whether id is the zero value.
func (id FeatureCardID) IsZero() bool { return id.value == "" }
