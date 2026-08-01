package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// LifecycleDefinitionRepository persists the one configured Lifecycle
// Definition and its immutable versions. It is deliberately separate from
// ArtifactEnvelopeRepository because PEOS gives these values their own
// identities.
type LifecycleDefinitionRepository interface {
	PutDefinition(context.Context, engineering.LifecycleDefinitionEnvelope) error
	GetDefinition(context.Context, string) (engineering.LifecycleDefinitionEnvelope, bool, error)
	ListDefinitions(context.Context) ([]engineering.LifecycleDefinitionEnvelope, error)
	PutVersion(context.Context, engineering.LifecycleDefinitionVersionEnvelope) error
	GetVersion(context.Context, engineering.LifecycleDefinitionVersionKey) (engineering.LifecycleDefinitionVersionEnvelope, bool, error)
	ListVersions(context.Context, string) ([]engineering.LifecycleDefinitionVersionEnvelope, error)
}
