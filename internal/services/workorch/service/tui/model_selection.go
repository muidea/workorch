package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *AppModel) beginModelSelection(clear bool) tea.Cmd {
	if m.activeWorkspaceID == "" {
		m.lastError = fmt.Errorf("no active workspace")
		return nil
	}
	m.cancelWorkspaceSelection()
	m.cancelModelSelection()
	m.modelSelectMode = true
	m.modelSelectWorkspaceID = m.activeWorkspaceID
	m.modelSelectRequestSeq++
	m.modelSelectCurrent = runtimecontract.CloneProviderModelTarget(m.activeWorkspaceOverviewTarget())
	m.modelSelectClearConfirming = clear
	m.modelSelectConfirming = clear
	m.modelSelectLoading = !clear
	m.modelSelectError = nil
	m.lastError = nil
	if clear {
		m.progressText = "Confirm clearing workspace model selection"
		return nil
	}
	m.progressText = "Loading available chat models"
	return FetchChatModelTargets(m.be, m.activeWorkspaceID, m.modelSelectRequestSeq, false)
}

func (m *AppModel) activeWorkspaceOverviewTarget() *runtimecontract.ProviderModelTarget {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return nil
	}
	return ws.DefaultLLMTarget
}

func displayModelTarget(target *runtimecontract.ProviderModelTarget, empty string) string {
	if target == nil || target.Validate() != nil {
		return empty
	}
	return target.ProviderID + "/" + target.ModelID
}

func (m *AppModel) cancelModelSelection() {
	m.modelSelectMode = false
	m.modelSelectLoading = false
	m.modelSelectRefreshing = false
	m.modelSelectConfirming = false
	m.modelSelectSubmitting = false
	m.modelSelectClearConfirming = false
	m.modelSelectCursor = 0
	m.modelSelectOptions = nil
	m.modelSelectCurrent = nil
	m.modelSelectPending = nil
	m.modelSelectDefault = runtimecontract.ProviderModelTarget{}
	m.modelSelectError = nil
	m.modelSelectWorkspaceID = ""
}

func (m *AppModel) handleModelSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if msg.Type == tea.KeyRunes {
		// A modal must not swallow the sole explicit quit command.  Keep this
		// deliberately narrow: regular input remains unavailable while choosing
		// a model, but /exit is accepted incrementally and handled on Enter.
		candidate := m.input + string(msg.Runes)
		if strings.HasPrefix("/exit", candidate) {
			m.insertInput(string(msg.Runes))
			return m, nil
		}
	}
	if key == KeyEnter && strings.TrimSpace(m.input) == "/exit" {
		m.addInputHistory(m.input)
		m.mode = modeQuit
		return m, tea.Quit
	}
	switch key {
	case "esc":
		m.cancelModelSelection()
		m.progressText = "Model selection cancelled"
		return m, nil
	case "up", KeyUp:
		if !m.modelSelectConfirming && len(m.modelSelectOptions) > 0 {
			m.modelSelectCursor--
			if m.modelSelectCursor < 0 {
				m.modelSelectCursor = len(m.modelSelectOptions) - 1
			}
		}
		return m, nil
	case "down", KeyDown:
		if !m.modelSelectConfirming && len(m.modelSelectOptions) > 0 {
			m.modelSelectCursor = (m.modelSelectCursor + 1) % len(m.modelSelectOptions)
		}
		return m, nil
	case "r":
		if m.modelSelectSubmitting || m.modelSelectClearConfirming {
			return m, nil
		}
		m.modelSelectConfirming = false
		m.modelSelectPending = nil
		m.modelSelectRequestSeq++
		m.modelSelectRefreshing = true
		m.modelSelectError = nil
		return m, FetchChatModelTargets(m.be, m.modelSelectWorkspaceID, m.modelSelectRequestSeq, true)
	case KeyEnter:
		if m.modelSelectSubmitting || m.modelSelectLoading {
			return m, nil
		}
		if m.modelSelectClearConfirming {
			m.modelSelectSubmitting = true
			return m, PersistWorkspaceLLMTarget(m.be, m.modelSelectWorkspaceID, m.modelSelectRequestSeq, nil)
		}
		if !m.modelSelectConfirming {
			if len(m.modelSelectOptions) == 0 {
				return m, nil
			}
			option := m.modelSelectOptions[m.modelSelectCursor]
			if !option.CapabilityReady {
				m.modelSelectError = fmt.Errorf("selected target %s/%s is unavailable; refresh the catalog or choose a ready target", option.ProviderID, option.ModelID)
				return m, nil
			}
			target := runtimecontract.ProviderModelTarget{ProviderID: option.ProviderID, ModelID: option.ModelID}.Normalized()
			m.modelSelectPending = &target
			m.modelSelectConfirming = true
			return m, nil
		}
		if m.modelSelectPending == nil {
			return m, nil
		}
		m.modelSelectSubmitting = true
		return m, PersistWorkspaceLLMTarget(m.be, m.modelSelectWorkspaceID, m.modelSelectRequestSeq, m.modelSelectPending)
	default:
		return m, nil
	}
}

