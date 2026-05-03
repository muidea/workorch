package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Launcher struct {
	config Config
	cmd    *exec.Cmd
	port   int
	done   chan struct{}
}

type Config struct {
	BinaryPath string
	ConfigDir  string
	DataDir    string
	Port       int
}

func NewLauncher(cfg Config) *Launcher {
	return &Launcher{
		config: cfg,
		done:   make(chan struct{}),
	}
}

func (l *Launcher) Port() int {
	return l.port
}

func (l *Launcher) Start(ctx context.Context) (string, error) {
	binaryPath := l.config.BinaryPath
	if binaryPath == "" {
		// Try to find workorchd in the same directory as the CLI or in PATH
		if exe, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exe), "workorchd")
			if _, err := os.Stat(candidate); err == nil {
				binaryPath = candidate
			}
		}
	}
	if binaryPath == "" {
		binaryPath = "workorchd"
	}

	// Allocate a random port
	port := l.config.Port
	if port == 0 {
		var err error
		port, err = allocatePort()
		if err != nil {
			return "", fmt.Errorf("allocate port: %w", err)
		}
	}
	l.port = port

	configDir, cleanup, err := l.prepareConfigDir(port)
	if err != nil {
		return "", fmt.Errorf("prepare config: %w", err)
	}

	l.cmd = exec.CommandContext(ctx, binaryPath, "--config-dir", configDir)
	l.cmd.Stdout = os.Stderr
	l.cmd.Stderr = os.Stderr
	l.cmd.Env = os.Environ()
	l.cmd.Env = append(l.cmd.Env, "CONFIG_PATH="+configDir)

	if err := l.cmd.Start(); err != nil {
		cleanup()
		return "", fmt.Errorf("start workorchd: %w", err)
	}

	// Wait for the service to be ready
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := l.waitReady(ctx, addr); err != nil {
		l.cmd.Process.Kill()
		l.cmd.Wait()
		cleanup()
		return "", fmt.Errorf("wait for workorchd: %w", err)
	}

	// Background cleanup goroutine
	go func() {
		l.cmd.Wait()
		cleanup()
		close(l.done)
	}()

	return addr, nil
}

func (l *Launcher) Stop() error {
	if l.cmd == nil || l.cmd.Process == nil {
		return nil
	}
	l.cmd.Process.Signal(os.Interrupt)

	// Wait up to 5 seconds for graceful shutdown
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-l.done:
		return nil
	case <-timer.C:
		l.cmd.Process.Kill()
		<-l.done
		return nil
	}
}

func (l *Launcher) prepareConfigDir(port int) (string, func(), error) {
	configDir := l.config.ConfigDir
	var tmpDir string

	if configDir == "" {
		// Create a temporary config directory
		var err error
		tmpDir, err = os.MkdirTemp("", "workorch-cli-*")
		if err != nil {
			return "", nil, err
		}
		configDir = tmpDir
	} else {
		// Use source configDir as a base; copy to temp
		var err error
		tmpDir, err = os.MkdirTemp("", "workorch-cli-*")
		if err != nil {
			return "", nil, err
		}
	}

	cleanup := func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	}

	// Read source config or create minimal config
	var configData map[string]any
	srcConfig := filepath.Join(configDir, "application.toml")
	if data, err := os.ReadFile(srcConfig); err == nil {
		if err := toml.Unmarshal(data, &configData); err != nil {
			return "", cleanup, fmt.Errorf("parse config: %w", err)
		}
	} else {
		configData = defaultConfig()
	}

	// Override server address with our dynamic port
	if serverSection, ok := configData["server"].(map[string]any); ok {
		serverSection["addr"] = fmt.Sprintf("127.0.0.1:%d", port)
	} else {
		configData["server"] = map[string]any{"addr": fmt.Sprintf("127.0.0.1:%d", port)}
	}

	// Override data dir if specified
	if l.config.DataDir != "" {
		if repoSection, ok := configData["repository"].(map[string]any); ok {
			repoSection["dataDir"] = l.config.DataDir
		} else {
			configData["repository"] = map[string]any{"dataDir": l.config.DataDir}
		}
	}

	data, err := toml.Marshal(configData)
	if err != nil {
		return "", cleanup, fmt.Errorf("marshal config: %w", err)
	}

	cfgPath := filepath.Join(tmpDir, "application.toml")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return "", cleanup, fmt.Errorf("write config: %w", err)
	}

	return tmpDir, cleanup, nil
}

func (l *Launcher) waitReady(ctx context.Context, addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	url := addr + "/healthz"

	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("workorchd did not become ready within 10s")
		case <-ticker.C:
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var status struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(body, &status) == nil && status.Status == "ok" {
				return nil
			}
		}
	}
}

func allocatePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func defaultConfig() map[string]any {
	return map[string]any{
		"endpointName": "workorchd",
		"server": map[string]any{
			"addr":              "127.0.0.1:0",
			"readHeaderTimeout": "5s",
			"readTimeout":       "15s",
			"writeTimeout":      "180s",
			"idleTimeout":       "60s",
			"shutdownTimeout":   "5s",
			"maxConcurrentRuns": 1,
		},
		"repository": map[string]any{
			"backend": "memory",
			"dataDir": ".workorch-data",
		},
	}
}
