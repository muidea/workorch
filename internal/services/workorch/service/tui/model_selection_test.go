package tui

import (
	"context"
	"errors"
	"strings"
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
	lastTarget  *runtimecontract.ProviderModelTarget
	updateCalls int
	updateErr   error
}

func (b *modelSelectionWorkspaceBackend) UpdateLLMTarget(_ context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	b.updateCalls++
	if b.updateErr != nil {
		return nil, b.updateErr
	}
	b.lastTarget = runtimecontract.CloneProviderModelTarget(target)
	return &backend.WorkspaceInfo{ID: id, Title: "Workspace", State: common.WorkspaceStateActive, DefaultLLMTarget: runtimecontract.CloneProviderModelTarget(target)}, nil
}

type modelSelectionConfigBackend struct {
	*fakeC
	options      []backend.ChatModelTargetOption
	listErr      error
	refreshErr   error
	refreshCalls int
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
	return &backend.ServiceConfig{DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "default-model"}, ChatModelTargets: append([]backend.ChatModelTargetOption(nil), b.options...)}, nil
}

func (b *modelSelectionConfigBackend) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return append([]backend.ChatModelTargetOption(nil), b.options...), b.listErr
}

func (b *modelSelectionConfigBackend) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	b.refreshCalls++
	return append([]backend.ChatModelTargetOption(nil), b.options...), b.refreshErr
}

func TestModelCommandRequiresActiveWorkspace(t *testing.T) {
	be := &modelSelectionBackend{fakeBE: newFake(t), workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}}, config: &modelSelectionConfigBackend{fakeC: &fakeC{}}}
	m := NewAppModel(be, "", "")
	m.setInput("/model")
	next, cmd := m.submitInput()
	m = appModelFromTea(t, next)
	if cmd != nil || m.modelSelectMode || m.lastError == nil || !strings.Contains(m.lastError.Error(), "no active workspace") {
		t.Fatalf("model without workspace = mode:%v cmd:%v err:%v", m.modelSelectMode, cmd, m.lastError)
	}
}

func TestModelOptionsPreferCurrentWorkspaceProviderGroup(t *testing.T) {
	current := runtimecontract.ProviderModelTarget{ProviderID: "provider-b", ModelID: "model-current"}
	defaultTarget := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-default"}
	got := orderModelOptions([]backend.ChatModelTargetOption{
		{ProviderID: "provider-c", ModelID: "model-c", CapabilityReady: true},
		{ProviderID: "provider-b", ModelID: "model-other", CapabilityReady: true},
		{ProviderID: "provider-a", ModelID: "model-default", CapabilityReady: true},
		{ProviderID: "provider-b", ModelID: "model-current", CapabilityReady: true},
	}, &current, defaultTarget)
	if len(got) != 4 || got[0].ProviderID != "provider-b" || got[0].ModelID != "model-current" || got[1].ProviderID != "provider-b" || got[2].ProviderID != "provider-a" {
		t.Fatalf("ordered options = %+v", got)
	}
}

func TestModelEscCancelsWithoutWrite(t *testing.T) {
	be := &modelSelectionBackend{fakeBE: newFake(t), workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}}, config: &modelSelectionConfigBackend{fakeC: &fakeC{}}}
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-1"}}
	m.activeWorkspaceID = "ws-1"
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = "ws-1"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = appModelFromTea(t, next)
	if cmd != nil || m.modelSelectMode || be.workspace.updateCalls != 0 {
		t.Fatalf("escape = mode:%v cmd:%v writes:%d", m.modelSelectMode, cmd, be.workspace.updateCalls)
	}
}

func TestModelRefreshFailurePreservesLoadedOptions(t *testing.T) {
	be := &modelSelectionBackend{fakeBE: newFake(t), workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}}, config: &modelSelectionConfigBackend{fakeC: &fakeC{}, refreshErr: errors.New("refresh failed")}}
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = "ws-1"
	m.modelSelectRequestSeq = 7
	m.modelSelectOptions = []backend.ChatModelTargetOption{{ProviderID: "existing", ModelID: "model", CapabilityReady: true}}
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = appModelFromTea(t, next)
	if cmd == nil || be.config.refreshCalls != 0 {
		t.Fatalf("refresh command = %v calls=%d", cmd, be.config.refreshCalls)
	}
	msg := cmd().(ChatModelTargetsLoadedMsg)
	next, _ = m.Update(msg)
	m = appModelFromTea(t, next)
	if be.config.refreshCalls != 1 || len(m.modelSelectOptions) != 1 || m.modelSelectOptions[0].ProviderID != "existing" || m.modelSelectError == nil {
		t.Fatalf("refresh failure state = calls:%d options:%+v err:%v", be.config.refreshCalls, m.modelSelectOptions, m.modelSelectError)
	}
}

