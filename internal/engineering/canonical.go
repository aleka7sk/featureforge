package engineering

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Digest is a lowercase-hex SHA-256 digest of a canonical JSON value
// (FF-009 §4.1).
type Digest struct {
	hex string
}

// NewDigest validates and returns a Digest from a lowercase-hex string.
func NewDigest(value string) (Digest, error) {
	if len(value) != sha256.Size*2 {
		return Digest{}, fmt.Errorf("%w: digest must be %d hex characters, got %d", ErrInvalidContent, sha256.Size*2, len(value))
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return Digest{}, fmt.Errorf("%w: digest must be lowercase hex", ErrInvalidContent)
		}
	}
	return Digest{hex: value}, nil
}

// ComputeDigest returns the lowercase-hex SHA-256 digest of canonicalJSON.
func ComputeDigest(canonicalJSON []byte) Digest {
	sum := sha256.Sum256(canonicalJSON)
	return Digest{hex: hex.EncodeToString(sum[:])}
}

// Hex returns the digest's lowercase-hex string.
func (d Digest) Hex() string { return d.hex }

// String returns the digest's lowercase-hex string.
func (d Digest) String() string { return d.hex }

// IsZero reports whether d is the zero value.
func (d Digest) IsZero() bool { return d.hex == "" }

// Equal reports whether d and other are the same digest.
func (d Digest) Equal(other Digest) bool { return d.hex == other.hex }

// canonicalEncode marshals v with HTML escaping disabled and no trailing
// newline (FF-009 §4.1 rule 3). Go's struct JSON marshaling emits fields in
// declaration order, so field order (rule 1) is the caller's responsibility
// via its wire struct's field declarations; nil-slice-to-null (violating
// rule 4) is likewise the caller's responsibility to normalize before calling
// this function.
func canonicalEncode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
