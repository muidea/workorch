package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MemorySelectionOptions struct {
	WorkspaceID             common.WorkspaceID
	Hub                     magicevent.Hub
	IDs                     []string
	Goal                    *workspacemodel.Goal
	DoD                     *workspacemodel.DoD
	EmbeddingProviderID     string
	EmbeddingProviderConfig error
}

type DefaultEmbeddingProviderLoader func(context.Context) (string, error)

type WorkspaceMemorySelectionRequest struct {
	WorkspaceID                    common.WorkspaceID
	Hub                            magicevent.Hub
	IDs                            []string
	Goal                           *workspacemodel.Goal
	DoD                            *workspacemodel.DoD
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
}

func SelectedWorkspaceMemoryItems(ctx context.Context, request WorkspaceMemorySelectionRequest) ([]*memorymodel.MemoryItem, map[string]any, error) {
	selection := MemorySelectionOptions{
		WorkspaceID: request.WorkspaceID,
		Hub:         request.Hub,

		IDs:  request.IDs,
		Goal: request.Goal,
		DoD:  request.DoD,
	}
	if len(selection.IDs) == 0 && request.DefaultEmbeddingProviderLoader != nil {
		providerID, err := request.DefaultEmbeddingProviderLoader(ctx)
		if err != nil {
			selection.EmbeddingProviderConfig = err
		} else {
			selection.EmbeddingProviderID = providerID
		}
	}
	return SelectedMemoryItems(ctx, selection)
}

func SelectedMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, map[string]any, error) {
	if opts.Hub == nil {
		if len(opts.IDs) > 0 {
			return nil, nil, common.ErrInvalidArgument
		}
		return nil, nil, nil
	}
	if len(opts.IDs) > 0 {
		return selectedExplicitMemoryItems(ctx, opts)
	}
	semanticItems, semanticEvidence, semanticHandled := semanticMemoryItems(ctx, opts)
	if semanticHandled {
		selected := activationcore.VisibleMemoryItems(semanticItems, 8)
		if semanticEvidence == nil {
			semanticEvidence = map[string]any{}
		}
		semanticEvidence["retrievedMemoryIds"] = activationcore.MemoryItemIDs(selected)
		if len(selected) == 0 && len(semanticItems) > 0 {
			semanticEvidence["status"] = "none_selected"
		}
		return selected, semanticEvidence, nil
	}
	return selectedWorkspaceListMemoryItems(ctx, opts)
}
