package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/proposal"
)

// validatePersistedProposalHistorySources completes the M.6 extension of a
// capability's managed-history proof. Callers first run validateManagedHistory,
// which validates every Artifact/Revision/content/order/journal member; this
// pass then validates the source members of every AI-assisted Revision in the
// same deterministic sequence order. A later valid target must never hide a
// corrupt earlier AI-assisted act in the shared capability history.
func validatePersistedProposalHistorySources(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	artifactID string,
) error {
	revisions, err := repos.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	orders, err := repos.RevisionOrder.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	orderByKey := make(map[engineering.RevisionKey]engineering.RevisionOrderMetadata, len(orders))
	for _, order := range orders {
		orderByKey[order.Key] = order
	}
	sort.Slice(revisions, func(i, j int) bool {
		return orderByKey[revisions[i].Key].Sequence < orderByKey[revisions[j].Key].Sequence
	})
	for _, revision := range revisions {
		order, found := orderByKey[revision.Key]
		if !found {
			return integrityError("AI-assisted history Revision has no order metadata", nil)
		}
		_, _, sources, assisted, err := inspector.InspectAIAssistedCapabilityRevision(revision)
		if err != nil {
			return integrityError("AI-assisted history witness is unreadable", err)
		}
		if !assisted {
			continue
		}
		if err := validatePersistedProposalSources(ctx, repos, inspector, revision.Key, order, sources); err != nil {
			return err
		}
	}
	return nil
}

