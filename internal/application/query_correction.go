package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// CorrectionEdge is one correction reference among claims: From corrects,
// replaces, or invalidates To.
type CorrectionEdge struct {
	From engineering.RecordKey
	To   engineering.RecordKey
	Kind string
}

// RejectedClaim is a claim excluded from being the current one, with why.
type RejectedClaim struct {
	Key    engineering.RecordKey
	Reason string
}

// CorrectionRationale explains how a current-claim resolution reached its
// answer (FF-010 §6).
type CorrectionRationale struct {
	Chain    string
	Edges    []CorrectionEdge
	Rejected []RejectedClaim
}

// CurrentClaimResult is the outcome of resolving the applicable claim for a
// subject/scope/criteria triple. Found is false, with no error, when every
// claim for that triple has been invalidated -- a legitimate state
// (FF-010 §6 step 7).
type CurrentClaimResult struct {
	Found     bool
	Claim     engineering.RecordEnvelope
	Rationale CorrectionRationale
}

// ResolveCurrentClaim implements the FF-010 §6 correction-chain algorithm
// exactly: claims form a directed graph via their correction references,
// and the current claim is the unique head -- the claim nothing points at.
// Time is never used to select; a backdated correction remains valid if it
// is the unique graph head.
func ResolveCurrentClaim(ctx context.Context, repos Repositories, subjectKey, scope string, criterionKeys []string) (CurrentClaimResult, error) {
	all, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindClaim, subjectKey)
	if err != nil {
		return CurrentClaimResult{}, err
	}

	// Step 2: restrict to claims matching the requested scope and criteria.
	claims := make([]engineering.RecordEnvelope, 0, len(all))
	for _, c := range all {
		if c.Scope == scope && sameCriteria(c.CriterionKeys, criterionKeys) {
			claims = append(claims, c)
		}
	}
	if len(claims) == 0 {
		return CurrentClaimResult{Found: false, Rationale: CorrectionRationale{Chain: "no claims recorded for this subject, scope, and criteria"}}, nil
	}

	byID := make(map[string]engineering.RecordEnvelope, len(claims))
	for _, c := range claims {
		byID[c.Key.ID] = c
	}

	// Step 3: build edges, validating every correction reference.
	rationale := CorrectionRationale{}
	edgesByFrom := make(map[string]CorrectionEdge, len(claims))
	for _, c := range claims {
		if !c.HasCorrection() {
			continue
		}
		targetID := c.CorrectionTargetID
		if targetID == c.Key.ID {
			return CurrentClaimResult{}, fmt.Errorf("%w: claim %s", ErrCorrectionSelfReference, c.Key)
		}
		if _, ok := byID[targetID]; !ok {
			// The target might exist as a claim outside this
			// subject/scope/criteria triple, or as a non-claim record
			// entirely; either way it cannot be resolved within this
			// triple's graph, so distinguish "missing entirely" from
			// "wrong family" for a clearer diagnostic.
			targetKey, kerr := engineering.NewRecordKey(engineering.RecordKindClaim, targetID)
			if kerr == nil {
				if _, found, gerr := repos.Records.Get(ctx, targetKey); gerr == nil && found {
					// Exists as a claim, just outside this triple's
					// filtered set -- treat as missing-from-this-chain.
					return CurrentClaimResult{}, fmt.Errorf("%w: claim %s corrects %s, which is not in this subject/scope/criteria's claim set", ErrCorrectionTargetMissing, c.Key, targetID)
				}
			}
			if familyMismatch(ctx, repos, targetID) {
				return CurrentClaimResult{}, fmt.Errorf("%w: claim %s names correction target %s, which is not a claim", ErrCorrectionFamilyMismatch, c.Key, targetID)
			}
			return CurrentClaimResult{}, fmt.Errorf("%w: claim %s corrects %s, which does not exist", ErrCorrectionTargetMissing, c.Key, targetID)
		}
		edge := CorrectionEdge{From: c.Key, To: byID[targetID].Key, Kind: c.CorrectionKind}
		edgesByFrom[c.Key.ID] = edge
		rationale.Edges = append(rationale.Edges, edge)
	}
	sort.Slice(rationale.Edges, func(i, j int) bool { return rationale.Edges[i].From.ID < rationale.Edges[j].From.ID })

	// Step 4: reject cycles, bounded by |claims|.
	for _, c := range claims {
		if cycleFrom(c.Key.ID, edgesByFrom, len(claims)) {
			return CurrentClaimResult{}, fmt.Errorf("%w: starting from claim %s", ErrCorrectionCycle, c.Key)
		}
	}

	// Step 5: partition into superseded, invalidated.
	superseded := map[string]bool{}
	invalidated := map[string]bool{}
	for _, e := range rationale.Edges {
		switch e.Kind {
		case engineering.CorrectionKindCorrect, engineering.CorrectionKindReplace:
			superseded[e.To.ID] = true
		case engineering.CorrectionKindInvalidate:
			invalidated[e.To.ID] = true
		}
	}

	// Step 6: heads are claims neither superseded nor invalidated.
	var heads []engineering.RecordEnvelope
	for _, c := range claims {
		switch {
		case superseded[c.Key.ID]:
			rationale.Rejected = append(rationale.Rejected, RejectedClaim{Key: c.Key, Reason: "superseded"})
		case invalidated[c.Key.ID]:
			rationale.Rejected = append(rationale.Rejected, RejectedClaim{Key: c.Key, Reason: "invalidated"})
		default:
			heads = append(heads, c)
		}
	}

	// Step 7: no heads is a legitimate non-error result.
	if len(heads) == 0 {
		rationale.Chain = "every claim for this subject, scope, and criteria has been superseded or invalidated"
		return CurrentClaimResult{Found: false, Rationale: rationale}, nil
	}
	// Step 8: more than one head is ambiguous.
	if len(heads) > 1 {
		return CurrentClaimResult{}, fmt.Errorf("%w: %d competing heads for this subject, scope, and criteria", ErrCorrectionAmbiguous, len(heads))
	}

	// Step 9.
	head := heads[0]
	rationale.Chain = describeChain(head, rationale.Edges)
	return CurrentClaimResult{Found: true, Claim: head, Rationale: rationale}, nil
}

