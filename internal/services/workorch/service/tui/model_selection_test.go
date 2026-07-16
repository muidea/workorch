package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type modelSelectionBackend struct {
	*fakeBE
	workspace *modelSelectionWorkspaceBackend
	config    *modelSelectionConfigBackend
}

func (b *modelSelectionBackend) Workspace() backend.WorkspaceBackend { return b.workspace }
func (b *modelSelectionBackend) Config() backend.ConfigBackend       { return b.config }

type modelSelectionWorkspaceBackend struct {
	*fakeWS
	lastTarget *runtimecontract.ProviderModelTarget
}

func (b *modelSelectionWorkspaceBackend) UpdateLLMTarget(_ context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	b.lastTarget = cloneModelTarget(target)
	return &backend.WorkspaceInfo{ID: id, Title: "Workspace", State: common.WorkspaceStateActive, DefaultLLMTarget: cloneModelTarget(target)}, nil
}

type modelSelectionConfigBackend struct {
	*fakeC
	options []backend.ChatModelTargetOption
}

func appModelFromTea(t *testing.T, model tea.Model) AppModel {
	t.Helper()
	switch value := model.(type) {
	case AppModel:
		return value
	case *AppModel:
		return *value
	default:
		t.Fatalf("tea model type = %T", model)
		return AppModel{}
	}
}

func (b *modelSelectionConfigBackend) Get(context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{DefaultChat: backend.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "default-model"}, ChatModelTargets: append([]backend.ChatModelTargetOption(nil), b.options...)}, nil
}

func (b *modelSelectionConfigBackend) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return append([]backend.ChatModelTargetOption(nil), b.options...), nil
}

func (b *modelSelectionConfigBackend) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return append([]backend.ChatModelTargetOption(nil), b.options...), nil
}

func TestModelCommandSelectsAndPersistsProviderModelPair(t *testing.T) {
	be := &modelSelectionBackend{
		fakeBE:    newFake(t),
		workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}},
		config: &modelSelectionConfigBackend{fakeC: &fakeC{}, options: []backend.ChatModelTargetOption{
			{ProviderID: "custom", ModelID: "model-b", CapabilityReady: true},
			{ProviderID: "ai-proxy", ModelID: "default-model", CapabilityReady: true},
		}},
	}
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-1", Title: "Workspace", State: common.WorkspaceStateActive}}
	m.activeWorkspaceID = "ws-1"
	m.setInput("/model")
	next, cmd := m.submitInput()
	m = appModelFromTea(t, next)
	if !m.modelSelectMode || cmd == nil {
		t.Fatalf("model selection state = mode:%v cmd:%v", m.modelSelectMode, cmd)
	}
	loaded := cmd().(ChatModelTargetsLoadedMsg)
	next, _ = m.Update(loaded)
	m = appModelFromTea(t, next)
	if len(m.modelSelectOptions) != 2 || m.modelSelectOptions[0].ProviderID != "ai-proxy" || m.modelSelectOptions[0].ModelID != "default-model" {
		t.Fatalf("ordered options = %+v", m.modelSelectOptions)
	}
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = appModelFromTea(t, next)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = appModelFromTea(t, next)
	if !m.modelSelectConfirming || m.modelSelectPending == nil || m.modelSelectPending.ProviderID != "custom" || m.modelSelectPending.ModelID != "model-b" {
		t.Fatalf("pending target = %+v", m.modelSelectPending)
	}
	next, saveCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = appModelFromTea(t, next)
	if saveCmd == nil {
		t.Fatal("second enter did not submit")
	}
	updated := saveCmd().(WorkspaceLLMTargetUpdatedMsg)
	next, _ = m.Update(updated)
	m = appModelFromTea(t, next)
	if m.modelSelectMode {
		t.Fatal("selection mode remained open after save")
	}
	if be.workspace.lastTarget == nil || be.workspace.lastTarget.ProviderID != "custom" || be.workspace.lastTarget.ModelID != "model-b" {
		t.Fatalf("persisted target = %+v", be.workspace.lastTarget)
	}
}

func TestModelClearRequiresConfirmationAndPersistsNil(t *testing.T) {
	be := &modelSelectionBackend{fakeBE: newFake(t), workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}}, config: &modelSelectionConfigBackend{fakeC: &fakeC{}}}
	m := NewAppModel(be, "ws-1", "")
	target := runtimecontract.ProviderModelTarget{ProviderID: "custom", ModelID: "model-a"}
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-1", Title: "Workspace", State: common.WorkspaceStateActive, DefaultLLMTarget: &target}}
	m.activeWorkspaceID = "ws-1"
	m.setInput("/model clear")
	next, cmd := m.submitInput()
	m = appModelFromTea(t, next)
	if cmd != nil || !m.modelSelectClearConfirming || !m.modelSelectMode {
		t.Fatalf("clear confirmation state = mode:%v clear:%v cmd:%v", m.modelSelectMode, m.modelSelectClearConfirming, cmd)
	}
	next, saveCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = appModelFromTea(t, next)
	if saveCmd == nil {
		t.Fatal("clear confirmation did not submit")
	}
	updated := saveCmd().(WorkspaceLLMTargetUpdatedMsg)
	next, _ = m.Update(updated)
	m = appModelFromTea(t, next)
	if be.workspace.lastTarget != nil {
		t.Fatalf("clear persisted target = %+v, want nil", be.workspace.lastTarget)
	}
}