// validatePersistedProposalSources proves that every selected source in an
// occupied AI-assisted target still resolves as authoritative immutable state
// rooted in the target capability. Proposal sources are intentionally only a
// non-empty subset of ContextPack sources, so this function never requires an
// omitted ContextPack member or reconstructs the historical pack. It also
// never compares the historical ContextDigest with today's mutable context.
func validatePersistedProposalSources(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	target engineering.RevisionKey,
	targetOrder engineering.RevisionOrderMetadata,
	sources []string,
) error {
	set := make(map[string]struct{}, len(sources))
	for _, raw := range sources {
		if _, err := proposal.ParseSourceReference(raw); err != nil {
			return integrityError("proposal target source witness is malformed", err)
		}
		if _, duplicate := set[raw]; duplicate {
			return integrityError("proposal target source witness contains a duplicate", nil)
		}
		set[raw] = struct{}{}
	}

	priorCapabilityRoot := false
	for _, raw := range sources {
		switch {
		case strings.HasPrefix(raw, "artifact:"):
			artifactID := strings.TrimPrefix(raw, "artifact:")
			if artifactID != target.ArtifactID {
				return integrityError("proposal target cites a foreign Artifact source", nil)
			}
			artifact, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
			if err != nil {
				return err
			}
			if !found {
				return integrityError("proposal target Artifact source is dangling", nil)
			}
			if err := inspectArtifact(inspector, artifact); err != nil {
				return err
			}
			if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
				return integrityError("proposal target Artifact source is not a capability", err)
			}
			if err := validateProposalArtifactSourceMembership(ctx, repos, inspector, artifactID); err != nil {
				return err
			}

		case strings.HasPrefix(raw, "revision:"):
			key, err := proposalSourceRevisionKey(strings.TrimPrefix(raw, "revision:"))
			if err != nil {
				return integrityError("proposal target Revision source is malformed", err)
			}
			revision, found, err := repos.Revisions.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("proposal target Revision source is dangling", nil)
			}
			if err := inspectRevision(inspector, revision); err != nil {
				return err
			}
			switch revision.RevisionFamily {
			case engineering.RevisionFamilyCapability:
				if err := validateProposalPriorCapabilityRevision(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, key); err != nil {
					return err
				}
				accepted, err := proposalCapabilityRevisionWasAccepted(ctx, repos, key)
				if err != nil {
					return err
				}
				if accepted {
					priorCapabilityRoot = true
				} else if err := validateProposalRevisionDecisionMembership(ctx, repos, inspector, key); err != nil {
					return err
				}
				content, found, err := repos.StructuredContent.Get(ctx, key)
				if err != nil {
					return err
				}
				if !found {
					return integrityError("proposal target capability source has no content", nil)
				}
				if err := inspector.ValidateCapabilityContent(revision, content); err != nil {
					return integrityError("proposal target capability source content is invalid", err)
				}
			case engineering.RevisionFamilyRequirement:
				kind, subjectArtifactID, _, err := engineering.ParseSubjectKey(revision.SubjectKey)
				if err != nil || kind != engineering.SubjectKindArtifact || subjectArtifactID != target.ArtifactID {
					return integrityError("proposal target Requirement source is rooted in another capability", err)
				}
				if _, err := validateManagedHistory(ctx, repos, inspector, key.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
					return err
				}
				trace, found, err := repos.RequirementTraces.Get(ctx, key)
				if err != nil {
					return err
				}
				if !found {
					return integrityError("proposal target Requirement source has no criterion trace", nil)
				}
				if err := validateProposalPriorCapabilityRevision(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, trace.CapabilityRevision); err != nil {
					return err
				}
			case engineering.RevisionFamilyEvidence:
				if err := validateProposalEvidenceSource(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, key); err != nil {
					return err
				}
			default:
				return integrityError("proposal target cites an unsupported Revision source family", nil)
			}

		case strings.HasPrefix(raw, "requirement-trace:"):
			key, err := proposalSourceRevisionKey(strings.TrimPrefix(raw, "requirement-trace:"))
			if err != nil {
				return integrityError("proposal target Requirement trace source is malformed", err)
			}
			revision, found, err := repos.Revisions.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("proposal target Requirement trace Revision is dangling", nil)
			}
			if err := inspectRevision(inspector, revision); err != nil {
				return err
			}
			order, orderFound, err := repos.RevisionOrder.Get(ctx, key)
			if err != nil {
				return err
			}
			trace, traceFound, err := repos.RequirementTraces.Get(ctx, key)
			if err != nil {
				return err
			}
			if !orderFound || !traceFound {
				return integrityError("proposal target Requirement trace source is partial", nil)
			}
			if _, err := validateManagedHistory(ctx, repos, inspector, key.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
				return err
			}
			if err := validateRequirementCriterionTrace(ctx, repos, inspector, revision, order, trace); err != nil {
				return err
			}
			if trace.CapabilityRevision.ArtifactID != target.ArtifactID {
				return integrityError("proposal target Requirement trace is rooted in another capability", nil)
			}
			if err := validateProposalPriorCapabilityRevision(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, trace.CapabilityRevision); err != nil {
				return err
			}

		case strings.HasPrefix(raw, "criterion:"):
			key, criterionKey, err := proposalCriterionSourceKey(strings.TrimPrefix(raw, "criterion:"))
			if err != nil {
				return integrityError("proposal target criterion source is malformed", err)
			}
			if key.ArtifactID != target.ArtifactID {
				return integrityError("proposal target cites a foreign capability criterion source", nil)
			}
			if err := validateProposalPriorCapabilityRevision(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, key); err != nil {
				return err
			}
			if err := validateProposalAcceptedCapabilityRevision(ctx, repos, key); err != nil {
				return err
			}
			revision, found, err := repos.Revisions.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("proposal target criterion Revision is dangling", nil)
			}
			if err := inspectRevision(inspector, revision); err != nil {
				return err
			}
			if revision.RevisionFamily != engineering.RevisionFamilyCapability {
				return integrityError("proposal target criterion source names another Revision family", nil)
			}
			content, found, err := repos.StructuredContent.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found || !capabilityContentHasCriterion(content, criterionKey) {
				return integrityError("proposal target criterion source is dangling", nil)
			}
			if err := inspector.ValidateCapabilityContent(revision, content); err != nil {
				return integrityError("proposal target criterion source content is invalid", err)
			}

		case strings.HasPrefix(raw, "record:"):
			key, err := proposalRecordSourceKey(strings.TrimPrefix(raw, "record:"))
			if err != nil {
				return integrityError("proposal target Record source is malformed", err)
			}
			record, found, err := repos.Records.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("proposal target Record source is dangling", nil)
			}
			if err := inspectRecord(inspector, record); err != nil {
				return err
			}
			kind, subjectArtifactID, subjectRevisionID, err := engineering.ParseSubjectKey(record.SubjectKey)
			if err != nil || (kind != engineering.SubjectKindArtifact && kind != engineering.SubjectKindArtifactRevision) || subjectArtifactID != target.ArtifactID {
				return integrityError("proposal target Record source is rooted in another capability", err)
			}
			switch key.Kind {
			case engineering.RecordKindDecision:
				if err := validateStoredDecisionReferences(ctx, repos, inspector, record); err != nil {
					return err
				}
				paired := "artifact:" + subjectArtifactID
				if kind == engineering.SubjectKindArtifactRevision {
					paired = "revision:" + subjectArtifactID + "/" + subjectRevisionID
				}
				if _, present := set[paired]; !present {
					return integrityError("proposal target Decision source has no exact subject source", nil)
				}
			case engineering.RecordKindClaim:
				if kind != engineering.SubjectKindArtifactRevision {
					return integrityError("proposal target Claim source has no exact Revision subject", nil)
				}
				if err := validateProposalPriorCapabilityRevision(
					ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence,
					engineering.RevisionKey{ArtifactID: subjectArtifactID, RevisionID: subjectRevisionID},
				); err != nil {
					return err
				}
				if err := validateProposalAcceptedCapabilityRevision(
					ctx, repos,
					engineering.RevisionKey{ArtifactID: subjectArtifactID, RevisionID: subjectRevisionID},
				); err != nil {
					return err
				}
				if record.Scope != "featureforge:capability|"+target.ArtifactID {
					return integrityError("proposal target Claim source has another capability scope", nil)
				}
				if err := validateStoredClaimReferences(ctx, repos, inspector, record); err != nil {
					return err
				}
				if err := validateClaimChainIntegrity(ctx, repos, inspector, record); err != nil {
					return err
				}
				if err := validateProposalClaimRequirementRoot(ctx, repos, target.ArtifactID, record); err != nil {
					return err
				}
			case engineering.RecordKindExecution:
				if kind != engineering.SubjectKindArtifactRevision {
					return integrityError("proposal target Execution source has no exact Revision subject", nil)
				}
				if err := validateProposalPriorCapabilityRevision(
					ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence,
					engineering.RevisionKey{ArtifactID: subjectArtifactID, RevisionID: subjectRevisionID},
				); err != nil {
					return err
				}
				if err := validateProposalAcceptedCapabilityRevision(
					ctx, repos,
					engineering.RevisionKey{ArtifactID: subjectArtifactID, RevisionID: subjectRevisionID},
				); err != nil {
					return err
				}
				if _, _, _, err := validateStoredExecutionAct(ctx, repos, inspector, record); err != nil {
					return err
				}
				if err := validateProposalClaimSourceMembership(ctx, repos, inspector, target.ArtifactID, targetOrder.Sequence, key, engineering.RevisionKey{}); err != nil {
					return err
				}
			default:
				return integrityError("proposal target cites an unsupported Record source kind", nil)
			}
		}
	}

	if !priorCapabilityRoot {
		return integrityError("proposal target has no prior capability root source", nil)
	}
	return nil
}

