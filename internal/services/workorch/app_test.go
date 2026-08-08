package workorch

import (
	"context"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	cliconfig "github.com/muidea/workorch/internal/services/workorch/biz/config"
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

func TestRemoteCommandRequiresFullEventStreamOnlyForAgent(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "agent", args: []string{"--remote", "http://127.0.0.1:8080", "agent", "goal"}, want: true},
		{name: "repl", args: []string{"--remote", "http://127.0.0.1:8080", "repl"}, want: false},
		{name: "implicit tui", args: []string{"--remote", "http://127.0.0.1:8080"}, want: false},
		{name: "workspace command", args: []string{"--remote", "http://127.0.0.1:8080", "workspace", "list"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := remoteCommandRequiresFullEventStream(tc.args); got != tc.want {
				t.Fatalf("remoteCommandRequiresFullEventStream(%#v) = %v, want %v", tc.args, got, tc.want)
			}
		})
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

func TestResolveDefaultWorkspaceUsesCLIConfigLastActiveWorkspace(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "config.toml")
	if err := cliconfig.Save(configPath, &cliconfig.Config{
		Defaults: cliconfig.DefaultsConfig{
			LastActiveWorkspaceID: "ws-b",
		},
	}); err != nil {
		t.Fatalf("save cli config: %v", err)
	}

	be := serviceDefaultWorkspaceBackend{
		workspaces: []projectioncontract.Workspace{
			{ID: "ws-a", Title: "workspace A", DeliverableDirectory: filepath.Join(dir, "other")},
			{ID: "ws-b", Title: "workspace B", DeliverableDirectory: filepath.Join(dir, "target")},
		},
	}
	got := resolveDefaultWorkspace(context.Background(), be, []string{"--config", configPath})
	if got.ID != "ws-b" {
		t.Fatalf("default workspace = %q, want ws-b", got.ID)
	}
}

type serviceDefaultWorkspaceBackend struct {
	workspaces []projectioncontract.Workspace
}

func (b serviceDefaultWorkspaceBackend) Workspace() backend.WorkspaceBackend {
	return serviceDefaultWorkspaceWorkspaceBackend{workspaces: b.workspaces}
}
func (b serviceDefaultWorkspaceBackend) Goal() backend.GoalBackend         { return nil }
func (b serviceDefaultWorkspaceBackend) DoD() backend.DoDBackend           { return nil }
func (b serviceDefaultWorkspaceBackend) Run() backend.RunBackend           { return nil }
func (b serviceDefaultWorkspaceBackend) Decision() backend.DecisionBackend { return nil }
func (b serviceDefaultWorkspaceBackend) Approval() backend.ApprovalBackend { return nil }
func (b serviceDefaultWorkspaceBackend) Soul() backend.SoulBackend         { return nil }
func (b serviceDefaultWorkspaceBackend) Skill() backend.SkillBackend       { return nil }
func (b serviceDefaultWorkspaceBackend) MCP() backend.MCPBackend           { return nil }
func (b serviceDefaultWorkspaceBackend) Memory() backend.MemoryBackend     { return nil }
func (b serviceDefaultWorkspaceBackend) Template() backend.TemplateBackend { return nil }
func (b serviceDefaultWorkspaceBackend) Config() backend.ConfigBackend     { return nil }
func (b serviceDefaultWorkspaceBackend) Events() backend.EventsBackend     { return nil }
func (b serviceDefaultWorkspaceBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	return nil
}

type serviceDefaultWorkspaceWorkspaceBackend struct {
	workspaces []projectioncontract.Workspace
}

func (b serviceDefaultWorkspaceWorkspaceBackend) Create(context.Context, workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return nil, nil
}

func (b serviceDefaultWorkspaceWorkspaceBackend) Get(_ context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	for _, ws := range b.workspaces {
		if ws.ID == string(id) {
			return &ws, nil
		}
	}
	return nil, nil
}

func (b serviceDefaultWorkspaceWorkspaceBackend) List(context.Context) ([]projectioncontract.Workspace, error) {
	return b.workspaces, nil
}

func (b serviceDefaultWorkspaceWorkspaceBackend) Snapshot(context.Context, common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	return nil, nil
}

func (b serviceDefaultWorkspaceWorkspaceBackend) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (b serviceDefaultWorkspaceWorkspaceBackend) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	return b.Get(ctx, id)
}

func (b serviceDefaultWorkspaceWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}
