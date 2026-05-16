package app

import (
	"context"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
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

type stubWorkspaceBackend struct {
	createReq backend.CreateWorkspaceRequest
	created   backend.WorkspaceInfo
	list      []backend.WorkspaceInfo
}

func (s *stubWorkspaceBackend) Workspace() backend.WorkspaceBackend { return s }
func (s *stubWorkspaceBackend) Goal() backend.GoalBackend           { return nil }
func (s *stubWorkspaceBackend) DoD() backend.DoDBackend             { return nil }
func (s *stubWorkspaceBackend) Run() backend.RunBackend             { return nil }
func (s *stubWorkspaceBackend) Decision() backend.DecisionBackend   { return nil }
func (s *stubWorkspaceBackend) Approval() backend.ApprovalBackend   { return nil }
func (s *stubWorkspaceBackend) Soul() backend.SoulBackend           { return nil }
func (s *stubWorkspaceBackend) Skill() backend.SkillBackend         { return nil }
func (s *stubWorkspaceBackend) MCP() backend.MCPBackend             { return nil }
func (s *stubWorkspaceBackend) Memory() backend.MemoryBackend       { return nil }
func (s *stubWorkspaceBackend) Template() backend.TemplateBackend   { return nil }
func (s *stubWorkspaceBackend) Config() backend.ConfigBackend       { return nil }
func (s *stubWorkspaceBackend) Events() backend.EventsBackend       { return nil }

func (s *stubWorkspaceBackend) Create(_ context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	s.createReq = req
	s.created = backend.WorkspaceInfo{
		ID:                   common.WorkspaceID("ws-created"),
		Title:                req.Title,
		Description:          req.Description,
		DeliverableDirectory: req.DeliverableDirectory,
		State:                common.WorkspaceStateActive,
		DefaultSoulID:        "",
		SoulBindings:         nil,
	}
	return &s.created, nil
}

func (s *stubWorkspaceBackend) Get(context.Context, common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return nil, common.ErrNotFound
}

func (s *stubWorkspaceBackend) List(context.Context) ([]backend.WorkspaceInfo, error) {
	return append([]backend.WorkspaceInfo(nil), s.list...), nil
}

func (s *stubWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}

func TestResolveDefaultWorkspaceCreatesWorkspaceWithGenericGovernedTemplate(t *testing.T) {
	be := &stubWorkspaceBackend{}
	ws := ResolveDefaultWorkspace(context.Background(), be, t.TempDir(), "")
	if ws.ID == "" {
		t.Fatalf("workspace not created")
	}
	if be.createReq.TemplateID != defaultWorkspaceTemplateID {
		t.Fatalf("template id = %q, want %q for fresh startup", be.createReq.TemplateID, defaultWorkspaceTemplateID)
	}
	if be.createReq.DeliverableDirectory == "" {
		t.Fatalf("deliverable directory was not forwarded")
	}
}
