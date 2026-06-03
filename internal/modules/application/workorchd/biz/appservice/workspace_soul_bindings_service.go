package appservice

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	SoulBindingStagePlanning   = "planning"
	SoulBindingStageExecution  = "execution"
	SoulBindingStageDecision   = "decision"
	SoulBindingStageApproval   = "approval"
	SoulBindingStageValidation = "validation"
	SoulBindingStageArtifact   = "artifact"
)

var soulBindingStages = []string{
	SoulBindingStagePlanning,
	SoulBindingStageExecution,
	SoulBindingStageDecision,
	SoulBindingStageApproval,
	SoulBindingStageValidation,
	SoulBindingStageArtifact,
}

var soulRoutingPriorityPolicy = []string{
	"run_stage_override",
	"workspace_stage_binding",
	"run_default",
	"workspace_default",
	"system_default",
	"unbound",
}

type WorkspaceSoulBindings struct {
	WorkspaceID     string            `json:"workspaceId,omitempty"`
	DefaultSoulID   string            `json:"defaultSoulId,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
	PriorityPolicy  []string          `json:"priorityPolicy,omitempty"`
	AvailableStages []string          `json:"availableStages,omitempty"`
}

type SoulRoutingBinding struct {
	SoulID string         `json:"soulId"`
	Source string         `json:"source"`
	Soul   map[string]any `json:"soul,omitempty"`
}

type SoulRouting struct {
	Status                 string                        `json:"status"`
	Reason                 string                        `json:"reason,omitempty"`
	PriorityPolicy         []string                      `json:"priorityPolicy,omitempty"`
	EffectiveDefaultSoulID string                        `json:"effectiveDefaultSoulId,omitempty"`
	Bindings               map[string]SoulRoutingBinding `json:"bindings,omitempty"`
}

func ReadWorkspaceSoulBindingsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSoulBindings, err error) {
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	if exports.Infrastructure.EventHub == nil {
		return WorkspaceSoulBindings{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := readWorkspaceByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	return workspaceSoulBindingsFromModel(item.ID, item.DefaultSoulID, item.SoulBindings), nil
}

func ReadWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSoulBindings, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	return ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
}

func UpdateWorkspaceSoulBindingsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd WorkspaceSoulBindings) (ret WorkspaceSoulBindings, err error) {
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	if exports.Infrastructure.EventHub == nil {
		return WorkspaceSoulBindings{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := readWorkspaceByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	defaultSoulID := common.SoulID(strings.TrimSpace(cmd.DefaultSoulID))
	bindings, err := normalizeSoulBindings(cmd.Bindings)
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	if defaultSoulID != "" {
		if _, err := readSoulByOwnerCommand(ctx, exports.Infrastructure.EventHub, defaultSoulID); err != nil {
			return WorkspaceSoulBindings{}, err
		}
	}
	for _, soulID := range bindings {
		if soulID == "" {
			continue
		}
		if _, err := readSoulByOwnerCommand(ctx, exports.Infrastructure.EventHub, soulID); err != nil {
			return WorkspaceSoulBindings{}, err
		}
	}

	item.DefaultSoulID = defaultSoulID
	item.SoulBindings = bindings
	if err := publishOwnerCommand(ctx, exports.Infrastructure.EventHub, events.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, map[string]any{
		events.PayloadKeyWorkspace: item,
	}); err != nil {
		return WorkspaceSoulBindings{}, err
	}
	return workspaceSoulBindingsFromModel(item.ID, item.DefaultSoulID, item.SoulBindings), nil
}

func UpdateWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd WorkspaceSoulBindings) (WorkspaceSoulBindings, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSoulBindings{}, err
	}
	return UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
}

func workspaceSoulBindingsFromModel(workspaceID common.WorkspaceID, defaultSoulID common.SoulID, bindings map[string]common.SoulID) WorkspaceSoulBindings {
	return WorkspaceSoulBindings{
		WorkspaceID:     string(workspaceID),
		DefaultSoulID:   string(defaultSoulID),
		Bindings:        mapSoulBindingIDs(bindings),
		PriorityPolicy:  cloneStrings(soulRoutingPriorityPolicy),
		AvailableStages: cloneStrings(soulBindingStages),
	}
}

func normalizeSoulBindings(input map[string]string) (map[string]common.SoulID, error) {
	if len(input) == 0 {
		return nil, nil
	}
	ret := map[string]common.SoulID{}
	for key, value := range input {
		stage := strings.TrimSpace(key)
		if !isSoulBindingStage(stage) {
			return nil, common.ErrInvalidArgument
		}
		soulID := common.SoulID(strings.TrimSpace(value))
		if soulID == "" {
			continue
		}
		ret[stage] = soulID
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func mapSoulBindingIDs(input map[string]common.SoulID) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]string{}
	for _, stage := range soulBindingStages {
		if value := input[stage]; value != "" {
			ret[stage] = string(value)
		}
	}
	return ret
}

func isSoulBindingStage(stage string) bool {
	for _, candidate := range soulBindingStages {
		if stage == candidate {
			return true
		}
	}
	return false
}

func validateSoulCapabilitySelection(ctx context.Context, hub events.Hub, soulID common.SoulID, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if soulID == "" {
		return nil
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	return publishOwnerCommand(ctx, hub, events.TopicSoulCommandValidateCapability, "soul", string(soulID), "", "", map[string]any{
		events.PayloadKeySoulID:               string(soulID),
		events.PayloadKeySkillRefs:            cloneStrings(skillIDs),
		events.PayloadKeyMCPServerRefs:        cloneStrings(mcpServerIDs),
		events.PayloadKeyCapabilityRefs:       cloneStrings(capabilityIDs),
		events.PayloadKeyCapabilityCategories: cloneStrings(capabilityCategories),
		events.PayloadKeyToolKinds:            cloneStrings(toolKinds),
	})
}

func validateSoulRoutingCapabilitySelection(ctx context.Context, hub events.Hub, routing SoulRouting, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if routing.Status != "resolved" {
		return nil
	}
	execution := routing.Bindings[SoulBindingStageExecution]
	soulID := common.SoulID(strings.TrimSpace(execution.SoulID))
	if soulID == "" {
		soulID = common.SoulID(strings.TrimSpace(routing.EffectiveDefaultSoulID))
	}
	return validateSoulCapabilitySelection(ctx, hub, soulID, skillIDs, mcpServerIDs, capabilityIDs, capabilityCategories, toolKinds)
}
