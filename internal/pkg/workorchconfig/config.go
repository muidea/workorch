package workorchconfig

import (
	"bytes"
	"encoding/json"
	"errors"
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
	SSH          SSHConfig        `json:"ssh" toml:"ssh"`
}

type ServerConfig struct {
	Addr                  string `json:"addr" toml:"addr"`
	ReadHeaderTimeout     string `json:"readHeaderTimeout" toml:"readHeaderTimeout"`
	ReadTimeout           string `json:"readTimeout" toml:"readTimeout"`
	WriteTimeout          string `json:"writeTimeout" toml:"writeTimeout"`
	IdleTimeout           string `json:"idleTimeout" toml:"idleTimeout"`
	ShutdownTimeout       string `json:"shutdownTimeout" toml:"shutdownTimeout"`
	DefaultDialogLanguage string `json:"defaultDialogLanguage" toml:"defaultDialogLanguage"`
}

// UnmarshalJSON rejects removed execution-scheduler/watchdog settings instead
// of silently accepting configuration that no longer controls any production
// behavior. The server section is transport configuration only.
func (c *ServerConfig) UnmarshalJSON(data []byte) error {
	type serverConfigAlias ServerConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var alias serverConfigAlias
	if err := decoder.Decode(&alias); err != nil {
		return fmt.Errorf("%w: server configuration rejects legacy or unknown fields: %v", common.ErrInvalidArgument, err)
	}
	*c = ServerConfig(alias)
	return nil
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
	MaxDocumentRecoveryAttempts int `json:"maxDocumentRecoveryAttempts" toml:"maxDocumentRecoveryAttempts"`
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

// Known LLM operations for ai-proxy catalog and model override declarations.
const (
	LLMOperationResponses  = "responses"
	LLMOperationEmbeddings = "embeddings"
)

// LLMConfig is the canonical ai-proxy-only configuration contract.
// Model access always traverses ai-proxy; direct provider declarations are not
// part of the configuration surface.
type LLMConfig struct {
	AIProxy          AIProxyProviderConfig        `json:"aiProxy" toml:"aiProxy"`
	DefaultChat      ProviderModelTargetConfig    `json:"defaultChat" toml:"defaultChat"`
	DefaultEmbedding *ProviderModelTargetConfig   `json:"defaultEmbedding,omitempty" toml:"defaultEmbedding,omitempty"`
	Budget           LLMContextBudgetPolicyConfig `json:"budget" toml:"budget"`
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

// UnmarshalJSON rejects unknown LLM fields so direct-provider configuration
// cannot silently re-enter the supported surface.
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
	if c.ProviderID != BuiltinAIProxyProviderID {
		return fmt.Errorf("%w: llm target providerID must be %q", common.ErrInvalidArgument, BuiltinAIProxyProviderID)
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
		AIProxy: DefaultAIProxyProviderConfig(),
		Budget:  DefaultLLMContextBudgetPolicyConfig(),
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
	if err := loadSection("ssh", &cfg.SSH); err != nil {
		return Config{}, err
	}
	if err := normalizeConfig(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// CurrentSSH loads only the owner-private SSH section. Capability startup must
// not depend on unrelated mutable application sections such as LLM settings.
func CurrentSSH() (SSHConfig, error) {
	if !configuration.IsConfigManagerInitialized() {
		if err := configuration.InitDefaultConfigManager(""); err != nil {
			return SSHConfig{}, err
		}
	}
	var cfg SSHConfig
	if err := loadSection("ssh", &cfg); err != nil {
		return SSHConfig{}, err
	}
	return NormalizeSSHConfig(cfg)
}

// Persisted returns the configuration that will be loaded by the next process
// start without mutating the active framework configuration manager. It is
// used by configuration writers whose changes deliberately require restart.
func Persisted() (Config, error) {
	configPath, err := ApplicationConfigPath()
	if err != nil {
		return Config{}, err
	}
	payload, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Current()
		}
		return Config{}, err
	}
	var cfg Config
	decoder := toml.NewDecoder(bytes.NewReader(payload)).DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			unknownFields := make([]string, 0, len(strictErr.Errors))
			for index := range strictErr.Errors {
				if key := strictErr.Errors[index].Key(); len(key) != 0 {
					unknownFields = append(unknownFields, strings.Join(key, "."))
				}
			}
			return Config{}, fmt.Errorf("%w: application configuration rejects legacy or unknown fields: %s", common.ErrInvalidArgument, strings.Join(unknownFields, ", "))
		}
		return Config{}, fmt.Errorf("%w: invalid application configuration: %v", common.ErrInvalidArgument, err)
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
	return writeConfigAtomically(configPath, payload)
}

func writeConfigAtomically(configPath string, payload []byte) error {
	directory := filepath.Dir(configPath)
	temporary, err := os.CreateTemp(directory, ".application.toml-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, configPath); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func DefaultLLMOutputRuntimeConfig() LLMOutputRuntimeConfig {
	return LLMOutputRuntimeConfig{
		MaxDocumentRecoveryAttempts: 3,
	}
}

func NormalizeLLMOutputRuntimeConfig(cfg LLMOutputRuntimeConfig) (LLMOutputRuntimeConfig, error) {
	defaults := DefaultLLMOutputRuntimeConfig()
	applyDefault := func(value *int, fallback int) {
		if value != nil && *value == 0 {
			*value = fallback
		}
	}
	applyDefault(&cfg.MaxDocumentRecoveryAttempts, defaults.MaxDocumentRecoveryAttempts)
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
		"maxDocumentRecoveryAttempts": normalized.MaxDocumentRecoveryAttempts,
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
	ssh, err := NormalizeSSHConfig(cfg.SSH)
	if err != nil {
		return err
	}
	cfg.SSH = ssh
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
	// Model IDs are exact ai-proxy protocol values; only trim collisions are invalid.
	var err error
	cfg.AIProxy.ModelOverrides, err = canonicalizeAIProxyModelOverrides(cfg.AIProxy.ModelOverrides)
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
			case LLMOperationResponses, LLMOperationEmbeddings:
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
	if cfg.MaxDocumentRecoveryAttempts <= 0 {
		return fmt.Errorf("%w: runtime.llmOutput.maxDocumentRecoveryAttempts must be positive", common.ErrInvalidArgument)
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
