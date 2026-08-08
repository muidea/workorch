package workorchport_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefaultDoDItemsForWorkspaceUsesMatchingTemplate(t *testing.T) {
	be := newFakeBackend()
	be.workspaces = append(be.workspaces, projectioncontract.Workspace{
		ID:            "ws-template",
		SoulBindings:  map[string]string{"execution": "soul-execution"},
		DefaultSoulID: "soul-default",
	})
	be.templates = append(be.templates, workorchmanagementcontract.WorkspaceTemplate{
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
	be.workspaces = append(be.workspaces, projectioncontract.Workspace{ID: "ws-generic"})

	got := backend.DefaultDoDItemsForWorkspace(context.Background(), be, "ws-generic")

	want := []string{"目标已完成，输出包含明确的执行结果"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DoD items = %+v, want %+v", got, want)
	}
}

type fakeBackend struct {
	workspaces []projectioncontract.Workspace
	templates  []workorchmanagementcontract.WorkspaceTemplate
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

func (b fakeWorkspaceBackend) Create(context.Context, workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeWorkspaceBackend) Get(_ context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	for idx := range b.b.workspaces {
		if b.b.workspaces[idx].ID == string(id) {
			return &b.b.workspaces[idx], nil
		}
	}
	return nil, common.ErrNotFound
}

func (b fakeWorkspaceBackend) List(context.Context) ([]projectioncontract.Workspace, error) {
	return projectioncontract.CloneWorkspaces(b.b.workspaces), nil
}

func (b fakeWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	workspace, err := b.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &projectioncontract.WorkspaceSnapshot{Workspace: *workspace}, nil
}

func (b fakeWorkspaceBackend) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (b fakeWorkspaceBackend) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	return b.Get(ctx, id)
}

func (b fakeWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}

type fakeTemplateBackend struct {
	b *fakeBackend
}

func (b fakeTemplateBackend) Create(context.Context, *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeTemplateBackend) Get(context.Context, string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrNotFound
}

func (b fakeTemplateBackend) List(context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return append([]workorchmanagementcontract.WorkspaceTemplate(nil), b.b.templates...), nil
}

func (b fakeTemplateBackend) Update(context.Context, string, *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrInvalidArgument
}

func (b fakeTemplateBackend) Delete(context.Context, string) error {
	return nil
}