func FetchChatModelTargets(be backend.Backend, workspaceID string, requestSeq int64, refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if be == nil || be.Config() == nil {
			return ChatModelTargetsLoadedMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, Refreshing: refresh, Error: common.ErrInvalidArgument}
		}
		var (
			options []backend.ChatModelTargetOption
			err     error
		)
		if refresh {
			options, err = be.Config().RefreshChatModelTargets(ctx)
		} else {
			options, err = be.Config().ListChatModelTargets(ctx)
		}
		cfg, cfgErr := be.Config().Get(ctx)
		if err == nil && cfgErr != nil {
			err = cfgErr
		}
		return ChatModelTargetsLoadedMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, Refreshing: refresh, Options: options, Config: cfg, Error: err}
	}
}

func PersistWorkspaceLLMTarget(be backend.Backend, workspaceID string, requestSeq int64, target *runtimecontract.ProviderModelTarget) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if be == nil || be.Workspace() == nil {
			return WorkspaceLLMTargetUpdatedMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, Error: common.ErrInvalidArgument}
		}
		item, err := be.Workspace().UpdateLLMTarget(ctx, common.WorkspaceID(workspaceID), runtimecontract.CloneProviderModelTarget(target))
		return WorkspaceLLMTargetUpdatedMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, Workspace: item, Target: runtimecontract.CloneProviderModelTarget(target), Cleared: target == nil, Error: err}
	}
}

func FetchWorkspaceLLMTargetShow(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if be == nil || be.Config() == nil {
			return WorkspaceLLMTargetShowMsg{WorkspaceID: workspaceID, Error: common.ErrInvalidArgument}
		}
		cfg, err := be.Config().Get(ctx)
		return WorkspaceLLMTargetShowMsg{WorkspaceID: workspaceID, Config: cfg, Error: err}
	}
}

func (m *AppModel) applyChatModelTargetsLoaded(msg ChatModelTargetsLoadedMsg) (tea.Model, tea.Cmd) {
	if !m.modelSelectMode || msg.WorkspaceID != m.modelSelectWorkspaceID || msg.WorkspaceID != m.activeWorkspaceID || msg.RequestSeq != m.modelSelectRequestSeq {
		return m, nil
	}
	m.modelSelectLoading = false
	m.modelSelectRefreshing = false
	if msg.Error != nil {
		if errors.Is(msg.Error, common.ErrNotFound) {
			m.cancelModelSelection()
			m.setLastError(msg.Error)
			return m, nil
		}
		m.modelSelectError = msg.Error
		return m, nil
	}
	if msg.Config != nil {
		m.modelSelectDefault = runtimecontract.ProviderModelTarget{ProviderID: msg.Config.DefaultChat.ProviderID, ModelID: msg.Config.DefaultChat.ModelID}.Normalized()
	}
	m.modelSelectOptions = orderModelOptions(msg.Options, m.modelSelectCurrent, m.modelSelectDefault)
	m.modelSelectCursor = 0
	m.modelSelectError = nil
	return m, nil
}

func orderModelOptions(items []backend.ChatModelTargetOption, current *runtimecontract.ProviderModelTarget, systemDefault runtimecontract.ProviderModelTarget) []backend.ChatModelTargetOption {
	ret := backend.NormalizeChatModelTargetOptions(items)
	if current != nil && current.Validate() == nil {
		found := false
		for _, item := range ret {
			if (runtimecontract.ProviderModelTarget{ProviderID: item.ProviderID, ModelID: item.ModelID}).Same(*current) {
				found = true
				break
			}
		}
		if !found {
			ret = append(ret, backend.ChatModelTargetOption{
				ProviderID:      current.ProviderID,
				ModelID:         current.ModelID,
				DisplayName:     current.ModelID,
				CapabilityReady: false,
				Source:          "workspace_persisted_unavailable",
			})
		}
	}
	providerScore := func(item backend.ChatModelTargetOption) int {
		if current != nil && item.ProviderID == current.ProviderID {
			return 0
		}
		if systemDefault.Validate() == nil && item.ProviderID == systemDefault.ProviderID {
			return 1
		}
		return 2
	}
	modelScore := func(item backend.ChatModelTargetOption) int {
		target := runtimecontract.ProviderModelTarget{ProviderID: item.ProviderID, ModelID: item.ModelID}
		if current != nil && target.Same(*current) {
			return 0
		}
		if systemDefault.Validate() == nil && target.Same(systemDefault) {
			return 1
		}
		return 2
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if left, right := providerScore(ret[i]), providerScore(ret[j]); left != right {
			return left < right
		}
		if ret[i].ProviderID != ret[j].ProviderID {
			return ret[i].ProviderID < ret[j].ProviderID
		}
		if left, right := modelScore(ret[i]), modelScore(ret[j]); left != right {
			return left < right
		}
		return ret[i].ModelID < ret[j].ModelID
	})
	return ret
}

