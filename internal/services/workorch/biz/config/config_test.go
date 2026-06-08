package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Remote.DefaultAddr != "http://localhost:8080" {
		t.Fatalf("expected default remote addr, got %s", cfg.Remote.DefaultAddr)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Remote.DefaultAddr != "http://localhost:8080" {
		t.Fatal("expected default config for missing file")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := Default()
	cfg.Remote.DefaultAddr = "http://10.0.0.1:9090"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Remote.DefaultAddr != "http://10.0.0.1:9090" {
		t.Fatalf("expected http://10.0.0.1:9090, got %s", loaded.Remote.DefaultAddr)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("config file not created")
	}
}
