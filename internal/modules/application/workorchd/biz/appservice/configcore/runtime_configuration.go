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
		DefaultTimeout: strings.TrimSpace(cfg.DefaultTimeout),
	}
}

func runtimeLLMOutputConfiguration(cfg workorchconfig.LLMOutputRuntimeConfig) workorchconfig.LLMOutputRuntimeConfig {
	normalized, err := workorchconfig.NormalizeLLMOutputRuntimeConfig(cfg)
	if err != nil {
		normalized = workorchconfig.DefaultLLMOutputRuntimeConfig()
	}
	return normalized
}