func (m *AppModel) applyWorkspaceLLMTargetUpdated(msg WorkspaceLLMTargetUpdatedMsg) (tea.Model, tea.Cmd) {
	if !m.modelSelectMode || msg.WorkspaceID != m.modelSelectWorkspaceID || msg.WorkspaceID != m.activeWorkspaceID || msg.RequestSeq != m.modelSelectRequestSeq {
		return m, nil
	}
	m.modelSelectSubmitting = false
	if msg.Error != nil {
		if errors.Is(msg.Error, common.ErrNotFound) {
			m.cancelModelSelection()
			m.setLastError(msg.Error)
			return m, nil
		}
		m.modelSelectError = msg.Error
		return m, nil
	}
	if msg.Workspace != nil {
		m.upsertWorkspace(*msg.Workspace)
	}
	label := displayModelTarget(msg.Target, "inherit system default")
	sealed := displayModelTarget(m.activeWorkspaceSealedTarget(), "no active sealed target")
	m.cancelModelSelection()
	m.lastError = nil
	m.result = "Workspace model updated\n- next-run workspace default: " + label + "\n- current-run sealed target: " + sealed
	switch m.activeWorkspacePromptMode() {
	case workspacePromptModeDecision:
		m.result += "\n- next: resolve the current decision, then use /run resume"
	case workspacePromptModeApproval:
		m.result += "\n- next: resolve the current approval, then use /run resume"
	default:
		m.result += "\n- continue with: /run resume"
	}
	m.progressText = "Workspace model configuration saved"
	m.resetMainViewport()
	return m, m.requestWorkspaceListRefresh()
}

func (m *AppModel) applyWorkspaceLLMTargetShow(msg WorkspaceLLMTargetShowMsg) (tea.Model, tea.Cmd) {
	if msg.WorkspaceID != m.activeWorkspaceID {
		return m, nil
	}
	if msg.Error != nil {
		m.setLastError(msg.Error)
		return m, nil
	}
	workspaceTarget := displayModelTarget(m.activeWorkspaceOverviewTarget(), "inherit system default")
	systemTarget := "unavailable"
	if msg.Config != nil {
		systemTarget = msg.Config.DefaultChat.ProviderID + "/" + msg.Config.DefaultChat.ModelID
	}
	sealedTarget := displayModelTarget(m.activeWorkspaceSealedTarget(), "no active sealed target")
	m.result = "Workspace model configuration\n- next-run workspace default: " + workspaceTarget + "\n- current-run sealed target: " + sealedTarget + "\n- system default: " + systemTarget
	m.progressText = "Showing workspace model configuration"
	m.lastError = nil
	m.resetMainViewport()
	return m, nil
}

func (m AppModel) renderModelSelection(width, height int) string {
	ws := m.activeWorkspaceOverview()
	title := m.modelSelectWorkspaceID
	if ws != nil && strings.TrimSpace(ws.Title) != "" {
		title = ws.Title + " (" + string(ws.ID) + ")"
	}
	lines := []string{"Workspace model selection", "", "Workspace: " + title}
	current := "inherit system default"
	if m.modelSelectCurrent != nil {
		current = m.modelSelectCurrent.ProviderID + "/" + m.modelSelectCurrent.ModelID
	}
	lines = append(lines, "Current: "+current)
	if m.modelSelectClearConfirming {
		lines = append(lines, "", "Clear the workspace target?", "Future runs will inherit the latest system default.", "Current run remains unchanged.", "", "Enter: confirm clear | Esc: cancel")
		return padLines(strings.Join(lines, "\n"), height)
	}
	if m.modelSelectLoading {
		lines = append(lines, "", "Loading available chat models...")
	} else if m.modelSelectConfirming && m.modelSelectPending != nil {
		lines = append(lines, "", "Change to: "+m.modelSelectPending.ProviderID+"/"+m.modelSelectPending.ModelID, "Future new/replacement runs will use this target.", "Current run remains unchanged.", "", "Enter: confirm save | Esc: cancel")
	} else {
		lines = append(lines, "", "Available provider/model targets:")
		if len(m.modelSelectOptions) == 0 {
			lines = append(lines, "  No ready chat models available.")
		}
		for i, option := range m.modelSelectOptions {
			prefix := "  "
			if i == m.modelSelectCursor {
				prefix = "> "
			}
			label := option.ProviderID + "/" + option.ModelID
			if !option.CapabilityReady {
				label += " [unavailable]"
			}
			lines = append(lines, truncateDisplay(prefix+label, width-2))
		}
		lines = append(lines, "", "↑/↓ or j/k: select | Enter: review | r: refresh | Esc: cancel")
	}
	if m.modelSelectRefreshing {
		lines = append(lines, "", "Refreshing model catalog...")
	}
	if m.modelSelectSubmitting {
		lines = append(lines, "", "Saving workspace model configuration...")
	}
	if m.modelSelectError != nil {
		lines = append(lines, "", "Error: "+m.modelSelectError.Error())
	}
	return padLines(strings.Join(lines, "\n"), height)
}
