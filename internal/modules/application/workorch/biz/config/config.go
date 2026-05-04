package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type RemoteConfig struct {
	DefaultAddr string `toml:"defaultAddr"`
}

type LocalConfig struct {
	BinaryPath string `toml:"binaryPath"`
	ConfigDir  string `toml:"configDir"`
	DataDir    string `toml:"dataDir"`
	Port       int    `toml:"port"`
	AutoStart  bool   `toml:"autoStart"`
}

type DefaultsConfig struct {
	PollInterval          string `toml:"pollInterval"`
	DefaultSoulID         string `toml:"defaultSoulId"`
	LastActiveWorkspaceID string `toml:"lastActiveWorkspaceId"`
}

type Config struct {
	Remote   RemoteConfig   `toml:"remote"`
	Local    LocalConfig    `toml:"local"`
	Defaults DefaultsConfig `toml:"defaults"`
}

func Default() *Config {
	return &Config{
		Remote: RemoteConfig{
			DefaultAddr: "http://localhost:8080",
		},
		Local: LocalConfig{
			DataDir: ".workorch-data",
		},
		Defaults: DefaultsConfig{
			PollInterval: "2s",
		},
	}
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workorch", "config.toml"), nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
