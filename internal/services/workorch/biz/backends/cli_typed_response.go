package backend

import (
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func bindCLIEvent(event magicevent.Event, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID) {
	event.BindLaneKey(events.EventHubLaneKeyFor(workorchdevents.Contracts(), topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
}

func cliWorkspaceCreateResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceCreateResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceCreateResult)
	if !ok {
		return workorchdevents.CLIWorkspaceCreateResult{}, fmt.Errorf("%w: workspace create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceGetResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceGetResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceGetResult)
	if !ok {
		return workorchdevents.CLIWorkspaceGetResult{}, fmt.Errorf("%w: workspace get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceListResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceListResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceListResult)
	if !ok {
		return workorchdevents.CLIWorkspaceListResult{}, fmt.Errorf("%w: workspace list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceSnapshotResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceSnapshotResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceSnapshotResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceSnapshotResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceSnapshotResult)
	if !ok {
		return workorchdevents.CLIWorkspaceSnapshotResult{}, fmt.Errorf("%w: workspace snapshot result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceRuntimeViewResponse(response magicevent.Result) (workorchdevents.ReadWorkspaceRuntimeViewResult, error) {
	if response == nil {
		return workorchdevents.ReadWorkspaceRuntimeViewResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.ReadWorkspaceRuntimeViewResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.ReadWorkspaceRuntimeViewResult{}, err
	}
	result, ok := data.(workorchdevents.ReadWorkspaceRuntimeViewResult)
	if !ok {
		return workorchdevents.ReadWorkspaceRuntimeViewResult{}, fmt.Errorf("%w: workspace runtime view result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceDeleteResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceDeleteResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceDeleteResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceDeleteResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceDeleteResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceDeleteResult)
	if !ok {
		return workorchdevents.CLIWorkspaceDeleteResult{}, fmt.Errorf("%w: workspace delete result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliWorkspaceUpdateLLMTargetResponse(response magicevent.Result) (workorchdevents.CLIWorkspaceUpdateLLMTargetResult, error) {
	if response == nil {
		return workorchdevents.CLIWorkspaceUpdateLLMTargetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIWorkspaceUpdateLLMTargetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIWorkspaceUpdateLLMTargetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIWorkspaceUpdateLLMTargetResult)
	if !ok {
		return workorchdevents.CLIWorkspaceUpdateLLMTargetResult{}, fmt.Errorf("%w: workspace llm target update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliGoalDefineResponse(response magicevent.Result) (workorchdevents.CLIGoalDefineResult, error) {
	if response == nil {
		return workorchdevents.CLIGoalDefineResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIGoalDefineResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIGoalDefineResult{}, err
	}
	result, ok := data.(workorchdevents.CLIGoalDefineResult)
	if !ok {
		return workorchdevents.CLIGoalDefineResult{}, fmt.Errorf("%w: goal define result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliGoalListResponse(response magicevent.Result) (workorchdevents.CLIGoalListResult, error) {
	if response == nil {
		return workorchdevents.CLIGoalListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIGoalListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIGoalListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIGoalListResult)
	if !ok {
		return workorchdevents.CLIGoalListResult{}, fmt.Errorf("%w: goal list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliGoalGetResponse(response magicevent.Result) (workorchdevents.CLIGoalGetResult, error) {
	if response == nil {
		return workorchdevents.CLIGoalGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIGoalGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIGoalGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIGoalGetResult)
	if !ok {
		return workorchdevents.CLIGoalGetResult{}, fmt.Errorf("%w: goal get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliGoalUpdateResponse(response magicevent.Result) (workorchdevents.CLIGoalUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLIGoalUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIGoalUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIGoalUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIGoalUpdateResult)
	if !ok {
		return workorchdevents.CLIGoalUpdateResult{}, fmt.Errorf("%w: goal update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDoDDefineResponse(response magicevent.Result) (workorchdevents.CLIDoDDefineResult, error) {
	if response == nil {
		return workorchdevents.CLIDoDDefineResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDoDDefineResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDoDDefineResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDoDDefineResult)
	if !ok {
		return workorchdevents.CLIDoDDefineResult{}, fmt.Errorf("%w: dod define result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDoDListResponse(response magicevent.Result) (workorchdevents.CLIDoDListResult, error) {
	if response == nil {
		return workorchdevents.CLIDoDListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDoDListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDoDListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDoDListResult)
	if !ok {
		return workorchdevents.CLIDoDListResult{}, fmt.Errorf("%w: dod list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDoDGetResponse(response magicevent.Result) (workorchdevents.CLIDoDGetResult, error) {
	if response == nil {
		return workorchdevents.CLIDoDGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDoDGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDoDGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDoDGetResult)
	if !ok {
		return workorchdevents.CLIDoDGetResult{}, fmt.Errorf("%w: dod get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDoDUpdateResponse(response magicevent.Result) (workorchdevents.CLIDoDUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLIDoDUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDoDUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDoDUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDoDUpdateResult)
	if !ok {
		return workorchdevents.CLIDoDUpdateResult{}, fmt.Errorf("%w: dod update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliRunGetResponse(response magicevent.Result) (workorchdevents.CLIRunGetResult, error) {
	if response == nil {
		return workorchdevents.CLIRunGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIRunGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIRunGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIRunGetResult)
	if !ok {
		return workorchdevents.CLIRunGetResult{}, fmt.Errorf("%w: run get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliRunListResponse(response magicevent.Result) (workorchdevents.CLIRunListResult, error) {
	if response == nil {
		return workorchdevents.CLIRunListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIRunListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIRunListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIRunListResult)
	if !ok {
		return workorchdevents.CLIRunListResult{}, fmt.Errorf("%w: run list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliRunStatusResponse(response magicevent.Result) (workorchdevents.CLIRunStatusResult, error) {
	if response == nil {
		return workorchdevents.CLIRunStatusResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIRunStatusResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIRunStatusResult{}, err
	}
	result, ok := data.(workorchdevents.CLIRunStatusResult)
	if !ok {
		return workorchdevents.CLIRunStatusResult{}, fmt.Errorf("%w: run status result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDecisionResolveResponse(response magicevent.Result) (workorchdevents.CLIDecisionResolveResult, error) {
	if response == nil {
		return workorchdevents.CLIDecisionResolveResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDecisionResolveResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDecisionResolveResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDecisionResolveResult)
	if !ok {
		return workorchdevents.CLIDecisionResolveResult{}, fmt.Errorf("%w: decision resolve result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDecisionBatchResolveResponse(response magicevent.Result) (workorchdevents.CLIDecisionBatchResolveResult, error) {
	if response == nil {
		return workorchdevents.CLIDecisionBatchResolveResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDecisionBatchResolveResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDecisionBatchResolveResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDecisionBatchResolveResult)
	if !ok {
		return workorchdevents.CLIDecisionBatchResolveResult{}, fmt.Errorf("%w: decision batch resolve result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDecisionListResponse(response magicevent.Result) (workorchdevents.CLIDecisionListResult, error) {
	if response == nil {
		return workorchdevents.CLIDecisionListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDecisionListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDecisionListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDecisionListResult)
	if !ok {
		return workorchdevents.CLIDecisionListResult{}, fmt.Errorf("%w: decision list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliDecisionGetResponse(response magicevent.Result) (workorchdevents.CLIDecisionGetResult, error) {
	if response == nil {
		return workorchdevents.CLIDecisionGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIDecisionGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIDecisionGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIDecisionGetResult)
	if !ok {
		return workorchdevents.CLIDecisionGetResult{}, fmt.Errorf("%w: decision get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliApprovalApproveResponse(response magicevent.Result) (workorchdevents.CLIApprovalApproveResult, error) {
	if response == nil {
		return workorchdevents.CLIApprovalApproveResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIApprovalApproveResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIApprovalApproveResult{}, err
	}
	result, ok := data.(workorchdevents.CLIApprovalApproveResult)
	if !ok {
		return workorchdevents.CLIApprovalApproveResult{}, fmt.Errorf("%w: approval approve result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliApprovalRejectResponse(response magicevent.Result) (workorchdevents.CLIApprovalRejectResult, error) {
	if response == nil {
		return workorchdevents.CLIApprovalRejectResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIApprovalRejectResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIApprovalRejectResult{}, err
	}
	result, ok := data.(workorchdevents.CLIApprovalRejectResult)
	if !ok {
		return workorchdevents.CLIApprovalRejectResult{}, fmt.Errorf("%w: approval reject result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliApprovalBatchResolveResponse(response magicevent.Result) (workorchdevents.CLIApprovalBatchResolveResult, error) {
	if response == nil {
		return workorchdevents.CLIApprovalBatchResolveResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIApprovalBatchResolveResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIApprovalBatchResolveResult{}, err
	}
	result, ok := data.(workorchdevents.CLIApprovalBatchResolveResult)
	if !ok {
		return workorchdevents.CLIApprovalBatchResolveResult{}, fmt.Errorf("%w: approval batch resolve result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliApprovalListResponse(response magicevent.Result) (workorchdevents.CLIApprovalListResult, error) {
	if response == nil {
		return workorchdevents.CLIApprovalListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIApprovalListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIApprovalListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIApprovalListResult)
	if !ok {
		return workorchdevents.CLIApprovalListResult{}, fmt.Errorf("%w: approval list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliApprovalGetResponse(response magicevent.Result) (workorchdevents.CLIApprovalGetResult, error) {
	if response == nil {
		return workorchdevents.CLIApprovalGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIApprovalGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIApprovalGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIApprovalGetResult)
	if !ok {
		return workorchdevents.CLIApprovalGetResult{}, fmt.Errorf("%w: approval get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliSoulCreateResponse(response magicevent.Result) (workorchdevents.CLISoulCreateResult, error) {
	if response == nil {
		return workorchdevents.CLISoulCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISoulCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISoulCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLISoulCreateResult)
	if !ok {
		return workorchdevents.CLISoulCreateResult{}, fmt.Errorf("%w: soul create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSoulGetResponse(response magicevent.Result) (workorchdevents.CLISoulGetResult, error) {
	if response == nil {
		return workorchdevents.CLISoulGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISoulGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISoulGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLISoulGetResult)
	if !ok {
		return workorchdevents.CLISoulGetResult{}, fmt.Errorf("%w: soul get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSoulListResponse(response magicevent.Result) (workorchdevents.CLISoulListResult, error) {
	if response == nil {
		return workorchdevents.CLISoulListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISoulListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISoulListResult{}, err
	}
	result, ok := data.(workorchdevents.CLISoulListResult)
	if !ok {
		return workorchdevents.CLISoulListResult{}, fmt.Errorf("%w: soul list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSoulUpdateResponse(response magicevent.Result) (workorchdevents.CLISoulUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLISoulUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISoulUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISoulUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLISoulUpdateResult)
	if !ok {
		return workorchdevents.CLISoulUpdateResult{}, fmt.Errorf("%w: soul update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSoulDeleteResponse(response magicevent.Result) (workorchdevents.CLISoulDeleteResult, error) {
	if response == nil {
		return workorchdevents.CLISoulDeleteResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISoulDeleteResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISoulDeleteResult{}, err
	}
	result, ok := data.(workorchdevents.CLISoulDeleteResult)
	if !ok {
		return workorchdevents.CLISoulDeleteResult{}, fmt.Errorf("%w: soul delete result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSkillCreateResponse(response magicevent.Result) (workorchdevents.CLISkillCreateResult, error) {
	if response == nil {
		return workorchdevents.CLISkillCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISkillCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISkillCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLISkillCreateResult)
	if !ok {
		return workorchdevents.CLISkillCreateResult{}, fmt.Errorf("%w: skill create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSkillGetResponse(response magicevent.Result) (workorchdevents.CLISkillGetResult, error) {
	if response == nil {
		return workorchdevents.CLISkillGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISkillGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISkillGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLISkillGetResult)
	if !ok {
		return workorchdevents.CLISkillGetResult{}, fmt.Errorf("%w: skill get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliSkillListResponse(response magicevent.Result) (workorchdevents.CLISkillListResult, error) {
	if response == nil {
		return workorchdevents.CLISkillListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLISkillListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLISkillListResult{}, err
	}
	result, ok := data.(workorchdevents.CLISkillListResult)
	if !ok {
		return workorchdevents.CLISkillListResult{}, fmt.Errorf("%w: skill list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMCPServerCreateResponse(response magicevent.Result) (workorchdevents.CLIMCPServerCreateResult, error) {
	if response == nil {
		return workorchdevents.CLIMCPServerCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMCPServerCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMCPServerCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMCPServerCreateResult)
	if !ok {
		return workorchdevents.CLIMCPServerCreateResult{}, fmt.Errorf("%w: mcp server create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMCPServerGetResponse(response magicevent.Result) (workorchdevents.CLIMCPServerGetResult, error) {
	if response == nil {
		return workorchdevents.CLIMCPServerGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMCPServerGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMCPServerGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMCPServerGetResult)
	if !ok {
		return workorchdevents.CLIMCPServerGetResult{}, fmt.Errorf("%w: mcp server get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMCPServerListResponse(response magicevent.Result) (workorchdevents.CLIMCPServerListResult, error) {
	if response == nil {
		return workorchdevents.CLIMCPServerListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMCPServerListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMCPServerListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMCPServerListResult)
	if !ok {
		return workorchdevents.CLIMCPServerListResult{}, fmt.Errorf("%w: mcp server list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMCPServerUpdateResponse(response magicevent.Result) (workorchdevents.CLIMCPServerUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLIMCPServerUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMCPServerUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMCPServerUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMCPServerUpdateResult)
	if !ok {
		return workorchdevents.CLIMCPServerUpdateResult{}, fmt.Errorf("%w: mcp server update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMCPCapabilityListResponse(response magicevent.Result) (workorchdevents.CLIMCPCapabilityListResult, error) {
	if response == nil {
		return workorchdevents.CLIMCPCapabilityListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMCPCapabilityListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMCPCapabilityListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMCPCapabilityListResult)
	if !ok {
		return workorchdevents.CLIMCPCapabilityListResult{}, fmt.Errorf("%w: mcp capability list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliMemoryCreateResponse(response magicevent.Result) (workorchdevents.CLIMemoryCreateResult, error) {
	if response == nil {
		return workorchdevents.CLIMemoryCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemoryCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemoryCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemoryCreateResult)
	if !ok {
		return workorchdevents.CLIMemoryCreateResult{}, fmt.Errorf("%w: memory create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMemoryGetResponse(response magicevent.Result) (workorchdevents.CLIMemoryGetResult, error) {
	if response == nil {
		return workorchdevents.CLIMemoryGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemoryGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemoryGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemoryGetResult)
	if !ok {
		return workorchdevents.CLIMemoryGetResult{}, fmt.Errorf("%w: memory get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMemoryUpdateResponse(response magicevent.Result) (workorchdevents.CLIMemoryUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLIMemoryUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemoryUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemoryUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemoryUpdateResult)
	if !ok {
		return workorchdevents.CLIMemoryUpdateResult{}, fmt.Errorf("%w: memory update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMemoryDeleteResponse(response magicevent.Result) (workorchdevents.CLIMemoryDeleteResult, error) {
	if response == nil {
		return workorchdevents.CLIMemoryDeleteResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemoryDeleteResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemoryDeleteResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemoryDeleteResult)
	if !ok {
		return workorchdevents.CLIMemoryDeleteResult{}, fmt.Errorf("%w: memory delete result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMemoryListResponse(response magicevent.Result) (workorchdevents.CLIMemoryListResult, error) {
	if response == nil {
		return workorchdevents.CLIMemoryListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemoryListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemoryListResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemoryListResult)
	if !ok {
		return workorchdevents.CLIMemoryListResult{}, fmt.Errorf("%w: memory list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliMemorySearchResponse(response magicevent.Result) (workorchdevents.CLIMemorySearchResult, error) {
	if response == nil {
		return workorchdevents.CLIMemorySearchResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIMemorySearchResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIMemorySearchResult{}, err
	}
	result, ok := data.(workorchdevents.CLIMemorySearchResult)
	if !ok {
		return workorchdevents.CLIMemorySearchResult{}, fmt.Errorf("%w: memory search result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliTemplateCreateResponse(response magicevent.Result) (workorchdevents.CLITemplateCreateResult, error) {
	if response == nil {
		return workorchdevents.CLITemplateCreateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLITemplateCreateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLITemplateCreateResult{}, err
	}
	result, ok := data.(workorchdevents.CLITemplateCreateResult)
	if !ok {
		return workorchdevents.CLITemplateCreateResult{}, fmt.Errorf("%w: template create result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliTemplateGetResponse(response magicevent.Result) (workorchdevents.CLITemplateGetResult, error) {
	if response == nil {
		return workorchdevents.CLITemplateGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLITemplateGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLITemplateGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLITemplateGetResult)
	if !ok {
		return workorchdevents.CLITemplateGetResult{}, fmt.Errorf("%w: template get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliTemplateListResponse(response magicevent.Result) (workorchdevents.CLITemplateListResult, error) {
	if response == nil {
		return workorchdevents.CLITemplateListResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLITemplateListResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLITemplateListResult{}, err
	}
	result, ok := data.(workorchdevents.CLITemplateListResult)
	if !ok {
		return workorchdevents.CLITemplateListResult{}, fmt.Errorf("%w: template list result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliTemplateUpdateResponse(response magicevent.Result) (workorchdevents.CLITemplateUpdateResult, error) {
	if response == nil {
		return workorchdevents.CLITemplateUpdateResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLITemplateUpdateResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLITemplateUpdateResult{}, err
	}
	result, ok := data.(workorchdevents.CLITemplateUpdateResult)
	if !ok {
		return workorchdevents.CLITemplateUpdateResult{}, fmt.Errorf("%w: template update result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliTemplateDeleteResponse(response magicevent.Result) (workorchdevents.CLITemplateDeleteResult, error) {
	if response == nil {
		return workorchdevents.CLITemplateDeleteResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLITemplateDeleteResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLITemplateDeleteResult{}, err
	}
	result, ok := data.(workorchdevents.CLITemplateDeleteResult)
	if !ok {
		return workorchdevents.CLITemplateDeleteResult{}, fmt.Errorf("%w: template delete result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
func cliConfigurationGetResponse(response magicevent.Result) (workorchdevents.CLIConfigurationGetResult, error) {
	if response == nil {
		return workorchdevents.CLIConfigurationGetResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIConfigurationGetResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIConfigurationGetResult{}, err
	}
	result, ok := data.(workorchdevents.CLIConfigurationGetResult)
	if !ok {
		return workorchdevents.CLIConfigurationGetResult{}, fmt.Errorf("%w: configuration get result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}

func cliConfigurationRefreshChatModelsResponse(response magicevent.Result) (workorchdevents.CLIConfigurationRefreshChatModelsResult, error) {
	if response == nil {
		return workorchdevents.CLIConfigurationRefreshChatModelsResult{}, common.ErrInvalidArgument
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIConfigurationRefreshChatModelsResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIConfigurationRefreshChatModelsResult{}, err
	}
	result, ok := data.(workorchdevents.CLIConfigurationRefreshChatModelsResult)
	if !ok {
		return workorchdevents.CLIConfigurationRefreshChatModelsResult{}, fmt.Errorf("%w: configuration refresh chat models result type %T", common.ErrInvalidArgument, data)
	}
	return result, nil
}
