package peos

import (
	"encoding/json"
	"testing"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/requirement"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func TestBuildCapabilityRevisionOrdinaryWireFormatUnchanged(t *testing.T) {
	content, err := engineering.NewCapabilitySpecificationContent(1, "Homework after a lesson", "No follow-up work today.")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := content.Digest()
	if err != nil {
		t.Fatal(err)
	}
	env, err := BuildCapabilityRevision(CapabilityRevisionInput{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", ContentDigest: digest, RecordedAt: fixedTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-1","origin":{"kind":"peos:known"},"provenance":{"actor":{"namespace":"featureforge","identifier":"local-user"},"recorded_at":"2026-03-01T00:00:00Z"},"integrity":{"mechanism":"peos:content-addressed-reference","value":"sha256:60867499de21745d80ed55b784def464e91d68c85c4825360a47bca20a3dda71","protected_scopes":["peos:content"]},"representations":[{"content":{"kind":"content_address","ref":{"algorithm":"featureforge:sha256","digest":"60867499de21745d80ed55b784def464e91d68c85c4825360a47bca20a3dda71"}},"media_type":"featureforge:specification-content","classification":["peos:authoritative"]}]}`
	if got := string(env.Payload); got != want {
		t.Fatalf("ordinary capability revision payload changed\ngot:  %s\nwant: %s", got, want)
	}
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		t.Fatalf("ordinary capability revision is invalid: %v", err)
	}
	proposal, context, sources, found, err := (Recorder{}).InspectAIAssistedCapabilityRevision(env)
	if err != nil {
		t.Fatal(err)
	}
	if found || !proposal.IsZero() || !context.IsZero() || sources != nil {
		t.Fatalf("ordinary capability revision classified as AI-assisted: found=%t proposal=%s context=%s sources=%v", found, proposal, context, sources)
	}
}

func TestAIAssistedCapabilityRevisionRoundTrip(t *testing.T) {
	f := buildFixtures(t)
	proposalDigest := engineering.ComputeDigest([]byte("proposal"))
	contextDigest := engineering.ComputeDigest([]byte("context"))
	sources := []string{
		"artifact:CAP-1",
		"record:decision/DEC-1",
		"revision:CAP-1/CAP-1-REV-1",
	}
	in := CapabilityRevisionInput{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-AI", ContentDigest: f.contentDigest, RecordedAt: f.t,
	}
	env, err := (Recorder{}).RecordAIAssistedCapabilityRevision(in, proposalDigest, contextDigest, sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		t.Fatalf("ValidateRevision: %v", err)
	}

	revision, err := DecodeArtifactRevision(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	actor, hasActor := revision.Provenance().Actor()
	if !hasActor || actor != LocalActorRef || env.ProvenanceActor != "featureforge:local-user" {
		t.Fatalf("actor = %v (present %t), projection = %q", actor, hasActor, env.ProvenanceActor)
	}
	method, hasMethod := revision.Provenance().Method()
	if !hasMethod || method.String() != engineering.AIAssistedMethod {
		t.Fatalf("method = %q (present %t), want %q", method.String(), hasMethod, engineering.AIAssistedMethod)
	}
	witness, err := engineering.NewAIAssistanceWitness(proposalDigest, contextDigest, sources)
	if err != nil {
		t.Fatal(err)
	}
	note, hasNote := revision.Origin().Note()
	if !hasNote || note != witness.OriginNote() {
		t.Fatalf("origin note = %q (present %t), want %q", note, hasNote, witness.OriginNote())
	}

	gotProposal, gotContext, gotSources, found, err := (Recorder{}).InspectAIAssistedCapabilityRevision(env)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !gotProposal.Equal(proposalDigest) || !gotContext.Equal(contextDigest) {
		t.Fatalf("inspection = proposal %s context %s found %t", gotProposal, gotContext, found)
	}
	if len(gotSources) != len(sources) {
		t.Fatalf("sources = %v, want %v", gotSources, sources)
	}
	for idx := range sources {
		if gotSources[idx] != sources[idx] {
			t.Fatalf("sources = %v, want %v", gotSources, sources)
		}
	}
	gotSources[0] = "artifact:MUTATED"
	_, _, secondSources, _, err := (Recorder{}).InspectAIAssistedCapabilityRevision(env)
	if err != nil {
		t.Fatal(err)
	}
	if secondSources[0] != sources[0] {
		t.Fatal("inspection sources are not defensively copied")
	}
}

func TestRecordAIAssistedCapabilityRevisionRejectsNonCanonicalSources(t *testing.T) {
	f := buildFixtures(t)
	in := CapabilityRevisionInput{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-AI", ContentDigest: f.contentDigest, RecordedAt: f.t,
	}
	_, err := (Recorder{}).RecordAIAssistedCapabilityRevision(
		in,
		engineering.ComputeDigest([]byte("proposal")),
		engineering.ComputeDigest([]byte("context")),
		[]string{"revision:CAP-1/CAP-1-REV-1", "artifact:CAP-1"},
	)
	if err == nil {
		t.Fatal("RecordAIAssistedCapabilityRevision accepted unsorted sources")
	}
}

func rebuildCapabilityRevisionForInspection(t *testing.T, original core.ArtifactRevision, origin core.Origin, provenance core.Provenance, integrity core.IntegrityIdentity, representations []core.Representation) core.ArtifactRevision {
	t.Helper()
	revision, err := core.NewArtifactRevision(original.ArtifactID(), original.RevisionID(), origin, provenance, integrity)
	if err != nil {
		t.Fatal(err)
	}
	if len(representations) > 0 {
		revision, err = revision.WithRepresentations(representations...)
		if err != nil {
			t.Fatal(err)
		}
	}
	return revision
}

func encodeCapabilityRevisionForInspection(t *testing.T, env engineering.RevisionEnvelope, revision core.ArtifactRevision) engineering.RevisionEnvelope {
	t.Helper()
	var err error
	env.Payload, err = json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	env.PayloadDigest = engineering.ComputeDigest(env.Payload)
	return env
}

func TestValidateRevisionRejectsInvalidAIAssistanceShapes(t *testing.T) {
	f := buildFixtures(t)
	proposalDigest := engineering.ComputeDigest([]byte("proposal"))
	contextDigest := engineering.ComputeDigest([]byte("context"))
	sources := []string{"artifact:CAP-1", "revision:CAP-1/CAP-1-REV-1"}
	env, err := (Recorder{}).RecordAIAssistedCapabilityRevision(CapabilityRevisionInput{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-AI", ContentDigest: f.contentDigest, RecordedAt: f.t,
	}, proposalDigest, contextDigest, sources)
	if err != nil {
		t.Fatal(err)
	}
	original, err := DecodeArtifactRevision(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryOrigin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		t.Fatal(err)
	}
	witness, err := engineering.NewAIAssistanceWitness(proposalDigest, contextDigest, sources)
	if err != nil {
		t.Fatal(err)
	}
	aiOrigin, err := core.NewOrigin(core.OriginKindKnown, witness.OriginNote())
	if err != nil {
		t.Fatal(err)
	}
	malformedOrigin, err := core.NewOrigin(core.OriginKindKnown, engineering.AIAssistedMethod+";proposal=broken")
	if err != nil {
		t.Fatal(err)
	}
	ordinaryProvenance, err := provenanceFor(f.t)
	if err != nil {
		t.Fatal(err)
	}
	aiProvenance := ordinaryProvenance.WithMethod(ProvenanceMethodAIAssisted)
	extension, err := core.NewExtension().With("featureforge-test", json.RawMessage(`{"unsupported":true}`))
	if err != nil {
		t.Fatal(err)
	}
	representations := original.Representations()

	tests := map[string]func() core.ArtifactRevision{
		"AI origin without method": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, ordinaryProvenance, original.Integrity(), representations)
		},
		"AI method without origin": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, ordinaryOrigin, aiProvenance, original.Integrity(), representations)
		},
		"malformed AI origin": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, malformedOrigin, aiProvenance, original.Integrity(), representations)
		},
		"arbitrary method": func() core.ArtifactRevision {
			provenance := ordinaryProvenance.WithMethod(mustVocabularyValue("another-method"))
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, provenance, original.Integrity(), representations)
		},
		"source": func() core.ArtifactRevision {
			provenance := aiProvenance.WithSource(mustVocabularyValue("imported"))
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, provenance, original.Integrity(), representations)
		},
		"external source": func() core.ArtifactRevision {
			provenance := aiProvenance.WithExternalSourceID("external-1")
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, provenance, original.Integrity(), representations)
		},
		"provenance extension": func() core.ArtifactRevision {
			provenance := aiProvenance.WithExtension(extension)
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, provenance, original.Integrity(), representations)
		},
		"origin extension": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin.WithExtension(extension), aiProvenance, original.Integrity(), representations)
		},
		"integrity extension": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, aiProvenance, original.Integrity().WithExtension(extension), representations)
		},
		"revision extension": func() core.ArtifactRevision {
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, aiProvenance, original.Integrity(), representations).WithExtension(extension)
		},
		"representation extension": func() core.ArtifactRevision {
			modified := append([]core.Representation(nil), representations...)
			modified[0] = modified[0].WithExtension(extension)
			return rebuildCapabilityRevisionForInspection(t, original, aiOrigin, aiProvenance, original.Integrity(), modified)
		},
	}

	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			stored := encodeCapabilityRevisionForInspection(t, env, build())
			if err := (Recorder{}).ValidateRevision(stored); err == nil {
				t.Fatal("ValidateRevision accepted invalid AI-assisted metadata")
			}
			if _, _, _, _, err := (Recorder{}).InspectAIAssistedCapabilityRevision(stored); err == nil {
				t.Fatal("InspectAIAssistedCapabilityRevision accepted invalid AI-assisted metadata")
			}
		})
	}
}

func TestValidateRevisionRejectsAIAssistanceOnAnotherFamily(t *testing.T) {
	f := buildFixtures(t)
	storedRevision, err := DecodeRequirementRevision(f.requirementRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	storedArtifact, err := DecodeArtifact(f.requirementArtifact.Payload)
	if err != nil {
		t.Fatal(err)
	}
	requirementArtifact, err := requirement.New(storedArtifact)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := engineering.NewAIAssistanceWitness(
		engineering.ComputeDigest([]byte("proposal")),
		engineering.ComputeDigest([]byte("context")),
		[]string{"artifact:CAP-1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, witness.OriginNote())
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := provenanceFor(f.t)
	if err != nil {
		t.Fatal(err)
	}
	coreRevision := rebuildCapabilityRevisionForInspection(
		t,
		storedRevision.Core(),
		origin,
		provenance.WithMethod(ProvenanceMethodAIAssisted),
		storedRevision.Core().Integrity(),
		storedRevision.Core().Representations(),
	)
	revision, err := requirement.NewRevision(requirementArtifact, coreRevision, storedRevision.Content())
	if err != nil {
		t.Fatal(err)
	}
	env := f.requirementRevision
	env.Payload, err = json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	env.PayloadDigest = engineering.ComputeDigest(env.Payload)
	if err := (Recorder{}).ValidateRevision(env); err == nil {
		t.Fatal("ValidateRevision accepted the AI-assisted provenance form on a Requirement revision")
	}
	if _, _, _, _, err := (Recorder{}).InspectAIAssistedCapabilityRevision(env); err == nil {
		t.Fatal("InspectAIAssistedCapabilityRevision accepted the AI-assisted provenance form on a Requirement revision")
	}
}
