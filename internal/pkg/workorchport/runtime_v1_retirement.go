package workorchport

import "context"

type RuntimeV1RetirementOptions struct {
	DryRun bool
}

type RuntimeV1RetirementFailure struct {
	SubjectType string
	SubjectID   string
	Reason      string
}

type RuntimeV1RetirementReport struct {
	ScannedV1Runs             int
	SupersededRuns            int
	ClearedActiveRunIDs       int
	SupersededTickets         int
	SupersededApprovals       int
	ResourceScopesToRelease   int
	ReleasedResourceScopes    int
	ProjectionRebuildRevision int64
	Failures                  []RuntimeV1RetirementFailure
}

type RuntimeV1RetirementResult struct {
	DryRun          bool
	BackupDirectory string
	Report          RuntimeV1RetirementReport
}

type RuntimeV1RetirementBackend interface {
	RetireRuntimeV1(context.Context, RuntimeV1RetirementOptions) (RuntimeV1RetirementResult, error)
}
