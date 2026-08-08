package configcore

type UpdateRuntimeConfigurationCommand struct {
	MaxRuntimeRepairAttempts                     *int                                 `json:"maxRuntimeRepairAttempts,omitempty"`
	MaxProviderRetryAttempts                     *int                                 `json:"maxProviderRetryAttempts,omitempty"`
	MaxContractViolationRetriesPerStep           *int                                 `json:"maxContractViolationRetriesPerStep,omitempty"`
	MaxContractViolationRetriesPerRun            *int                                 `json:"maxContractViolationRetriesPerRun,omitempty"`
	MaxRepeatedContractViolationSignature        *int                                 `json:"maxRepeatedContractViolationSignature,omitempty"`
	MaxConsecutiveContractViolationNoShapeChange *int                                 `json:"maxConsecutiveContractViolationNoShapeChange,omitempty"`
	ShellCommandTimeout                          *string                              `json:"shellCommandTimeout,omitempty"`
	LLMOutput                                    *UpdateRuntimeLLMOutputConfiguration `json:"llmOutput,omitempty"`
}

type UpdateRuntimeLLMOutputConfiguration struct {
	MaxPatchContentBytes               *int `json:"maxPatchContentBytes,omitempty"`
	MaxGraphBytes                      *int `json:"maxGraphBytes,omitempty"`
	MaxGraphFileWrites                 *int `json:"maxGraphFileWrites,omitempty"`
	SchemaInvalidLargePreviewThreshold *int `json:"schemaInvalidLargePreviewThreshold,omitempty"`
	MaxArtifactChunkBytes              *int `json:"maxArtifactChunkBytes,omitempty"`
	MaxDocumentRecoveryAttempts        *int `json:"maxDocumentRecoveryAttempts,omitempty"`
	MaxArtifactPlanAttempts            *int `json:"maxArtifactPlanAttempts,omitempty"`
	MaxArtifactReplanAttempts          *int `json:"maxArtifactReplanAttempts,omitempty"`
}
