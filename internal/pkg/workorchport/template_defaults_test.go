package workorchport_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefaultDoDItemsForWorkspaceUsesMatchingTemplate(t *testing.T) {
	be := newFakeBackend()
	be.workspaces = append(be.workspaces, backend.WorkspaceInfo{
		ID:            "ws-template",
		SoulBindings:  map[string]common.SoulID{"execution": "soul-execution"},
		DefaultSoulID: "soul-default",
	})
	be.templates = append(be.templates, backend.TemplateInfo{
		ID:            "software-dev",
		DefaultSoulID: "soul-default",
		SoulBindings:  map[string]string{"execution": "soul-execution"},
		DoDItems:      []string{"tests pass", "diff is delivered"},
	})

	got := backend.DefaultDoDItemsForWorkspace(context.Background(), be, "ws-template")

	want := []string{"tests pass", "diff is delivered"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DoD items = %+v, want %+v", got, want)
	}
}

func TestDefaultDoDItemsForWorkspaceFallsBackWhenTemplateDoesNotMatch(t *testing.T) {
	be := newFakeBackend()
	be.workspaces = append(be.workspaces, backend.WorkspaceInfo{ID: "ws-generic"})

	got := backend.DefaultDoDItemsForWorkspace(context.Background(), be, "ws-generic")

	want := []string{"目标已完成，输出包含明确的执行结果"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DoD items = %+v, want %+v", got, want)
	}
}

type fakeBackend struct {
	workspaces []backend.WorkspaceInfo
	templates  []backend.TemplateInfo
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{}
}

func (b *fakeBackend) Workspace() backend.WorkspaceBackend                        { return fakeWorkspaceBackend{b: b} }
func (b *fakeBackend) Goal() backend.GoalBackend                                  { return nil }
func (b *fakeBackend) DoD() backend.DoDBackend                                    { return nil }
func (b *fakeBackend) Run() backend.RunBackend                                    { return nil }
func (b *fakeBackend) Decision() backend.DecisionBackend                          { return nil }
func (b *fakeBackend) Approval() backend.ApprovalBackend                          { return nil }
func (b *fakeBackend) Soul() backend.SoulBackend                                  { return nil }
func (b *fakeBackend) Skill() backend.SkillBackend                                { return nil }
func (b *fakeBackend) MCP() backend.MCPBackend                                    { return nil }
func (b *fakeBackend) Memory() backend.MemoryBackend                              { return nil }
func (b *fakeBackend) Template() backend.TemplateBackend                          { return fakeTemplateBackend{b: b} }
func (b *fakeBackend) Config() backend.ConfigBackend                              { return nil }
func (b *fakeBackend) Events() backend.EventsBackend                              { return nil }
func (b *fakeBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend { return nil }

type fakeWorkspaceBackend struct {
	b *fakeBackend
}

func (b fakeWorkspaceBackend) Create(context.Context, workorchmanagementcontract.WorkspaceCreateRequest) (*backend.WorkspaceInfo, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeWorkspaceBackend) Get(_ context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	for idx := range b.b.workspaces {
		if b.b.workspaces[idx].ID == id {
			return &b.b.workspaces[idx], nil
		}
	}
	return nil, common.ErrNotFound
}

func (b fakeWorkspaceBackend) List(context.Context) ([]backend.WorkspaceInfo, error) {
	return append([]backend.WorkspaceInfo(nil), b.b.workspaces...), nil
}

func (b fakeWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	workspace, err := b.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &backend.WorkspaceSnapshotInfo{Workspace: *workspace}, nil
}

func (b fakeWorkspaceBackend) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (b fakeWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	return b.Get(ctx, id)
}

func (b fakeWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}

type fakeTemplateBackend struct {
	b *fakeBackend
}

func (b fakeTemplateBackend) Create(context.Context, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeTemplateBackend) Get(context.Context, string) (*backend.TemplateInfo, error) {
	return nil, common.ErrNotFound
}

func (b fakeTemplateBackend) List(context.Context) ([]backend.TemplateInfo, error) {
	return append([]backend.TemplateInfo(nil), b.b.templates...), nil
}

func (b fakeTemplateBackend) Update(context.Context, string, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeTemplateBackend) Delete(context.Context, string) error {
	return nil
}
