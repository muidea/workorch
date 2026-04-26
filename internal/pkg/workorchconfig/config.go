package workorchconfig

import (
	"os"
	"strings"

	"github.com/muidea/magicCommon/framework/configuration"
)

const EnvConfigDir = "CONFIG_PATH"

type Config struct {
	Server     ServerConfig     `json:"server"`
	Repository RepositoryConfig `json:"repository"`
	LLM        LLMConfig        `json:"llm"`
}

type ServerConfig struct {
	Addr              string `json:"addr"`
	ReadHeaderTimeout string `json:"readHeaderTimeout"`
	ReadTimeout       string `json:"readTimeout"`
	WriteTimeout      string `json:"writeTimeout"`
	IdleTimeout       string `json:"idleTimeout"`
	ShutdownTimeout   string `json:"shutdownTimeout"`
	MaxConcurrentRuns int64  `json:"maxConcurrentRuns"`
}

type RepositoryConfig struct {
	Backend string `json:"backend"`
	DataDir string `json:"dataDir"`
}

type LLMConfig struct {
	DefaultProvider string                       `json:"defaultProvider"`
	Providers       map[string]LLMProviderConfig `json:"providers"`
}

type LLMProviderConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseURL"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
	Timeout  string `json:"timeout"`
	Disabled *bool  `json:"disabled"`
}

func ConfigureDirectory(configDir string) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		return
	}
	_ = os.Setenv(EnvConfigDir, configDir)
}

func Current() (Config, error) {
	if !configuration.IsConfigManagerInitialized() {
		if err := configuration.InitDefaultConfigManager(""); err != nil {
			return Config{}, err
		}
	}
	var cfg Config
	if err := loadSection("server", &cfg.Server); err != nil {
		return Config{}, err
	}
	if err := loadSection("repository", &cfg.Repository); err != nil {
		return Config{}, err
	}
	if err := loadSection("llm", &cfg.LLM); err != nil {
		return Config{}, err
	}
	return cfg, nil
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

func Close() error {
	if !configuration.IsConfigManagerInitialized() {
		return nil
	}
	if err := configuration.CloseConfigManager(); err != nil {
		return err
	}
	configuration.DefaultConfigManager = nil
	return nil
}
