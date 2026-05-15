package workorchconfig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/muidea/magicCommon/framework/configuration"
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
	Addr                  string `json:"addr" toml:"addr"`
	ReadHeaderTimeout     string `json:"readHeaderTimeout" toml:"readHeaderTimeout"`
	ReadTimeout           string `json:"readTimeout" toml:"readTimeout"`
	WriteTimeout          string `json:"writeTimeout" toml:"writeTimeout"`
	IdleTimeout           string `json:"idleTimeout" toml:"idleTimeout"`
	ShutdownTimeout       string `json:"shutdownTimeout" toml:"shutdownTimeout"`
	MaxConcurrentRuns     int64  `json:"maxConcurrentRuns" toml:"maxConcurrentRuns"`
	DefaultDialogLanguage string `json:"defaultDialogLanguage" toml:"defaultDialogLanguage"`
}

type RepositoryConfig struct {
	Backend string `json:"backend" toml:"backend"`
	DataDir string `json:"dataDir" toml:"dataDir"`
}

type RuntimeConfig struct {
	DefaultSoulID            string `json:"defaultSoulId" toml:"defaultSoulId"`
	MaxProviderRetryAttempts int    `json:"maxProviderRetryAttempts" toml:"maxProviderRetryAttempts"`
	ShellCommandTimeout      string `json:"shellCommandTimeout" toml:"shellCommandTimeout"`
}

type LLMConfig struct {
	DefaultProvider          string                       `json:"defaultProvider" toml:"defaultProvider"`
	DefaultEmbeddingProvider string                       `json:"defaultEmbeddingProvider" toml:"defaultEmbeddingProvider"`
	Providers                map[string]LLMProviderConfig `json:"providers" toml:"providers"`
}

type LLMProviderConfig struct {
	Provider               string   `json:"provider" toml:"provider"`
	BaseURL                string   `json:"baseURL" toml:"baseURL"`
	APIKey                 string   `json:"apiKey" toml:"apiKey"`
	Model                  string   `json:"model" toml:"model"`
	Timeout                string   `json:"timeout" toml:"timeout"`
	Disabled               *bool    `json:"disabled" toml:"disabled"`
	ChatCompletions        *bool    `json:"chatCompletions" toml:"chatCompletions"`
	Streaming              *bool    `json:"streaming" toml:"streaming"`
	NonChatOperations      []string `json:"nonChatOperations" toml:"nonChatOperations"`
	FailoverEnabled        *bool    `json:"failoverEnabled" toml:"failoverEnabled"`
	FailoverProviders      []string `json:"failoverProviders" toml:"failoverProviders"`
	ContextWindowTokens    int      `json:"contextWindowTokens" toml:"contextWindowTokens"`
	MaxInputTokens         int      `json:"maxInputTokens" toml:"maxInputTokens"`
	ReservedOutputTokens   int      `json:"reservedOutputTokens" toml:"reservedOutputTokens"`
	SafetyRatio            float64  `json:"safetyRatio" toml:"safetyRatio"`
	BytesHardLimit         int      `json:"bytesHardLimit" toml:"bytesHardLimit"`
	EstimatedTokensPerByte float64  `json:"estimatedTokensPerByte" toml:"estimatedTokensPerByte"`
	InputTokenPrice        float64  `json:"inputTokenPrice" toml:"inputTokenPrice"`
	OutputTokenPrice       float64  `json:"outputTokenPrice" toml:"outputTokenPrice"`
	Currency               string   `json:"currency" toml:"currency"`
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
	if next.LLM.Providers == nil {
		next.LLM.Providers = map[string]LLMProviderConfig{}
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
		ret[id] = llmProviderValues(provider)
	}
	return ret
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
	set(values, "baseURL", provider.BaseURL)
	set(values, "apiKey", provider.APIKey)
	set(values, "model", provider.Model)
	set(values, "timeout", provider.Timeout)
	if provider.ContextWindowTokens > 0 {
		values["contextWindowTokens"] = intString(provider.ContextWindowTokens)
	}
	if provider.MaxInputTokens > 0 {
		values["maxInputTokens"] = intString(provider.MaxInputTokens)
	}
	if provider.ReservedOutputTokens > 0 {
		values["reservedOutputTokens"] = intString(provider.ReservedOutputTokens)
	}
	if provider.SafetyRatio > 0 {
		values["safetyRatio"] = formatFloat(provider.SafetyRatio)
	}
	if provider.BytesHardLimit > 0 {
		values["bytesHardLimit"] = intString(provider.BytesHardLimit)
	}
	if provider.EstimatedTokensPerByte > 0 {
		values["estimatedTokensPerByte"] = formatFloat(provider.EstimatedTokensPerByte)
	}
	if provider.InputTokenPrice > 0 {
		values["inputTokenPrice"] = formatFloat(provider.InputTokenPrice)
	}
	if provider.OutputTokenPrice > 0 {
		values["outputTokenPrice"] = formatFloat(provider.OutputTokenPrice)
	}
	set(values, "currency", provider.Currency)
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
	if provider.FailoverEnabled != nil {
		values["failoverEnabled"] = boolString(*provider.FailoverEnabled)
	}
	if len(provider.FailoverProviders) > 0 {
		values["failoverProviders"] = strings.Join(provider.FailoverProviders, ",")
	}
	return values
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
