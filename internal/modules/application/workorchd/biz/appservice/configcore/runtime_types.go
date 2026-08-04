package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

type RuntimeConfiguration struct {
	DefaultSoulID                                string                                `json:"defaultSoulId,omitempty"`
	MaxRuntimeRepairAttempts                     int                                   `json:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts                     int                                   `json:"maxProviderRetryAttempts"`
	MaxContractViolationRetriesPerStep           int                                   `json:"maxContractViolationRetriesPerStep"`
	MaxContractViolationRetriesPerRun            int                                   `json:"maxContractViolationRetriesPerRun"`
	MaxRepeatedContractViolationSignature        int                                   `json:"maxRepeatedContractViolationSignature"`
	MaxConsecutiveContractViolationNoShapeChange int                                   `json:"maxConsecutiveContractViolationNoShapeChange"`
	ShellCommandTimeout                          string                                `json:"shellCommandTimeout"`
	Git                                          RuntimeGitConfiguration               `json:"git"`
	LLMOutput                                    workorchconfig.LLMOutputRuntimeConfig `json:"llmOutput"`
}

type RuntimeGitConfiguration struct {
	DefaultTimeout string `json:"defaultTimeout,omitempty"`
}
