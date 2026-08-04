package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

type ServiceConfiguration struct {
	Server     ServerConfiguration             `json:"server"`
	Repository workorchconfig.RepositoryConfig `json:"repository"`
	Runtime    RuntimeConfiguration            `json:"runtime"`
	LLM        LLMConfiguration                `json:"llm"`
	WebSearch  WebSearchConfiguration          `json:"webSearch"`
	Governance GovernanceConfiguration         `json:"governance"`
}

type ServerConfiguration struct {
	Addr                      string `json:"addr"`
	ReadHeaderTimeout         string `json:"readHeaderTimeout"`
	ReadTimeout               string `json:"readTimeout"`
	WriteTimeout              string `json:"writeTimeout"`
	IdleTimeout               string `json:"idleTimeout"`
	ShutdownTimeout           string `json:"shutdownTimeout"`
	MaxConcurrentRuns         int64  `json:"maxConcurrentRuns"`
	StaleDispatchedStepMinAge string `json:"staleDispatchedStepMinAge"`
	DefaultDialogLanguage     string `json:"defaultDialogLanguage,omitempty"`
}

type UpdateServerConfigurationCommand struct {
	DefaultDialogLanguage *string `json:"defaultDialogLanguage,omitempty"`
}
