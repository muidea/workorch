package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type MemorySelectionOptions struct {
	WorkspaceID             common.WorkspaceID
	Hub                     magicevent.Hub
	IDs                     []string
	Goal                    *workspacemodel.Goal
	DoD                     *workspacemodel.DoD
	EmbeddingTarget         runtimecontract.ProviderModelTarget
	EmbeddingProviderConfig error
}

type DefaultEmbeddingProviderLoader func(context.Context) (runtimecontract.ProviderModelTarget, error)

type WorkspaceMemorySelectionRequest struct {
	WorkspaceID                    common.WorkspaceID
	Hub                            magicevent.Hub
	IDs                            []string
	Goal                           *workspacemodel.Goal
	DoD                            *workspacemodel.DoD
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
}

func SelectedWorkspaceMemoryItems(ctx context.Context, request WorkspaceMemorySelectionRequest) ([]*memorymodel.MemoryItem, *MemorySelectionEvidence, error) {
	selection := MemorySelectionOptions{
		WorkspaceID: request.WorkspaceID,
		Hub:         request.Hub,
		IDs:         request.IDs,
		Goal:        request.Goal,
		DoD:         request.DoD,
	}
	if len(selection.IDs) == 0 && request.DefaultEmbeddingProviderLoader != nil {
		target, err := request.DefaultEmbeddingProviderLoader(ctx)
		if err != nil {
			// Keep degraded evidence path, but still allow workspace list fallback.
			selection.EmbeddingProviderConfig = err
		} else {
			target = target.Normalized()
			// Empty target means embedding is intentionally not configured.
			if target.Validate() == nil {
				selection.EmbeddingTarget = target
			}
		}
	}
	return SelectedMemoryItems(ctx, selection)
}

func SelectedMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *MemorySelectionEvidence, error) {
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
			semanticEvidence = &MemorySelectionEvidence{}
		}
		// Successful semantic path (including empty selected set) returns as-is.
		if semanticEvidence.Status != "degraded" {
			semanticEvidence.RetrievedMemoryIDs = activationcore.MemoryItemIDs(selected)
			if len(selected) == 0 && len(semanticItems) > 0 {
				semanticEvidence.Status = "none_selected"
			}
			return selected, semanticEvidence, nil
		}
		// Degraded semantic recall must fall back to workspace list baseline while
		// preserving degraded evidence for audit.
		listItems, listEvidence, err := selectedWorkspaceListMemoryItems(ctx, opts)
		if err != nil {
			return nil, semanticEvidence, err
		}
		if listEvidence == nil {
			listEvidence = &MemorySelectionEvidence{Status: "selected"}
		}
		listEvidence.SemanticRecall = semanticEvidence
		if listEvidence.Status == "" {
			listEvidence.Status = "selected"
		}
		listEvidence.FallbackFrom = "semantic_vector_search"
		return listItems, listEvidence, nil
	}
	return selectedWorkspaceListMemoryItems(ctx, opts)
}
