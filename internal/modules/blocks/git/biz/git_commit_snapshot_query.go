package git

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeCommitSnapshotQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.QueryRuntimeCommitSnapshotCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := command.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(event.Context(), command.WorkspaceID)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	// Only an exact persisted original receipt authorizes this read. A receipt
	// miss cannot become an execution/reconciliation or filesystem-read fallback.
	receipt, found, err := m.queryRuntimeOperationReceiptWithSource(ctx, command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution, &command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash ||
		receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed ||
		receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable ||
		!reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &command.OriginalDependencies) ||
		!reflect.DeepEqual(receipt.Evidence.ContentAddress, &command.SnapshotAddress) || receipt.Evidence.ContentHash != command.SnapshotAddress.Digest {
		setCapabilityError(result, fmt.Errorf("%w: Git snapshot requires exact original committed receipt", common.ErrConflict))
		return
	}
	if command.OriginalSnapshot != nil && !reflect.DeepEqual(receipt.Evidence.OwnerSnapshot, command.OriginalSnapshot) {
		setCapabilityError(result, fmt.Errorf("%w: Git terminal query identity differs from original committed receipt", common.ErrConflict))
		return
	}
	workspaceCommand := workspaceevents.GetWorkspaceCommand{WorkspaceID: command.WorkspaceID}
	query := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceCommand)
	query.BindLaneKey(eventcontract.QueryLaneKey(workspaceevents.TopicWorkspaceCommandGet, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response := m.SendEvent(query)
	if response == nil {
		setCapabilityError(result, fmt.Errorf("Git snapshot workspace query returned no result"))
		return
	}
	data, queryErr := response.Get()
	if queryErr != nil {
		setCapabilityError(result, queryErr)
		return
	}
	workspace, ok := data.(workspaceevents.WorkspaceResult)
	if !ok || workspace.ValidateFor(workspaceCommand) != nil {
		setCapabilityError(result, common.ErrConflict)
		return
	}
	observation, err := m.runtimeAdapter.observeOriginalCommitSnapshot(ctx, command, workspace.Workspace.InternalDirectory, workspace.Workspace.DeliverableDirectory)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation.Clone(), nil)
}

// One bounded observation, never a continuously-current resource guarantee or
// a working-tree proof. V2 additionally compares requested input bytes to the
// original tree/blob; review provenance is independently checked by Orchestrator.
func (a *RuntimeAdapter) observeOriginalCommitSnapshot(ctx context.Context, command gitevents.QueryRuntimeCommitSnapshotCommand, internalDirectory, deliverableDirectory string) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
	if err := command.Validate(); err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	timeout := a.timeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	record, err := readGitCommitSnapshot(ctx, internalDirectory, gitCommitSnapshotScopeForExecution(command.RuntimeOperationExecutionContext), command.SnapshotAddress)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	if command.OriginalSnapshot != nil && (record.Head != command.OriginalSnapshot.SnapshotID || record.RootTree != command.OriginalSnapshot.RootSnapshotID) {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("%w: Git terminal archive identity differs from original committed receipt", common.ErrConflict)
	}
	original, err := gitCommitObjectDependencies(command.Transaction.DefinitionRevision, string(command.WorkspaceID), record.Objects)
	if err != nil || !reflect.DeepEqual(original, &command.OriginalDependencies) {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("%w: Git archived object bytes do not match original receipt dependencies", common.ErrConflict)
	}
	repo, err := a.resolveRepo(ctx, RuntimeOperationRequest{WorkspaceDeliverableDirectory: deliverableDirectory}, timeout)
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	head, err := a.gitOutput(ctx, repo.RepoRoot, timeout, "--no-replace-objects", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	var statusBefore string
	if command.ProtocolRef == gitevents.RuntimeCommitSnapshotTerminalQueryProtocol {
		statusBefore, err = a.completeTerminalSnapshotStatus(ctx, repo.RepoRoot)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
	}
	observation := gitevents.QueryRuntimeCommitSnapshotResult{
		ProtocolRef: command.ProtocolRef, RequestID: command.RequestID, IssuedAtStateRevision: command.IssuedAtStateRevision,
		SourceReceiptHash: command.SourceReceiptHash, SnapshotAddress: command.SnapshotAddress,
		SnapshotHead: record.Head, SnapshotRootTree: record.RootTree, ObservedHead: strings.TrimSuffix(head, "\n"), ObjectCount: len(record.Objects),
	}
	for _, object := range record.Objects {
		observation.ObjectBytes += len(object.Body)
	}
	if command.InputBinding != nil {
		memberships, err := gitCommitSnapshotInputMemberships(ctx, record, command.InputBinding.InputReview.Files)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		observation.InputSubjectHash = command.InputBinding.InputReview.Hash
		observation.InputMemberships = memberships
	}
	if command.ValidationDependencies != nil {
		members, err := gitCommitSnapshotValidationMemberships(ctx, record, *command.ValidationDependencies)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		observation.ValidationMemberships = members
	}
	if observation.ObservedHead == record.Head {
		objects, err := a.readCommitObjectGraph(ctx, repo.RepoRoot, record.Head)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		// Equal object IDs alone are not a readback proof: capture verifies each
		// body and the complete graph; independently recompute its full read set.
		current, err := gitCommitObjectDependencies(command.Transaction.DefinitionRevision, string(command.WorkspaceID), objects)
		if err != nil || !reflect.DeepEqual(original, current) {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, common.ErrConflict
		}
		observation.CurrentDependencies = current
	}
	headAfter, err := a.gitOutput(ctx, repo.RepoRoot, timeout, "--no-replace-objects", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	if headAfter != head {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("%w: Git HEAD changed during snapshot query", common.ErrConflict)
	}
	if command.ProtocolRef == gitevents.RuntimeCommitSnapshotTerminalQueryProtocol {
		statusAfter, err := a.completeTerminalSnapshotStatus(ctx, repo.RepoRoot)
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		if statusBefore != statusAfter {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, fmt.Errorf("%w: Git workspace/index changed during terminal observation", common.ErrConflict)
		}
		clean := statusBefore == ""
		observation.WorkspaceClean = &clean
	}
	if err := ctx.Err(); err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	if err := observation.ValidateFor(command); err != nil {
		return gitevents.QueryRuntimeCommitSnapshotResult{}, err
	}
	return observation, nil
}

func (a *RuntimeAdapter) completeTerminalSnapshotStatus(ctx context.Context, repo string) (string, error) {
	result, err := a.runGit(ctx, repo, a.timeout, maxOutputBytes, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return "", err
	}
	if result.Truncated {
		return "", fmt.Errorf("Git terminal status was not completely retained: stdout=%d stderr=%d limit=%d", result.StdoutBytes, result.StderrBytes, maxOutputBytes)
	}
	return result.Stdout, nil
}
