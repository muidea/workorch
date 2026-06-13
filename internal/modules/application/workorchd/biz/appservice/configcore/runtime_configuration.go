package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func runtimeShellCommandTimeout(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	return "60s"
}

func runtimeGitConfiguration(cfg workorchconfig.GitRuntimeConfig) RuntimeGitConfiguration {
	return RuntimeGitConfiguration{
		Enabled:        workorchconfig.GitRuntimeEnabled(cfg),
		WriteEnabled:   workorchconfig.GitRuntimeWriteEnabled(cfg),
		DefaultTimeout: strings.TrimSpace(cfg.DefaultTimeout),
	}
}

func runtimeLLMOutputConfiguration(cfg workorchconfig.LLMOutputRuntimeConfig) RuntimeLLMOutputConfiguration {
	normalized, err := workorchconfig.NormalizeLLMOutputRuntimeConfig(cfg)
	if err != nil {
		normalized = workorchconfig.DefaultLLMOutputRuntimeConfig()
	}
	return RuntimeLLMOutputConfiguration{
		MaxPatchContentBytes:               normalized.MaxPatchContentBytes,
		MaxGraphBytes:                      normalized.MaxGraphBytes,
		MaxGraphFileWrites:                 normalized.MaxGraphFileWrites,
		SchemaInvalidLargePreviewThreshold: normalized.SchemaInvalidLargePreviewThreshold,
		MaxArtifactChunkBytes:              normalized.MaxArtifactChunkBytes,
		PreviousChunkTailBytes:             normalized.PreviousChunkTailBytes,
		MaxChunkRetryAttempts:              normalized.MaxChunkRetryAttempts,
		MaxArtifactPlanAttempts:            normalized.MaxArtifactPlanAttempts,
		MaxArtifactReplanAttempts:          normalized.MaxArtifactReplanAttempts,
	}
}
