package application

import (
	"context"
	"fmt"

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

// CapabilityRevisionsResult lists every revision of a capability alongside
// its resolved current revision (FF-018 §3.2, Q6).
type CapabilityRevisionsResult struct {
	Revisions []engineering.RevisionEnvelope
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
		current, err := ResolveCurrentRevision(ctx, r, artifactID)
		if err != nil {
			return err
		}
		result = CapabilityRevisionsResult{Revisions: revisions, Current: current}
		return nil
	})
	if err != nil {
		return CapabilityRevisionsResult{}, err
	}
	return result, nil
}

// GetCapabilityRevision fetches exactly one revision by key (FF-018 §6.5,
// Q7). Found is false, with no error, when it does not exist -- the
// handler maps that to 404 directly (FF-018 §3.2); this function does not
// manufacture ErrNotFound.
func GetCapabilityRevision(ctx context.Context, uow UnitOfWork, key engineering.RevisionKey) (engineering.RevisionEnvelope, bool, error) {
	var (
		result engineering.RevisionEnvelope
		found  bool
	)
	err := uow.Do(ctx, func(r Repositories) error {
		env, ok, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		result, found = env, ok
		return nil
	})
	if err != nil {
		return engineering.RevisionEnvelope{}, false, err
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
// only a FeatureCardID (FF-018 §6.4, Q3).
func GetFeatureOverview(ctx context.Context, uow UnitOfWork, featureCardID domain.FeatureCardID) (FeatureOverviewResult, error) {
	var result FeatureOverviewResult
	err := uow.Do(ctx, func(r Repositories) error {
		card, state, err := resolveFeatureCardAndState(ctx, r, featureCardID)
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
// when no current revision is found.
func GetFeatureEngineeringStateForCard(ctx context.Context, uow UnitOfWork, featureCardID domain.FeatureCardID) (EngineeringStateResult, error) {
	var result EngineeringStateResult
	err := uow.Do(ctx, func(r Repositories) error {
		_, state, err := resolveFeatureCardAndState(ctx, r, featureCardID)
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
// holding only a FeatureCardID (FF-018 §6.4, Q5).
func GetFeatureTimelineForCard(ctx context.Context, uow UnitOfWork, featureCardID domain.FeatureCardID) (TimelineResult, error) {
	var result TimelineResult
	err := uow.Do(ctx, func(r Repositories) error {
		project, card, err := resolveProjectAndCard(ctx, r, featureCardID)
		if err != nil {
			return err
		}
		artifactID, _ := card.CapabilityArtifactID()
		components, err := discoverEngineeringStateComponents(ctx, r, artifactID)
		if err != nil {
			return err
		}
		timeline, err := GetFeatureTimeline(ctx, r, TimelineInput{
			Project: project, FeatureCard: card,
			CapabilityArtifactID:   artifactID,
			RequirementArtifactIDs: components.requirementArtifactIDs,
			DecisionIDs:            components.decisionIDs,
			PlanArtifactID:         components.planArtifactID,
			ExecutionIDs:           components.executionIDs,
			EvidenceArtifactIDs:    components.evidenceArtifactIDs,
			ClaimIDs:               components.claimIDs,
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
func resolveFeatureCardAndState(ctx context.Context, repos Repositories, featureCardID domain.FeatureCardID) (domain.FeatureCard, EngineeringStateResult, error) {
	card, found, err := repos.FeatureCards.Get(ctx, featureCardID)
	if err != nil {
		return domain.FeatureCard{}, EngineeringStateResult{}, err
	}
	if !found {
		return domain.FeatureCard{}, EngineeringStateResult{}, fmt.Errorf("%w: feature card %s", ErrNotFound, featureCardID)
	}
	artifactID, _ := card.CapabilityArtifactID()

	components, err := discoverEngineeringStateComponents(ctx, repos, artifactID)
	if err != nil {
		return domain.FeatureCard{}, EngineeringStateResult{}, err
	}
	state, err := GetFeatureEngineeringState(ctx, repos, EngineeringStateInput{
		CapabilityArtifactID:   artifactID,
		RequirementArtifactIDs: components.requirementArtifactIDs,
		DecisionIDs:            components.decisionIDs,
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

// engineeringStateComponents holds the identifier lists FF-018 §6.4's
// composition sequence discovers, shared by every Q3/Q4/Q5 entry point so
// the discovery logic exists exactly once.
type engineeringStateComponents struct {
	currentRevision        CurrentRevisionResult
	requirementArtifactIDs []string
	decisionIDs            []string
	planArtifactID         string
	executionIDs           []string
	claimIDs               []string
	evidenceArtifactIDs    []string
}

// discoverEngineeringStateComponents runs FF-018 §6.4 steps 3-6. An empty
// capabilityArtifactID (a feature card with no linked capability) returns
// the zero value without error, matching step 2's "empty-but-well-formed
// state" rule.
func discoverEngineeringStateComponents(ctx context.Context, repos Repositories, capabilityArtifactID string) (engineeringStateComponents, error) {
	var c engineeringStateComponents
	if capabilityArtifactID == "" {
		return c, nil
	}

	currentRevision, err := ResolveCurrentRevision(ctx, repos, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.currentRevision = currentRevision

	requirementIDs, err := DiscoverRequirementArtifactIDs(ctx, repos, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.requirementArtifactIDs = requirementIDs

	decisionIDs, err := DiscoverDecisionIDs(ctx, repos, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.decisionIDs = decisionIDs

	planID, err := ResolveApplicableValidationPlanID(ctx, repos, capabilityArtifactID)
	if err != nil {
		return engineeringStateComponents{}, err
	}
	c.planArtifactID = planID

	if currentRevision.Found {
		executionIDs, claimIDs, err := DiscoverExecutionAndClaimIDs(ctx, repos, capabilityArtifactID, currentRevision.Revision.Key.RevisionID)
		if err != nil {
			return engineeringStateComponents{}, err
		}
		c.executionIDs = executionIDs
		c.claimIDs = claimIDs

		evidenceIDs, err := DiscoverEvidenceArtifactIDs(ctx, repos, executionIDs, claimIDs)
		if err != nil {
			return engineeringStateComponents{}, err
		}
		c.evidenceArtifactIDs = evidenceIDs
	}

	return c, nil
}
