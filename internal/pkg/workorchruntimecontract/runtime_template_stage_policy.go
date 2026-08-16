package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

const RuntimeTemplateStagePolicySchemaCurrent = "workorch.template_stage_policy"

type RuntimeTemplateStagePolicy struct {
	SchemaVersion string
	Stages        []RuntimeTemplateStage
}

type RuntimeTemplateStage struct {
	ID              string
	SoulBinding     string
	RequiredOutputs []RuntimeTemplateStageRequiredOutput
	Completion      RuntimeTemplateStageCompletion
	Governance      RuntimeTemplateStageGovernance
	PolicyRefs      RuntimeTemplateStagePolicyRefs
}

type RuntimeTemplateStageRequiredOutput struct {
	Kind          string
	EvidenceKinds []string
}

type RuntimeTemplateStageCompletion struct {
	Policy string
}

type RuntimeTemplateStageGovernance struct {
	DecisionMode string
	ApprovalMode string
}

type RuntimeTemplateStagePolicyRefs struct {
	EvidencePolicies        []string
	CapabilityUsagePolicies []string
}

func (p RuntimeTemplateStagePolicy) Normalized() RuntimeTemplateStagePolicy {
	p.SchemaVersion = strings.TrimSpace(p.SchemaVersion)
	byID := make(map[string]RuntimeTemplateStage, len(p.Stages))
	for _, stage := range p.Stages {
		stage.ID = strings.TrimSpace(stage.ID)
		stage.SoulBinding = strings.TrimSpace(stage.SoulBinding)
		stage.Completion.Policy = strings.TrimSpace(stage.Completion.Policy)
		stage.Governance.DecisionMode = strings.TrimSpace(stage.Governance.DecisionMode)
		stage.Governance.ApprovalMode = strings.TrimSpace(stage.Governance.ApprovalMode)
		stage.PolicyRefs.EvidencePolicies = normalizeStrings(stage.PolicyRefs.EvidencePolicies)
		stage.PolicyRefs.CapabilityUsagePolicies = normalizeStrings(stage.PolicyRefs.CapabilityUsagePolicies)
		outputs := make([]RuntimeTemplateStageRequiredOutput, 0, len(stage.RequiredOutputs))
		for _, output := range stage.RequiredOutputs {
			output.Kind = strings.TrimSpace(output.Kind)
			output.EvidenceKinds = normalizeStrings(output.EvidenceKinds)
			if output.Kind != "" {
				outputs = append(outputs, output)
			}
		}
		sort.Slice(outputs, func(i, j int) bool { return outputs[i].Kind < outputs[j].Kind })
		stage.RequiredOutputs = nilIfEmpty(outputs)
		if stage.ID != "" {
			byID[stage.ID] = stage
		}
	}
	p.Stages = sortedMapValues(byID)
	return p
}

func (p RuntimeTemplateStagePolicy) Validate() error {
	p = p.Normalized()
	if p.SchemaVersion != RuntimeTemplateStagePolicySchemaCurrent {
		return fmt.Errorf("runtime template stage policy has invalid schema %q", p.SchemaVersion)
	}
	if len(p.Stages) == 0 {
		return fmt.Errorf("runtime template stage policy requires stages")
	}
	for _, stage := range p.Stages {
		if stage.ID == "" {
			return fmt.Errorf("runtime template stage requires id")
		}
		for _, output := range stage.RequiredOutputs {
			if output.Kind == "" || len(output.EvidenceKinds) == 0 {
				return fmt.Errorf("runtime template stage required output requires kind and evidence kinds")
			}
		}
	}
	return nil
}

func SameRuntimeTemplateStagePolicy(left *RuntimeTemplateStagePolicy, right *RuntimeTemplateStagePolicy) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	if l.SchemaVersion != r.SchemaVersion || len(l.Stages) != len(r.Stages) {
		return false
	}
	for idx := range l.Stages {
		ls, rs := l.Stages[idx], r.Stages[idx]
		if ls.ID != rs.ID || ls.SoulBinding != rs.SoulBinding || ls.Completion != rs.Completion || ls.Governance != rs.Governance ||
			len(ls.RequiredOutputs) != len(rs.RequiredOutputs) || !sameStrings(ls.PolicyRefs.EvidencePolicies, rs.PolicyRefs.EvidencePolicies) ||
			!sameStrings(ls.PolicyRefs.CapabilityUsagePolicies, rs.PolicyRefs.CapabilityUsagePolicies) {
			return false
		}
		for outputIdx := range ls.RequiredOutputs {
			if ls.RequiredOutputs[outputIdx].Kind != rs.RequiredOutputs[outputIdx].Kind ||
				!sameStrings(ls.RequiredOutputs[outputIdx].EvidenceKinds, rs.RequiredOutputs[outputIdx].EvidenceKinds) {
				return false
			}
		}
	}
	return true
}
