package workspaceports

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type workspaceCreateIDConflictError struct {
	err error
}

func (e workspaceCreateIDConflictError) Error() string {
	if e.err == nil {
		return common.ErrConflict.Error()
	}
	return e.err.Error()
}

func (e workspaceCreateIDConflictError) Unwrap() error {
	return e.err
}

func isWorkspaceCreateIDConflict(err error) bool {
	var target workspaceCreateIDConflictError
	return errors.As(err, &target)
}

func createWorkspaceViewAttempt(ctx context.Context, hub magicevent.Hub, req CreateWorkspaceViewRequest) (*workspacemodel.Workspace, error) {
	id := workspacecore.GenerateWorkspaceID()
	layout, err := workspacecore.ResolveDirectoryLayout(req.RepositoryBackend, req.RepositoryDataDir, string(id), req.Command.DeliverableDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace directory layout for %q: %w", id, err)
	}
	if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
		return nil, fmt.Errorf("ensure workspace directory layout for %q: %w", id, err)
	}
	if err := CreateWorkspaceRecord(ctx, hub, id, req.Command.Title, req.Command.Description, layout.DeliverableDirectory, layout.InternalDirectory); err != nil {
		if errors.Is(err, common.ErrConflict) {
			return nil, workspaceCreateIDConflictError{err: fmt.Errorf("create workspace record %q: %w", id, err)}
		}
		return nil, fmt.Errorf("create workspace record %q: %w", id, err)
	}
	item, err := ReadWorkspaceRecord(ctx, hub, id)
	if err != nil {
		return nil, fmt.Errorf("read created workspace record %q: %w", id, err)
	}
	item, err = ApplyWorkspaceTemplateOnCreate(ctx, hub, item, req.Command.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("apply workspace template for %q: %w", id, err)
	}
	item, err = ApplyWorkspaceContractOverlayOnCreate(ctx, hub, item, req.Command.ContextHints, req.Command.EvidencePolicies)
	if err != nil {
		return nil, fmt.Errorf("apply workspace contract declaration for %q: %w", id, err)
	}
	return item, nil
}

// ApplyWorkspaceContractOverlayOnCreate records an optional declaration owned
// by this workspace after a reusable template is applied. It deliberately
// changes only Workspace metadata through the Workspace owner command: it does
// not construct a Run, activate execution, or invoke Runtime V2.
func ApplyWorkspaceContractOverlayOnCreate(ctx context.Context, hub magicevent.Hub, item *workspacemodel.Workspace, contextHints workorcheventpayload.Payload, evidencePolicies []workorcheventpayload.Payload) (*workspacemodel.Workspace, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	if len(contextHints.Fields) == 0 && len(evidencePolicies) == 0 {
		return item, nil
	}
	mergedHints, err := workorcheventpayload.FromMap(activationcore.MergeTemplateWorkspaceContextHintsForExecutionContract(item.ContextHints.Map(), contextHints.Map()))
	if err != nil {
		return nil, fmt.Errorf("merge workspace context hints: %w", err)
	}
	item.ContextHints = mergedHints
	item.EvidencePolicies = activationcore.MergeTemplateWorkspaceEvidencePolicies(item.EvidencePolicies, evidencePolicies)
	if err := UpdateWorkspaceRecord(ctx, hub, item); err != nil {
		return nil, err
	}
	return ReadWorkspaceRecord(ctx, hub, item.ID)
}
