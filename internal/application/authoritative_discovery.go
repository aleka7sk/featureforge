package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// listValidatedRevisions enumerates the complete RevisionEnvelope population
// before validating any projected family or subject. A family-scoped query
// cannot be used for integrity-sensitive discovery: a payload whose stored
// RevisionFamily projection drifted would otherwise be invisible to the
// family it authoritatively belongs to.
func listValidatedRevisions(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector) ([]engineering.RevisionEnvelope, error) {
	revisions, err := repos.Revisions.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, revision := range revisions {
		if err := inspectRevision(inspector, revision); err != nil {
			return nil, err
		}
	}
	return revisions, nil
}

// listValidatedRecords is the RecordEnvelope counterpart to
// listValidatedRevisions. Callers filter Kind and SubjectKey only after every
// envelope has passed authoritative payload/projection validation.
func listValidatedRecords(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector) ([]engineering.RecordEnvelope, error) {
	records, err := repos.Records.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if err := inspectRecord(inspector, record); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func listValidatedRecordsByKindAndSubject(
	ctx context.Context,
	repos Repositories,
	inspector EngineeringReplayInspector,
	kind engineering.RecordKind,
	subjectKey string,
) ([]engineering.RecordEnvelope, error) {
	all, err := listValidatedRecords(ctx, repos, inspector)
	if err != nil {
		return nil, err
	}
	out := make([]engineering.RecordEnvelope, 0)
	for _, record := range all {
		if record.Kind == kind && record.SubjectKey == subjectKey {
			out = append(out, record)
		}
	}
	return out, nil
}

func listValidatedRecordsByKind(
	ctx context.Context,
	repos Repositories,
	inspector EngineeringReplayInspector,
	kind engineering.RecordKind,
) ([]engineering.RecordEnvelope, error) {
	all, err := listValidatedRecords(ctx, repos, inspector)
	if err != nil {
		return nil, err
	}
	out := make([]engineering.RecordEnvelope, 0)
	for _, record := range all {
		if record.Kind == kind {
			out = append(out, record)
		}
	}
	return out, nil
}