func sameCriteria(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := append([]string(nil), a...)
	sb := append([]string(nil), b...)
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// cycleFrom reports whether following correction edges from start
// eventually revisits start, bounded by maxHops (the total claim count).
func cycleFrom(start string, edgesByFrom map[string]CorrectionEdge, maxHops int) bool {
	current := start
	for hop := 0; hop < maxHops+1; hop++ {
		edge, ok := edgesByFrom[current]
		if !ok {
			return false
		}
		if edge.To.ID == start {
			return true
		}
		current = edge.To.ID
	}
	return true // exceeded bound without terminating -- treat as a cycle
}

// familyMismatch reports whether targetID names a record of some kind
// other than claim.
func familyMismatch(ctx context.Context, repos Repositories, targetID string) bool {
	for _, kind := range []engineering.RecordKind{
		engineering.RecordKindExecution, engineering.RecordKindDecision, engineering.RecordKindStateAssignment,
	} {
		key, err := engineering.NewRecordKey(kind, targetID)
		if err != nil {
			continue
		}
		if _, found, err := repos.Records.Get(ctx, key); err == nil && found {
			return true
		}
	}
	return false
}

// describeChain renders the correction chain leading to head in prose, per
// FF-010 §6's worked example.
func describeChain(head engineering.RecordEnvelope, edges []CorrectionEdge) string {
	incoming := map[string][]CorrectionEdge{}
	for _, e := range edges {
		incoming[e.To.ID] = append(incoming[e.To.ID], e)
	}
	chain := ""
	current := head.Key.ID
	visited := map[string]bool{current: true}
	for {
		sources, ok := incoming[current]
		if !ok || len(sources) == 0 {
			break
		}
		e := sources[0]
		if visited[e.From.ID] {
			break
		}
		if chain == "" {
			chain = fmt.Sprintf("%s was corrected by %s", e.From, e.To)
		} else {
			chain = fmt.Sprintf("%s was corrected by %s; %s", e.From, e.To, chain)
		}
		current = e.From.ID
		visited[current] = true
	}
	if chain == "" {
		return fmt.Sprintf("%s is not corrected by anything; %s stands", head.Key, head.Key)
	}
	return fmt.Sprintf("%s; %s is not corrected by anything; %s stands", chain, head.Key, head.Key)
}
