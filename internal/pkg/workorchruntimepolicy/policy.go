package workorchruntimepolicy

const (
	defaultMaxRuntimeRepairAttempts                     = 10
	defaultMaxProviderRetryAttempts                     = 10
	defaultMaxContractViolationRetriesPerStep           = 2
	defaultMaxContractViolationRetriesPerRun            = 8
	defaultMaxRepeatedContractViolationSignature        = 1
	defaultMaxConsecutiveContractViolationNoShapeChange = 1
	defaultMaxPatchContentBytes                         = 65536
	defaultMaxGraphBytes                                = 131072
	defaultMaxGraphFileWrites                           = 3
	defaultSchemaInvalidPreviewBytes                    = 32768
	defaultMaxArtifactChunkBytes                        = 16384
	defaultPreviousChunkTailBytes                       = 300
	defaultMaxChunkRetryAttempts                        = 3
	defaultMaxArtifactPlanAttempts                      = 2
	defaultMaxArtifactReplanAttempts                    = 2
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