func validateProposalEvidenceSource(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	targetArtifactID string,
	targetSequence int,
	key engineering.RevisionKey,
) error {
	owners, err := evidenceOwners(ctx, repos, inspector, key)
	if err != nil {
		return err
	}
	if len(owners) != 1 {
		return integrityError("proposal target Evidence source has no unique Execution owner", nil)
	}
	if _, _, owned, err := validateStoredExecutionAct(ctx, repos, inspector, owners[0]); err != nil {
		return err
	} else if owned != key {
		return integrityError("proposal target Evidence source owner is contradictory", nil)
	}
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(owners[0].SubjectKey)
	if err != nil || kind != engineering.SubjectKindArtifactRevision || artifactID != targetArtifactID {
		return integrityError("proposal target Evidence source is rooted in another capability", err)
	}
	if err := validateProposalPriorCapabilityRevision(
		ctx, repos, inspector, targetArtifactID, targetSequence,
		engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID},
	); err != nil {
		return err
	}
	if err := validateProposalAcceptedCapabilityRevision(
		ctx, repos, engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID},
	); err != nil {
		return err
	}
	if err := validateProposalClaimSourceMembership(ctx, repos, inspector, targetArtifactID, targetSequence, engineering.RecordKey{}, key); err != nil {
		return err
	}
	return nil
}

