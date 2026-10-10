package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceExecutionInternalEventKind string

const (
	workspaceExecutionInternalEventActivationProviderSelectionObserved workspaceExecutionInternalEventKind = "activation_provider_selection_observed"
)

// workspaceExecutionInternalEvent is the only owner-private input allowed to
// materialize Workspace execution metadata. A caller can submit a sealed
// bootstrap fact, but cannot select the persisted Workspace mutation.
type workspaceExecutionInternalEvent struct {
	Kind              workspaceExecutionInternalEventKind
	WorkspaceID       common.WorkspaceID
	StartupInputHash  string
	ProviderSelection runtimecontract.ProviderInferenceSelection
}

type workspaceExecutionTransition struct {
	Next *workspacemodel.Workspace
	Noop bool
}

func normalizeWorkspaceExecutionInternalEvent(event workspaceExecutionInternalEvent) (workspaceExecutionInternalEvent, error) {
	event.StartupInputHash = strings.TrimSpace(event.StartupInputHash)
	event.ProviderSelection = event.ProviderSelection.Normalized()
	if event.Kind != workspaceExecutionInternalEventActivationProviderSelectionObserved || event.WorkspaceID == "" || event.StartupInputHash == "" {
		return workspaceExecutionInternalEvent{}, common.ErrInvalidArgument
	}
	if err := event.ProviderSelection.Validate(); err != nil {
		return workspaceExecutionInternalEvent{}, fmt.Errorf("%w: activation provider selection: %v", common.ErrInvalidArgument, err)
	}
	return event, nil
}

func reduceWorkspaceExecutionInternalEvent(current *workspacemodel.Workspace, event workspaceExecutionInternalEvent) (workspaceExecutionTransition, error) {
	if current == nil || current.ID != event.WorkspaceID {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if err := workspaceDeletionRejectsMutation(current); err != nil {
		return workspaceExecutionTransition{}, err
	}
	_, present, err := current.ValidateActivationAdmissionPersistence()
	if err != nil || !present || current.StartupInput == nil || current.ActivationAdmission == nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	startup := current.StartupInput.Normalized()
	admission := current.ActivationAdmission.Normalized()
	if startup.StartupInputHash != event.StartupInputHash || admission.State != workspacemodel.WorkspaceActivationEligible {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	// Existing metadata is idempotent only after the current sealed activation
	// fence has been revalidated. Otherwise a stale restart could use an old
	// bootstrap merely because an earlier run already materialized a default.
	if current.DefaultLLMSelection != nil {
		return workspaceExecutionTransition{Next: current, Noop: true}, nil
	}

	// A bootstrap's reasoning effort is sealed for that Run. The Workspace
	// fallback remains a target-only preference, preserving the existing
	// ownership rule that a Run-scoped effort does not become mutable default.
	selection := runtimecontract.ProviderInferenceSelection{Target: event.ProviderSelection.Target.Normalized()}
	if err := selection.Validate(); err != nil {
		return workspaceExecutionTransition{}, fmt.Errorf("%w: workspace default provider selection: %v", common.ErrInvalidArgument, err)
	}
	next := *current
	next.DefaultLLMSelection = &selection
	return workspaceExecutionTransition{Next: &next}, nil
}

func (m *Biz) submitWorkspaceExecutionInternalEvent(ctx context.Context, event workspaceExecutionInternalEvent) (*workspacemodel.Workspace, error) {
	if m == nil || m.workspaces == nil {
		return nil, common.ErrInvalidArgument
	}
	event, err := normalizeWorkspaceExecutionInternalEvent(event)
	if err != nil {
		return nil, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		current, err := m.workspaces.Get(bound, event.WorkspaceID)
		if err != nil {
			return nil, err
		}
		transition, err := reduceWorkspaceExecutionInternalEvent(current, event)
		if err != nil {
			return nil, err
		}
		if transition.Noop {
			return current, nil
		}
		if transition.Next == nil {
			return nil, common.ErrInvalidArgument
		}
		if err := m.workspaces.CompareAndSwap(bound, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		m.publishWorkspaceUpdated(bound, transition.Next)
		return transition.Next, nil
	}
	return nil, common.ErrConflict
}
