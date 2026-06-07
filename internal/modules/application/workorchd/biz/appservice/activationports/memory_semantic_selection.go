package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func semanticMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, map[string]any, bool) {
	if opts.EmbeddingProviderConfig != nil {
		return nil, map[string]any{
			"policy": "semantic_vector_search",
			"status": "degraded",
			"reason": opts.EmbeddingProviderConfig.Error(),
		}, true
	}
	providerID := strings.TrimSpace(opts.EmbeddingProviderID)
	if providerID == "" {
		return nil, nil, false
	}
	queryText := activationcore.MemoryRetrievalQueryText(opts.Goal, opts.DoD)
	evidence := map[string]any{
		"policy":              "semantic_vector_search",
		"providerId":          providerID,
		"operation":           llmcommon.ProviderOperationEmbeddings,
		"queryTextPreview":    activationcore.PreviewRetrievalQuery(queryText),
		"queryTextSha256":     activationcore.SHA256Hex(queryText),
		"minScore":            0.1,
		"limit":               8,
		"retrievedMemoryIds":  []string{},
		"retrievedMemoryHits": []map[string]any{},
	}
	embedding, err := GenerateEmbedding(ctx, opts.Hub, providerID, queryText)
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	items, err := ownerreadports.SearchMemoryByOwnerCommand(ctx, opts.Hub, repository.MemorySearchQuery{
		WorkspaceID: opts.WorkspaceID,
		Vector:      embedding.Vector,
		MinScore:    0.1,
		Limit:       8,
	})
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	evidence["status"] = "selected"
	evidence["embeddingModel"] = embedding.Model
	evidence["vectorDimension"] = len(embedding.Vector)
	evidence["retrievedMemoryIds"] = activationcore.MemoryItemIDs(items)
	evidence["retrievedMemoryHits"] = activationcore.MemoryRetrievalHits(items)
	return items, evidence, true
}
