package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// activityProjector is a typed, stateless read-only facade table.
type activityProjector struct {
	EventRecords              func([]*eventstreammodel.EventRecord) []projectioncontract.EventRecord
	ProjectedEventRecords     func([]*eventstreammodel.EventRecord) []projectioncontract.EventRecord
	MemoryRecallTraces        func([]*memorymodel.MemoryRecallTrace) []projectioncontract.MemoryRecallTrace
	ClosureTraces             func(readmodelcore.RunSnapshotState) []projectioncontract.ClosureTrace
	GitOperationTraces        func([]projectioncontract.StepView) []projectioncontract.GitOperationTrace
	WebSourceTraces           func([]projectioncontract.StepView) []projectioncontract.WebSourceTrace
	LLMContextBudgetTraces    func([]projectioncontract.StepView) []projectioncontract.LLMContextBudgetTrace
	LLMUsageTracesFromRecords func([]projectioncontract.EventRecord, string) []projectioncontract.LLMUsageTrace
	LLMUsageSummary           func([]projectioncontract.LLMUsageTrace) projectioncontract.LLMUsageSummary
	Timeline                  func(readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem
	BuildWorkspaceFeed        func(readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceActivityFeed
	CommitWorkspaceFeed       func(projectioncontract.WorkspaceActivityFeed, projectioncontract.WorkspaceActivityFeed, int64) projectioncontract.WorkspaceActivityFeed
	PageWorkspaceFeed         func(projectioncontract.WorkspaceActivityFeed, int, string) projectioncontract.WorkspaceActivityFeed
}

// ActivityProjection returns the one typed application-facing activity facade.
func ActivityProjection() activityProjector {
	return activityProjector{
		EventRecords:              mapEventRecords,
		ProjectedEventRecords:     mapProjectedEventRecords,
		MemoryRecallTraces:        mapMemoryRecallTraces,
		ClosureTraces:             mapClosureTraces,
		GitOperationTraces:        mapGitOperationTraces,
		WebSourceTraces:           mapWebSourceTraces,
		LLMContextBudgetTraces:    mapLLMContextBudgetTraces,
		LLMUsageTracesFromRecords: mapLLMUsageTracesFromEventRecords,
		LLMUsageSummary:           summarizeLLMUsage,
		Timeline:                  runtimeTimeline,
		BuildWorkspaceFeed:        buildWorkspaceActivityFeedContent,
		CommitWorkspaceFeed:       commitActivityFeedTransaction,
		PageWorkspaceFeed:         pageWorkspaceActivityFeed,
	}
}
