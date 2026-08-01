package peos

import (
	"encoding/json"
	"fmt"

	"github.com/aleka7sk/PEOS/peos/lifecycle"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// BuildLifecycleConfiguration returns the canonical persisted carriers for
// FeatureForge's one fixed Lifecycle Definition and Definition Version.
func BuildLifecycleConfiguration() (engineering.LifecycleDefinitionEnvelope, engineering.LifecycleDefinitionVersionEnvelope, error) {
	definitionPayload, err := json.Marshal(configuredDefinition)
	if err != nil {
		return engineering.LifecycleDefinitionEnvelope{}, engineering.LifecycleDefinitionVersionEnvelope{}, wrapPEOS("lifecycle definition marshal", err)
	}
	definitionEnvelope, err := engineering.NewLifecycleDefinitionEnvelope(
		configuredDefinition.ID().String(), definitionPayload, engineering.ComputeDigest(definitionPayload),
	)
	if err != nil {
		return engineering.LifecycleDefinitionEnvelope{}, engineering.LifecycleDefinitionVersionEnvelope{}, err
	}

	versionPayload, err := json.Marshal(configuredDefinitionVersion)
	if err != nil {
		return engineering.LifecycleDefinitionEnvelope{}, engineering.LifecycleDefinitionVersionEnvelope{}, wrapPEOS("lifecycle definition version marshal", err)
	}
	versionKey, err := engineering.NewLifecycleDefinitionVersionKey(
		configuredDefinitionVersion.Definition().LifecycleDefinitionID().String(),
		configuredDefinitionVersion.ID().String(),
	)
	if err != nil {
		return engineering.LifecycleDefinitionEnvelope{}, engineering.LifecycleDefinitionVersionEnvelope{}, err
	}
	versionEnvelope, err := engineering.NewLifecycleDefinitionVersionEnvelope(
		versionKey, versionPayload, engineering.ComputeDigest(versionPayload), configurationRecordedAt,
	)
	if err != nil {
		return engineering.LifecycleDefinitionEnvelope{}, engineering.LifecycleDefinitionVersionEnvelope{}, err
	}
	return definitionEnvelope, versionEnvelope, nil
}

// RecordLifecycleConfiguration implements application.EngineeringRecorder.
func (Recorder) RecordLifecycleConfiguration() (engineering.LifecycleDefinitionEnvelope, engineering.LifecycleDefinitionVersionEnvelope, error) {
	return BuildLifecycleConfiguration()
}

// ConfiguredLifecycleVersionKey implements application.EngineeringReplayInspector.
func (Recorder) ConfiguredLifecycleVersionKey() engineering.LifecycleDefinitionVersionKey {
	key, err := engineering.NewLifecycleDefinitionVersionKey(
		configuredDefinitionVersion.Definition().LifecycleDefinitionID().String(),
		configuredDefinitionVersion.ID().String(),
	)
	if err != nil {
		panic(err)
	}
	return key
}

// InspectLifecycleConfiguration validates both authoritative payloads and
// projects only the lifecycle policy facts application needs.
func (Recorder) InspectLifecycleConfiguration(definitionEnvelope engineering.LifecycleDefinitionEnvelope, versionEnvelope engineering.LifecycleDefinitionVersionEnvelope) (engineering.LifecyclePolicy, error) {
	if !engineering.ComputeDigest(definitionEnvelope.Payload).Equal(definitionEnvelope.PayloadDigest) {
		return engineering.LifecyclePolicy{}, fmt.Errorf("lifecycle definition payload digest mismatch")
	}
	if !engineering.ComputeDigest(versionEnvelope.Payload).Equal(versionEnvelope.PayloadDigest) {
		return engineering.LifecyclePolicy{}, fmt.Errorf("lifecycle definition version payload digest mismatch")
	}

	definition, err := DecodeLifecycleDefinition(definitionEnvelope.Payload)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	version, err := DecodeLifecycleDefinitionVersion(versionEnvelope.Payload)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	if definition.ID().String() != definitionEnvelope.DefinitionID {
		return engineering.LifecyclePolicy{}, fmt.Errorf("lifecycle definition identity projection mismatch")
	}
	if version.Definition().LifecycleDefinitionID().String() != versionEnvelope.Key.DefinitionID || version.ID().String() != versionEnvelope.Key.VersionID {
		return engineering.LifecyclePolicy{}, fmt.Errorf("lifecycle definition version identity projection mismatch")
	}
	if err := version.ValidateArtifactBinding(definition); err != nil {
		return engineering.LifecyclePolicy{}, err
	}

	expectedDefinition, expectedVersion, err := BuildLifecycleConfiguration()
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	if !definitionEnvelope.Equal(expectedDefinition) || !versionEnvelope.Equal(expectedVersion) || !versionEnvelope.RecordedAt.Equal(expectedVersion.RecordedAt) {
		return engineering.LifecyclePolicy{}, fmt.Errorf("stored lifecycle configuration differs from the governed configuration")
	}

	initialStates := make([]string, 0, len(version.InitialStates()))
	for _, state := range version.InitialStates() {
		initialStates = append(initialStates, trimFeatureForgeNamespace(state.String()))
	}
	transitions := make([]engineering.LifecycleTransitionPolicy, 0, len(version.Transitions()))
	for _, transition := range version.Transitions() {
		sources := make([]string, 0, len(transition.SourceStates()))
		for _, state := range transition.SourceStates() {
			sources = append(sources, trimFeatureForgeNamespace(state.String()))
		}
		targets := make([]string, 0, len(transition.TargetStates()))
		for _, state := range transition.TargetStates() {
			targets = append(targets, trimFeatureForgeNamespace(state.String()))
		}
		transitions = append(transitions, engineering.LifecycleTransitionPolicy{
			TransitionID: trimFeatureForgeNamespace(transition.ID().String()),
			SourceStates: sources,
			TargetStates: targets,
		})
	}
	return engineering.NewLifecyclePolicy(
		definition.ID().String(), version.ID().String(),
		trimFeatureForgeNamespace(version.EntryTransition().String()), initialStates, transitions,
	)
}

// DecodeLifecycleDefinition decodes one stored authoritative definition.
func DecodeLifecycleDefinition(payload []byte) (lifecycle.Definition, error) {
	var definition lifecycle.Definition
	if err := json.Unmarshal(payload, &definition); err != nil {
		return lifecycle.Definition{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return definition, nil
}

// DecodeLifecycleDefinitionVersion decodes one stored authoritative version.
func DecodeLifecycleDefinitionVersion(payload []byte) (lifecycle.DefinitionVersion, error) {
	var version lifecycle.DefinitionVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return lifecycle.DefinitionVersion{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return version, nil
}

func trimFeatureForgeNamespace(value string) string {
	const prefix = Namespace + ":"
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}
