package workspace

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Owner-private complete inputs; never a caller's validation flag. The reducer
// recompiles them against its actual Workspace CAS preimage and current clock.
type workspaceFirstExecutionPreparation struct {
	Goal      workspace.Goal
	DoD       workspace.DoD
	Bootstrap runtimecontract.RuntimeBootstrap
	Files     filepreparation.Observation
}

func requiresFirstExecutionPreparation(w *workspace.Workspace) bool {
	return workspace.RequiresFirstExecutionPreparation(w)
}

func (m *Biz) observeFirstExecutionPreparation(p preparedWorkspaceExecutionIntent) (*workspaceFirstExecutionPreparation, error) {
	w := p.workspace
	if p.command.Bootstrap == nil || p.goal == nil || p.dod == nil {
		return nil, common.ErrConflict
	}
	// Cheap rejection precedes File I/O. This archived epoch is NOT a current
	// observation: the actual reread below and the reducer repeat the full gate.
	if _, err := preparationcompiler.CompileFirstExecutionPreparation(w, p.goal, p.dod, *p.command.Bootstrap, time.Now().UTC(), workspace.PreparationFileObservationEpoch(w)); err != nil {
		return nil, err
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	var scope *filepreparation.Scope
	for _, d := range w.PreparationDirectoryEvidence {
		if d.ContentHash == a.DirectoryEvidenceHash && d.Revalidation != nil {
			if scope != nil {
				return nil, common.ErrConflict
			}
			v := d.Revalidation.Scope.Clone()
			scope = &v
		}
	}
	if scope == nil {
		return nil, common.ErrConflict
	}
	ctx, cancel := context.WithTimeout(p.ctx, time.Second)
	defer cancel()
	command := fileevents.ReobservePreparationFilesCommand{Scope: *scope}
	ev := magicevent.NewEventWithContext(fileevents.TopicReobservePreparationFiles, ModuleID, "blocks/file", magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(w.ID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return nil, common.ErrConflict
	}
	v, err := result.Get()
	r, ok := v.(fileevents.ReobservePreparationFilesResult)
	if err != nil || !ok || r.ValidateFor(command) != nil || ctx.Err() != nil {
		return nil, common.ErrConflict
	}
	return &workspaceFirstExecutionPreparation{Goal: *workspace.CloneGoal(p.goal), DoD: *workspace.CloneDoD(p.dod), Bootstrap: p.command.Bootstrap.Clone(), Files: r.Observation.Clone()}, nil
}
