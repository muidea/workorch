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
	MaxDocumentRecoveryAttempts *int `json:"maxDocumentRecoveryAttempts,omitempty"`
}
