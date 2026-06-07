package workorchruntimepolicy

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func FromConfig(cfg workorchconfig.Config) RuntimePolicy {
	return RuntimePolicy{
		MaxRuntimeRepairAttempts: cfg.Runtime.MaxRuntimeRepairAttempts,
		MaxProviderRetryAttempts: cfg.Runtime.MaxProviderRetryAttempts,
		LLMOutput: LLMOutputRuntimePolicy{
			MaxPatchContentBytes:               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			MaxGraphBytes:                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			MaxGraphFileWrites:                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			SchemaInvalidLargePreviewThreshold: cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			MaxArtifactChunkBytes:              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			PreviousChunkTailBytes:             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			MaxChunkRetryAttempts:              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			MaxArtifactPlanAttempts:            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			MaxArtifactReplanAttempts:          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}
}
