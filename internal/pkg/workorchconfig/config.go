package workorchconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/muidea/magicCommon/framework/configuration"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/pelletier/go-toml/v2"
)

const EnvConfigDir = "CONFIG_PATH"

var configRevision atomic.Uint64

type Config struct {
	EndpointName string           `json:"endpointName" toml:"endpointName"`
	Server       ServerConfig     `json:"server" toml:"server"`
	Repository   RepositoryConfig `json:"repository" toml:"repository"`
	Runtime      RuntimeConfig    `json:"runtime" toml:"runtime"`
	LLM          LLMConfig        `json:"llm" toml:"llm"`
	WebSearch    WebSearchConfig  `json:"webSearch" toml:"webSearch"`
	Governance   GovernanceConfig `json:"governance" toml:"governance"`
}

type ServerConfig struct {
	Addr                      string `json:"addr" toml:"addr"`
	ReadHeaderTimeout         string `json:"readHeaderTimeout" toml:"readHeaderTimeout"`
	ReadTimeout               string `json:"readTimeout" toml:"readTimeout"`
	WriteTimeout              string `json:"writeTimeout" toml:"writeTimeout"`
	IdleTimeout               string `json:"idleTimeout" toml:"idleTimeout"`
	ShutdownTimeout           string `json:"shutdownTimeout" toml:"shutdownTimeout"`
	MaxConcurrentRuns         int64  `json:"maxConcurrentRuns" toml:"maxConcurrentRuns"`
	StaleDispatchedStepMinAge string `json:"staleDispatchedStepMinAge" toml:"staleDispatchedStepMinAge"`
	DefaultDialogLanguage     string `json:"defaultDialogLanguage" toml:"defaultDialogLanguage"`
}

type RepositoryConfig struct {
	Backend string `json:"backend" toml:"backend"`
	DataDir string `json:"dataDir" toml:"dataDir"`
}

type RuntimeConfig struct {
	DefaultSoulID                                string                 `json:"defaultSoulId" toml:"defaultSoulId"`
	MaxRuntimeRepairAttempts                     int                    `json:"maxRuntimeRepairAttempts" toml:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts                     int                    `json:"maxProviderRetryAttempts" toml:"maxProviderRetryAttempts"`
	MaxContractViolationRetriesPerStep           int                    `json:"maxContractViolationRetriesPerStep" toml:"maxContractViolationRetriesPerStep"`
	MaxContractViolationRetriesPerRun            int                    `json:"maxContractViolationRetriesPerRun" toml:"maxContractViolationRetriesPerRun"`
	MaxRepeatedContractViolationSignature        int                    `json:"maxRepeatedContractViolationSignature" toml:"maxRepeatedContractViolationSignature"`
	MaxConsecutiveContractViolationNoShapeChange int                    `json:"maxConsecutiveContractViolationNoShapeChange" toml:"maxConsecutiveContractViolationNoShapeChange"`
	ShellCommandTimeout                          string                 `json:"shellCommandTimeout" toml:"shellCommandTimeout"`
	Git                                          GitRuntimeConfig       `json:"git" toml:"git"`
	LLMOutput                                    LLMOutputRuntimeConfig `json:"llmOutput" toml:"llmOutput"`
}

type GitRuntimeConfig struct {
	Enabled        *bool  `json:"enabled" toml:"enabled"`
	WriteEnabled   *bool  `json:"writeEnabled" toml:"writeEnabled"`
	DefaultTimeout string `json:"defaultTimeout" toml:"defaultTimeout"`
}

type LLMOutputRuntimeConfig struct {
	SemanticPlanMaxOutputTokens        int `json:"semanticPlanMaxOutputTokens" toml:"semanticPlanMaxOutputTokens"`
	CandidateMaxOutputTokens           int `json:"candidateMaxOutputTokens" toml:"candidateMaxOutputTokens"`
	MaxPatchContentBytes               int `json:"maxPatchContentBytes" toml:"maxPatchContentBytes"`
	MaxGraphBytes                      int `json:"maxGraphBytes" toml:"maxGraphBytes"`
	MaxGraphFileWrites                 int `json:"maxGraphFileWrites" toml:"maxGraphFileWrites"`
	SchemaInvalidLargePreviewThreshold int `json:"schemaInvalidLargePreviewThreshold" toml:"schemaInvalidLargePreviewThreshold"`
	MaxArtifactChunkBytes              int `json:"maxArtifactChunkBytes" toml:"maxArtifactChunkBytes"`
	PreviousChunkTailBytes             int `json:"previousChunkTailBytes" toml:"previousChunkTailBytes"`
	MaxChunkRetryAttempts              int `json:"maxChunkRetryAttempts" toml:"maxChunkRetryAttempts"`
	MaxArtifactPlanAttempts            int `json:"maxArtifactPlanAttempts" toml:"maxArtifactPlanAttempts"`
	MaxArtifactReplanAttempts          int `json:"maxArtifactReplanAttempts" toml:"maxArtifactReplanAttempts"`
}

// BuiltinAIProxyProviderID is the reserved WorkOrch provider identity for the
// built-in ai-proxy endpoint.
const BuiltinAIProxyProviderID = runtimecontract.BuiltinAIProxyProviderID

// AI-proxy request timeout is bounded so a provider call cannot outlive the
// runtime recovery budget. Keep the default and ceiling in the config owner so
// CLI, HTTP and startup validation share one contract.
const (
	DefaultAIProxyTimeout = "180s"
	MaxAIProxyTimeout     = 10 * time.Minute
)

// Known LLM operations for custom provider model declarations.
const (
	LLMOperationChatCompletions = "chat_completions"
	LLMOperationResponses       = "responses"
	LLMOperationEmbeddings      = "embeddings"
)

// LLMConfig is the canonical ai-proxy-first configuration contract.
// Old defaultProvider/providers/modelCapabilities fields are intentionally removed
// and rejected by UnmarshalJSON during startup config loading.
type LLMConfig struct {
	AIProxy          AIProxyProviderConfig              `json:"aiProxy" toml:"aiProxy"`
	DefaultChat      ProviderModelTargetConfig          `json:"defaultChat" toml:"defaultChat"`
	DefaultEmbedding *ProviderModelTargetConfig         `json:"defaultEmbedding,omitempty" toml:"defaultEmbedding,omitempty"`
	Budget           LLMContextBudgetPolicyConfig       `json:"budget" toml:"budget"`
	CustomProviders  map[string]CustomLLMProviderConfig `json:"customProviders,omitempty" toml:"customProviders,omitempty"`
}

type AIProxyProviderConfig struct {
	BaseURL                 string                                `json:"baseURL" toml:"baseURL"`
	APIKey                  string                                `json:"apiKey" toml:"apiKey"`
	Timeout                 string                                `json:"timeout" toml:"timeout"`
	CatalogRequestTimeout   string                                `json:"catalogRequestTimeout" toml:"catalogRequestTimeout"`
	CatalogTTL              string                                `json:"catalogTTL" toml:"catalogTTL"`
	CatalogMaxStaleAge      string                                `json:"catalogMaxStaleAge" toml:"catalogMaxStaleAge"`
	MaxCatalogResponseBytes int                                   `json:"maxCatalogResponseBytes" toml:"maxCatalogResponseBytes"`
	MaxCatalogModels        int                                   `json:"maxCatalogModels" toml:"maxCatalogModels"`
	ModelOverrides          map[string]AIProxyModelOverrideConfig `json:"modelOverrides,omitempty" toml:"modelOverrides,omitempty"`
}

// ProviderModelTargetConfig is the persisted configuration representation of
// the canonical provider/model value. The named type preserves the config
// boundary without repeating its field contract.
type ProviderModelTargetConfig runtimecontract.ProviderModelTarget

type LLMContextBudgetPolicyConfig struct {
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio" toml:"contextUtilizationRatio"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte" toml:"estimatedTokensPerByte"`
}