func validateProposalArtifactSourceMembership(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	artifactID string,
) error {
	decisions, err := listValidatedRecordsByKind(ctx, repos, inspector, engineering.RecordKindDecision)
	if err != nil {
		return err
	}
	want := engineering.ArtifactSubjectKey(artifactID)
	for _, decision := range decisions {
		if decision.SubjectKey != want {
			continue
		}
		if err := validateStoredDecisionReferences(ctx, repos, inspector, decision); err != nil {
			return err
		}
		return nil
	}
	return integrityError("proposal target Artifact source has no Artifact-subject Decision", nil)
}

func validateProposalClaimSourceMembership(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	targetArtifactID string,
	targetSequence int,
	executionKey engineering.RecordKey,
	evidenceKey engineering.RevisionKey,
) error {
	claims, err := listValidatedRecordsByKind(ctx, repos, inspector, engineering.RecordKindClaim)
	if err != nil {
		return err
	}
	wantSubjectPrefix := "artifact-revision:" + targetArtifactID + "/"
	wantExecution := ""
	if !executionKey.IsZero() {
		wantExecution = "execution:" + executionKey.ID
	}
	wantEvidence := ""
	if !evidenceKey.IsZero() {
		wantEvidence = engineering.EvidenceKey(evidenceKey.ArtifactID, evidenceKey.RevisionID)
	}
	for _, claim := range claims {
		if !strings.HasPrefix(claim.SubjectKey, wantSubjectPrefix) || claim.Scope != "featureforge:capability|"+targetArtifactID {
			continue
		}
		matches := wantExecution != "" && slicesContainString(claim.ExecutionKeys, wantExecution)
		matches = matches || wantEvidence != "" && slicesContainString(claim.EvidenceKeys, wantEvidence)
		if !matches {
			continue
		}
		if err := validateStoredClaimReferences(ctx, repos, inspector, claim); err != nil {
			return err
		}
		if err := validateClaimChainIntegrity(ctx, repos, inspector, claim); err != nil {
			return err
		}
		kind, artifactID, revisionID, err := engineering.ParseSubjectKey(claim.SubjectKey)
		if err != nil || kind != engineering.SubjectKindArtifactRevision || artifactID != targetArtifactID {
			return integrityError("proposal source Claim membership has another subject", err)
		}
		if err := validateProposalPriorCapabilityRevision(
			ctx, repos, inspector, targetArtifactID, targetSequence,
			engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID},
		); err != nil {
			return err
		}
		if err := validateProposalAcceptedCapabilityRevision(
			ctx, repos, engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID},
		); err != nil {
			return err
		}
		if err := validateProposalClaimRequirementRoot(ctx, repos, targetArtifactID, claim); err != nil {
			return err
		}
		return nil
	}
	return integrityError("proposal target Execution/Evidence source has no target-rooted Claim", nil)
}

func slicesContainString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func proposalCapabilityRevisionWasAccepted(
	ctx context.Context,
	repos Repositories,
	key engineering.RevisionKey,
) (bool, error) {
	journal, err := repos.RevisionAcceptance.ListByRevision(ctx, key)
	if err != nil {
		return false, err
	}
	if err := validateAcceptanceHistory(key, journal); err != nil {
		return false, err
	}
	accepted := 0
	for _, record := range journal {
		if record.State == engineering.AcceptanceStateAccepted {
			accepted++
		}
	}
	if accepted > 1 {
		return false, integrityError("proposal source capability Revision has contradictory accepted history", nil)
	}
	return accepted == 1, nil
}

