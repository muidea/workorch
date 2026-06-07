package appmodelcore

import "github.com/muidea/workorch/internal/pkg/common"

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

func SoulRoutingPriorityPolicy() []string {
	return []string{
		"run_stage_override",
		"workspace_stage_binding",
		"run_default",
		"workspace_default",
		"system_default",
		"unbound",
	}
}

func StageSoulBinding(stage string, runStageBindings map[string]common.SoulID, workspaceStageBindings map[string]common.SoulID, defaultSoulID common.SoulID, defaultSource string) (common.SoulID, string) {
	if value := runStageBindings[stage]; value != "" {
		return value, "run_stage_override"
	}
	if value := workspaceStageBindings[stage]; value != "" {
		return value, "workspace_stage_binding"
	}
	if defaultSoulID != "" {
		return defaultSoulID, defaultSource
	}
	return "", ""
}

func UnboundSoulRouting(reason string) SoulRouting {
	return SoulRouting{
		Status:         "unbound",
		Reason:         reason,
		PriorityPolicy: SoulRoutingPriorityPolicy(),
		Bindings:       map[string]SoulRoutingBinding{},
	}
}

func AppendSoulIDOnce(input []common.SoulID, value common.SoulID) []common.SoulID {
	if value == "" {
		return input
	}
	for _, item := range input {
		if item == value {
			return input
		}
	}
	return append(input, value)
}
