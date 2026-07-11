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
