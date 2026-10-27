package biz

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// PreparationSkillHubInventory is an explicit diagnostic observation, not a
// persisted check, selected Skill proof or M3. Raw output is restricted and must
// not be exposed through the public preparation-state projection.
type PreparationSkillHubInventory struct {
	WorkspaceID       common.WorkspaceID
	WorkspaceRevision uint64
	SnapshotHash      string
	Global            contextregistryports.SkillHubPreparationInstructions
	Workspace         contextregistryports.SkillHubPreparationInstructions
	BindingCurrent    bool
}

func (b *Biz) ObservePreparationSkillHub(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (PreparationSkillHubInventory, error) {
	return b.observePreparationSkillHub(ctx, command, contextregistryports.SkillHubPreparationExecRunner{})
}

func (b *Biz) observePreparationSkillHub(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand, runner contextregistryports.SkillHubPreparationRunner) (PreparationSkillHubInventory, error) {
	if ctx == nil || command.Validate() != nil {
		return PreparationSkillHubInventory{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return PreparationSkillHubInventory{}, err
	}
	bounded, cancel := context.WithTimeout(bound, 5*time.Second)
	defer cancel()
	before, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return PreparationSkillHubInventory{}, err
	}
	value := PreparationSkillHubInventory{WorkspaceID: command.WorkspaceID, WorkspaceRevision: command.ExpectedWorkspaceRevision, SnapshotHash: before.Snapshot.SnapshotHash}
	value.Global, err = contextregistryports.ObserveSkillHubPreparationInstructions(bounded, runner, command.WorkspaceID, "", true)
	if err != nil {
		return value, err
	}
	value.Workspace, err = contextregistryports.ObserveSkillHubPreparationInstructions(bounded, runner, command.WorkspaceID, before.Snapshot.StartupInput.ResolvedProjectWorkspaceDirectory, false)
	if err != nil {
		return value, err
	}
	after, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return value, err
	}
	if after.Snapshot.SnapshotHash != value.SnapshotHash {
		return value, common.ErrConflict
	}
	value.BindingCurrent = true
	return value, nil
}
