package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
)

func TestEnsureLifecycleConfigurationCreatesAndVerifiesExactPair(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	uow := memory.NewUnitOfWork(store)
	recorder := peos.NewRecorder()
	wantDefinition, wantVersion, err := recorder.RecordLifecycleConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); err != nil {
		t.Fatalf("initializing lifecycle configuration: %v", err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		definition, found, err := repos.LifecycleDefinitions.GetDefinition(ctx, wantDefinition.DefinitionID)
		if err != nil || !found || !definition.Equal(wantDefinition) {
			t.Fatalf("definition = %+v, found=%v, err=%v", definition, found, err)
		}
		version, found, err := repos.LifecycleDefinitions.GetVersion(ctx, wantVersion.Key)
		if err != nil || !found || !version.Equal(wantVersion) || !version.RecordedAt.Equal(wantVersion.RecordedAt) {
			t.Fatalf("version = %+v, found=%v, err=%v", version, found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	store.SetFailureHook(func(kind string, _ int) error {
		if kind == "lifecycle-definition" || kind == "lifecycle-definition-version" {
			return errors.New("verification attempted a write")
		}
		return nil
	})
	defer store.SetFailureHook(nil)
	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); err != nil {
		t.Fatalf("verifying exact lifecycle configuration must be zero-write: %v", err)
	}
}

func TestEnsureLifecycleConfigurationRejectsPartialExtraAndCorruptOccupancy(t *testing.T) {
	ctx := context.Background()
	recorder := peos.NewRecorder()
	wantDefinition, wantVersion, err := recorder.RecordLifecycleConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		seed func(t *testing.T, repos application.Repositories)
	}{
		{
			name: "definition without version",
			seed: func(t *testing.T, repos application.Repositories) {
				if err := repos.LifecycleDefinitions.PutDefinition(ctx, wantDefinition); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unexpected second definition",
			seed: func(t *testing.T, repos application.Repositories) {
				if err := repos.LifecycleDefinitions.PutDefinition(ctx, wantDefinition); err != nil {
					t.Fatal(err)
				}
				if err := repos.LifecycleDefinitions.PutVersion(ctx, wantVersion); err != nil {
					t.Fatal(err)
				}
				payload := []byte(`{"definition":"unexpected"}`)
				extra, err := engineering.NewLifecycleDefinitionEnvelope("LCD-EXTRA", payload, engineering.ComputeDigest(payload))
				if err != nil {
					t.Fatal(err)
				}
				if err := repos.LifecycleDefinitions.PutDefinition(ctx, extra); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "corrupt governed payload",
			seed: func(t *testing.T, repos application.Repositories) {
				payload := []byte(`{"definition":"corrupt"}`)
				corrupt, err := engineering.NewLifecycleDefinitionEnvelope(wantDefinition.DefinitionID, payload, engineering.ComputeDigest(payload))
				if err != nil {
					t.Fatal(err)
				}
				if err := repos.LifecycleDefinitions.PutDefinition(ctx, corrupt); err != nil {
					t.Fatal(err)
				}
				if err := repos.LifecycleDefinitions.PutVersion(ctx, wantVersion); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := memory.NewStore()
			uow := memory.NewUnitOfWork(store)
			if err := uow.Do(ctx, func(repos application.Repositories) error {
				tt.seed(t, repos)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
			}
		})
	}
}

func TestEnsureLifecycleConfigurationRollsBackDefinitionWhenVersionWriteFails(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	uow := memory.NewUnitOfWork(store)
	recorder := peos.NewRecorder()
	injected := errors.New("injected lifecycle version failure")
	store.SetFailureHook(func(kind string, _ int) error {
		if kind == "lifecycle-definition-version" {
			return injected
		}
		return nil
	})
	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); !errors.Is(err, injected) {
		t.Fatalf("err = %v, want injected version failure", err)
	}
	store.SetFailureHook(nil)

	if err := uow.Do(ctx, func(repos application.Repositories) error {
		definitions, err := repos.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		if len(definitions) != 0 {
			t.Fatalf("rollback left lifecycle definitions: %+v", definitions)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); err != nil {
		t.Fatalf("initialization after rollback: %v", err)
	}
}
