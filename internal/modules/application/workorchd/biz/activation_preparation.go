package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// QueryExactConfirmedStartupSnapshot is the explicit preparation/compiler read
// boundary. The persisted-state GET deliberately does not call it.
func (b *Biz) QueryExactConfirmedStartupSnapshot(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (workspaceevents.QueryExactConfirmedStartupSnapshotResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	response, ok := value.(workspaceevents.QueryExactConfirmedStartupSnapshotResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	response.Snapshot = response.Snapshot.Clone()
	return response, nil
}

// ReadStartupPreparationView sends only the persisted-state query. It does not
// invoke exact-seal compilation, resource observation or execution bootstrap.
func (b *Biz) ReadStartupPreparationView(ctx context.Context, workspaceID common.WorkspaceID) (workorchmanagementcontract.StartupPreparationView, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || workspaceID == "" {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	command := workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: workspaceID}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationStateQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspacePreparationStateQuery, eventcontract.LaneScope{WorkspaceID: string(workspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	response, ok := value.(workspaceevents.QueryWorkspacePreparationStateResult)
	if !ok || response.ValidateFor(command) != nil {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrConflict
	}
	view := workorchmanagementcontract.StartupPreparationView{
		WorkspaceID: string(response.WorkspaceID), WorkspaceRevision: response.WorkspaceRevision, ActiveRunID: string(response.ActiveRunID), OperationState: response.State, CurrentValidity: response.CurrentValidity,
	}
	if response.Record != nil {
		view.Record = &workorchmanagementcontract.PreparationRecordView{Version: response.Record.Version, Attempts: make([]workorchmanagementcontract.PreparationAttemptView, 0, len(response.Record.Attempts))}
		for _, a := range response.Record.Attempts {
			binding := a.Binding
			attempt := workorchmanagementcontract.PreparationAttemptView{
				RequestID: a.RequestID, PayloadHash: a.PayloadHash, ExpectedWorkspaceRevision: a.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: a.CommittedWorkspaceRevision, StartedAt: a.StartedAt, State: string(a.State),
				Binding: workorchmanagementcontract.PreparationBindingView{
					InputHash: binding.AuthoringRef.InputHash, DerivationPermitID: binding.AuthoringRef.DerivationPermitID, ProposalHash: binding.AuthoringRef.ProposalHash, SpecificationHash: binding.AuthoringRef.SpecificationHash,
					CandidateID: binding.CandidateID, CandidateRevision: binding.CandidateRevision, CandidateHash: binding.CandidateHash, DefinitionRevision: binding.DefinitionRevision, StartupInputHash: binding.StartupInputHash,
					AuthoringConfigurationHash: binding.AuthoringConfigurationHash, SnapshotHash: binding.SnapshotHash, ManifestHash: binding.ManifestHash, CheckPolicyVersion: binding.CheckPolicyVersion, CompilerVersion: binding.CompilerVersion,
				},
			}
			if !a.CompletedAt.IsZero() {
				completed := a.CompletedAt
				attempt.CompletedAt = &completed
			}
			if !a.ExpiresAt.IsZero() {
				expires := a.ExpiresAt
				attempt.ExpiresAt = &expires
			}
			view.Record.Attempts = append(view.Record.Attempts, attempt)
		}
	}
	if err := view.ValidateFor(string(workspaceID)); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	return view, nil
}
