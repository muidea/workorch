package configcore

type UpdateRuntimeConfigurationCommand struct {
	MaxRuntimeRepairAttempts *int                                 `json:"maxRuntimeRepairAttempts,omitempty"`
	MaxProviderRetryAttempts *int                                 `json:"maxProviderRetryAttempts,omitempty"`
	ShellCommandTimeout      *string                              `json:"shellCommandTimeout,omitempty"`
	LLMOutput                *UpdateRuntimeLLMOutputConfiguration `json:"llmOutput,omitempty"`
}

type UpdateRuntimeLLMOutputConfiguration struct {
	MaxPatchContentBytes               *int `json:"maxPatchContentBytes,omitempty"`
	MaxGraphBytes                      *int `json:"maxGraphBytes,omitempty"`
	MaxGraphFileWrites                 *int `json:"maxGraphFileWrites,omitempty"`
	SchemaInvalidLargePreviewThreshold *int `json:"schemaInvalidLargePreviewThreshold,omitempty"`
	MaxArtifactChunkBytes              *int `json:"maxArtifactChunkBytes,omitempty"`
	PreviousChunkTailBytes             *int `json:"previousChunkTailBytes,omitempty"`
	MaxChunkRetryAttempts              *int `json:"maxChunkRetryAttempts,omitempty"`
	MaxArtifactPlanAttempts            *int `json:"maxArtifactPlanAttempts,omitempty"`
	MaxArtifactReplanAttempts          *int `json:"maxArtifactReplanAttempts,omitempty"`
}
