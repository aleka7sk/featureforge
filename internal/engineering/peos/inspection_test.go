package peos

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/decision"
	"github.com/aleka7sk/PEOS/peos/validation"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func encodeEvidenceRevisionForInspection(t *testing.T, env engineering.RevisionEnvelope, mutate func(core.Representation) core.Representation) engineering.RevisionEnvelope {
	t.Helper()
	revision, err := DecodeArtifactRevision(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	representations := revision.Representations()
	if len(representations) != 1 {
		t.Fatalf("evidence representations = %d, want 1", len(representations))
	}
	revision, err = revision.WithRepresentations(mutate(representations[0]))
	if err != nil {
		t.Fatal(err)
	}
	env.Payload, err = json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	env.PayloadDigest = engineering.ComputeDigest(env.Payload)
	return env
}

func encodeDecisionForInspection(t *testing.T, env engineering.RecordEnvelope, value decision.Decision) engineering.RecordEnvelope {
	t.Helper()
	var err error
	env.Payload, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	env.PayloadDigest = engineering.ComputeDigest(env.Payload)
	return env
}

func TestValidateRecordRejectsDecisionOccurrenceProjectionDrift(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()

	for _, mutate := range []func(){
		func() { f.decision.HasOccurredAt = false },
		func() { f.decision.OccurredAt = f.decision.OccurredAt.AddDate(0, 0, 1) },
	} {
		stored := f.decision
		mutate()
		if err := recorder.ValidateRecord(f.decision); err == nil {
			t.Fatal("ValidateRecord accepted a decision with a contradictory occurrence projection")
		}
		f.decision = stored
	}
}

func TestValidateRecordRejectsIrrelevantExtraProjections(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()

	execution := f.execution
	execution.Scope = "featureforge:unexpected-scope"
	if err := recorder.ValidateRecord(execution); err == nil {
		t.Fatal("ValidateRecord accepted an execution with an unsupported scope projection")
	}

	assignment := f.resultingAssignment
	assignment.EvidenceKeys = []string{"evidence:EV-GHOST:REV-GHOST"}
	if err := recorder.ValidateRecord(assignment); err == nil {
		t.Fatal("ValidateRecord accepted a state assignment with an unsupported evidence projection")
	}
}

func TestValidateEvidenceArtifactRequiresEvidenceRole(t *testing.T) {
	f := buildFixtures(t)
	artifact, err := BuildArtifact(
		f.evidenceArtifact.Key.ArtifactID,
		ArtifactTypeValidationEvidence,
		nil,
		f.t,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := (Recorder{}).ValidateEvidenceArtifact(artifact); err == nil {
		t.Fatal("ValidateEvidenceArtifact accepted a validation-evidence artifact without the evidence role")
	}
}

func TestValidateEvidenceRevisionRejectsUnsupportedRepresentationMetadata(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()
	extension, err := core.NewExtension().With("featureforge-test", json.RawMessage(`{"unsupported":true}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(core.Representation) core.Representation{
		"language": func(value core.Representation) core.Representation {
			return value.WithLanguage(mustVocabularyValue("evidence-language"))
		},
		"transformation": func(value core.Representation) core.Representation {
			return value.WithTransformation(mustVocabularyValue("evidence-transformation"))
		},
		"extension": func(value core.Representation) core.Representation { return value.WithExtension(extension) },
	} {
		t.Run(name, func(t *testing.T) {
			stored := encodeEvidenceRevisionForInspection(t, f.evidenceRevision, mutate)
			if err := recorder.ValidateRevision(stored); err == nil {
				t.Fatal("ValidateRevision accepted unsupported Evidence representation metadata")
			}
		})
	}
}

func TestValidateRevisionRejectsPersistedProjectionDrift(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()

	tests := []struct {
		name   string
		mutate func()
	}{
		{name: "family", mutate: func() { f.requirementRevision.RevisionFamily = "capability" }},
		{name: "artifact type", mutate: func() { f.requirementRevision.ArtifactType = ArtifactTypeProductCapability.String() }},
		{name: "subject", mutate: func() { f.requirementRevision.SubjectKey = "artifact:CAP-OTHER" }},
		{name: "content digest", mutate: func() { f.requirementRevision.ContentDigest = f.capabilityRevision.ContentDigest }},
		{name: "integrity", mutate: func() { f.requirementRevision.IntegrityValue = "sha256:deadbeef" }},
		{name: "provenance actor", mutate: func() { f.requirementRevision.ProvenanceActor = "featureforge:another-actor" }},
		{name: "provenance presence", mutate: func() { f.requirementRevision.HasProvenanceTime = false }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := f.requirementRevision
			tt.mutate()
			if err := recorder.ValidateRevision(f.requirementRevision); err == nil {
				t.Fatal("ValidateRevision accepted a contradictory persisted projection")
			}
			f.requirementRevision = stored
		})
	}
}

func replaceRecordPayloadToken(t *testing.T, env engineering.RecordEnvelope, oldValue, newValue string) engineering.RecordEnvelope {
	t.Helper()
	oldToken := []byte(`"` + oldValue + `"`)
	newToken := []byte(`"` + newValue + `"`)
	if !bytes.Contains(env.Payload, oldToken) {
		t.Fatalf("payload does not contain %q: %s", oldValue, env.Payload)
	}
	env.Payload = bytes.Replace(env.Payload, oldToken, newToken, 1)
	env.PayloadDigest = engineering.ComputeDigest(env.Payload)
	return env
}

func TestValidateRecordRejectsUnsupportedStoredVocabulary(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()

	t.Run("execution method", func(t *testing.T) {
		stored := replaceRecordPayloadToken(t, f.execution, ValidationMethodManualReview.String(), Namespace+":automated-analysis")
		if err := recorder.ValidateRecord(stored); err == nil {
			t.Fatal("ValidateRecord accepted an unsupported stored execution method")
		}
	})

	t.Run("execution outcome", func(t *testing.T) {
		stored := replaceRecordPayloadToken(t, f.execution, core.ExecutionOutcomeCompleted.String(), Namespace+":unexpected-outcome")
		stored.Outcome = Namespace + ":unexpected-outcome"
		if err := recorder.ValidateRecord(stored); err == nil {
			t.Fatal("ValidateRecord accepted an unsupported stored execution outcome")
		}
	})

	t.Run("claim type", func(t *testing.T) {
		stored := replaceRecordPayloadToken(t, f.claim, core.ClaimTypeSatisfaction.String(), core.ClaimTypeQuality.String())
		if err := recorder.ValidateRecord(stored); err == nil {
			t.Fatal("ValidateRecord accepted a non-satisfaction stored claim")
		}
	})

	t.Run("claim method", func(t *testing.T) {
		stored := replaceRecordPayloadToken(t, f.claim, ValidationMethodManualReview.String(), Namespace+":automated-analysis")
		if err := recorder.ValidateRecord(stored); err == nil {
			t.Fatal("ValidateRecord accepted an unsupported stored claim method")
		}
	})

	t.Run("claim outcome", func(t *testing.T) {
		stored := replaceRecordPayloadToken(t, f.claim, core.ClaimOutcomeSatisfied.String(), Namespace+":unexpected-outcome")
		stored.Outcome = Namespace + ":unexpected-outcome"
		if err := recorder.ValidateRecord(stored); err == nil {
			t.Fatal("ValidateRecord accepted an unsupported stored claim outcome")
		}
	})
}

func TestValidateDecisionRejectsUnsupportedNestedMetadata(t *testing.T) {
	f := buildFixtures(t)
	recorder := NewRecorder()
	extension, err := core.NewExtension().With("featureforge-test", json.RawMessage(`{"unsupported":true}`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("alternative", func(t *testing.T) {
		value, err := DecodeDecision(f.decision.Payload)
		if err != nil {
			t.Fatal(err)
		}
		alternatives := value.Alternatives()
		alternatives[0], err = alternatives[0].WithNote("unsupported note")
		if err != nil {
			t.Fatal(err)
		}
		value, err = value.WithAlternatives(alternatives...)
		if err != nil {
			t.Fatal(err)
		}
		if err := recorder.ValidateRecord(encodeDecisionForInspection(t, f.decision, value)); err == nil {
			t.Fatal("ValidateRecord accepted alternative metadata")
		}
	})

	t.Run("assumption", func(t *testing.T) {
		value, err := DecodeDecision(f.decision.Payload)
		if err != nil {
			t.Fatal(err)
		}
		basis, ok := value.Basis()
		if !ok {
			t.Fatal("decision has no basis")
		}
		assumptions := basis.Assumptions()
		assumptions[0], err = assumptions[0].WithSource("unsupported source")
		if err != nil {
			t.Fatal(err)
		}
		basis, err = basis.WithAssumptions(assumptions...)
		if err != nil {
			t.Fatal(err)
		}
		value, err = value.WithBasis(basis)
		if err != nil {
			t.Fatal(err)
		}
		if err := recorder.ValidateRecord(encodeDecisionForInspection(t, f.decision, value)); err == nil {
			t.Fatal("ValidateRecord accepted assumption metadata")
		}
	})

	t.Run("constraint", func(t *testing.T) {
		value, err := DecodeDecision(f.decision.Payload)
		if err != nil {
			t.Fatal(err)
		}
		basis, ok := value.Basis()
		if !ok {
			t.Fatal("decision has no basis")
		}
		constraints := basis.Constraints()
		constraints[0], err = constraints[0].WithSource("unsupported source")
		if err != nil {
			t.Fatal(err)
		}
		basis, err = basis.WithConstraints(constraints...)
		if err != nil {
			t.Fatal(err)
		}
		value, err = value.WithBasis(basis)
		if err != nil {
			t.Fatal(err)
		}
		if err := recorder.ValidateRecord(encodeDecisionForInspection(t, f.decision, value)); err == nil {
			t.Fatal("ValidateRecord accepted constraint metadata")
		}
	})

	t.Run("uncertainty", func(t *testing.T) {
		value, err := DecodeDecision(f.decision.Payload)
		if err != nil {
			t.Fatal(err)
		}
		basis, ok := value.Basis()
		if !ok {
			t.Fatal("decision has no basis")
		}
		uncertainties := basis.Uncertainties()
		uncertainties[0] = uncertainties[0].WithExtension(extension)
		basis, err = basis.WithUncertainties(uncertainties...)
		if err != nil {
			t.Fatal(err)
		}
		value, err = value.WithBasis(basis)
		if err != nil {
			t.Fatal(err)
		}
		if err := recorder.ValidateRecord(encodeDecisionForInspection(t, f.decision, value)); err == nil {
			t.Fatal("ValidateRecord accepted uncertainty metadata")
		}
	})
}

func TestValidateValidationPlanContentRejectsUnsupportedFeatureForgeSubset(t *testing.T) {
	f := buildFixtures(t)
	revision, err := DecodePlanRevision(f.planRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	content := revision.Content()
	activity := content.Activities()[0]

	t.Run("method", func(t *testing.T) {
		customMethod := core.NewValidationMethod(mustVocabularyValue("automated-analysis"))
		custom, err := validation.NewPlannedActivity(activity.Key(), activity.Subject(), customMethod, activity.OutcomeInterpretation())
		if err != nil {
			t.Fatal(err)
		}
		custom, err = custom.WithCriteria(activity.Criteria())
		if err != nil {
			t.Fatal(err)
		}
		if expected := activity.ExpectedEvidence(); len(expected) > 0 {
			custom, err = custom.WithExpectedEvidence(expected)
			if err != nil {
				t.Fatal(err)
			}
		}
		customContent, err := validation.NewPlanContent(content.Scope(), content.Applicability(), content.Provenance(), []validation.PlannedActivity{custom})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateValidationPlanContent(customContent, f.planRevision.RecordedAt); err == nil {
			t.Fatal("stored plan content accepted an unsupported activity method")
		}
	})

	t.Run("optional metadata", func(t *testing.T) {
		custom, err := activity.WithPrerequisites([]string{"not part of the FeatureForge plan contract"})
		if err != nil {
			t.Fatal(err)
		}
		customContent, err := validation.NewPlanContent(content.Scope(), content.Applicability(), content.Provenance(), []validation.PlannedActivity{custom})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateValidationPlanContent(customContent, f.planRevision.RecordedAt); err == nil {
			t.Fatal("stored plan content accepted unsupported optional activity metadata")
		}
	})
}

func TestValidateFeatureForgeProvenanceRejectsExtraMetadata(t *testing.T) {
	f := buildFixtures(t)
	execution, err := DecodeExecution(f.execution.Payload)
	if err != nil {
		t.Fatal(err)
	}
	provenance := execution.Provenance().WithSource(mustVocabularyValue("imported"))
	if err := validateFeatureForgeProvenance(provenance); err == nil {
		t.Fatal("FeatureForge provenance accepted an unsupported source field")
	}
}
