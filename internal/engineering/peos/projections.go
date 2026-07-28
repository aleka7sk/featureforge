package peos

import "github.com/aleka7sk/featureforge/internal/engineering"

// The functions below decode a stored Payload back into the PEOS-free
// projections FF-020 defines in internal/engineering, using the existing
// Decode* functions this package already carries and round-trip-tests. They
// are the read-side counterpart to BuildRequirement/BuildDecision/
// BuildValidationPlan/BuildClaim: those construct a payload from an Input;
// these reconstruct display content from that same payload. Neither a PEOS
// type nor a Payload byte slice crosses this package's boundary (AD-005).

// ProjectRequirementStatement decodes a stored requirement revision payload
// and returns its statement text (FF-020, FF-001 §3.4). BuildRequirement
// writes exactly one Statement per revision; an empty result means the
// payload carried none, which the caller treats as an empty state, not an
// error.
func ProjectRequirementStatement(payload []byte) (string, error) {
	rev, err := DecodeRequirementRevision(payload)
	if err != nil {
		return "", err
	}
	statements := rev.Content().Statements()
	if len(statements) == 0 {
		return "", nil
	}
	return statements[0].Text(), nil
}

// ProjectDecisionDetail decodes a stored decision payload into its full
// basis (FF-020, FF-001 §3.5: "the basis is displayed, not collapsed").
func ProjectDecisionDetail(payload []byte) (engineering.DecisionDetail, error) {
	dec, err := DecodeDecision(payload)
	if err != nil {
		return engineering.DecisionDetail{}, err
	}
	question, _ := dec.Question()
	rationale, _ := dec.Rationale()
	detail := engineering.DecisionDetail{
		Question:         question,
		OutcomeStatement: dec.Outcome().Statement(),
		Rationale:        rationale,
	}
	for _, alt := range dec.Alternatives() {
		detail.Alternatives = append(detail.Alternatives, alt.Statement())
	}
	if basis, ok := dec.Basis(); ok {
		for _, a := range basis.Assumptions() {
			detail.Assumptions = append(detail.Assumptions, a.Statement())
		}
		for _, c := range basis.Constraints() {
			detail.Constraints = append(detail.Constraints, c.Statement())
		}
		for _, u := range basis.Uncertainties() {
			detail.Uncertainties = append(detail.Uncertainties, u.Statement())
		}
	}
	return detail, nil
}

// ProjectPlanActivities decodes a stored validation plan revision payload
// into its planned activities (FF-020, FF-001 §3.6: "plan revision and its
// activities").
func ProjectPlanActivities(payload []byte) ([]engineering.PlanActivityDetail, error) {
	rev, err := DecodePlanRevision(payload)
	if err != nil {
		return nil, err
	}
	activities := rev.Content().Activities()
	out := make([]engineering.PlanActivityDetail, 0, len(activities))
	for _, a := range activities {
		out = append(out, engineering.PlanActivityDetail{
			Key:                   a.Key().String(),
			Method:                a.Method().String(),
			OutcomeInterpretation: a.OutcomeInterpretation(),
			ExpectedEvidence:      append([]string(nil), a.ExpectedEvidence()...),
		})
	}
	return out, nil
}

// ProjectClaimReasoning decodes a stored claim payload and returns its
// reasoning text, if any (FF-020, FF-001 §3.6).
func ProjectClaimReasoning(payload []byte) (string, error) {
	claim, err := DecodeClaim(payload)
	if err != nil {
		return "", err
	}
	reasoning, _ := claim.Reasoning()
	return reasoning, nil
}
