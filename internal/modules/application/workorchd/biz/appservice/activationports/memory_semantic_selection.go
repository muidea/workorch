package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func semanticMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *MemorySelectionEvidence, bool) {
	if opts.EmbeddingProviderConfig != nil {
		return nil, &MemorySelectionEvidence{
			Policy: "semantic_vector_search",
			Status: "degraded",
			Reason: opts.EmbeddingProviderConfig.Error(),
		}, true
	}
	target := opts.EmbeddingTarget.Normalized()
	if target.Validate() != nil {
		return nil, nil, false
	}
	queryText := activationcore.MemoryRetrievalQueryText(opts.Goal, opts.DoD)
	evidence := &MemorySelectionEvidence{
		Policy:              "semantic_vector_search",
		ProviderID:          target.ProviderID,
		ModelID:             target.ModelID,
		Operation:           llmcommon.ProviderOperationEmbeddings,
		QueryTextPreview:    activationcore.PreviewRetrievalQuery(queryText),
		QueryTextSHA256:     activationcore.SHA256Hex(queryText),
		MinScore:            0.1,
		Limit:               100,
		RefreshSource:       "semantic_vector_search",
		RetrievedMemoryIDs:  []string{},
		RetrievedMemoryHits: []MemorySelectionHit{},
	}
	embedding, err := GenerateEmbedding(ctx, opts.Hub, target, queryText)
	if err != nil {
		evidence.Status = "degraded"
		evidence.Reason = err.Error()
		return nil, evidence, true
	}
	items, err := ownerreadports.SearchMemoryByOwnerCommand(ctx, opts.Hub, workorchquerycontract.MemorySearchQuery{
		WorkspaceID: opts.WorkspaceID,
		Vector:      embedding.Vector,
		MinScore:    0.1,
		Limit:       100,
	})
	if err != nil {
		evidence.Status = "degraded"
		evidence.Reason = err.Error()
		return nil, evidence, true
	}
	evidence.Status = "selected"
	evidence.EmbeddingModel = embedding.Model
	evidence.VectorDimension = len(embedding.Vector)
	return items, evidence, true
}
