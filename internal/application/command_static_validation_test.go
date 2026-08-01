package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type failIfTransactionStarts struct{}

func (failIfTransactionStarts) Do(context.Context, func(application.Repositories) error) error {
	panic("static validation entered UnitOfWork")
}

func assertInvalidCommandWithoutWrites[T any](t *testing.T, f commandFixture, execute func() (T, error)) {
	t.Helper()
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := execute(); !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("err = %v, want ErrInvalidCommand", err)
	}
}

func staticPlanCommand() application.EstablishValidationPlanCommand {
	return application.EstablishValidationPlanCommand{
		ArtifactID: "VP-STATIC", RevisionID: "VP-STATIC-REV-1", ScopeArtifactID: "CAP-STATIC",
		AcceptanceRecordID: memberID("MEM-VP-STATIC"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "ACT-STATIC", SubjectArtifactID: "CAP-STATIC", SubjectRevisionID: "CAP-STATIC-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Static validation succeeds.",
			RequirementArtifactID: "REQ-STATIC", RequirementRevisionID: "REQ-STATIC-REV-1",
			ExpectedEvidence: []string{"review report"},
		}},
	}
}

func staticRunCommand() application.RecordValidationRunCommand {
	return application.RecordValidationRunCommand{
		ExecutionID: "ER-STATIC", PlanArtifactID: "VP-STATIC", PlanRevisionID: "VP-STATIC-REV-1",
		ActivityKey: "ACT-STATIC", SubjectArtifactID: "CAP-STATIC", SubjectRevisionID: "CAP-STATIC-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-STATIC", EvidenceRevisionID: "EV-STATIC-REV-1",
		EvidenceLocator: "https://evidence.example/static",
	}
}

func staticClaimCommand() application.RecordValidationClaimCommand {
	return application.RecordValidationClaimCommand{
		ClaimID: "CLM-STATIC", ScopeArtifactID: "CAP-STATIC",
		SubjectArtifactID: "CAP-STATIC", SubjectRevisionID: "CAP-STATIC-REV-1",
		RequirementArtifactID: "REQ-STATIC", RequirementRevisionID: "REQ-STATIC-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-STATIC", EvidenceRevisionID: "EV-STATIC-REV-1", ExecutionID: "ER-STATIC",
	}
}

func staticCorrectionCommand() application.CorrectValidationClaimCommand {
	return application.CorrectValidationClaimCommand{
		ClaimID: "CLM-STATIC-CORRECTION", CorrectionTarget: "CLM-STATIC-TARGET", CorrectionKind: "correct",
		ScopeArtifactID: "CAP-STATIC", SubjectArtifactID: "CAP-STATIC", SubjectRevisionID: "CAP-STATIC-REV-1",
		RequirementArtifactID: "REQ-STATIC", RequirementRevisionID: "REQ-STATIC-REV-1",
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-STATIC", EvidenceRevisionID: "EV-STATIC-REV-1", ExecutionID: "ER-STATIC",
	}
}

func TestC6StaticValidationRejectsInvalidStateAndTransition(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid state vocabulary", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-STATIC", SubjectArtifactID: "CAP-STATIC", State: "unknown-state", IsEntry: true,
			TransitionRecordArtifactID: "TR-STATIC", TransitionRecordRevisionID: "TR-STATIC-REV-0",
		}
		assertInvalidCommandWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("invalid transition vocabulary", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-STATIC", SubjectArtifactID: "CAP-STATIC", State: "specified",
			TransitionRecordArtifactID: "TR-STATIC", TransitionRecordRevisionID: "TR-STATIC-REV-1",
			TransitionKey: "unknown-transition", FromAssignmentID: "SA-STATIC-PREVIOUS",
		}
		assertInvalidCommandWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("transition target mismatch", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-STATIC", SubjectArtifactID: "CAP-STATIC", State: "under-validation",
			TransitionRecordArtifactID: "TR-STATIC", TransitionRecordRevisionID: "TR-STATIC-REV-1",
			TransitionKey: "specify", FromAssignmentID: "SA-STATIC-PREVIOUS",
		}
		assertInvalidCommandWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestOptionalWhitespaceIsRejectedBeforeStoredStateInspection(t *testing.T) {
	ctx := context.Background()

	t.Run("C8 rationale", func(t *testing.T) {
		cmd := application.RecordArchitectureDecisionCommand{
			DecisionID: "DEC-WHITESPACE", SubjectArtifactID: "CAP-WHITESPACE", SubjectRevisionID: "CAP-WHITESPACE-REV-1",
			Question: "Is whitespace rationale valid?", OutcomeStatement: "No.",
			EvidenceArtifactID: "EV-WHITESPACE", EvidenceRevisionID: "EV-WHITESPACE-REV-1", Rationale: "   ",
		}
		if _, err := cmd.Execute(ctx, failIfTransactionStarts{}, nil, nil, nil); !errors.Is(err, application.ErrInvalidCommand) {
			t.Fatalf("err = %v, want ErrInvalidCommand", err)
		}
	})

	t.Run("C11 reasoning", func(t *testing.T) {
		cmd := staticClaimCommand()
		cmd.Reasoning = "\t "
		if _, err := cmd.Execute(ctx, failIfTransactionStarts{}, nil, nil, nil); !errors.Is(err, application.ErrInvalidCommand) {
			t.Fatalf("err = %v, want ErrInvalidCommand", err)
		}
	})

	t.Run("C12 reasoning", func(t *testing.T) {
		cmd := staticCorrectionCommand()
		cmd.Reasoning = "\n "
		if _, err := cmd.Execute(ctx, failIfTransactionStarts{}, nil, nil, nil); !errors.Is(err, application.ErrInvalidCommand) {
			t.Fatalf("err = %v, want ErrInvalidCommand", err)
		}
	})
}

