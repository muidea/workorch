package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

const (
	ContractViolationRecoverabilityRetrySameContract  = "retry_same_contract"
	ContractViolationRecoverabilityNarrowContract     = "narrow_contract"
	ContractViolationRecoverabilityNormalizeThenRetry = "normalize_then_retry"
	ContractViolationRecoverabilityDecisionRequired   = "decision_required"
	ContractViolationRecoverabilityFailFast           = "fail_fast"
)

type ContractViolation struct {
	ID                   string             `json:"id"`
	WorkspaceID          common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID                common.RunID       `json:"runId,omitempty"`
	StepID               common.StepID      `json:"stepId,omitempty"`
	SourceStepID         common.StepID      `json:"sourceStepId,omitempty"`
	Stage                string             `json:"stage,omitempty"`
	RuntimeStage         string             `json:"runtimeStage,omitempty"`
	RuntimeStageMode     string             `json:"runtimeStageMode,omitempty"`
	RepairObjective      string             `json:"repairObjective,omitempty"`
	ViolatedRule         string             `json:"violatedRule,omitempty"`
	ExpectedContract     map[string]any     `json:"expectedContract,omitempty"`
	ActualOutputSummary  map[string]any     `json:"actualOutputSummary,omitempty"`
	RejectedOutputRef    string             `json:"rejectedOutputRef,omitempty"`
	FailureClass         string             `json:"failureClass,omitempty"`
	FailureSignature     string             `json:"failureSignature,omitempty"`
	Recoverability       string             `json:"recoverability,omitempty"`
	RetryHint            string             `json:"retryHint,omitempty"`
	ForbiddenRepeat      []string           `json:"forbiddenRepeat,omitempty"`
	CorrectionProfileID  string             `json:"correctionProfileId,omitempty"`
	CorrectionMode       string             `json:"correctionMode,omitempty"`
	CorrectionContract   map[string]any     `json:"correctionContract,omitempty"`
	TargetScope          string             `json:"targetScope,omitempty"`
	SourceRefs           []string           `json:"sourceRefs,omitempty"`
	Attempt              int                `json:"attempt,omitempty"`
	MaxAttempts          int                `json:"maxAttempts,omitempty"`
	SignatureRepeatCount int                `json:"signatureRepeatCount,omitempty"`
	CreatedAt            string             `json:"createdAt,omitempty"`
}

type ContractViolationRecoveryPolicy struct {
	Enabled                       bool `json:"enabled"`
	MaxRetriesPerStep             int  `json:"maxRetriesPerStep"`
	MaxRetriesPerRun              int  `json:"maxRetriesPerRun"`
	MaxRepeatedViolationSignature int  `json:"maxRepeatedViolationSignature"`
	MaxConsecutiveNoShapeChange   int  `json:"maxConsecutiveNoShapeChange"`
	EscalateOnUnsafeOutput        bool `json:"escalateOnUnsafeOutput"`
}
