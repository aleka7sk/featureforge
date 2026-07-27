// Package scenario drives the canonical "Homework after a lesson" scenario
// (FF-011) through the application layer's commands, against whatever
// UnitOfWork and EngineeringRecorder it is given. It is non-test code so
// M.4's PostgreSQL adapter can reuse it verbatim (FF-013 §1).
package scenario

import (
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// FixedStart is the scenario's reference clock start (FF-011 §2).
var FixedStart = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

// Identities, fixed and deterministic (FF-011 §2).
const (
	ProjectID     = "PRJ-1"
	FeatureCardID = "FC-1"

	CapabilityArtifactID = "CAP-1"
	CapabilityRevision1  = "CAP-1-REV-1"
	CapabilityRevision2  = "CAP-1-REV-2"

	DecisionID = "DEC-1"

	PlanArtifactID = "VP-1"
	PlanRevisionID = "VP-1-REV-1"

	TransitionRecordArtifactID = "TR-1"
	EntryTransitionRevisionID  = "TR-1-REV-0"
	FirstTransitionRevisionID  = "TR-1-REV-1"
	EntryAssignmentID          = "SA-1"
	FirstAssignmentID          = "SA-2"
)

// RequirementArtifactIDs are REQ-1 .. REQ-4 (FF-011 §5). REQ-4 is
// deliberately never given a validation activity or a claim.
var RequirementArtifactIDs = []string{"REQ-1", "REQ-2", "REQ-3", "REQ-4"}

func requirementRevisionID(artifactID string) string { return artifactID + "-REV-1" }

// EvidenceIDs, keyed by the activity/claim they support (FF-011 §7). EV-0
// supports the decision's basis.
var (
	DecisionEvidenceID = "EV-0"
	EvidenceIDs        = map[string]string{"A-1": "EV-1", "A-2": "EV-2", "A-3": "EV-3", "A-2-rerun": "EV-4"}
)

func evidenceRevisionID(artifactID string) string { return artifactID + "-REV-1" }

// ExecutionIDs, keyed by activity (FF-011 §7).
var ExecutionIDs = map[string]string{"A-1": "ER-1", "A-2": "ER-2", "A-3": "ER-3", "A-2-rerun": "ER-4"}

// ClaimIDs (FF-011 §7). ClaimIncorrect is superseded by ClaimCorrecting.
const (
	ClaimForR1      = "CLM-1"
	ClaimIncorrect  = "CLM-2" // R2, wrongly satisfied
	ClaimForR3      = "CLM-3"
	ClaimCorrecting = "CLM-4" // corrects CLM-2, not-satisfied
)

// capabilityRevision1Content is the founding specification (FF-005 §3).
func capabilityRevision1Content() (engineering.CapabilitySpecificationContent, error) {
	c, err := engineering.NewCapabilitySpecificationContent(1, "Homework after a lesson",
		"After a lesson ends, a teacher has no way to give the student follow-up work, so assignments are passed verbally and lost.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithUserOutcome("A student can see the homework their teacher set after a lesson."); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithFunctionalBehaviours([]string{
		"A teacher can attach homework to a completed lesson.",
		"A teacher can publish homework.",
		"Published homework becomes visible to that lesson's student.",
	}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithConstraints([]string{"Homework is visible only to the student of that lesson."}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac1, err := engineering.NewAcceptanceCriterion("AC-1", "Published homework is visible to the intended student.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac2, err := engineering.NewAcceptanceCriterion("AC-2", "Homework is not visible to any unrelated user.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithAcceptanceCriteria([]engineering.AcceptanceCriterion{ac1, ac2}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithDependencies([]string{"Lesson completion state must be available."}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithOpenQuestions([]string{
		"Should homework support an audio attachment?",
		"What is the acceptable publication latency?",
	}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	return c, nil
}

// capabilityRevision2Content resolves Revision 1's open questions via DEC-1
// (FF-005 §3).
func capabilityRevision2Content() (engineering.CapabilitySpecificationContent, error) {
	c, err := engineering.NewCapabilitySpecificationContent(1, "Homework after a lesson",
		"After a lesson ends, a teacher has no way to give the student follow-up work, so assignments are passed verbally and lost.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithUserOutcome("A student can see the homework their teacher set after a lesson, including any audio attachment."); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithFunctionalBehaviours([]string{
		"A teacher can attach homework to a completed lesson.",
		"A teacher can publish homework.",
		"Published homework becomes visible to that lesson's student.",
		"A teacher may attach one optional audio file to homework.",
		"A published audio attachment is retrievable by the student.",
	}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithConstraints([]string{
		"Homework is visible only to the student of that lesson.",
		"Publication completes within 5 seconds of the teacher's action.",
	}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac1, err := engineering.NewAcceptanceCriterion("AC-1", "Published homework is visible to the intended student.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac2, err := engineering.NewAcceptanceCriterion("AC-2", "Homework is not visible to any unrelated user.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac3, err := engineering.NewAcceptanceCriterion("AC-3", "An optional audio attachment has a resolvable representation for the student.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	ac4, err := engineering.NewAcceptanceCriterion("AC-4", "Publication is observable to the student within 5 seconds.")
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithAcceptanceCriteria([]engineering.AcceptanceCriterion{ac1, ac2, ac3, ac4}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithDependencies([]string{"Lesson completion state must be available."}); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	return c, nil
}

// requirementStatements maps each requirement artifact ID to its PEOS-owned
// statement text (FF-011 §5).
var requirementStatements = map[string]string{
	"REQ-1": "Published homework SHALL be visible to the student of the lesson it belongs to.",
	"REQ-2": "Published homework SHALL NOT be visible to any user who is not the student of that lesson.",
	"REQ-3": "Where homework has an audio attachment, that attachment SHALL have a representation the student can resolve.",
	"REQ-4": "Published homework SHALL become observable to the student within 5 seconds of publication.",
}
