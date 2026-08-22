package run

import (
	"context"
	"strings"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runCreationInternalEventKind string

const runCreationInternalEventRequested runCreationInternalEventKind = "creation_requested"

type runCreationRequestedFact struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	GoalID              common.GoalID
	DoDID               common.DoDID
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	DefinitionRevision  string
	StartupInputHash    string
	InitialStepKind     string
	RuntimeBootstrap    *runtimecontract.RuntimeBootstrap
	ActivationRequestID string
	ReconciliationScope runtimecontract.RuntimeCWIReconciliationScope
	Priority            int
}

// runCreationInternalEvent deliberately has no State or timestamps. Those
// lifecycle values are selected only by the Run owner dispatcher.
type runCreationInternalEvent struct {
	Kind      runCreationInternalEventKind
	Requested *runCreationRequestedFact
}

// submitRunCreationInternalEvent is the sole Run creation-state dispatcher.
// It is shared by ordinary CreateRun and replacement materialization inside
// the same owner; neither caller can select the initial SM-1 state.
func submitRunCreationInternalEvent(ctx context.Context, runs repository.RunRepository, event runCreationInternalEvent) (*runmodel.Run, error) {
	if runs == nil || event.Kind != runCreationInternalEventRequested || event.Requested == nil {
		return nil, common.ErrInvalidArgument
	}
	fact := *event.Requested
	if fact.WorkspaceID == "" || fact.RunID == "" {
		return nil, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, fact.WorkspaceID)
	if err != nil {
		return nil, err
	}
	run := &runmodel.Run{
		ID:                  fact.RunID,
		WorkspaceID:         fact.WorkspaceID,
		GoalID:              fact.GoalID,
		DoDID:               fact.DoDID,
		Lineage:             createRunLineage(fact.PredecessorRunID, fact.PredecessorResultID),
		DefinitionRevision:  strings.TrimSpace(fact.DefinitionRevision),
		StartupInputHash:    strings.TrimSpace(fact.StartupInputHash),
		InitialStepKind:     strings.TrimSpace(fact.InitialStepKind),
		RuntimeBootstrap:    cloneRuntimeBootstrap(fact.RuntimeBootstrap),
		ActivationRequestID: strings.TrimSpace(fact.ActivationRequestID),
		ReconciliationScope: fact.ReconciliationScope,
		State:               common.RunStatePending,
		Priority:            fact.Priority,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runs.Create(bound, run); err != nil {
		return nil, err
	}
	return run, nil
}

func createRunLineage(predecessorRunID common.RunID, predecessorResultID common.ResultID) runmodel.RunLineage {
	return runmodel.RunLineage{
		PredecessorRunID:    common.RunID(strings.TrimSpace(string(predecessorRunID))),
		PredecessorResultID: common.ResultID(strings.TrimSpace(string(predecessorResultID))),
		LineageState:        "active",
	}
}
