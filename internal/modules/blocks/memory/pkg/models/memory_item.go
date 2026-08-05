package memory

import "github.com/muidea/workorch/internal/pkg/common"

type MemoryItem struct {
	ID                  common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	Vector              []float64
	VectorScore         float64
}

func CloneMemoryItem(item *MemoryItem) *MemoryItem {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Tags = append([]string(nil), item.Tags...)
	cloned.Vector = append([]float64(nil), item.Vector...)
	return &cloned
}

func CloneMemoryItems(items []*MemoryItem) []*MemoryItem {
	if items == nil {
		return nil
	}
	ret := make([]*MemoryItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneMemoryItem(item))
	}
	return ret
}