func TestC8StaticValidationRejectsBlankCollectionMembers(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*application.RecordArchitectureDecisionCommand){
		"alternative": func(c *application.RecordArchitectureDecisionCommand) { c.Alternatives = []string{" "} },
		"assumption":  func(c *application.RecordArchitectureDecisionCommand) { c.Assumptions = []string{" "} },
		"constraint":  func(c *application.RecordArchitectureDecisionCommand) { c.Constraints = []string{" "} },
		"uncertainty": func(c *application.RecordArchitectureDecisionCommand) { c.Uncertainties = []string{" "} },
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			cmd := application.RecordArchitectureDecisionCommand{
				DecisionID: "DEC-STATIC", SubjectArtifactID: "CAP-STATIC", SubjectRevisionID: "CAP-STATIC-REV-1",
				Question: "Is the request valid?", OutcomeStatement: "Only valid requests continue.",
				EvidenceArtifactID: "EV-STATIC", EvidenceRevisionID: "EV-STATIC-REV-1",
			}
			mutate(&cmd)
			assertInvalidCommandWithoutWrites(t, f, func() (application.RecordArchitectureDecisionResult, error) {
				return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC9StaticValidationRejectsInvalidActivityFields(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*application.EstablishValidationPlanCommand){
		"invalid method": func(c *application.EstablishValidationPlanCommand) {
			c.Activities[0].Method = "automated-analysis"
		},
		"blank outcome interpretation": func(c *application.EstablishValidationPlanCommand) {
			c.Activities[0].OutcomeInterpretation = " "
		},
		"blank expected evidence": func(c *application.EstablishValidationPlanCommand) {
			c.Activities[0].ExpectedEvidence = []string{" "}
		},
		"duplicate activity key": func(c *application.EstablishValidationPlanCommand) {
			c.Activities = append(c.Activities, c.Activities[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			cmd := staticPlanCommand()
			mutate(&cmd)
			assertInvalidCommandWithoutWrites(t, f, func() (application.EstablishValidationPlanResult, error) {
				return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC10StaticValidationRejectsInvalidMethodAndOutcomeWithoutWrites(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid method", func(t *testing.T) {
		f := newCommandFixture()
		cmd := staticRunCommand()
		cmd.Method = "automated-analysis"
		assertInvalidCommandWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("absent primary with invalid outcome writes no Evidence", func(t *testing.T) {
		f := newCommandFixture()
		cmd := staticRunCommand()
		cmd.Outcome = "unknown-outcome"
		assertInvalidCommandWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestC10StaticValidationPrecedesCorruptPrimaryInspection(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	cmd := semanticValidationRun("ER-STATIC-CORRUPT")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed validation run: %v", err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: cmd.EvidenceArtifactID},
		}
	}}

	release := forbidPersistenceWrites(f)
	if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		release()
		t.Fatalf("valid replay over corrupt primary err = %v, want ErrStoredStateIntegrity", err)
	}
	release()

	invalid := cmd
	invalid.Outcome = "unknown-outcome"
	assertInvalidCommandWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
		return invalid.Execute(ctx, uow, f.rec, f.rec, f.clock)
	})
}

func TestC11AndC12StaticValidationRejectsInvalidMethodAndOutcome(t *testing.T) {
	ctx := context.Background()

	for name, execute := range map[string]func(commandFixture) error{
		"C11 invalid method": func(f commandFixture) error {
			cmd := staticClaimCommand()
			cmd.Method = "automated-analysis"
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			return err
		},
		"C11 invalid outcome": func(f commandFixture) error {
			cmd := staticClaimCommand()
			cmd.Outcome = "unknown-outcome"
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			return err
		},
		"C12 invalid method": func(f commandFixture) error {
			cmd := staticCorrectionCommand()
			cmd.Method = "automated-analysis"
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			return err
		},
		"C12 invalid outcome": func(f commandFixture) error {
			cmd := staticCorrectionCommand()
			cmd.Outcome = "unknown-outcome"
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			release := forbidPersistenceWrites(f)
			defer release()
			if err := execute(f); !errors.Is(err, application.ErrInvalidCommand) {
				t.Fatalf("err = %v, want ErrInvalidCommand", err)
			}
		})
	}
}
