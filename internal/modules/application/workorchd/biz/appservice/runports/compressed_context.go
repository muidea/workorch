package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type RecoverySnapshotFunc func(context.Context, common.RunID) (readmodelcore.RunSnapshot, error)

type CompressedContextDependencies struct {
	Runs             readmodelcore.RunReader
	Tickets          readmodelcore.DecisionTicketReader
	Resolutions      readmodelcore.DecisionResolutionReader
	Approvals        readmodelcore.ApprovalReader
	MemoryRecalls    readmodelcore.MemoryRecallTraceReader
	CommandHub       magicevent.Hub
	EventHub         events.Hub
	RecoverySnapshot RecoverySnapshotFunc
	DefaultLanguage  string
}

func AttachCompressedContextToRun(ctx context.Context, deps CompressedContextDependencies, runID common.RunID) error {
	item, err := readmodelcore.GetRunFromReader(ctx, deps.Runs, runID)
	if err != nil {
		return err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	return AttachCompressedContextToLoadedRun(ctx, deps, item)
}

func CompressedContextPatchForRun(ctx context.Context, deps CompressedContextDependencies, runID common.RunID) (map[string]any, *runmodel.Run, error) {
	item, err := readmodelcore.GetRunFromReader(ctx, deps.Runs, runID)
	if err != nil {
		return nil, nil, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	compressed, err := CompressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return nil, nil, err
	}
	if err := RecordRecoveryMemoryRecallTrace(ctx, deps.CommandHub, deps.EventHub, deps.MemoryRecalls, item, compressed); err != nil {
		return nil, nil, err
	}
	patch, err := WorkspaceDefinitionContextPatch(ctx, deps.CommandHub, item)
	if err != nil {
		return nil, nil, err
	}
	if patch == nil {
		patch = map[string]any{}
	}
	patch["compressedContext"] = compressed
	return patch, item, nil
}

func AttachCompressedContextToLoadedRun(ctx context.Context, deps CompressedContextDependencies, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	compressed, err := CompressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return err
	}
	executionContext := runtimecore.CloneContextMap(item.ExecutionContext)
	if executionContext == nil {
		executionContext = map[string]any{}
	}
	executionContext["compressedContext"] = compressed
	item.ExecutionContext = executionContext
	return UpdateRun(ctx, deps.CommandHub, item)
}
