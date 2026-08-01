package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// CorrectionEdge is one correction reference among claims: From corrects,
// replaces, or invalidates To.
type CorrectionEdge struct {
	From engineering.RecordKey
	To   engineering.RecordKey
	Kind string
}

// RejectedClaim is a claim excluded from being the current one, with why
// (FF-001 §3.6: "superseded claims shown inline, visibly marked, with
// their outcome intact and a link to the claim that corrected them" --
// "shown, not hidden"). Outcome and CorrectedBy are populated only when
// Reason is "superseded"; an invalidated claim was never superseded by a
// specific claim, so CorrectedBy stays empty for that case. Reasoning is
// left for the caller with a projector to fill in (FF-020 §5); it needs a
// payload decode this function's callers do not all have.
type RejectedClaim struct {
	Key         engineering.RecordKey
	Reason      string
	Outcome     string
	CorrectedBy string
	Reasoning   string
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
func ResolveCurrentClaim(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, subjectKey, scope string, criterionKeys []string) (CurrentClaimResult, error) {
	allRecords, err := listValidatedRecords(ctx, repos, inspector)
	if err != nil {
		return CurrentClaimResult{}, err
	}

	// Step 2: restrict to claims matching the requested scope and criteria.
	claims := make([]engineering.RecordEnvelope, 0, len(allRecords))
	for _, c := range allRecords {
		if c.Kind == engineering.RecordKindClaim && c.SubjectKey == subjectKey && c.Scope == scope && sameCriteria(c.CriterionKeys, criterionKeys) {
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

	// Step 4: reject cycles, bounded by |claims|. Enumerate the complete
	// conflicting population rather than reporting whichever start node the
	// repository happened to return first (FF-010 §6, FF-001 §6.4).
	if cycleNodeIDs := correctionCycleNodeIDs(claims, edgesByFrom); len(cycleNodeIDs) > 0 {
		return CurrentClaimResult{}, fmt.Errorf(
			"%w: Claim IDs [%s] form one or more correction cycles",
			ErrCorrectionCycle,
			strings.Join(cycleNodeIDs, ", "),
		)
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

	// correctedBy maps a superseded claim's ID to the claim that corrected
	// it, i.e. the edge pointing at it -- FF-001 §3.6's "a link to the
	// claim that corrected them". An invalidated claim has no such link:
	// invalidation retracts, it does not point to a replacement.
	correctedBy := map[string]string{}
	for _, e := range rationale.Edges {
		if e.Kind == engineering.CorrectionKindCorrect || e.Kind == engineering.CorrectionKindReplace {
			correctedBy[e.To.ID] = e.From.ID
		}
	}

	// Step 6: heads are claims neither superseded nor invalidated.
	var heads []engineering.RecordEnvelope
	for _, c := range claims {
		switch {
		case superseded[c.Key.ID]:
			rationale.Rejected = append(rationale.Rejected, RejectedClaim{Key: c.Key, Reason: "superseded", Outcome: c.Outcome, CorrectedBy: correctedBy[c.Key.ID]})
		case invalidated[c.Key.ID]:
			rationale.Rejected = append(rationale.Rejected, RejectedClaim{Key: c.Key, Reason: "invalidated", Outcome: c.Outcome})
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
		headIDs := make([]string, 0, len(heads))
		for _, head := range heads {
			headIDs = append(headIDs, head.Key.ID)
		}
		sort.Strings(headIDs)
		return CurrentClaimResult{}, fmt.Errorf(
			"%w: competing Claim IDs [%s] for this subject, scope, and criteria",
			ErrCorrectionAmbiguous,
			strings.Join(headIDs, ", "),
		)
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

// correctionCycleNodeIDs returns the sorted union of every Claim ID that is
// actually inside a correction cycle. A non-cyclic head or tail leading into a
// cycle is deliberately excluded. Each traversal is bounded by |claims| + 1;
// all edges have already been proven to stay inside this claim population.
func correctionCycleNodeIDs(claims []engineering.RecordEnvelope, edgesByFrom map[string]CorrectionEdge) []string {
	cycleNodes := make(map[string]struct{})
	for _, claim := range claims {
		path := make([]string, 0, len(claims))
		position := make(map[string]int, len(claims))
		current := claim.Key.ID
		for hop := 0; hop <= len(claims); hop++ {
			if cycleStart, repeated := position[current]; repeated {
				for _, nodeID := range path[cycleStart:] {
					cycleNodes[nodeID] = struct{}{}
				}
				break
			}
			position[current] = len(path)
			path = append(path, current)
			edge, found := edgesByFrom[current]
			if !found {
				break
			}
			current = edge.To.ID
		}
	}

	ids := make([]string, 0, len(cycleNodes))
	for nodeID := range cycleNodes {
		ids = append(ids, nodeID)
	}
	sort.Strings(ids)
	return ids
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
	byCorrectingClaim := make(map[string]CorrectionEdge, len(edges))
	for _, e := range edges {
		byCorrectingClaim[e.From.ID] = e
	}
	chain := make([]CorrectionEdge, 0, len(edges))
	current := head.Key.ID
	visited := map[string]bool{current: true}
	for {
		edge, ok := byCorrectingClaim[current]
		if !ok {
			break
		}
		if visited[edge.To.ID] {
			break
		}
		chain = append(chain, edge)
		current = edge.To.ID
		visited[current] = true
	}
	if len(chain) == 0 {
		return fmt.Sprintf("%s is not corrected by anything; %s stands", head.Key, head.Key)
	}
	parts := make([]string, 0, len(chain)+2)
	for i := len(chain) - 1; i >= 0; i-- {
		edge := chain[i]
		parts = append(parts, describeCorrectionEdge(edge))
	}
	parts = append(parts,
		fmt.Sprintf("%s is not corrected by anything", head.Key),
		fmt.Sprintf("%s stands", head.Key),
	)
	return strings.Join(parts, "; ")
}

func describeCorrectionEdge(edge CorrectionEdge) string {
	verb := "was corrected by"
	switch edge.Kind {
	case engineering.CorrectionKindReplace:
		verb = "was replaced by"
	case engineering.CorrectionKindInvalidate:
		verb = "was invalidated by"
	}
	return fmt.Sprintf("%s %s %s", edge.To, verb, edge.From)
}
