package orchestrator

import (
	"context"
	"fmt"
	"reflect"

	magicevent "github.com/muidea/magicCommon/event"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Original sealed sources are compiled from the complete aggregate. This read
// does not persist freshness, reopen a CWI, seal material or grant credit.
func (m *Biz) queryRuntimeGitCommitSnapshotFromAggregate(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID string) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
	return m.queryRuntimeGitCommitSnapshotWithInputs(ctx, aggregate, dispatchID, operationID, receiptHash, requestID, nil)
}

// Both sources come from the same complete validated aggregate. The validation
// receipt, exact current binding and accepted semantic report are independently
// reverse-proven before Git is asked about bytes in its original commit tree.
func (m *Biz) queryRuntimeGitCommitSnapshotWithReviewedInputsFromAggregate(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID, validationDispatchID, validationOperationID, validationReceiptHash string) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
	input, err := runtime.CompileRuntimeValidationInputSnapshotSource(aggregate, validationDispatchID, validationOperationID, validationReceiptHash)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	return m.queryRuntimeGitCommitSnapshotWithInputs(ctx, aggregate, dispatchID, operationID, receiptHash, requestID, &input)
}

func (m *Biz) queryRuntimeGitCommitSnapshotWithInputs(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID string, input *runtimecontract.RuntimeValidationProofRequirementBinding, terminal ...bool) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
	source, err := runtime.CompileRuntimeOperationReadSource(aggregate, dispatchID, operationID, receiptHash)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	if source.SourceContentAddress == nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("Git snapshot query requires original archive address")
	}
	sealed, err := gitevents.SealRuntimeOperation(source.Operation)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	protocol := gitevents.RuntimeCommitSnapshotQueryProtocol
	if input != nil {
		protocol = gitevents.RuntimeCommitSnapshotInputQueryProtocol
	}
	if len(terminal) > 1 {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrInvalidArgument
	}
	if len(terminal) == 1 && terminal[0] {
		protocol = gitevents.RuntimeCommitSnapshotTerminalQueryProtocol
	}
	command := gitevents.QueryRuntimeCommitSnapshotCommand{
		RuntimeOperationExecutionContext: source.Context, SealedOperation: sealed,
		ProtocolRef: protocol, RequestID: requestID, IssuedAtStateRevision: aggregate.StateRevision, InputBinding: input,
		SourceReceiptHash: source.SourceReceiptHash, SnapshotAddress: *source.SourceContentAddress, OriginalDependencies: source.OriginalDependencies,
	}
	if protocol == gitevents.RuntimeCommitSnapshotTerminalQueryProtocol {
		plans, err := runtime.CompileRuntimeTerminalSnapshotSourcesFromAggregate(aggregate)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		for _, plan := range plans {
			if plan.Snapshot.DispatchID != dispatchID || plan.Snapshot.OperationID != operationID || input == nil || input.InputReview == nil || plan.InputSubjectHash != input.InputReview.Hash {
				continue
			}
			validation, err := runtime.CompileRuntimeOperationReadSource(aggregate, plan.Validation.DispatchID, plan.Validation.OperationID, plan.Validation.ReceiptHash)
			if err != nil {
				return gitevents.QueryRuntimeCommitSnapshotResult{}, err
			}
			value := *runtimecontract.CloneRuntimeOwnerRevisionSet(&validation.OriginalDependencies)
			if command.ValidationDependencies != nil && !reflect.DeepEqual(*command.ValidationDependencies, value) {
				return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrConflict
			}
			identity := plan.SnapshotIdentity
			if command.OriginalSnapshot != nil && *command.OriginalSnapshot != identity {
				return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrConflict
			}
			command.OriginalSnapshot = &identity
			command.ValidationDependencies = &value
		}
		if command.ValidationDependencies == nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrConflict
		}
	}
	return m.queryRuntimeGitCommitSnapshot(ctx, command)
}

func (m *Biz) queryRuntimeGitCommitSnapshot(ctx context.Context, command gitevents.QueryRuntimeCommitSnapshotCommand) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
	if m == nil || m.EventHub() == nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	command.RuntimeOperationExecutionContext = runtimecontract.CloneRuntimeOperationExecutionContext(command.RuntimeOperationExecutionContext)
	command.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
	command.ValidationDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(command.ValidationDependencies)
	if command.OriginalSnapshot != nil {
		identity := *command.OriginalSnapshot
		command.OriginalSnapshot = &identity
	}
	command.SealedOperation.Operation = command.SealedOperation.Operation.Clone()
	if command.InputBinding != nil {
		value := command.InputBinding.Clone()
		command.InputBinding = &value
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	event := magicevent.NewEventWithContext(gitevents.TopicQueryRuntimeCommitSnapshot, ModuleID, runtimecontract.RuntimeModuleGit, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(eventcontract.QueryLaneKey(gitevents.TopicQueryRuntimeCommitSnapshot, eventcontract.LaneScope{
		WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID), SubjectID: command.RequestID,
	}))
	result := m.SendEvent(event)
	if result == nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("Git snapshot query returned no result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, resultErr
	}
	response, ok := data.(gitevents.QueryRuntimeCommitSnapshotResult)
	if !ok {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("Git snapshot query returned invalid result type %T", data)
	}
	if err := response.ValidateFor(command); err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	return response.Clone(), nil
}
