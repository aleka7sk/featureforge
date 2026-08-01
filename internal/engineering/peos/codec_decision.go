package peos

import (
	"encoding/json"
	"fmt"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/decision"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// BuildDecision constructs a Decision with its Basis and returns its
// persistence envelope (FF-011 §4.3, §6). A Decision is a standalone
// immutable record with its own core.DecisionID -- not an Artifact/Revision
// -- so it is a RecordEnvelope, not an ArtifactEnvelope/RevisionEnvelope.
func BuildDecision(in DecisionInput) (engineering.RecordEnvelope, error) {
	did, err := core.NewDecisionID(in.DecisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision id", err)
	}
	subjectArtifactID, err := core.NewArtifactID(in.SubjectArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision subject artifact id", err)
	}
	var subject core.EngineeringSubjectRef
	var subjectKey string
	if in.SubjectRevisionID == "" {
		subjectRef, err := core.NewArtifactRef(subjectArtifactID)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision subject artifact ref", err)
		}
		subject, err = core.EngineeringSubjectRefFromArtifact(subjectRef)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision subject", err)
		}
		subjectKey = engineering.ArtifactSubjectKey(in.SubjectArtifactID)
	} else {
		subjectRevisionID, err := core.NewArtifactRevisionID(in.SubjectRevisionID)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision subject revision id", err)
		}
		subjectRevRef, err := core.NewArtifactRevisionRef(subjectArtifactID, subjectRevisionID)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision subject revision ref", err)
		}
		subject, err = core.EngineeringSubjectRefFromArtifactRevision(subjectRevRef)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision subject", err)
		}
		subjectKey = engineering.ArtifactRevisionSubjectKey(in.SubjectArtifactID, in.SubjectRevisionID)
	}

	outcome, err := decision.NewOutcome(in.OutcomeStatement, decision.CommitmentEffectEstablishes)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision outcome", err)
	}
	authority, err := decision.NewAuthority(
		[]core.AuthorityRef{LocalAuthorityRef}, []core.AuthorityRef{LocalAuthorityRef},
	)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision authority", err)
	}
	scope, err := core.NewScope(CapabilityScopeKind, in.SubjectArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision scope", err)
	}

	dec, err := decision.New(did, []core.EngineeringSubjectRef{subject}, in.Question, outcome, scope, authority)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	dec, err = dec.WithProvenance(provenance)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision provenance", err)
	}
	if in.Rationale != "" {
		dec, err = dec.WithRationale(in.Rationale)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision rationale", err)
		}
	}
	if len(in.Alternatives) > 0 {
		alts := make([]decision.Alternative, len(in.Alternatives))
		for i, statement := range in.Alternatives {
			alts[i], err = decision.NewAlternative(statement)
			if err != nil {
				return engineering.RecordEnvelope{}, wrapPEOS("decision alternative", err)
			}
		}
		dec, err = dec.WithAlternatives(alts...)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision alternatives", err)
		}
	}

	evidenceArtifactID, err := core.NewArtifactID(in.EvidenceArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision evidence artifact id", err)
	}
	evidenceRevisionID, err := core.NewArtifactRevisionID(in.EvidenceRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision evidence revision id", err)
	}
	evidenceRef, err := core.NewEvidenceArtifactRevisionRef(evidenceArtifactID, evidenceRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision evidence ref", err)
	}
	assumptions := make([]decision.Assumption, len(in.Assumptions))
	for i, s := range in.Assumptions {
		assumptions[i], err = decision.NewAssumption(s)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision assumption", err)
		}
	}
	constraints := make([]decision.Constraint, len(in.Constraints))
	for i, s := range in.Constraints {
		constraints[i], err = decision.NewConstraint(s)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision constraint", err)
		}
	}
	uncertainties := make([]decision.Uncertainty, len(in.Uncertainties))
	for i, s := range in.Uncertainties {
		uncertainties[i], err = decision.NewUncertainty(s)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("decision uncertainty", err)
		}
	}
	basis, err := decision.NewBasisFrom([]core.EvidenceArtifactRevisionRef{evidenceRef}, assumptions, constraints, uncertainties)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision basis", err)
	}
	dec, err = dec.WithBasis(basis)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision with basis", err)
	}

	payload, err := json.Marshal(dec)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("decision marshal", err)
	}
	evidenceKey := engineering.EvidenceKey(in.EvidenceArtifactID, in.EvidenceRevisionID)
	occurredAt, hasOccurredAt := projectTimestamp(provenance.RecordedAt())

	key, err := engineering.NewRecordKey(engineering.RecordKindDecision, in.DecisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	return engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key:           key,
		SubjectKey:    subjectKey,
		Scope:         projectScope(scope),
		OccurredAt:    occurredAt,
		HasOccurredAt: hasOccurredAt,
		EvidenceKeys:  []string{evidenceKey},
		Payload:       payload,
		PayloadDigest: engineering.ComputeDigest(payload),
		RecordedAt:    in.RecordedAt,
	})
}

// DecodeDecision decodes a stored payload back into a decision.Decision.
// Used only by this package's own tests and by projection-fidelity checks.
func DecodeDecision(payload []byte) (decision.Decision, error) {
	var d decision.Decision
	if err := json.Unmarshal(payload, &d); err != nil {
		return decision.Decision{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return d, nil
}