type AIProxyModelOverrideConfig struct {
	ContextWindowTokens int      `json:"contextWindowTokens" toml:"contextWindowTokens"`
	MaxOutputTokens     int      `json:"maxOutputTokens" toml:"maxOutputTokens"`
	Operations          []string `json:"operations" toml:"operations"`
}

type CustomLLMProviderConfig struct {
	Provider   string                          `json:"provider" toml:"provider"`
	APISurface string                          `json:"apiSurface" toml:"apiSurface"`
	BaseURL    string                          `json:"baseURL" toml:"baseURL"`
	APIKey     string                          `json:"apiKey" toml:"apiKey"`
	Timeout    string                          `json:"timeout" toml:"timeout"`
	Models     map[string]CustomLLMModelConfig `json:"models" toml:"models"`
}

type CustomLLMModelConfig struct {
	ContextWindowTokens  int                 `json:"contextWindowTokens" toml:"contextWindowTokens"`
	MaxOutputTokens      int                 `json:"maxOutputTokens" toml:"maxOutputTokens"`
	Operations           []string            `json:"operations" toml:"operations"`
	Temperature          *float64            `json:"temperature,omitempty" toml:"temperature,omitempty"`
	ReasoningEffort      string              `json:"reasoningEffort,omitempty" toml:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int                 `json:"thinkingBudgetTokens,omitempty" toml:"thinkingBudgetTokens,omitempty"`
	Cache                ProviderCacheConfig `json:"cache,omitempty" toml:"cache,omitempty"`
}

type ProviderCacheConfig struct {
	Mode                string   `json:"mode,omitempty" toml:"mode,omitempty"`
	PromptCacheKey      *bool    `json:"promptCacheKey,omitempty" toml:"promptCacheKey,omitempty"`
	MessageCacheControl *bool    `json:"messageCacheControl,omitempty" toml:"messageCacheControl,omitempty"`
	UsageFields         []string `json:"usageFields,omitempty" toml:"usageFields,omitempty"`
}

// UnmarshalJSON rejects unknown LLM fields so old TOML/JSON contracts fail fast.
func (c *LLMConfig) UnmarshalJSON(data []byte) error {
	type llmConfigAlias LLMConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var alias llmConfigAlias
	if err := decoder.Decode(&alias); err != nil {
		return fmt.Errorf("%w: llm configuration rejects legacy or unknown fields: %v", common.ErrInvalidArgument, err)
	}
	*c = LLMConfig(alias)
	return nil
}

func (c ProviderModelTargetConfig) Normalized() ProviderModelTargetConfig {
	return ProviderModelTargetConfig(runtimecontract.ProviderModelTarget(c).Normalized())
}

func (c ProviderModelTargetConfig) Validate() error {
	c = c.Normalized()
	if c.ProviderID == "" {
		return fmt.Errorf("%w: llm target requires providerID", common.ErrInvalidArgument)
	}
	if c.ModelID == "" {
		return fmt.Errorf("%w: llm target requires modelID", common.ErrInvalidArgument)
	}
	return nil
}

func (c ProviderModelTargetConfig) ToRuntimeTarget() runtimecontract.ProviderModelTarget {
	return runtimecontract.ProviderModelTarget(c).Normalized()
}

func DefaultAIProxyProviderConfig() AIProxyProviderConfig {
	return AIProxyProviderConfig{
		Timeout:                 DefaultAIProxyTimeout,
		CatalogRequestTimeout:   "5s",
		CatalogTTL:              "5m",
		CatalogMaxStaleAge:      "24h",
		MaxCatalogResponseBytes: 1 << 20,
		MaxCatalogModels:        1000,
		ModelOverrides:          map[string]AIProxyModelOverrideConfig{},
	}
}

func DefaultLLMContextBudgetPolicyConfig() LLMContextBudgetPolicyConfig {
	return LLMContextBudgetPolicyConfig{
		ContextUtilizationRatio: 0.75,
		EstimatedTokensPerByte:  0.25,
	}
}

func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		AIProxy:         DefaultAIProxyProviderConfig(),
		Budget:          DefaultLLMContextBudgetPolicyConfig(),
		CustomProviders: map[string]CustomLLMProviderConfig{},
	}
}

type WebSearchConfig struct {
	DefaultProvider string                             `json:"defaultProvider" toml:"defaultProvider"`
	Providers       map[string]WebSearchProviderConfig `json:"providers" toml:"providers"`
}

type WebSearchProviderConfig struct {
	Provider       string            `json:"provider" toml:"provider"`
	SearchEndpoint string            `json:"searchEndpoint" toml:"searchEndpoint"`
	FetchEndpoint  string            `json:"fetchEndpoint" toml:"fetchEndpoint"`
	APIKey         string            `json:"apiKey" toml:"apiKey"`
	Headers        map[string]string `json:"headers" toml:"headers"`
	Timeout        string            `json:"timeout" toml:"timeout"`
	MaxResults     int               `json:"maxResults" toml:"maxResults"`
	MaxFetchBytes  int64             `json:"maxFetchBytes" toml:"maxFetchBytes"`
	MaxRedirects   int               `json:"maxRedirects" toml:"maxRedirects"`
	AllowDomains   []string          `json:"allowDomains" toml:"allowDomains"`
	DenyDomains    []string          `json:"denyDomains" toml:"denyDomains"`
	Disabled       *bool             `json:"disabled" toml:"disabled"`
}

type GovernanceConfig struct {
	AutoTicket   GovernanceAutoTicketConfig   `json:"autoTicket" toml:"autoTicket"`
	AutoApproval GovernanceAutoApprovalConfig `json:"autoApproval" toml:"autoApproval"`
	Budget       GovernanceBudgetConfig       `json:"budget" toml:"budget"`
}

type GovernanceAutoTicketConfig struct {
	Enabled       *bool   `json:"enabled" toml:"enabled"`
	MinConfidence float64 `json:"minConfidence" toml:"minConfidence"`
	MaxPerRun     int     `json:"maxPerRun" toml:"maxPerRun"`
}

type GovernanceAutoApprovalConfig struct {
	Enabled             *bool    `json:"enabled" toml:"enabled"`
	MinConfidence       float64  `json:"minConfidence" toml:"minConfidence"`
	MaxPerRun           int      `json:"maxPerRun" toml:"maxPerRun"`
	AllowReadOnly       *bool    `json:"allowReadOnly" toml:"allowReadOnly"`
	AllowWorkspaceWrite *bool    `json:"allowWorkspaceWrite" toml:"allowWorkspaceWrite"`
	RequireHumanFor     []string `json:"requireHumanFor" toml:"requireHumanFor"`
}

type GovernanceBudgetConfig struct {
	MaxConsecutiveFailures int `json:"maxConsecutiveFailures" toml:"maxConsecutiveFailures"`
	MaxRepeatedSamePrompt  int `json:"maxRepeatedSamePrompt" toml:"maxRepeatedSamePrompt"`
}

func ConfigureDirectory(configDir string) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		return
	}
	_ = os.Setenv(EnvConfigDir, configDir)
	configRevision.Add(1)
}

func Current() (Config, error) {
	if !configuration.IsConfigManagerInitialized() {
		if err := configuration.InitDefaultConfigManager(""); err != nil {
			return Config{}, err
		}
	}
	var cfg Config
	if endpointName, err := configuration.GetString("endpointName"); err == nil {
		cfg.EndpointName = endpointName
	} else if !isMissingSectionError(err) {
		return Config{}, err
	}
	if err := loadSection("server", &cfg.Server); err != nil {
		return Config{}, err
	}
	if err := loadSection("repository", &cfg.Repository); err != nil {
		return Config{}, err
	}
	if err := loadSection("runtime", &cfg.Runtime); err != nil {
		return Config{}, err
	}
	if err := loadSection("llm", &cfg.LLM); err != nil {
		return Config{}, err
	}
	if err := loadSection("webSearch", &cfg.WebSearch); err != nil {
		return Config{}, err
	}
	if err := loadSection("governance", &cfg.Governance); err != nil {
		return Config{}, err
	}
	if err := normalizeConfig(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ApplicationConfigPath() (string, error) {
	configDir := strings.TrimSpace(os.Getenv(EnvConfigDir))
	if configDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(wd, "config")
	}
	return filepath.Join(configDir, "application.toml"), nil
}

func Save(next Config) error {
	if strings.TrimSpace(next.EndpointName) == "" {
		next.EndpointName = "workorch"
	}
	if err := normalizeConfig(&next); err != nil {
		return err
	}
	if next.LLM.CustomProviders == nil {
		next.LLM.CustomProviders = map[string]CustomLLMProviderConfig{}
	}
	if next.LLM.AIProxy.ModelOverrides == nil {
		next.LLM.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{}
	}
	if next.WebSearch.Providers == nil {
		next.WebSearch.Providers = map[string]WebSearchProviderConfig{}
	}
	if next.Governance.AutoApproval.RequireHumanFor == nil {
		next.Governance.AutoApproval.RequireHumanFor = []string{}
	}
	configPath, err := ApplicationConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	payload, err := toml.Marshal(next)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, payload, 0o600)
}

func DefaultLLMOutputRuntimeConfig() LLMOutputRuntimeConfig {
	return LLMOutputRuntimeConfig{
		SemanticPlanMaxOutputTokens:        8192,
		CandidateMaxOutputTokens:           8192,
		MaxPatchContentBytes:               65536,
		MaxGraphBytes:                      131072,
		MaxGraphFileWrites:                 3,
		SchemaInvalidLargePreviewThreshold: 32768,
		MaxArtifactChunkBytes:              16384,
		PreviousChunkTailBytes:             300,
		MaxChunkRetryAttempts:              3,
		MaxArtifactPlanAttempts:            2,
		MaxArtifactReplanAttempts:          2,
	}
}

func NormalizeLLMOutputRuntimeConfig(cfg LLMOutputRuntimeConfig) (LLMOutputRuntimeConfig, error) {
	defaults := DefaultLLMOutputRuntimeConfig()
	applyDefault := func(value *int, fallback int) {
		if value != nil && *value == 0 {
			*value = fallback
		}
	}
	applyDefault(&cfg.SemanticPlanMaxOutputTokens, defaults.SemanticPlanMaxOutputTokens)
	applyDefault(&cfg.CandidateMaxOutputTokens, defaults.CandidateMaxOutputTokens)
	applyDefault(&cfg.MaxPatchContentBytes, defaults.MaxPatchContentBytes)
	applyDefault(&cfg.MaxGraphBytes, defaults.MaxGraphBytes)
	applyDefault(&cfg.MaxGraphFileWrites, defaults.MaxGraphFileWrites)
	applyDefault(&cfg.SchemaInvalidLargePreviewThreshold, defaults.SchemaInvalidLargePreviewThreshold)
	applyDefault(&cfg.MaxArtifactChunkBytes, defaults.MaxArtifactChunkBytes)
	applyDefault(&cfg.PreviousChunkTailBytes, defaults.PreviousChunkTailBytes)
	applyDefault(&cfg.MaxChunkRetryAttempts, defaults.MaxChunkRetryAttempts)
	applyDefault(&cfg.MaxArtifactPlanAttempts, defaults.MaxArtifactPlanAttempts)
	applyDefault(&cfg.MaxArtifactReplanAttempts, defaults.MaxArtifactReplanAttempts)
	if err := validateLLMOutputRuntimeConfig(cfg); err != nil {
		return LLMOutputRuntimeConfig{}, err
	}
	return cfg, nil
}

func LLMOutputRuntimeConfigValues(cfg LLMOutputRuntimeConfig) map[string]any {
	normalized, err := NormalizeLLMOutputRuntimeConfig(cfg)
	if err != nil {
		return nil
	}
	return map[string]any{
		"semanticPlanMaxOutputTokens":        normalized.SemanticPlanMaxOutputTokens,
		"candidateMaxOutputTokens":           normalized.CandidateMaxOutputTokens,
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

func normalizeConfig(cfg *Config) error {
	if cfg == nil {
		return common.ErrInvalidArgument
	}
	normalized, err := NormalizeLLMOutputRuntimeConfig(cfg.Runtime.LLMOutput)
	if err != nil {
		return err
	}
	applyRuntimeConfigDefaults(&cfg.Runtime)
	cfg.Runtime.LLMOutput = normalized
	llm, err := NormalizeLLMConfig(cfg.LLM)
	if err != nil {
		return err
	}
	cfg.LLM = llm
	return nil
}

func NormalizeLLMConfig(cfg LLMConfig) (LLMConfig, error) {
	defaults := DefaultLLMConfig()
	if strings.TrimSpace(cfg.AIProxy.Timeout) == "" {
		cfg.AIProxy.Timeout = defaults.AIProxy.Timeout
	}
	if strings.TrimSpace(cfg.AIProxy.CatalogRequestTimeout) == "" {
		cfg.AIProxy.CatalogRequestTimeout = defaults.AIProxy.CatalogRequestTimeout
	}
	if strings.TrimSpace(cfg.AIProxy.CatalogTTL) == "" {
		cfg.AIProxy.CatalogTTL = defaults.AIProxy.CatalogTTL
	}
	if strings.TrimSpace(cfg.AIProxy.CatalogMaxStaleAge) == "" {
		cfg.AIProxy.CatalogMaxStaleAge = defaults.AIProxy.CatalogMaxStaleAge
	}
	if cfg.AIProxy.MaxCatalogResponseBytes <= 0 {
		cfg.AIProxy.MaxCatalogResponseBytes = defaults.AIProxy.MaxCatalogResponseBytes
	}
	if cfg.AIProxy.MaxCatalogModels <= 0 {
		cfg.AIProxy.MaxCatalogModels = defaults.AIProxy.MaxCatalogModels
	}
	if cfg.AIProxy.ModelOverrides == nil {
		cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{}
	}
	if cfg.Budget.ContextUtilizationRatio <= 0 {
		cfg.Budget.ContextUtilizationRatio = defaults.Budget.ContextUtilizationRatio
	}
	if cfg.Budget.EstimatedTokensPerByte <= 0 {
		cfg.Budget.EstimatedTokensPerByte = defaults.Budget.EstimatedTokensPerByte
	}
	if cfg.CustomProviders == nil {
		cfg.CustomProviders = map[string]CustomLLMProviderConfig{}
	}
	// Canonicalize map keys (trim) so readiness/revision/client lookup share one key space.
	// Model IDs are exact provider protocol values; only trim collisions are invalid.
	var err error
	cfg.AIProxy.ModelOverrides, err = canonicalizeAIProxyModelOverrides(cfg.AIProxy.ModelOverrides)
	if err != nil {
		return LLMConfig{}, err
	}
	cfg.CustomProviders, err = canonicalizeCustomProviders(cfg.CustomProviders)
	if err != nil {
		return LLMConfig{}, err
	}
	cfg.DefaultChat = cfg.DefaultChat.Normalized()
	if cfg.DefaultEmbedding != nil {
		normalized := cfg.DefaultEmbedding.Normalized()
		if normalized.ProviderID == "" && normalized.ModelID == "" {
			cfg.DefaultEmbedding = nil
		} else {
			cfg.DefaultEmbedding = &normalized
		}
	}
	if err := ValidateLLMConfig(cfg); err != nil {
		return LLMConfig{}, err
	}
	return cfg, nil
}

func canonicalizeAIProxyModelOverrides(source map[string]AIProxyModelOverrideConfig) (map[string]AIProxyModelOverrideConfig, error) {
	if len(source) == 0 {
		return map[string]AIProxyModelOverrideConfig{}, nil
	}
	ret := make(map[string]AIProxyModelOverrideConfig, len(source))
	for modelID, override := range source {
		key := strings.TrimSpace(modelID)
		if key == "" {
			return nil, fmt.Errorf("%w: llm.aiProxy.modelOverrides requires non-empty model id", common.ErrInvalidArgument)
		}
		if _, exists := ret[key]; exists {
			return nil, fmt.Errorf("%w: llm.aiProxy.modelOverrides has duplicate model id %q after trim", common.ErrInvalidArgument, key)
		}
		ops := make([]string, 0, len(override.Operations))
		for _, op := range override.Operations {
			op = strings.TrimSpace(op)
			if op != "" {
				ops = append(ops, op)
			}
		}
		override.Operations = ops
		ret[key] = override
	}
	return ret, nil
}

func canonicalizeCustomProviders(source map[string]CustomLLMProviderConfig) (map[string]CustomLLMProviderConfig, error) {
	if len(source) == 0 {
		return map[string]CustomLLMProviderConfig{}, nil
	}
	ret := make(map[string]CustomLLMProviderConfig, len(source))
	// fold -> first original key (after trim) that claimed this fold.
	seenProviders := map[string]string{}
	for providerID, provider := range source {
		key := strings.TrimSpace(providerID)
		if key == "" {
			return nil, fmt.Errorf("%w: llm.customProviders requires non-empty provider id", common.ErrInvalidArgument)
		}
		fold := strings.ToLower(key)
		if prev, ok := seenProviders[fold]; ok {
			return nil, fmt.Errorf("%w: llm.customProviders has duplicate provider id %q (conflicts with %q after trim/case-fold)", common.ErrInvalidArgument, key, prev)
		}
		if _, exists := ret[key]; exists {
			return nil, fmt.Errorf("%w: llm.customProviders has duplicate provider id %q after trim", common.ErrInvalidArgument, key)
		}
		seenProviders[fold] = key
		provider.Provider = strings.TrimSpace(provider.Provider)
		provider.APISurface = strings.TrimSpace(provider.APISurface)
		provider.BaseURL = strings.TrimSpace(provider.BaseURL)
		provider.Timeout = strings.TrimSpace(provider.Timeout)
		models, err := canonicalizeCustomProviderModels(key, provider.Models)
		if err != nil {
			return nil, err
		}
		provider.Models = models
		ret[key] = provider
	}
	return ret, nil
}

func canonicalizeCustomProviderModels(providerID string, source map[string]CustomLLMModelConfig) (map[string]CustomLLMModelConfig, error) {
	if len(source) == 0 {
		return map[string]CustomLLMModelConfig{}, nil
	}
	ret := make(map[string]CustomLLMModelConfig, len(source))
	for modelID, model := range source {
		mKey := strings.TrimSpace(modelID)
		if mKey == "" {
			return nil, fmt.Errorf("%w: llm.customProviders[%s].models requires non-empty model id", common.ErrInvalidArgument, providerID)
		}
		if _, exists := ret[mKey]; exists {
			return nil, fmt.Errorf("%w: llm.customProviders[%s].models has duplicate model id %q after trim", common.ErrInvalidArgument, providerID, mKey)
		}
		ops := make([]string, 0, len(model.Operations))
		for _, op := range model.Operations {
			op = strings.TrimSpace(op)
			if op != "" {
				ops = append(ops, op)
			}
		}
		model.Operations = ops
		ret[mKey] = model
	}
	return ret, nil
}

func ValidateLLMConfig(cfg LLMConfig) error {
	if err := validateAIProxyProviderConfig(cfg.AIProxy); err != nil {
		return err
	}
	if err := cfg.DefaultChat.Validate(); err != nil {
		return fmt.Errorf("%w: llm.defaultChat: %v", common.ErrInvalidArgument, err)
	}
	if cfg.DefaultEmbedding != nil {
		if err := cfg.DefaultEmbedding.Validate(); err != nil {
			return fmt.Errorf("%w: llm.defaultEmbedding: %v", common.ErrInvalidArgument, err)
		}
	}
	if err := validateLLMContextBudgetPolicyConfig(cfg.Budget); err != nil {
		return err
	}
	seenProviders := map[string]string{}
	for id, provider := range cfg.CustomProviders {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return fmt.Errorf("%w: llm.customProviders requires non-empty provider id", common.ErrInvalidArgument)
		}
		fold := strings.ToLower(trimmed)
		if prev, ok := seenProviders[fold]; ok {
			return fmt.Errorf("%w: llm.customProviders has duplicate provider id %q (conflicts with %q after trim/case-fold)", common.ErrInvalidArgument, trimmed, prev)
		}
		seenProviders[fold] = trimmed
		if strings.EqualFold(trimmed, BuiltinAIProxyProviderID) {
			return fmt.Errorf("%w: llm.customProviders id %q is reserved for built-in ai-proxy", common.ErrInvalidArgument, trimmed)
		}
		if err := validateCustomLLMProviderConfig(trimmed, provider); err != nil {
			return err
		}
		// Model IDs are case-sensitive protocol values; reject only trim collisions.
		seenModels := map[string]string{}
		for modelID := range provider.Models {
			modelTrim := strings.TrimSpace(modelID)
			if modelTrim == "" {
				return fmt.Errorf("%w: llm.customProviders[%s].models requires non-empty model id", common.ErrInvalidArgument, trimmed)
			}
			if prev, ok := seenModels[modelTrim]; ok {
				return fmt.Errorf("%w: llm.customProviders[%s].models has duplicate model id %q (conflicts with %q)", common.ErrInvalidArgument, trimmed, modelTrim, prev)
			}
			seenModels[modelTrim] = modelTrim
		}
	}
	return nil
}

func validateAIProxyProviderConfig(cfg AIProxyProviderConfig) error {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("%w: llm.aiProxy.baseURL is required", common.ErrInvalidArgument)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("%w: llm.aiProxy.baseURL is invalid: %v", common.ErrInvalidArgument, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: llm.aiProxy.baseURL must use http or https", common.ErrInvalidArgument)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: llm.aiProxy.baseURL requires host", common.ErrInvalidArgument)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: llm.aiProxy.baseURL must not include userinfo", common.ErrInvalidArgument)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: llm.aiProxy.baseURL must not include query or fragment", common.ErrInvalidArgument)
	}
	timeout, err := parsePositiveDuration("llm.aiProxy.timeout", cfg.Timeout)
	if err != nil {
		return err
	}
	if timeout > MaxAIProxyTimeout {
		return fmt.Errorf("%w: llm.aiProxy.timeout %q must not exceed %s", common.ErrInvalidArgument, strings.TrimSpace(cfg.Timeout), MaxAIProxyTimeout)
	}
	catalogTimeout, err := parsePositiveDuration("llm.aiProxy.catalogRequestTimeout", cfg.CatalogRequestTimeout)
	if err != nil {
		return err
	}
	catalogTTL, err := parsePositiveDuration("llm.aiProxy.catalogTTL", cfg.CatalogTTL)
	if err != nil {
		return err
	}
	catalogMaxStaleAge, err := parsePositiveDuration("llm.aiProxy.catalogMaxStaleAge", cfg.CatalogMaxStaleAge)
	if err != nil {
		return err
	}
	if catalogMaxStaleAge < catalogTTL {
		return fmt.Errorf("%w: llm.aiProxy.catalogMaxStaleAge must be >= catalogTTL", common.ErrInvalidArgument)
	}
	if catalogTimeout > timeout {
		// catalog timeout is independent; allow shorter only, no hard relation required beyond positive.
	}
	if cfg.MaxCatalogResponseBytes <= 0 {
		return fmt.Errorf("%w: llm.aiProxy.maxCatalogResponseBytes must be positive", common.ErrInvalidArgument)
	}
	if cfg.MaxCatalogModels <= 0 {
		return fmt.Errorf("%w: llm.aiProxy.maxCatalogModels must be positive", common.ErrInvalidArgument)
	}
	seenOverrides := map[string]string{}
	for modelID, override := range cfg.ModelOverrides {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return fmt.Errorf("%w: llm.aiProxy.modelOverrides requires non-empty model id", common.ErrInvalidArgument)
		}
		if prev, ok := seenOverrides[modelID]; ok {
			return fmt.Errorf("%w: llm.aiProxy.modelOverrides has duplicate model id %q (conflicts with %q)", common.ErrInvalidArgument, modelID, prev)
		}
		seenOverrides[modelID] = modelID
		if override.ContextWindowTokens <= 0 || override.MaxOutputTokens <= 0 {
			return fmt.Errorf("%w: llm.aiProxy.modelOverrides[%s] requires positive contextWindowTokens and maxOutputTokens", common.ErrInvalidArgument, modelID)
		}
		if override.MaxOutputTokens >= override.ContextWindowTokens {
			return fmt.Errorf("%w: llm.aiProxy.modelOverrides[%s] maxOutputTokens must be less than contextWindowTokens", common.ErrInvalidArgument, modelID)
		}
		if len(override.Operations) == 0 {
			return fmt.Errorf("%w: llm.aiProxy.modelOverrides[%s].operations is required", common.ErrInvalidArgument, modelID)
		}
		seenOps := map[string]struct{}{}
		for _, operation := range override.Operations {
			operation = strings.TrimSpace(operation)
			switch operation {
			case LLMOperationChatCompletions, LLMOperationResponses, LLMOperationEmbeddings:
			default:
				return fmt.Errorf("%w: llm.aiProxy.modelOverrides[%s].operations has unknown operation %q", common.ErrInvalidArgument, modelID, operation)
			}
			if _, ok := seenOps[operation]; ok {
				return fmt.Errorf("%w: llm.aiProxy.modelOverrides[%s].operations has duplicate operation %q", common.ErrInvalidArgument, modelID, operation)
			}
			seenOps[operation] = struct{}{}
		}
	}
	return nil
}

func validateLLMContextBudgetPolicyConfig(cfg LLMContextBudgetPolicyConfig) error {
	if cfg.ContextUtilizationRatio <= 0 || cfg.ContextUtilizationRatio > 1 {
		return fmt.Errorf("%w: llm.budget.contextUtilizationRatio must be in (0, 1]", common.ErrInvalidArgument)
	}
	if cfg.EstimatedTokensPerByte <= 0 {
		return fmt.Errorf("%w: llm.budget.estimatedTokensPerByte must be positive", common.ErrInvalidArgument)
	}
	return nil
}

func validateCustomLLMProviderConfig(id string, cfg CustomLLMProviderConfig) error {
	if strings.TrimSpace(cfg.Provider) == "" {
		return fmt.Errorf("%w: llm.customProviders[%s].provider is required", common.ErrInvalidArgument, id)
	}
	if strings.TrimSpace(cfg.APISurface) == "" {
		return fmt.Errorf("%w: llm.customProviders[%s].apiSurface is required", common.ErrInvalidArgument, id)
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("%w: llm.customProviders[%s].baseURL is required", common.ErrInvalidArgument, id)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("%w: llm.customProviders[%s].baseURL is invalid: %v", common.ErrInvalidArgument, id, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: llm.customProviders[%s].baseURL must use http or https", common.ErrInvalidArgument, id)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: llm.customProviders[%s].baseURL requires host", common.ErrInvalidArgument, id)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: llm.customProviders[%s].baseURL must not include userinfo, query or fragment", common.ErrInvalidArgument, id)
	}
	if strings.TrimSpace(cfg.Timeout) != "" {
		if _, err := parsePositiveDuration(fmt.Sprintf("llm.customProviders[%s].timeout", id), cfg.Timeout); err != nil {
			return err
		}
	}
	if len(cfg.Models) == 0 {
		return fmt.Errorf("%w: llm.customProviders[%s].models must declare at least one model", common.ErrInvalidArgument, id)
	}
	for modelID, model := range cfg.Models {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return fmt.Errorf("%w: llm.customProviders[%s].models requires non-empty model id", common.ErrInvalidArgument, id)
		}
		if model.ContextWindowTokens <= 0 || model.MaxOutputTokens <= 0 {
			return fmt.Errorf("%w: llm.customProviders[%s].models[%s] requires positive contextWindowTokens and maxOutputTokens", common.ErrInvalidArgument, id, modelID)
		}
		if model.MaxOutputTokens >= model.ContextWindowTokens {
			return fmt.Errorf("%w: llm.customProviders[%s].models[%s] maxOutputTokens must be less than contextWindowTokens", common.ErrInvalidArgument, id, modelID)
		}
		if len(model.Operations) == 0 {
			return fmt.Errorf("%w: llm.customProviders[%s].models[%s].operations is required", common.ErrInvalidArgument, id, modelID)
		}
		seenOps := map[string]struct{}{}
		for _, operation := range model.Operations {
			operation = strings.TrimSpace(operation)
			switch operation {
			case LLMOperationChatCompletions, LLMOperationResponses, LLMOperationEmbeddings:
			default:
				return fmt.Errorf("%w: llm.customProviders[%s].models[%s].operations has unknown operation %q", common.ErrInvalidArgument, id, modelID, operation)
			}
			if _, ok := seenOps[operation]; ok {
				return fmt.Errorf("%w: llm.customProviders[%s].models[%s].operations has duplicate operation %q", common.ErrInvalidArgument, id, modelID, operation)
			}
			seenOps[operation] = struct{}{}
		}
	}
	return nil
}

func parsePositiveDuration(field string, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%w: %s is required", common.ErrInvalidArgument, field)
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is invalid: %v", common.ErrInvalidArgument, field, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%w: %s must be positive", common.ErrInvalidArgument, field)
	}
	return value, nil
}

func applyRuntimeConfigDefaults(cfg *RuntimeConfig) {
	if cfg == nil {
		return
	}
	if cfg.MaxRuntimeRepairAttempts <= 0 {
		cfg.MaxRuntimeRepairAttempts = 10
	}
	if cfg.MaxProviderRetryAttempts <= 0 {
		cfg.MaxProviderRetryAttempts = 10
	}
	if cfg.MaxContractViolationRetriesPerStep <= 0 {
		cfg.MaxContractViolationRetriesPerStep = 2
	}
	if cfg.MaxContractViolationRetriesPerRun <= 0 {
		cfg.MaxContractViolationRetriesPerRun = 8
	}
	if cfg.MaxRepeatedContractViolationSignature <= 0 {
		cfg.MaxRepeatedContractViolationSignature = 1
	}
	if cfg.MaxConsecutiveContractViolationNoShapeChange <= 0 {
		cfg.MaxConsecutiveContractViolationNoShapeChange = 1
	}
}

func validateLLMOutputRuntimeConfig(cfg LLMOutputRuntimeConfig) error {
	for _, item := range []struct {
		name  string
		value int
	}{
		{"semanticPlanMaxOutputTokens", cfg.SemanticPlanMaxOutputTokens},
		{"candidateMaxOutputTokens", cfg.CandidateMaxOutputTokens},
		{"maxPatchContentBytes", cfg.MaxPatchContentBytes},
		{"maxGraphBytes", cfg.MaxGraphBytes},
		{"maxGraphFileWrites", cfg.MaxGraphFileWrites},
		{"schemaInvalidLargePreviewThreshold", cfg.SchemaInvalidLargePreviewThreshold},
		{"maxArtifactChunkBytes", cfg.MaxArtifactChunkBytes},
		{"previousChunkTailBytes", cfg.PreviousChunkTailBytes},
		{"maxChunkRetryAttempts", cfg.MaxChunkRetryAttempts},
		{"maxArtifactPlanAttempts", cfg.MaxArtifactPlanAttempts},
		{"maxArtifactReplanAttempts", cfg.MaxArtifactReplanAttempts},
	} {
		if item.value <= 0 {
			return fmt.Errorf("%w: runtime.llmOutput.%s must be positive", common.ErrInvalidArgument, item.name)
		}
	}
	if cfg.MaxPatchContentBytes > cfg.MaxGraphBytes {
		return fmt.Errorf("%w: runtime.llmOutput.maxPatchContentBytes must not exceed maxGraphBytes", common.ErrInvalidArgument)
	}
	if cfg.SchemaInvalidLargePreviewThreshold > cfg.MaxGraphBytes {
		return fmt.Errorf("%w: runtime.llmOutput.schemaInvalidLargePreviewThreshold must not exceed maxGraphBytes", common.ErrInvalidArgument)
	}
	if cfg.PreviousChunkTailBytes > cfg.MaxArtifactChunkBytes {
		return fmt.Errorf("%w: runtime.llmOutput.previousChunkTailBytes must not exceed maxArtifactChunkBytes", common.ErrInvalidArgument)
	}
	return nil
}

func Reload() error {
	if !configuration.IsConfigManagerInitialized() {
		if err := configuration.InitDefaultConfigManager(""); err != nil {
			return err
		}
		configRevision.Add(1)
		return nil
	}
	if err := configuration.ReloadConfig(); err != nil {
		return err
	}
	configRevision.Add(1)
	return nil
}

func Revision() uint64 {
	return configRevision.Load()
}

func (c Config) WebSearchProviderValues() map[string]string {
	valuesByID := c.WebSearchProviderValueSets()
	return valuesByID[strings.TrimSpace(c.WebSearch.DefaultProvider)]
}

func (c Config) WebSearchProviderValueSets() map[string]map[string]string {
	ret := map[string]map[string]string{}
	for id, provider := range c.WebSearch.Providers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ret[id] = webSearchProviderValues(provider)
	}
	return ret
}

func webSearchProviderValues(provider WebSearchProviderConfig) map[string]string {
	values := map[string]string{}
	set(values, "provider", provider.Provider)
	set(values, "searchEndpoint", provider.SearchEndpoint)
	set(values, "fetchEndpoint", provider.FetchEndpoint)
	set(values, "apiKey", provider.APIKey)
	set(values, "timeout", provider.Timeout)
	if provider.MaxResults > 0 {
		values["maxResults"] = intString(provider.MaxResults)
	}
	if provider.MaxFetchBytes > 0 {
		values["maxFetchBytes"] = int64String(provider.MaxFetchBytes)
	}
	if provider.MaxRedirects > 0 {
		values["maxRedirects"] = intString(provider.MaxRedirects)
	}
	if len(provider.AllowDomains) > 0 {
		values["allowDomains"] = strings.Join(provider.AllowDomains, ",")
	}
	if len(provider.DenyDomains) > 0 {
		values["denyDomains"] = strings.Join(provider.DenyDomains, ",")
	}
	if provider.Disabled != nil {
		values["disabled"] = boolString(*provider.Disabled)
	}
	for key, value := range provider.Headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		set(values, "header."+key, value)
	}
	return values
}

func loadSection(section string, target any) error {
	err := configuration.GetSection(section, target)
	if err == nil {
		return nil
	}
	if isMissingSectionError(err) {
		return nil
	}
	return err
}

func isMissingSectionError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "config key not found:")
}

func set(values map[string]string, key string, value string) {
	if strings.TrimSpace(value) != "" {
		values[key] = strings.TrimSpace(value)
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func GitRuntimeEnabled(cfg GitRuntimeConfig) bool {
	_ = cfg
	return true
}

func GitRuntimeWriteEnabled(cfg GitRuntimeConfig) bool {
	_ = cfg
	return true
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func int64String(value int64) string {
	return formatInt(value)
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func Close() error {
	if !configuration.IsConfigManagerInitialized() {
		return nil
	}
	if err := configuration.CloseConfigManager(); err != nil {
		return err
	}
	configuration.DefaultConfigManager = nil
	configRevision.Add(1)
	return nil
}
