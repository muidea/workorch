package workorchconfig

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/muidea/magicCommon/framework/configuration"
	"github.com/muidea/workorch/internal/pkg/common"
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
	DefaultSoulID            string                 `json:"defaultSoulId" toml:"defaultSoulId"`
	MaxRuntimeRepairAttempts int                    `json:"maxRuntimeRepairAttempts" toml:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts int                    `json:"maxProviderRetryAttempts" toml:"maxProviderRetryAttempts"`
	ShellCommandTimeout      string                 `json:"shellCommandTimeout" toml:"shellCommandTimeout"`
	Git                      GitRuntimeConfig       `json:"git" toml:"git"`
	LLMOutput                LLMOutputRuntimeConfig `json:"llmOutput" toml:"llmOutput"`
}

type GitRuntimeConfig struct {
	Enabled        *bool  `json:"enabled" toml:"enabled"`
	WriteEnabled   *bool  `json:"writeEnabled" toml:"writeEnabled"`
	DefaultTimeout string `json:"defaultTimeout" toml:"defaultTimeout"`
}

type LLMOutputRuntimeConfig struct {
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

type LLMConfig struct {
	DefaultProvider          string                              `json:"defaultProvider" toml:"defaultProvider"`
	DefaultEmbeddingProvider string                              `json:"defaultEmbeddingProvider" toml:"defaultEmbeddingProvider"`
	ModelCapabilities        map[string]LLMModelCapabilityConfig `json:"modelCapabilities" toml:"modelCapabilities"`
	Providers                map[string]LLMProviderConfig        `json:"providers" toml:"providers"`
}

type LLMModelCapabilityConfig struct {
	Provider            string `json:"provider" toml:"provider"`
	ModelPattern        string `json:"modelPattern" toml:"modelPattern"`
	ContextWindowTokens int    `json:"contextWindowTokens" toml:"contextWindowTokens"`
}

type LLMProviderConfig struct {
	Provider                string   `json:"provider" toml:"provider"`
	APISurface              string   `json:"apiSurface" toml:"apiSurface,omitempty"`
	BaseURL                 string   `json:"baseURL" toml:"baseURL"`
	APIKey                  string   `json:"apiKey" toml:"apiKey"`
	Model                   string   `json:"model" toml:"model"`
	Timeout                 string   `json:"timeout" toml:"timeout,omitempty"`
	Disabled                *bool    `json:"disabled" toml:"disabled,omitempty"`
	ChatCompletions         *bool    `json:"chatCompletions" toml:"chatCompletions,omitempty"`
	Streaming               *bool    `json:"streaming" toml:"streaming,omitempty"`
	NonChatOperations       []string `json:"nonChatOperations" toml:"nonChatOperations,omitempty"`
	Temperature             *float64 `json:"temperature" toml:"temperature,omitempty"`
	MaxOutputTokens         int      `json:"maxOutputTokens" toml:"maxOutputTokens,omitempty"`
	ReasoningEffort         string   `json:"reasoningEffort" toml:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens    int      `json:"thinkingBudgetTokens" toml:"thinkingBudgetTokens,omitempty"`
	FailoverEnabled         *bool    `json:"failoverEnabled" toml:"failoverEnabled,omitempty"`
	FailoverProviders       []string `json:"failoverProviders" toml:"failoverProviders,omitempty"`
	CacheMode               string   `json:"cacheMode" toml:"cacheMode,omitempty"`
	PromptCacheKey          *bool    `json:"promptCacheKey" toml:"promptCacheKey,omitempty"`
	MessageCacheControl     *bool    `json:"messageCacheControl" toml:"messageCacheControl,omitempty"`
	CacheUsageFields        []string `json:"cacheUsageFields" toml:"cacheUsageFields,omitempty"`
	ContextProfileMode      string   `json:"contextProfileMode" toml:"contextProfileMode,omitempty"`
	ContextWindowTokens     int      `json:"contextWindowTokens" toml:"contextWindowTokens,omitempty"`
	MaxInputTokens          int      `json:"maxInputTokens" toml:"maxInputTokens,omitempty"`
	ReservedOutputTokens    int      `json:"reservedOutputTokens" toml:"reservedOutputTokens,omitempty"`
	SafetyRatio             float64  `json:"safetyRatio" toml:"safetyRatio,omitempty"`
	ContextUtilizationRatio float64  `json:"contextUtilizationRatio" toml:"contextUtilizationRatio,omitempty"`
	BytesHardLimit          int      `json:"bytesHardLimit" toml:"bytesHardLimit,omitempty"`
	MaxRequestBytes         int      `json:"maxRequestBytes" toml:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  float64  `json:"estimatedTokensPerByte" toml:"estimatedTokensPerByte,omitempty"`
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
	if next.LLM.Providers == nil {
		next.LLM.Providers = map[string]LLMProviderConfig{}
	}
	if next.LLM.ModelCapabilities == nil {
		next.LLM.ModelCapabilities = map[string]LLMModelCapabilityConfig{}
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
		MaxPatchContentBytes:               12288,
		MaxGraphBytes:                      32768,
		MaxGraphFileWrites:                 3,
		SchemaInvalidLargePreviewThreshold: 8192,
		MaxArtifactChunkBytes:              8192,
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
	cfg.Runtime.LLMOutput = normalized
	return nil
}

func validateLLMOutputRuntimeConfig(cfg LLMOutputRuntimeConfig) error {
	for _, item := range []struct {
		name  string
		value int
	}{
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

func (c Config) LLMProviderValues() map[string]string {
	valuesByID := c.LLMProviderValueSets()
	return valuesByID[strings.TrimSpace(c.LLM.DefaultProvider)]
}

func (c Config) LLMProviderValueSets() map[string]map[string]string {
	ret := map[string]map[string]string{}
	for id, provider := range c.LLM.Providers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		values := llmProviderValues(provider)
		if provider.ContextWindowTokens <= 0 {
			if capability, ok := c.LLM.ResolveModelCapability(provider.Provider, provider.Model); ok && capability.ContextWindowTokens > 0 {
				values["contextWindowTokens"] = intString(capability.ContextWindowTokens)
				values["contextCapabilitySource"] = "model_registry"
			}
		}
		ret[id] = values
	}
	return ret
}

func (c LLMConfig) ResolveModelCapability(provider string, model string) (LLMModelCapabilityConfig, bool) {
	provider = normalizeCapabilityMatchValue(provider)
	model = normalizeCapabilityMatchValue(model)
	if model == "" {
		return LLMModelCapabilityConfig{}, false
	}
	for _, capability := range c.ModelCapabilities {
		if capability.ContextWindowTokens <= 0 {
			continue
		}
		if capability.Provider != "" && normalizeCapabilityMatchValue(capability.Provider) != provider {
			continue
		}
		if modelPatternMatches(capability.ModelPattern, model) {
			return capability, true
		}
	}
	return LLMModelCapabilityConfig{}, false
}

func modelPatternMatches(patternValue string, model string) bool {
	patternValue = normalizeCapabilityMatchValue(patternValue)
	if patternValue == "" {
		return false
	}
	if patternValue == "*" {
		return true
	}
	if strings.ContainsAny(patternValue, "*?[") {
		if matched, err := path.Match(patternValue, model); err == nil && matched {
			return true
		}
	}
	return patternValue == model || strings.Contains(model, patternValue)
}

func normalizeCapabilityMatchValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	return value
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

func llmProviderValues(provider LLMProviderConfig) map[string]string {
	values := map[string]string{}
	set(values, "provider", provider.Provider)
	set(values, "apiSurface", provider.APISurface)
	set(values, "baseURL", provider.BaseURL)
	set(values, "apiKey", provider.APIKey)
	set(values, "model", provider.Model)
	set(values, "timeout", provider.Timeout)
	if strings.TrimSpace(provider.ContextProfileMode) != "" {
		values["contextProfileMode"] = strings.TrimSpace(provider.ContextProfileMode)
	}
	if provider.ContextWindowTokens > 0 {
		values["contextWindowTokens"] = intString(provider.ContextWindowTokens)
	}
	if isManualContextProfile(provider.ContextProfileMode) {
		if provider.MaxInputTokens > 0 {
			values["maxInputTokens"] = intString(provider.MaxInputTokens)
		}
		if provider.ReservedOutputTokens > 0 {
			values["reservedOutputTokens"] = intString(provider.ReservedOutputTokens)
		}
		if provider.SafetyRatio > 0 {
			values["safetyRatio"] = formatFloat(provider.SafetyRatio)
		}
		if provider.ContextUtilizationRatio > 0 {
			values["contextUtilizationRatio"] = formatFloat(provider.ContextUtilizationRatio)
		}
		if provider.BytesHardLimit > 0 {
			values["bytesHardLimit"] = intString(provider.BytesHardLimit)
		}
		if provider.MaxRequestBytes > 0 {
			values["maxRequestBytes"] = intString(provider.MaxRequestBytes)
		}
		if provider.EstimatedTokensPerByte > 0 {
			values["estimatedTokensPerByte"] = formatFloat(provider.EstimatedTokensPerByte)
		}
	}
	if provider.Disabled != nil {
		if *provider.Disabled {
			values["disabled"] = "true"
		} else {
			values["disabled"] = "false"
		}
	}
	if provider.ChatCompletions != nil {
		values["chatCompletions"] = boolString(*provider.ChatCompletions)
	}
	if provider.Streaming != nil {
		values["streaming"] = boolString(*provider.Streaming)
	}
	if len(provider.NonChatOperations) > 0 {
		values["nonChatOperations"] = strings.Join(provider.NonChatOperations, ",")
	}
	if provider.Temperature != nil {
		values["temperature"] = formatFloat(*provider.Temperature)
	}
	if provider.MaxOutputTokens > 0 {
		values["maxOutputTokens"] = intString(provider.MaxOutputTokens)
	}
	set(values, "reasoningEffort", provider.ReasoningEffort)
	if provider.ThinkingBudgetTokens > 0 {
		values["thinkingBudgetTokens"] = intString(provider.ThinkingBudgetTokens)
	}
	if provider.FailoverEnabled != nil {
		values["failoverEnabled"] = boolString(*provider.FailoverEnabled)
	}
	if len(provider.FailoverProviders) > 0 {
		values["failoverProviders"] = strings.Join(provider.FailoverProviders, ",")
	}
	set(values, "cacheMode", provider.CacheMode)
	if provider.PromptCacheKey != nil {
		values["promptCacheKey"] = boolString(*provider.PromptCacheKey)
	}
	if provider.MessageCacheControl != nil {
		values["messageCacheControl"] = boolString(*provider.MessageCacheControl)
	}
	if len(provider.CacheUsageFields) > 0 {
		values["cacheUsageFields"] = strings.Join(provider.CacheUsageFields, ",")
	}
	return values
}

func isManualContextProfile(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "manual")
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
