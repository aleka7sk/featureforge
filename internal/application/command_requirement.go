package application

import (
	"context"
	"fmt"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EstablishRequirementCommand records a Requirement Artifact and its
// founding revision together, for the same reason capability establishment
// does (FF-010 §3, "no CRUD-oriented service decomposition": one command
// covers both a requirement's first appearance and any later revision,
// since both record a requirement.Revision the identical way).
type EstablishRequirementCommand struct {
	ArtifactID         string
	RevisionID         string
	Statement          string
	SubjectArtifactID  string
	AcceptanceRecordID *string
}

// EstablishRequirementResult names the keys created.
type EstablishRequirementResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
}

// Execute validates the command and writes the Requirement Artifact and
// Revision in one transaction.
func (c EstablishRequirementCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (EstablishRequirementResult, error) {
	if err := requireIdentity("artifact id", c.ArtifactID); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireIdentity("revision id", c.RevisionID); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireNonEmpty("statement", c.Statement); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireIdentity("subject artifact id", c.SubjectArtifactID); err != nil {
		return EstablishRequirementResult{}, err
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return EstablishRequirementResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())

	var result EstablishRequirementResult
	err = uow.Do(ctx, func(r Repositories) error {
		revisionEnv, revisionFound, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		_, contentFound, err := r.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		order, orderFound, err := r.RevisionOrder.Get(ctx, key)
		if err != nil {
			return err
		}
		journal, err := r.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		pOccupied := revisionFound || contentFound || orderFound || len(journal) > 0

		if pOccupied {
			if !revisionFound {
				return integrityError("requirement pair is partially occupied", nil)
			}
			artifactEnv, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
			if err != nil {
				return err
			}
			if !artifactFound {
				return integrityError("occupied requirement pair has no owning artifact", nil)
			}
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}
			if err := inspectRevision(inspector, revisionEnv); err != nil {
				return err
			}

			if revisionEnv.RevisionFamily != engineering.RevisionFamilyRequirement {
				if err := validateManagedForeignOccupant(ctx, r, inspector, artifactEnv, revisionEnv, orderFound); err != nil {
					return err
				}
				if err := inspectDifferentAcceptanceCandidate(ctx, r, inspector, c.AcceptanceRecordID, ""); err != nil {
					return err
				}
				return immutableConflict("requirement pair is occupied by another revision family")
			}
			if !orderFound || order.Key != key || order.Sequence < 1 || order.RecordedAt.IsZero() {
				return integrityError("requirement pair has contradictory order metadata", nil)
			}
			if !canonicalTimeEqual(revisionEnv.RecordedAt, order.RecordedAt) {
				return integrityError("requirement revision and order metadata disagree on recorded time", nil)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
				return err
			}
			member, err := semanticAcceptanceMember(key, journal)
			if err != nil {
				return err
			}
			if err := inspectDifferentAcceptanceCandidate(ctx, r, inspector, c.AcceptanceRecordID, member.RecordID); err != nil {
				return err
			}

			expectedArtifact, expectedRevision, buildErr := recorder.RecordRequirement(engineering.RequirementInput{
				ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, Statement: c.Statement,
				SubjectArtifactID: c.SubjectArtifactID, RecordedAt: revisionEnv.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !artifactEnv.Equal(expectedArtifact) || !revisionEnv.Equal(expectedRevision) {
				return immutableConflict("requirement pair is occupied by different semantics")
			}
			if err := validateCapabilityArtifactReference(ctx, r, recorder, inspector, c.SubjectArtifactID, "requirement subject", true); err != nil {
				return err
			}
			result = EstablishRequirementResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key}
			return nil
		}

		if c.AcceptanceRecordID == nil {
			return &fieldError{field: "acceptance record id", reason: "is required for a new requirement act"}
		}
		if err := requireAcceptanceMemberIdentity("acceptance record id", *c.AcceptanceRecordID); err != nil {
			return err
		}
		if candidate, found, err := r.RevisionAcceptance.GetByRecordID(ctx, *c.AcceptanceRecordID); err != nil {
			return integrityError("acceptance identity lookup", err)
		} else if found {
			if err := validateAcceptanceCandidate(ctx, r, inspector, candidate); err != nil {
				return err
			}
			return immutableConflict("acceptance record identity already belongs to another act")
		}

		artifactEnv, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
		if err != nil {
			return err
		}
		next := 1
		if artifactFound {
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}
			family, familyErr := inspector.ArtifactFamily(artifactEnv)
			if familyErr != nil {
				return integrityError("inspect requirement artifact family", familyErr)
			}
			if family != engineering.RevisionFamilyRequirement {
				if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifactEnv); err != nil {
					return err
				}
				return immutableConflict("artifact identity belongs to another family")
			}
			historySize, historyErr := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyRequirement, true)
			if historyErr != nil {
				return historyErr
			}
			if err := validateRequestedManagedSubject(ctx, r, c.ArtifactID, engineering.ArtifactSubjectKey(c.SubjectArtifactID), "requirement artifact"); err != nil {
				return err
			}
			next = historySize + 1
		} else if err := validateAbsentArtifactHistory(ctx, r, inspector, c.ArtifactID); err != nil {
			return err
		}

		if err := validateCapabilityArtifactReference(ctx, r, recorder, inspector, c.SubjectArtifactID, "requirement subject", false); err != nil {
			return err
		}
		if now.IsZero() {
			return fmt.Errorf("application: clock returned a zero time for a new requirement act")
		}

		newArtifact, newRevision, err := recorder.RecordRequirement(engineering.RequirementInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, Statement: c.Statement,
			SubjectArtifactID: c.SubjectArtifactID, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if !artifactFound {
			if err := r.Artifacts.Put(ctx, newArtifact); err != nil {
				return err
			}
			artifactEnv = newArtifact
		}
		if err := r.Revisions.Put(ctx, newRevision); err != nil {
			return err
		}

		// Requirement revisions follow the same current-revision resolution
		// policy as capability revisions (FF-004 §3.2: "the same ordering
		// contract"), which requires both order metadata and an accepted
		// journal entry for every stored revision (FF-004 §2 rules 5-6).
		// FF-010 §3's command table lists EstablishRequirement's engineering
		// act as only "Requirement artifact + revision", omitting both --
		// a gap against FF-004 §3.2's own requirement, not a deliberate
		// narrowing. This command closes it the same way
		// EstablishCapabilitySpecification closes the equivalent gap for a
		// capability's founding revision: write sequence-1-or-next order
		// metadata and an immediate "accepted" journal entry, since no
		// separate accept-requirement command exists and this scenario
		// never revises or withdraws a requirement.
		newOrder, err := engineering.NewRevisionOrderMetadata(newRevision.Key, next, now)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionOrder.Put(ctx, newOrder); err != nil {
			return err
		}
		acceptance, err := engineering.NewRevisionAcceptanceRecord(
			*c.AcceptanceRecordID, newRevision.Key, engineering.AcceptanceStateAccepted, now,
			"featureforge:local-user", "requirement established",
		)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionAcceptance.Append(ctx, acceptance); err != nil {
			return err
		}

		result = EstablishRequirementResult{ArtifactKey: artifactEnv.Key, RevisionKey: newRevision.Key}
		return nil
	})
	if err != nil {
		return EstablishRequirementResult{}, err
	}
	return result, nil
}

func inspectDifferentAcceptanceCandidate(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, supplied *string, exactMemberID string) error {
	if supplied == nil || (exactMemberID != "" && *supplied == exactMemberID) {
		return nil
	}
	if err := requireAcceptanceMemberIdentity("acceptance record id", *supplied); err != nil {
		return err
	}
	candidate, found, err := r.RevisionAcceptance.GetByRecordID(ctx, *supplied)
	if err != nil {
		return integrityError("acceptance identity lookup", err)
	}
	if found {
		if err := validateAcceptanceCandidate(ctx, r, inspector, candidate); err != nil {
			return err
		}
	}
	return immutableConflict("a different acceptance member identity was supplied")
}