func validateProposalAcceptedCapabilityRevision(
	ctx context.Context,
	repos Repositories,
	key engineering.RevisionKey,
) error {
	accepted, err := proposalCapabilityRevisionWasAccepted(ctx, repos, key)
	if err != nil {
		return err
	}
	if !accepted {
		return integrityError("proposal source capability Revision was never accepted", nil)
	}
	return nil
}

func validateProposalRevisionDecisionMembership(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	key engineering.RevisionKey,
) error {
	decisions, err := listValidatedRecordsByKind(ctx, repos, inspector, engineering.RecordKindDecision)
	if err != nil {
		return err
	}
	want := engineering.ArtifactRevisionSubjectKey(key.ArtifactID, key.RevisionID)
	for _, decision := range decisions {
		if decision.SubjectKey != want {
			continue
		}
		if err := validateStoredDecisionReferences(ctx, repos, inspector, decision); err != nil {
			return err
		}
		return nil
	}
	return integrityError("proposal target cites an unaccepted Revision outside any Decision subject", nil)
}

func validateProposalClaimRequirementRoot(
	ctx context.Context,
	repos Repositories,
	targetArtifactID string,
	claim engineering.RecordEnvelope,
) error {
	if len(claim.CriterionKeys) != 1 {
		return integrityError("proposal source Claim has no exact Requirement criterion", nil)
	}
	requirementKey, err := requirementKeyFromCriterion(claim.CriterionKeys[0])
	if err != nil {
		return integrityError("proposal source Claim Requirement projection is malformed", err)
	}
	requirement, found, err := repos.Revisions.Get(ctx, requirementKey)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("proposal source Claim Requirement is dangling", nil)
	}
	kind, artifactID, _, err := engineering.ParseSubjectKey(requirement.SubjectKey)
	if err != nil || kind != engineering.SubjectKindArtifact || artifactID != targetArtifactID {
		return integrityError("proposal source Claim Requirement belongs to another capability", err)
	}
	return nil
}

func validateProposalPriorCapabilityRevision(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	targetArtifactID string,
	targetSequence int,
	key engineering.RevisionKey,
) error {
	if key.ArtifactID != targetArtifactID {
		return integrityError("proposal source capability Revision belongs to another Artifact", nil)
	}
	revision, found, err := repos.Revisions.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("proposal source capability Revision is dangling", nil)
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return err
	}
	if revision.RevisionFamily != engineering.RevisionFamilyCapability {
		return integrityError("proposal source capability Revision names another family", nil)
	}
	order, found, err := repos.RevisionOrder.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found || order.Key != key || order.Sequence < 1 || order.Sequence >= targetSequence {
		return integrityError("proposal source capability Revision is not prior to the AI-assisted target", nil)
	}
	return nil
}

func proposalSourceRevisionKey(value string) (engineering.RevisionKey, error) {
	artifactID, revisionID, ok := strings.Cut(value, "/")
	if !ok {
		return engineering.RevisionKey{}, fmt.Errorf("missing revision separator")
	}
	return engineering.NewRevisionKey(artifactID, revisionID)
}

func proposalCriterionSourceKey(value string) (engineering.RevisionKey, string, error) {
	revision, criterionKey, ok := strings.Cut(value, "#")
	if !ok {
		return engineering.RevisionKey{}, "", fmt.Errorf("missing criterion separator")
	}
	key, err := proposalSourceRevisionKey(revision)
	return key, criterionKey, err
}

func proposalRecordSourceKey(value string) (engineering.RecordKey, error) {
	kind, id, ok := strings.Cut(value, "/")
	if !ok {
		return engineering.RecordKey{}, fmt.Errorf("missing record separator")
	}
	return engineering.NewRecordKey(engineering.RecordKind(kind), id)
}
