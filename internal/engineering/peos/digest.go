package peos

import (
	"fmt"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// VerifyContentDigest recomputes content's digest and confirms it matches
// both rev's projected ContentDigest and the integrity value recorded in
// the immutable PEOS revision itself (FF-009 §4.1). This is what makes the
// link between a capability revision and its structured content verifiable
// rather than a bare foreign key.
func VerifyContentDigest(rev engineering.RevisionEnvelope, content engineering.CapabilitySpecificationContent) error {
	computed, err := content.Digest()
	if err != nil {
		return err
	}
	want := "sha256:" + computed.Hex()
	if rev.IntegrityValue != want {
		return fmt.Errorf("%w: revision %s integrity value %q does not match recomputed content digest %q",
			ErrPayloadDigestMismatch, rev.Key, rev.IntegrityValue, want)
	}
	if !rev.ContentDigest.Equal(computed) {
		return fmt.Errorf("%w: revision %s projected content digest %q does not match recomputed %q",
			ErrPayloadDigestMismatch, rev.Key, rev.ContentDigest, computed)
	}
	return nil
}

// VerifyPayloadDigest recomputes payload's digest and confirms it matches
// digest. Used to verify a stored envelope's payload has not been altered.
func VerifyPayloadDigest(payload []byte, digest engineering.Digest) error {
	computed := engineering.ComputeDigest(payload)
	if !computed.Equal(digest) {
		return fmt.Errorf("%w: computed %s, stored %s", ErrPayloadDigestMismatch, computed, digest)
	}
	return nil
}