func TestModelSelectionDiscardsStaleWorkspaceResponse(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-new", "")
	m.activeWorkspaceID = "ws-new"
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = "ws-new"
	m.modelSelectRequestSeq = 3
	next, _ := m.Update(ChatModelTargetsLoadedMsg{RequestSeq: 2, WorkspaceID: "ws-old", Options: []backend.ChatModelTargetOption{{ProviderID: "stale", ModelID: "model", CapabilityReady: true}}})
	m = appModelFromTea(t, next)
	if len(m.modelSelectOptions) != 0 {
		t.Fatalf("stale options applied = %+v", m.modelSelectOptions)
	}
}

func TestUnavailablePersistedTargetIsShownButCannotConfirm(t *testing.T) {
	current := runtimecontract.ProviderModelTarget{ProviderID: "removed-provider", ModelID: "removed-model"}
	options := orderModelOptions([]backend.ChatModelTargetOption{{ProviderID: "ready", ModelID: "model", CapabilityReady: true}}, &current, runtimecontract.ProviderModelTarget{})
	if len(options) != 2 || options[0].CapabilityReady || options[0].ProviderID != current.ProviderID {
		t.Fatalf("options = %+v", options)
	}
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = "ws-1"
	m.modelSelectOptions = options
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = appModelFromTea(t, next)
	if cmd != nil || m.modelSelectConfirming || m.modelSelectError == nil {
		t.Fatalf("unavailable confirm = cmd:%v confirming:%v err:%v", cmd, m.modelSelectConfirming, m.modelSelectError)
	}
}

func TestWorkspaceDeletedDuringModelUpdateExitsSelection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = "ws-1"
	m.modelSelectRequestSeq = 4
	next, _ := m.Update(WorkspaceLLMTargetUpdatedMsg{RequestSeq: 4, WorkspaceID: "ws-1", Error: common.ErrNotFound})
	m = appModelFromTea(t, next)
	if m.modelSelectMode || !errors.Is(m.lastError, common.ErrNotFound) {
		t.Fatalf("deleted workspace state = mode:%v err:%v", m.modelSelectMode, m.lastError)
	}
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

func TestModelShowDistinguishesNextRunDefaultFromCurrentRunSealedTarget(t *testing.T) {
	be := &modelSelectionBackend{fakeBE: newFake(t), workspace: &modelSelectionWorkspaceBackend{fakeWS: &fakeWS{}}, config: &modelSelectionConfigBackend{fakeC: &fakeC{}}}
	m := NewAppModel(be, "ws-model-show", "")
	nextTarget := runtimecontract.ProviderModelTarget{ProviderID: "provider-next", ModelID: "model-next"}
	sealedTarget := runtimecontract.ProviderModelTarget{ProviderID: "provider-current", ModelID: "model-current"}
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-model-show", State: common.WorkspaceStateActive, DefaultLLMTarget: &nextTarget}}
	m.activeWorkspaceID = "ws-model-show"
	entry := m.ensureWorkspaceRuntimeState("ws-model-show")
	entry.Display.ActiveRunID = "run-model-show"
	entry.Display.SealedLLMTarget = &sealedTarget
	entry.HasDisplay = true

	next, _ := m.applyWorkspaceLLMTargetShow(WorkspaceLLMTargetShowMsg{
		WorkspaceID: "ws-model-show",
		Config:      &backend.ServiceConfig{DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "provider-system", ModelID: "model-system"}},
	})
	got := appModelFromTea(t, next)
	for _, want := range []string{
		"next-run workspace default: provider-next/model-next",
		"current-run sealed target: provider-current/model-current",
		"system default: provider-system/model-system",
	} {
		if !strings.Contains(got.result, want) {
			t.Fatalf("model show result = %q, missing %q", got.result, want)
		}
	}
}
