package models

import "github.com/muidea/workorch/internal/pkg/common"

const (
	defaultMaxRuntimeRepairAttempts  = 10
	defaultMaxProviderRetryAttempts  = 10
	defaultMaxPatchContentBytes      = 12288
	defaultMaxGraphBytes             = 32768
	defaultMaxGraphFileWrites        = 3
	defaultSchemaInvalidPreviewBytes = 8192
	defaultMaxArtifactChunkBytes     = 8192
	defaultPreviousChunkTailBytes    = 300
	defaultMaxChunkRetryAttempts     = 3
	defaultMaxArtifactPlanAttempts   = 2
	defaultMaxArtifactReplanAttempts = 2
)

type RuntimePolicy struct {
	MaxRuntimeRepairAttempts int
	MaxProviderRetryAttempts int
	LLMOutput                LLMOutputRuntimePolicy
}

type LLMOutputRuntimePolicy struct {
	MaxPatchContentBytes               int
	MaxGraphBytes                      int
	MaxGraphFileWrites                 int
	SchemaInvalidLargePreviewThreshold int
	MaxArtifactChunkBytes              int
	PreviousChunkTailBytes             int
	MaxChunkRetryAttempts              int
	MaxArtifactPlanAttempts            int
	MaxArtifactReplanAttempts          int
}

type StepExecutionRequest struct {
	WorkspaceID                   common.WorkspaceID
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	Input                         map[string]any
}

type StepExecutionResult struct {
	Output map[string]any
}

type BlockExecutionGraph struct {
	Nodes []BlockExecutionNode `json:"nodes"`
}

type BlockExecutionNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	DependsOn []string       `json:"dependsOn,omitempty"`
	Branch    string         `json:"branch,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

func NormalizeRuntimePolicy(policy RuntimePolicy) RuntimePolicy {
	if policy.MaxRuntimeRepairAttempts <= 0 {
		policy.MaxRuntimeRepairAttempts = defaultMaxRuntimeRepairAttempts
	}
	if policy.MaxProviderRetryAttempts <= 0 {
		policy.MaxProviderRetryAttempts = defaultMaxProviderRetryAttempts
	}
	policy.LLMOutput = NormalizeLLMOutputRuntimePolicy(policy.LLMOutput)
	return policy
}

func NormalizeLLMOutputRuntimePolicy(policy LLMOutputRuntimePolicy) LLMOutputRuntimePolicy {
	if policy.MaxPatchContentBytes <= 0 {
		policy.MaxPatchContentBytes = defaultMaxPatchContentBytes
	}
	if policy.MaxGraphBytes <= 0 {
		policy.MaxGraphBytes = defaultMaxGraphBytes
	}
	if policy.MaxGraphFileWrites <= 0 {
		policy.MaxGraphFileWrites = defaultMaxGraphFileWrites
	}
	if policy.SchemaInvalidLargePreviewThreshold <= 0 {
		policy.SchemaInvalidLargePreviewThreshold = defaultSchemaInvalidPreviewBytes
	}
	if policy.MaxArtifactChunkBytes <= 0 {
		policy.MaxArtifactChunkBytes = defaultMaxArtifactChunkBytes
	}
	if policy.PreviousChunkTailBytes <= 0 {
		policy.PreviousChunkTailBytes = defaultPreviousChunkTailBytes
	}
	if policy.MaxChunkRetryAttempts <= 0 {
		policy.MaxChunkRetryAttempts = defaultMaxChunkRetryAttempts
	}
	if policy.MaxArtifactPlanAttempts <= 0 {
		policy.MaxArtifactPlanAttempts = defaultMaxArtifactPlanAttempts
	}
	if policy.MaxArtifactReplanAttempts <= 0 {
		policy.MaxArtifactReplanAttempts = defaultMaxArtifactReplanAttempts
	}
	return policy
}

func (p LLMOutputRuntimePolicy) Values() map[string]any {
	normalized := NormalizeLLMOutputRuntimePolicy(p)
	return map[string]any{
		"maxPatchContentBytes":               normalized.MaxPatchContentBytes,
		"maxGraphBytes":                      normalized.MaxGraphBytes,
		"maxGraphFileWrites":                 normalized.MaxGraphFileWrites,
		"schemaInvalidLargePreviewThreshold": normalized.SchemaInvalidLargePreviewThreshold,
		"maxArtifactChunkBytes":              normalized.MaxArtifactChunkBytes,
		"previousChunkTailBytes":             normalized.PreviousChunkTailBytes,
		"maxChunkRetryAttempts":              normalized.MaxChunkRetryAttempts,
		"maxArtifactPlanAttempts":            normalized.MaxArtifactPlanAttempts,
		"maxArtifactReplanAttempts":          normalized.MaxArtifactReplanAttempts,
	}
}
