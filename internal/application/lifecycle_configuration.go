package application

import (
	"context"
	"fmt"
)

// EnsureLifecycleConfiguration atomically creates or verifies FeatureForge's
// one governed lifecycle Definition/Version pair. Commands and queries never
// invoke it implicitly.
func EnsureLifecycleConfiguration(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector) error {
	expectedDefinition, expectedVersion, err := recorder.RecordLifecycleConfiguration()
	if err != nil {
		return fmt.Errorf("recording governed lifecycle configuration: %w", err)
	}
	return uow.Do(ctx, func(repos Repositories) error {
		definitions, err := repos.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		versions, err := repos.LifecycleDefinitions.ListVersions(ctx, expectedDefinition.DefinitionID)
		if err != nil {
			return err
		}
		definition, definitionFound, err := repos.LifecycleDefinitions.GetDefinition(ctx, expectedDefinition.DefinitionID)
		if err != nil {
			return err
		}
		version, versionFound, err := repos.LifecycleDefinitions.GetVersion(ctx, expectedVersion.Key)
		if err != nil {
			return err
		}

		if len(definitions) == 0 && len(versions) == 0 && !definitionFound && !versionFound {
			if err := repos.LifecycleDefinitions.PutDefinition(ctx, expectedDefinition); err != nil {
				return err
			}
			return repos.LifecycleDefinitions.PutVersion(ctx, expectedVersion)
		}
		if len(definitions) != 1 || definitions[0].DefinitionID != expectedDefinition.DefinitionID || len(versions) != 1 || versions[0].Key != expectedVersion.Key || !definitionFound || !versionFound {
			return integrityError("lifecycle configuration occupancy is partial or unexpected", nil)
		}
		if _, err := inspector.InspectLifecycleConfiguration(definition, version); err != nil {
			return integrityError("lifecycle configuration is unreadable or contradictory", err)
		}
		if !definition.Equal(expectedDefinition) || !version.Equal(expectedVersion) || !version.RecordedAt.Equal(expectedVersion.RecordedAt) {
			return integrityError("lifecycle configuration differs from the governed pair", nil)
		}
		return nil
	})
}
