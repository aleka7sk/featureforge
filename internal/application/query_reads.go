package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// This file composes the HTTP query surface (FF-018 §6.4, §6.5): thin
// UnitOfWork-driven wrappers around single repository reads, and the
// FeatureCardID-driven composition that resolves the identifier lists
// GetFeatureEngineeringState and GetFeatureTimeline require, so a caller
// holding only a FeatureCardID or an artifact ID -- never Repositories, and
// never UnitOfWork.Do itself (FF-015 §4.1) -- can reach them.

// ListProjects returns every stored project (FF-018 §6.5, Q1).
func ListProjects(ctx context.Context, uow UnitOfWork) ([]domain.Project, error) {
	var result []domain.Project
	err := uow.Do(ctx, func(r Repositories) error {
		list, err := r.Projects.List(ctx)
		if err != nil {
			return err
		}
		result = list
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListFeaturesByProject returns every feature card belonging to projectID
// (FF-018 §6.5, Q2).
func ListFeaturesByProject(ctx context.Context, uow UnitOfWork, projectID domain.ProjectID) ([]domain.FeatureCard, error) {
	var result []domain.FeatureCard
	err := uow.Do(ctx, func(r Repositories) error {
		list, err := r.FeatureCards.ListByProject(ctx, projectID)
		if err != nil {
			return err
		}
		result = list
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RevisionWithContent pairs a capability revision envelope with its
// structured content (FF-020 §2 class A, FF-001 §3.3: "full specification
// content per revision"). HasContent is false, with no error, when no
// content was ever stored for this revision -- an empty state, not an
// error; every capability revision this module's own commands create does
// store content, so absence means a revision from outside that path.
type RevisionWithContent struct {
	Revision   engineering.RevisionEnvelope
	Content    engineering.CapabilitySpecificationContent
	HasContent bool
}

// CapabilityRevisionsResult lists every revision of a capability, each with
// its content, alongside its resolved current revision (FF-018 §3.2, Q6).
type CapabilityRevisionsResult struct {
	Revisions []RevisionWithContent
	Current   CurrentRevisionResult
}

// GetCapabilityRevisions composes CapabilityRevisionsResult for a caller
// holding only a capability artifact ID (FF-018 §6.5, Q6).
func GetCapabilityRevisions(ctx context.Context, uow UnitOfWork, artifactID string) (CapabilityRevisionsResult, error) {
	var result CapabilityRevisionsResult
	err := uow.Do(ctx, func(r Repositories) error {
		revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
		if err != nil {
			return err
		}
		withContent := make([]RevisionWithContent, 0, len(revisions))
		for _, rev := range revisions {
			content, found, err := r.StructuredContent.Get(ctx, rev.Key)
			if err != nil {
				return err
			}
			withContent = append(withContent, RevisionWithContent{Revision: rev, Content: content, HasContent: found})
		}
		current, err := ResolveCurrentRevision(ctx, r, artifactID)
		if err != nil {
			return err
		}
		result = CapabilityRevisionsResult{Revisions: withContent, Current: current}
		return nil
	})
	if err != nil {
		return CapabilityRevisionsResult{}, err
	}
	return result, nil
}

// GetCapabilityRevision fetches exactly one revision by key, with its
// content (FF-018 §6.5, Q7; FF-020 §2 class A). Found is false, with no
// error, when the revision itself does not exist -- the handler maps that
// to 404 directly (FF-018 §3.2); this function does not manufacture
// ErrNotFound.
func GetCapabilityRevision(ctx context.Context, uow UnitOfWork, key engineering.RevisionKey) (RevisionWithContent, bool, error) {
	var (
		result RevisionWithContent
		found  bool
	)
	err := uow.Do(ctx, func(r Repositories) error {
		env, ok, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		content, hasContent, err := r.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		result, found = RevisionWithContent{Revision: env, Content: content, HasContent: hasContent}, true
		return nil
	})
	if err != nil {
		return RevisionWithContent{}, false, err
	}
	return result, found, nil
}

// FeatureOverviewResult composes a feature card with its current
// engineering state (FF-018 §3.2, Q3).
type FeatureOverviewResult struct {
	FeatureCard domain.FeatureCard
	State       EngineeringStateResult
}

// GetFeatureOverview composes FeatureOverviewResult for a caller holding
// only a FeatureCardID (FF-018 §6.4, Q3). projector decodes the FF-020
// display content GetFeatureEngineeringState now renders.
func GetFeatureOverview(ctx context.Context, uow UnitOfWork, projector EngineeringProjector, inspector EngineeringReplayInspector, featureCardID domain.FeatureCardID) (FeatureOverviewResult, error) {
	var result FeatureOverviewResult
	err := uow.Do(ctx, func(r Repositories) error {
		card, state, err := resolveFeatureCardAndState(ctx, r, projector, inspector, featureCardID)
		if err != nil {
			return err
		}
		result = FeatureOverviewResult{FeatureCard: card, State: state}
		return nil
	})
	if err != nil {
		return FeatureOverviewResult{}, err
	}
	return result, nil
}

// GetFeatureEngineeringStateForCard composes GetFeatureEngineeringState for
// a caller holding only a FeatureCardID (FF-018 §6.4, Q4). A card with no
// linked capability yields a well-formed, empty-but-Incomplete state, not
// an error -- the same fallback GetFeatureEngineeringState already applies
// when no current revision is found. projector decodes the FF-020 display
// content GetFeatureEngineeringState now renders.
func GetFeatureEngineeringStateForCard(ctx context.Context, uow UnitOfWork, projector EngineeringProjector, inspector EngineeringReplayInspector, featureCardID domain.FeatureCardID) (EngineeringStateResult, error) {
	var result EngineeringStateResult
	err := uow.Do(ctx, func(r Repositories) error {
		_, state, err := resolveFeatureCardAndState(ctx, r, projector, inspector, featureCardID)
		if err != nil {
			return err
		}
		result = state
		return nil
	})
	if err != nil {
		return EngineeringStateResult{}, err
	}
	return result, nil
}

// GetFeatureTimelineForCard composes GetFeatureTimeline for a caller
// holding only a FeatureCardID (FF-018 §6.4, Q5). Unlike Q3/Q4's
// discoverEngineeringStateComponents, execution and claim discovery here is
// history-wide, not scoped to the current revision; Evidence is the validated
// union of their citations and the discovered Decisions' citations
// (FF-018 §24, M.5 publication remediation D1/D2) -- an empty artifactID
// (no linked capability) yields no decision/execution/claim/evidence population,
// matching discoverEngineeringStateComponents's own empty-but-well-formed
// rule for that case.
func GetFeatureTimelineForCard(ctx context.Context, uow UnitOfWork, projector EngineeringProjector, inspector EngineeringReplayInspector, featureCardID domain.FeatureCardID) (TimelineResult, error) {
	var result TimelineResult
	err := uow.Do(ctx, func(r Repositories) error {
		project, card, err := resolveProjectAndCard(ctx, r, featureCardID)
		if err != nil {
			return err
		}
		artifactID, _ := card.CapabilityArtifactID()
		components, err := discoverEngineeringStateComponents(ctx, r, inspector, artifactID)
		if err != nil {
			return err
		}
		var executionIDs, claimIDs, evidenceArtifactIDs []string
		if artifactID != "" {
			executionIDs, claimIDs, err = DiscoverExecutionAndClaimIDsAllRevisions(ctx, r, inspector, artifactID)
			if err != nil {
				return err
			}
			executionAndClaimEvidence, discoverErr := DiscoverEvidenceArtifactIDs(ctx, r, inspector, executionIDs, claimIDs)
			if discoverErr != nil {
				return discoverErr
			}
			decisionEvidence, discoverErr := DiscoverDecisionEvidenceArtifactIDs(ctx, r, inspector, components.decisionIDs)
			if discoverErr != nil {
				return discoverErr
			}
			seenEvidence := make(map[string]bool, len(executionAndClaimEvidence)+len(decisionEvidence))
			for _, evidenceID := range append(executionAndClaimEvidence, decisionEvidence...) {
				if !seenEvidence[evidenceID] {
					seenEvidence[evidenceID] = true
					evidenceArtifactIDs = append(evidenceArtifactIDs, evidenceID)
				}
			}
			sort.Strings(evidenceArtifactIDs)
		}
		timeline, err := GetFeatureTimeline(ctx, r, projector, inspector, TimelineInput{
			Project: project, FeatureCard: card,
			CapabilityArtifactID:   artifactID,
			RequirementArtifactIDs: components.requirementArtifactIDs,
			DecisionIDs:            components.decisionIDs,
			PlanArtifactID:         components.planArtifactID,
			ExecutionIDs:           executionIDs,
			EvidenceArtifactIDs:    evidenceArtifactIDs,
			ClaimIDs:               claimIDs,
		})
		if err != nil {
			return err
		}
		result = timeline
		return nil
	})
	if err != nil {
		return TimelineResult{}, err
	}
	return result, nil
}

// resolveFeatureCardAndState is shared by GetFeatureOverview and
// GetFeatureEngineeringStateForCard so the card lookup and discovery
// sequence exists exactly once (FF-018 §6.4 steps 1-7).
func resolveFeatureCardAndState(ctx context.Context, repos Repositories, projector EngineeringProjector, inspector EngineeringReplayInspector, featureCardID domain.FeatureCardID) (domain.FeatureCard, EngineeringStateResult, error) {
	card, found, err := repos.FeatureCards.Get(ctx, featureCardID)
	if err != nil {
		return domain.FeatureCard{}, EngineeringStateResult{}, err
	}
	if !found {
		return domain.FeatureCard{}, EngineeringStateResult{}, fmt.Errorf("%w: feature card %s", ErrNotFound, featureCardID)
	}
	artifactID, _ := card.CapabilityArtifactID()

	components, err := discoverEngineeringStateComponents(ctx, repos, inspector, artifactID)
	if err != nil {
		return domain.FeatureCard{}, EngineeringStateResult{}, err
	}
	state, err := GetFeatureEngineeringState(ctx, repos, projector, inspector, EngineeringStateInput{
		CapabilityArtifactID:   artifactID,
		RequirementArtifactIDs: components.requirementArtifactIDs,
		DecisionIDs:            components.decisionIDs,
		PlanArtifactID:         components.planArtifactID,
	})
	if err != nil {
		return domain.FeatureCard{}, EngineeringStateResult{}, err
	}
	return card, state, nil
}

// resolveProjectAndCard fetches a feature card and its owning project, for
// GetFeatureTimelineForCard, which needs both to build TimelineInput.
func resolveProjectAndCard(ctx context.Context, repos Repositories, featureCardID domain.FeatureCardID) (domain.Project, domain.FeatureCard, error) {
	card, found, err := repos.FeatureCards.Get(ctx, featureCardID)
	if err != nil {
		return domain.Project{}, domain.FeatureCard{}, err
	}
	if !found {
		return domain.Project{}, domain.FeatureCard{}, fmt.Errorf("%w: feature card %s", ErrNotFound, featureCardID)
	}
	project, found, err := repos.Projects.Get(ctx, card.ProjectID())
	if err != nil {
		return domain.Project{}, domain.FeatureCard{}, err
	}
	if !found {
		return domain.Project{}, domain.FeatureCard{}, fmt.Errorf("%w: project %s", ErrNotFound, card.ProjectID())
	}
	return project, card, nil
}

// engineeringStateComponents holds the identifier lists shared by every
// Q3/Q4/Q5 entry point so this discovery logic exists exactly once
// (FF-018 §6.4 steps 3-5). Execution, claim, and evidence discovery are
// deliberately not included here: Q3/Q4 do not consume them (readiness
// resolves claims independently, scoped to the current revision, via
// ResolveCurrentClaim), and Q5's population must be history-wide, not
// current-revision-scoped -- a distinct concern GetFeatureTimelineForCard
// owns directly (FF-018 §24, M.5 publication remediation D1/D2).
type engineeringStateComponents struct {
	requirementArtifactIDs []string
	decisionIDs            []string
	planArtifactID         string
}

// discoverEngineeringStateComponents runs FF-018 §6.4 steps 3-5. An empty
// capabilityArtifactID (a feature card with no linked capability) returns
// the zero value without error, matching step 2's "empty-but-well-formed
// state" rule.
func discoverEngineeringStateComponents(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) (engineeringStateComponents, error) {
	var c engineeringStateComponents
	if capabilityArtifactID == "" {
		return c, nil
	}

	requirementIDs, err := DiscoverRequirementArtifactIDs(ctx, repos, inspector, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.requirementArtifactIDs = requirementIDs

	decisionIDs, err := DiscoverDecisionIDs(ctx, repos, inspector, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.decisionIDs = decisionIDs

	planID, err := ResolveApplicableValidationPlanID(ctx, repos, inspector, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.planArtifactID = planID

	return c, nil
}
