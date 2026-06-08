package workorch

import (
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStartupArgsDefaultsToInteractive(t *testing.T) {
	got, err := ParseStartupArgs([]string{"--config-dir", "/tmp/workorch", "hello"}, false)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if got.Mode != ModeInteractive {
		t.Fatalf("mode = %s, want interactive", got.Mode)
	}
	if got.ConfigDir != "/tmp/workorch" {
		t.Fatalf("config dir = %q, want /tmp/workorch", got.ConfigDir)
	}
	if len(got.CobraArgs) != 1 || got.CobraArgs[0] != "hello" {
		t.Fatalf("cobra args = %#v, want [hello]", got.CobraArgs)
	}
}

func TestParseStartupArgsRecognizesServerMode(t *testing.T) {
	got, err := ParseStartupArgs([]string{"--server", "--config-dir", "/tmp/workorch"}, false)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if got.Mode != ModeServer {
		t.Fatalf("mode = %s, want server", got.Mode)
	}
	if len(got.CobraArgs) != 0 {
		t.Fatalf("cobra args = %#v, want empty", got.CobraArgs)
	}
}

func TestParseStartupArgsKeepsRemoteBehindBackendBoundary(t *testing.T) {
	got, err := ParseStartupArgs([]string{"--remote", "http://127.0.0.1:8080", "workspace", "list"}, false)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if got.Mode != ModeRemote {
		t.Fatalf("mode = %s, want remote", got.Mode)
	}
	if got.Remote != "http://127.0.0.1:8080" {
		t.Fatalf("remote = %q", got.Remote)
	}
	if len(got.CobraArgs) != 4 {
		t.Fatalf("cobra args = %#v, want remote flag and command", got.CobraArgs)
	}
}

func TestInstallInteractiveLoggingWritesToConfigLogFile(t *testing.T) {
	dir := t.TempDir()
	restore := installInteractiveLogging(dir)
	slog.Info("interactive slog line")
	stdlog.Print("interactive std log line")
	restore()

	data, err := os.ReadFile(filepath.Join(dir, "logs", interactiveLogFileName))
	if err != nil {
		t.Fatalf("read interactive log: %v", err)
	}
	text := string(data)
	for _, want := range []string{"interactive slog line", "interactive std log line"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log file missing %q:\n%s", want, text)
		}
	}
}
