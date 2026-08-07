package workorchruntimepolicy

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

const (
	defaultMaxRuntimeRepairAttempts                     = 10
	defaultMaxProviderRetryAttempts                     = 10
	defaultMaxContractViolationRetriesPerStep           = 2
	defaultMaxContractViolationRetriesPerRun            = 8
	defaultMaxRepeatedContractViolationSignature        = 1
	defaultMaxConsecutiveContractViolationNoShapeChange = 1
)

type RuntimePolicy struct {
	MaxRuntimeRepairAttempts                     int
	MaxProviderRetryAttempts                     int
	MaxContractViolationRetriesPerStep           int
	MaxContractViolationRetriesPerRun            int
	MaxRepeatedContractViolationSignature        int
	MaxConsecutiveContractViolationNoShapeChange int
	LLMOutput                                    LLMOutputRuntimePolicy
}

// LLMOutputRuntimePolicy is the normalized Runtime-policy view of the
// persisted LLM output limits. The distinct named type preserves the policy
// boundary while sharing one structural field contract with configuration.
type LLMOutputRuntimePolicy workorchconfig.LLMOutputRuntimeConfig

func NormalizeRuntimePolicy(policy RuntimePolicy) RuntimePolicy {
	if policy.MaxRuntimeRepairAttempts <= 0 {
		policy.MaxRuntimeRepairAttempts = defaultMaxRuntimeRepairAttempts
	}
	if policy.MaxProviderRetryAttempts <= 0 {
		policy.MaxProviderRetryAttempts = defaultMaxProviderRetryAttempts
	}
	if policy.MaxContractViolationRetriesPerStep <= 0 {
		policy.MaxContractViolationRetriesPerStep = defaultMaxContractViolationRetriesPerStep
	}
	if policy.MaxContractViolationRetriesPerRun <= 0 {
		policy.MaxContractViolationRetriesPerRun = defaultMaxContractViolationRetriesPerRun
	}
	if policy.MaxRepeatedContractViolationSignature <= 0 {
		policy.MaxRepeatedContractViolationSignature = defaultMaxRepeatedContractViolationSignature
	}
	if policy.MaxConsecutiveContractViolationNoShapeChange <= 0 {
		policy.MaxConsecutiveContractViolationNoShapeChange = defaultMaxConsecutiveContractViolationNoShapeChange
	}
	policy.LLMOutput = NormalizeLLMOutputRuntimePolicy(policy.LLMOutput)
	return policy
}

func NormalizeLLMOutputRuntimePolicy(policy LLMOutputRuntimePolicy) LLMOutputRuntimePolicy {
	defaults := workorchconfig.DefaultLLMOutputRuntimeConfig()
	if policy.SemanticPlanMaxOutputTokens <= 0 {
		policy.SemanticPlanMaxOutputTokens = defaults.SemanticPlanMaxOutputTokens
	}
	if policy.CandidateMaxOutputTokens <= 0 {
		policy.CandidateMaxOutputTokens = defaults.CandidateMaxOutputTokens
	}
	if policy.MaxPatchContentBytes <= 0 {
		policy.MaxPatchContentBytes = defaults.MaxPatchContentBytes
	}
	if policy.MaxGraphBytes <= 0 {
		policy.MaxGraphBytes = defaults.MaxGraphBytes
	}
	if policy.MaxGraphFileWrites <= 0 {
		policy.MaxGraphFileWrites = defaults.MaxGraphFileWrites
	}
	if policy.SchemaInvalidLargePreviewThreshold <= 0 {
		policy.SchemaInvalidLargePreviewThreshold = defaults.SchemaInvalidLargePreviewThreshold
	}
	if policy.MaxArtifactChunkBytes <= 0 {
		policy.MaxArtifactChunkBytes = defaults.MaxArtifactChunkBytes
	}
	if policy.PreviousChunkTailBytes <= 0 {
		policy.PreviousChunkTailBytes = defaults.PreviousChunkTailBytes
	}
	if policy.MaxChunkRetryAttempts <= 0 {
		policy.MaxChunkRetryAttempts = defaults.MaxChunkRetryAttempts
	}
	if policy.MaxArtifactPlanAttempts <= 0 {
		policy.MaxArtifactPlanAttempts = defaults.MaxArtifactPlanAttempts
	}
	if policy.MaxArtifactReplanAttempts <= 0 {
		policy.MaxArtifactReplanAttempts = defaults.MaxArtifactReplanAttempts
	}
	return policy
}

func (p LLMOutputRuntimePolicy) Values() map[string]any {
	normalized := NormalizeLLMOutputRuntimePolicy(p)
	return workorchconfig.LLMOutputRuntimeConfigValues(workorchconfig.LLMOutputRuntimeConfig(normalized))
}
