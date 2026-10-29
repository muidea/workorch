package activationports

import (
	"context"
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"

	magicevent "github.com/muidea/magicCommon/event"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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

func SelectedWorkspaceMemoryItems(ctx context.Context, request WorkspaceMemorySelectionRequest) ([]*memorymodel.MemoryItem, *runtimeprojection.MemorySelectionEvidence, error) {
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

func SelectedMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *runtimeprojection.MemorySelectionEvidence, error) {
	if opts.Hub == nil {
		if len(opts.IDs) > 0 {
			return nil, nil, common.ErrInvalidArgument
		}
		return nil, nil, nil
	}
	if len(opts.IDs) > 0 {
		return selectedExplicitMemoryItems(ctx, opts)
	}
	// Read the owner baseline before attempting semantic recall. An empty owner
	// inventory has nothing to rank, so issuing an embedding request only adds
	// provider latency and can incorrectly mark a clean empty selection as
	// degraded. The same baseline is reused if semantic recall later degrades,
	// avoiding a second cross-owner list command.
	listItems, listEvidence, err := selectedWorkspaceListMemoryItems(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	if listEvidence == nil || len(listEvidence.CandidateMemoryIDs) == 0 {
		return listItems, listEvidence, nil
	}
	semanticItems, semanticEvidence, semanticHandled := semanticMemoryItems(ctx, opts)
	if semanticHandled {
		selected, semanticEvidence, err := applyRuntimeMemorySelectionBudget(semanticItems, semanticEvidence)
		if err != nil {
			return nil, nil, err
		}
		// Successful semantic path (including empty selected set) returns as-is.
		if semanticEvidence.Status != "degraded" {
			return selected, semanticEvidence, nil
		}
		// Degraded semantic recall must fall back to workspace list baseline while
		// preserving degraded evidence for audit.
		if listEvidence == nil {
			listEvidence = &runtimeprojection.MemorySelectionEvidence{Status: "selected"}
		}
		listEvidence.SemanticRecall = semanticEvidence
		if listEvidence.Status == "" {
			listEvidence.Status = "selected"
		}
		listEvidence.FallbackFrom = "semantic_vector_search"
		return listItems, listEvidence, nil
	}
	return listItems, listEvidence, nil
}
