package contextregistrycore

type MemoryItem struct {
	ID                  string   `json:"id"`
	WorkspaceID         string   `json:"workspaceId"`
	RunID               string   `json:"runId,omitempty"`
	SourceType          string   `json:"sourceType"`
	SourceRef           string   `json:"sourceRef,omitempty"`
	MemoryType          string   `json:"memoryType"`
	Content             string   `json:"content,omitempty"`
	Summary             string   `json:"summary"`
	Tags                []string `json:"tags,omitempty"`
	Importance          int      `json:"importance,omitempty"`
	Confidence          int      `json:"confidence,omitempty"`
	Visibility          string   `json:"visibility,omitempty"`
	EmbeddingProviderID string   `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string   `json:"embeddingModel,omitempty"`
	EmbeddingText       string   `json:"embeddingText,omitempty"`
	VectorReady         bool     `json:"vectorReady"`
	VectorDimension     int      `json:"vectorDimension,omitempty"`
	VectorScore         float64  `json:"vectorScore,omitempty"`
}

type CreateMemoryItemCommand struct {
	MemoryID            string    `json:"memoryId,omitempty"`
	RunID               string    `json:"runId,omitempty"`
	SourceType          string    `json:"sourceType"`
	SourceRef           string    `json:"sourceRef,omitempty"`
	MemoryType          string    `json:"memoryType"`
	Content             string    `json:"content,omitempty"`
	Summary             string    `json:"summary"`
	Tags                []string  `json:"tags,omitempty"`
	Importance          int       `json:"importance,omitempty"`
	Confidence          int       `json:"confidence,omitempty"`
	Visibility          string    `json:"visibility,omitempty"`
	EmbeddingProviderID string    `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string    `json:"embeddingModel,omitempty"`
	EmbeddingText       string    `json:"embeddingText,omitempty"`
	Vector              []float64 `json:"vector,omitempty"`
}

type UpdateMemoryItemCommand = CreateMemoryItemCommand

type MemoryQuery struct {
	WorkspaceID string
	Query       string
	Tags        []string
	Types       []string
	Limit       int
}
