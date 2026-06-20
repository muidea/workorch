package configcore

type RuntimeConfiguration struct {
	DefaultSoulID                                string                        `json:"defaultSoulId,omitempty"`
	MaxRuntimeRepairAttempts                     int                           `json:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts                     int                           `json:"maxProviderRetryAttempts"`
	MaxContractViolationRetriesPerStep           int                           `json:"maxContractViolationRetriesPerStep"`
	MaxContractViolationRetriesPerRun            int                           `json:"maxContractViolationRetriesPerRun"`
	MaxRepeatedContractViolationSignature        int                           `json:"maxRepeatedContractViolationSignature"`
	MaxConsecutiveContractViolationNoShapeChange int                           `json:"maxConsecutiveContractViolationNoShapeChange"`
	ShellCommandTimeout                          string                        `json:"shellCommandTimeout"`
	Git                                          RuntimeGitConfiguration       `json:"git"`
	LLMOutput                                    RuntimeLLMOutputConfiguration `json:"llmOutput"`
}

type RuntimeGitConfiguration struct {
	DefaultTimeout string `json:"defaultTimeout,omitempty"`
}

type RuntimeLLMOutputConfiguration struct {
	MaxPatchContentBytes               int `json:"maxPatchContentBytes"`
	MaxGraphBytes                      int `json:"maxGraphBytes"`
	MaxGraphFileWrites                 int `json:"maxGraphFileWrites"`
	SchemaInvalidLargePreviewThreshold int `json:"schemaInvalidLargePreviewThreshold"`
	MaxArtifactChunkBytes              int `json:"maxArtifactChunkBytes"`
	PreviousChunkTailBytes             int `json:"previousChunkTailBytes"`
	MaxChunkRetryAttempts              int `json:"maxChunkRetryAttempts"`
	MaxArtifactPlanAttempts            int `json:"maxArtifactPlanAttempts"`
	MaxArtifactReplanAttempts          int `json:"maxArtifactReplanAttempts"`
}
