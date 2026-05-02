package workorchconfig

import (
	"os"
	"path/filepath"
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
	DefaultSoulID string `json:"defaultSoulId" toml:"defaultSoulId"`
}

type LLMConfig struct {
	DefaultProvider          string                       `json:"defaultProvider" toml:"defaultProvider"`
	DefaultEmbeddingProvider string                       `json:"defaultEmbeddingProvider" toml:"defaultEmbeddingProvider"`
	Providers                map[string]LLMProviderConfig `json:"providers" toml:"providers"`
}

type LLMProviderConfig struct {
	Provider          string   `json:"provider" toml:"provider"`
	BaseURL           string   `json:"baseURL" toml:"baseURL"`
	APIKey            string   `json:"apiKey" toml:"apiKey"`
	Model             string   `json:"model" toml:"model"`
	Timeout           string   `json:"timeout" toml:"timeout"`
	Disabled          *bool    `json:"disabled" toml:"disabled"`
	ChatCompletions   *bool    `json:"chatCompletions" toml:"chatCompletions"`
	Streaming         *bool    `json:"streaming" toml:"streaming"`
	NonChatOperations []string `json:"nonChatOperations" toml:"nonChatOperations"`
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
		next.EndpointName = "workorchd"
	}
	if next.LLM.Providers == nil {
		next.LLM.Providers = map[string]LLMProviderConfig{}
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

func llmProviderValues(provider LLMProviderConfig) map[string]string {
	values := map[string]string{}
	set(values, "provider", provider.Provider)
	set(values, "baseURL", provider.BaseURL)
	set(values, "apiKey", provider.APIKey)
	set(values, "model", provider.Model)
	set(values, "timeout", provider.Timeout)
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
