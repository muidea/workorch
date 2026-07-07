package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	ContractViolationTypeLLMSchemaInvalid = "llm_schema_invalid"

	ContractViolationRuleUnparseableOutput = "unparseable_output"
)

const (
	ContractViolationRecoverabilityRetrySameContract               = "retry_same_contract"
	ContractViolationRecoverabilityNarrowContract                  = "narrow_contract"
	ContractViolationRecoverabilityNormalizeThenRetry              = "normalize_then_retry"
	ContractViolationRecoverabilityRecomputeAuthority              = "recompute_authority"
	ContractViolationRecoverabilityRebuildValidationRequiredOutput = "rebuild_validation_required_output_from_authority"
	ContractViolationRecoverabilityDecisionRequired                = "decision_required"
	ContractViolationRecoverabilityFailFast                        = "fail_fast"
)

type ContractViolation struct {
	ID                   string             `json:"id"`
	WorkspaceID          common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID                common.RunID       `json:"runId,omitempty"`
	StepID               common.StepID      `json:"stepId,omitempty"`
	SourceStepID         common.StepID      `json:"sourceStepId,omitempty"`
	ViolationType        string             `json:"violationType,omitempty"`
	Stage                string             `json:"stage,omitempty"`
	RuntimeStage         string             `json:"runtimeStage,omitempty"`
	RuntimeStageMode     string             `json:"runtimeStageMode,omitempty"`
	RepairObjective      string             `json:"repairObjective,omitempty"`
	ViolatedRule         string             `json:"violatedRule,omitempty"`
	Signature            string             `json:"signature,omitempty"`
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
	BudgetScope          map[string]any     `json:"budgetScope,omitempty"`
	SourceRefs           []string           `json:"sourceRefs,omitempty"`
	Attempt              int                `json:"attempt,omitempty"`
	MaxAttempts          int                `json:"maxAttempts,omitempty"`
	SignatureRepeatCount int                `json:"signatureRepeatCount,omitempty"`
	CreatedAt            string             `json:"createdAt,omitempty"`
}

func (v ContractViolation) Normalized() ContractViolation {
	v.ID = strings.TrimSpace(v.ID)
	v.ViolationType = strings.TrimSpace(v.ViolationType)
	v.Stage = strings.TrimSpace(v.Stage)
	v.RuntimeStage = strings.TrimSpace(v.RuntimeStage)
	v.RuntimeStageMode = strings.TrimSpace(v.RuntimeStageMode)
	v.RepairObjective = strings.TrimSpace(v.RepairObjective)
	v.ViolatedRule = strings.TrimSpace(v.ViolatedRule)
	v.Signature = strings.TrimSpace(v.Signature)
	v.RejectedOutputRef = strings.TrimSpace(v.RejectedOutputRef)
	v.FailureClass = strings.TrimSpace(v.FailureClass)
	v.FailureSignature = strings.TrimSpace(v.FailureSignature)
	v.Recoverability = normalizedContractViolationRecoverability(v.Recoverability)
	v.RetryHint = strings.TrimSpace(v.RetryHint)
	v.ForbiddenRepeat = compactContractViolationStrings(v.ForbiddenRepeat)
	v.CorrectionProfileID = strings.TrimSpace(v.CorrectionProfileID)
	v.CorrectionMode = strings.TrimSpace(v.CorrectionMode)
	v.TargetScope = strings.TrimSpace(v.TargetScope)
	v.SourceRefs = compactContractViolationStrings(v.SourceRefs)
	v.CreatedAt = strings.TrimSpace(v.CreatedAt)
	if v.Signature == "" {
		v.Signature = v.FailureSignature
	}
	return v
}

func (v ContractViolation) HasScope() bool {
	return v.WorkspaceID != "" && v.RunID != ""
}

func (v ContractViolation) Recoverable() bool {
	switch v.Normalized().Recoverability {
	case ContractViolationRecoverabilityRetrySameContract,
		ContractViolationRecoverabilityNarrowContract,
		ContractViolationRecoverabilityNormalizeThenRetry,
		ContractViolationRecoverabilityRebuildValidationRequiredOutput:
		return true
	default:
		return false
	}
}

type ContractViolationRecoveryPolicy struct {
	Enabled                       bool `json:"enabled"`
	MaxRetriesPerStep             int  `json:"maxRetriesPerStep"`
	MaxRetriesPerRun              int  `json:"maxRetriesPerRun"`
	MaxRepeatedViolationSignature int  `json:"maxRepeatedViolationSignature"`
	MaxConsecutiveNoShapeChange   int  `json:"maxConsecutiveNoShapeChange"`
	EscalateOnUnsafeOutput        bool `json:"escalateOnUnsafeOutput"`
}

func normalizedContractViolationRecoverability(value string) string {
	switch strings.TrimSpace(value) {
	case ContractViolationRecoverabilityRetrySameContract,
		ContractViolationRecoverabilityNarrowContract,
		ContractViolationRecoverabilityNormalizeThenRetry,
		ContractViolationRecoverabilityRecomputeAuthority,
		ContractViolationRecoverabilityRebuildValidationRequiredOutput,
		ContractViolationRecoverabilityDecisionRequired,
		ContractViolationRecoverabilityFailFast:
		return strings.TrimSpace(value)
	default:
		return ContractViolationRecoverabilityRetrySameContract
	}
}

func compactContractViolationStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
