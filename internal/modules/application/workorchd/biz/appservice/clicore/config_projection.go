package clicore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"

func ServiceConfigResourceMap(cfg configcore.ServiceConfiguration) map[string]any {
	providers := make([]map[string]any, 0, len(cfg.LLM.Providers))
	for _, item := range cfg.LLM.Providers {
		providers = append(providers, map[string]any{
			"ID":        item.ID,
			"Provider":  item.Provider,
			"BaseURL":   item.BaseURL,
			"Model":     item.Model,
			"Timeout":   item.Timeout,
			"Disabled":  item.Disabled,
			"Streaming": item.Streaming,
		})
	}
	return map[string]any{
		"ServerAddr":        cfg.Server.Addr,
		"Backend":           cfg.Repository.Backend,
		"DataDir":           cfg.Repository.DataDir,
		"DefaultProvider":   cfg.LLM.DefaultProviderID,
		"MaxConcurrentRuns": cfg.Server.MaxConcurrentRuns,
		"Providers":         providers,
		"LLMOutput": map[string]any{
			"MaxPatchContentBytes":               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			"MaxGraphBytes":                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			"MaxGraphFileWrites":                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			"SchemaInvalidLargePreviewThreshold": cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			"MaxArtifactChunkBytes":              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			"PreviousChunkTailBytes":             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			"MaxChunkRetryAttempts":              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			"MaxArtifactPlanAttempts":            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			"MaxArtifactReplanAttempts":          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}
}
