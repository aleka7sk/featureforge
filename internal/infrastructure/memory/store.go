// Package memory implements the FF-009 repository contracts as an
// in-memory adapter -- a semantic test double, not a shortcut. It never
// imports the PEOS SDK: every value it stores is an opaque, already-encoded
// engineering.*Envelope (AD-005).
package memory

import (
	"runtime"
	"strconv"
	"sync"

	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// state holds every collection the store persists, keyed by its typed key
// (FF-009 §7).
type state struct {
	projects        map[domain.ProjectID]domain.Project
	featureCards    map[domain.FeatureCardID]domain.FeatureCard
	capabilityLinks map[domain.FeatureCardID]string

	artifacts            map[engineering.ArtifactKey]engineering.ArtifactEnvelope
	revisions            map[engineering.RevisionKey]engineering.RevisionEnvelope
	content              map[engineering.RevisionKey]engineering.CapabilitySpecificationContent
	records              map[engineering.RecordKey]engineering.RecordEnvelope
	order                map[engineering.RevisionKey]engineering.RevisionOrderMetadata
	traces               map[engineering.RevisionKey]engineering.RequirementCriterionTrace
	lifecycleDefinitions map[string]engineering.LifecycleDefinitionEnvelope
	lifecycleVersions    map[engineering.LifecycleDefinitionVersionKey]engineering.LifecycleDefinitionVersionEnvelope
	// acceptance is append-only per revision; order within a slice is
	// insertion order, which callers must not rely on -- ListByRevision
	// and ListByArtifact sort by (EffectiveAt, RecordID) explicitly.
	acceptance map[engineering.RevisionKey][]engineering.RevisionAcceptanceRecord
}

func newState() *state {
	return &state{
		projects:             make(map[domain.ProjectID]domain.Project),
		featureCards:         make(map[domain.FeatureCardID]domain.FeatureCard),
		capabilityLinks:      make(map[domain.FeatureCardID]string),
		artifacts:            make(map[engineering.ArtifactKey]engineering.ArtifactEnvelope),
		revisions:            make(map[engineering.RevisionKey]engineering.RevisionEnvelope),
		content:              make(map[engineering.RevisionKey]engineering.CapabilitySpecificationContent),
		records:              make(map[engineering.RecordKey]engineering.RecordEnvelope),
		order:                make(map[engineering.RevisionKey]engineering.RevisionOrderMetadata),
		traces:               make(map[engineering.RevisionKey]engineering.RequirementCriterionTrace),
		lifecycleDefinitions: make(map[string]engineering.LifecycleDefinitionEnvelope),
		lifecycleVersions:    make(map[engineering.LifecycleDefinitionVersionKey]engineering.LifecycleDefinitionVersionEnvelope),
		acceptance:           make(map[engineering.RevisionKey][]engineering.RevisionAcceptanceRecord),
	}
}

// FailureHook lets tests inject a write failure for rollback testing
// (FF-009 §7), without contriving a real conflict. kind identifies which
// collection the write targets (e.g. "artifact", "revision", "claim"); n is
// the 1-indexed ordinal of the write to that kind across the store's
// lifetime. Returning a non-nil error fails that write.
type FailureHook func(kind string, n int) error

// Store is the in-memory adapter's shared state. It is safe for concurrent
// use: every transaction holds the store's lock for its full duration,
// which trivially guarantees the isolation FF-009 §7 requires (no reader
// ever observes another transaction's uncommitted writes) without needing
// optimistic conflict detection.
type Store struct {
	mu sync.Mutex

	// holderMu guards holderGoroutine, which names the goroutine currently
	// inside a Do call (0 if none). It exists solely so a same-goroutine
	// reentrant Do call can be detected and rejected with
	// ErrNestedTransaction *before* attempting mu.Lock(), which would
	// otherwise deadlock (sync.Mutex is not reentrant, and Go deliberately
	// exposes no other portable way to test "is this my own lock").
	// holderMu itself is held only for the few instructions needed to read
	// or write holderGoroutine, never across a Do call's body.
	holderMu        sync.Mutex
	holderGoroutine uint64

	committed *state

	failureHook FailureHook
	writeCounts map[string]int
}

// currentGoroutineID extracts the calling goroutine's ID from its own stack
// trace header ("goroutine 123 [running]:"). This is the standard technique
// Go code uses to detect same-goroutine reentrancy, since the runtime
// exposes no public goroutine-identity API. It is used here only for the
// defensive nested-transaction check -- never for correctness-critical
// synchronization, which remains sync.Mutex-based throughout.
func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	b := buf[:n]
	const prefix = "goroutine "
	if len(b) > len(prefix) {
		b = b[len(prefix):]
	}
	end := 0
	for end < len(b) && b[end] != ' ' {
		end++
	}
	id, _ := strconv.ParseUint(string(b[:end]), 10, 64)
	return id
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		committed:   newState(),
		writeCounts: make(map[string]int),
	}
}

// SetFailureHook installs hook, replacing any previously installed hook.
// Pass nil to remove it. Not safe to call concurrently with an in-flight Do.
func (s *Store) SetFailureHook(hook FailureHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureHook = hook
}

// countWrite increments and returns the ordinal for kind, then invokes the
// installed failure hook, if any.
func (s *Store) countWrite(kind string) error {
	s.writeCounts[kind]++
	if s.failureHook == nil {
		return nil
	}
	return s.failureHook(kind, s.writeCounts[kind])
}
