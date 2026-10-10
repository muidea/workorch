package governance

import (
	"context"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type approvalEvaluation struct {
	canAutoApprove    bool
	infrastructureErr error
	riskLevel         string
	confidence        float64
	reason            string
	policyMatched     string
	policyBlocked     string
	budgetBefore      int
	budgetAfter       int
	promptKey         string
	evidenceRefs      []string
}

func (s *Service) evaluateApproval(ctx context.Context, approval *approvalmodel.Approval, goal *workspace.Goal, dod *workspace.DoD, governanceCfg workorchconfig.GovernanceConfig) approvalEvaluation {
	ret := approvalEvaluation{
		canAutoApprove: false,
		riskLevel:      "L3",
		confidence:     0,
		reason:         "approval requires manual review",
		policyMatched:  "",
		policyBlocked:  "policy_manual_required",
		evidenceRefs:   []string{string(approval.ID)},
	}
	if approval == nil || approval.ID == "" {
		ret.policyBlocked = "invalid_approval"
		ret.reason = "invalid approval item"
		return ret
	}
	if approval.State != common.ApprovalStatePending {
		ret.policyBlocked = "approval_not_pending"
		ret.reason = "approval already resolved"
		return ret
	}
	ret.promptKey = compactPromptKey(approval.Action + "\n" + approval.Reason + "\n" + stableContextDigest(approval.Context))
	resolvedCount, err := s.countResolvedApprovalTraces(ctx, approval.WorkspaceID, approval.RunID)
	if err != nil {
		ret.infrastructureErr = err
		ret.policyBlocked = "trace_query_failed"
		ret.reason = "governance trace budget is unavailable"
		return ret
	}
	ret.budgetBefore = resolvedCount
	maxPerRun := governanceCfg.AutoApproval.MaxPerRun
	if maxPerRun <= 0 {
		maxPerRun = 3
	}
	if resolvedCount >= maxPerRun {
		ret.policyBlocked = "budget_exhausted"
		ret.reason = "auto approval budget exhausted"
		return ret
	}
	blockedReason, blocked, err := s.blockedBySharedBudgetPolicy(ctx, governanceCfg, approval.WorkspaceID, approval.RunID, "approval", string(approval.ID), ret.promptKey)
	if err != nil {
		ret.infrastructureErr = err
		ret.policyBlocked = "trace_query_failed"
		ret.reason = "governance trace policy history is unavailable"
		return ret
	}
	if blocked {
		ret.policyBlocked = blockedReason
		ret.reason = "auto governance budget policy blocked approval resolution"
		return ret
	}

	riskLevel, riskSignals := classifyApprovalRisk(approval)
	ret.riskLevel = riskLevel
	ret.evidenceRefs = append(ret.evidenceRefs, riskSignals...)
	if requiresHumanByPolicy(riskSignals, governanceCfg.AutoApproval.RequireHumanFor) {
		ret.policyBlocked = "require_human_policy_matched"
		ret.reason = "approval matched requireHumanFor policy"
		return ret
	}
	if !withinDoDBoundary(approval, goal, dod) {
		ret.policyBlocked = "dod_mismatch"
		ret.reason = "approval is outside current goal/DoD scope"
		return ret
	}
	switch riskLevel {
	case "L1":
		if !isEnabled(governanceCfg.AutoApproval.AllowReadOnly) {
			ret.policyBlocked = "read_only_auto_disabled"
			ret.reason = "read-only auto approval disabled by policy"
			return ret
		}
		ret.canAutoApprove = true
		ret.policyMatched = "read_only"
		ret.confidence = 0.96
		ret.reason = "read-only approval auto-resolved by policy"
	case "L2":
		if !isEnabled(governanceCfg.AutoApproval.AllowWorkspaceWrite) {
			ret.policyBlocked = "workspace_write_auto_disabled"
			ret.reason = "workspace write auto approval disabled by policy"
			return ret
		}
		ret.canAutoApprove = true
		ret.policyMatched = "workspace_low_risk_write"
		ret.confidence = 0.9
		ret.reason = "workspace low-risk write auto-approved by policy"
	default:
		ret.policyBlocked = "risk_level_requires_manual"
		ret.reason = "risk level requires manual approval"
		return ret
	}
	minConfidence := governanceCfg.AutoApproval.MinConfidence
	if minConfidence <= 0 {
		minConfidence = 0.9
	}
	if ret.confidence < minConfidence {
		ret.canAutoApprove = false
		ret.policyMatched = ""
		ret.policyBlocked = "low_confidence"
		ret.reason = "auto approval confidence below threshold"
		return ret
	}
	ret.policyBlocked = ""
	ret.budgetAfter = resolvedCount + 1
	return ret
}

func classifyApprovalRisk(approval *approvalmodel.Approval) (string, []string) {
	if approval == nil {
		return "L4", []string{"invalid_approval"}
	}
	signals := collectApprovalSignals(approval)
	if containsAnyToken(signals, []string{"secret_access", "production_operation", "external_publish", "destructive_file_change"}) {
		return "L4", signals
	}
	if containsAnyToken(signals, []string{"ssh_write", "external_write", "remote_operation", "network_write", "large_change"}) {
		return "L3", signals
	}
	if containsAnyToken(signals, []string{
		"workspace_write",
		"patch_apply",
		"patch_revert",
		"workspace_dependency_metadata",
		"low_risk_workspace_dependency_metadata",
	}) {
		return "L2", signals
	}
	if containsAnyToken(signals, []string{"read_only", "query", "list", "search"}) {
		return "L1", signals
	}
	return "L3", signals
}

func collectApprovalSignals(approval *approvalmodel.Approval) []string {
	signals := make([]string, 0, 8)
	actionText := normalize(approval.Action + " " + approval.Reason)
	contextText := normalize(flattenApprovalContextText(approval.Context))
	if actionText != "" {
		if containsAnyText(actionText, []string{"read", "list", "query", "search", "fetch", "inspect", "describe", "show"}) {
			signals = append(signals, "read_only")
		}
		if containsAnyText(actionText, []string{"write", "edit", "apply", "patch", "revert", "create", "update"}) {
			signals = append(signals, "workspace_write")
		}
		if containsAnyText(actionText, []string{"delete", "remove", "drop", "truncate", "overwrite"}) {
			signals = append(signals, "destructive_file_change")
		}
		if containsRemoteOperationSignal(actionText, approval.Context) {
			signals = append(signals, "remote_operation")
		}
		if containsAnyText(actionText, []string{"deploy", "publish", "release", "prod", "production"}) {
			signals = append(signals, "production_operation")
		}
	}
	if declaresLowRiskWorkspaceDependencyMetadata(approval, actionText, contextText) {
		signals = append(signals, "workspace_dependency_metadata")
	}
	contextRisk := normalize(stringFromMap(approval.Context, "risk"))
	if contextRisk != "" {
		signals = append(signals, contextRisk)
	}
	contextKind := normalize(stringFromMap(approval.Context, "kind"))
	if contextKind == "patch_approval_required" {
		signals = append(signals, "patch_apply", "workspace_write")
	}
	if contextKind == "patch_revert_approval_required" {
		signals = append(signals, "patch_revert", "workspace_write")
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "approvalPolicy")), []string{"required_before_apply", "required_before_revert"}) {
		signals = append(signals, "workspace_write")
	}
	if containsRiskReason(approval.Context, "sensitive_path") {
		signals = append(signals, "secret_access")
	}
	if containsRiskReason(approval.Context, "delete_file") || containsRiskReason(approval.Context, "delete_created_file") {
		signals = append(signals, "destructive_file_change")
	}
	if containsRiskReason(approval.Context, "large_change") || containsRiskReason(approval.Context, "lockfile_large_change") {
		signals = append(signals, "large_change")
	}
	if containsAnyText(actionText, []string{"token", "secret", "password", "credential", ".ssh", "key", "pem", "id_rsa", "id_ed25519"}) {
		signals = append(signals, "secret_access")
	}
	return compactUniqueStrings(signals)
}

func declaresLowRiskWorkspaceDependencyMetadata(approval *approvalmodel.Approval, actionText string, contextText string) bool {
	if approval == nil {
		return false
	}
	if actionText == "" && contextText == "" {
		return false
	}
	combined := strings.TrimSpace(actionText + "\n" + contextText)
	if combined == "" {
		return false
	}
	if containsAnyText(combined, []string{"deploy", "publish", "release", "prod", "production", "remote", "ssh", "external", "network"}) {
		return false
	}
	hasWorkspaceWrite := containsAnyText(combined, []string{"write", "edit", "apply", "patch", "create", "update", "tidy", "append"}) ||
		containsAnyText(normalize(stringFromMap(approval.Context, "kind")), []string{"patch_approval_required"})
	if !hasWorkspaceWrite {
		return false
	}
	return containsAnyText(combined, []string{"dependency", "dependencies", "manifest", "metadata", "lockfile", "lock file", "lock", "checksum", "checksums", "module", "modules", "package", "packages", "requirements"})
}

func flattenApprovalContextText(input workorcheventpayload.Payload) string {
	if len(input.Fields) == 0 {
		return ""
	}
	var b strings.Builder
	for _, field := range input.Fields {
		appendApprovalContextValue(&b, field.Value)
	}
	return b.String()
}

func appendApprovalContextValue(b *strings.Builder, value workorcheventpayload.Value) {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if text := strings.TrimSpace(value.String); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(text)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			appendApprovalContextValue(b, item)
		}
	case workorcheventpayload.ValueObject:
		for _, field := range value.Object {
			appendApprovalContextValue(b, field.Value)
		}
	}
}

func containsRemoteOperationSignal(actionText string, contextMap workorcheventpayload.Payload) bool {
	actionText = normalize(actionText)
	if actionText == "" {
		return false
	}
	if containsAnyText(actionText, []string{"ssh", "remote", "远程", "服务器"}) {
		return true
	}
	if containsAnyText(normalize(stringFromMap(contextMap, "risk")), []string{"remote_operation", "ssh_write"}) ||
		containsAnyText(normalize(stringFromMap(contextMap, "kind")), []string{"remote_operation", "ssh_write"}) {
		return true
	}
	for _, phrase := range []string{
		"server host",
		"server endpoint",
		"server operation",
		"server-side operation",
		"server side operation",
	} {
		if strings.Contains(actionText, phrase) {
			return true
		}
	}
	return false
}

func requiresHumanByPolicy(signals []string, requireHumanFor []string) bool {
	if len(signals) == 0 || len(requireHumanFor) == 0 {
		return false
	}
	matched := false
	for _, signal := range signals {
		signal = normalize(signal)
		if signal == "" {
			continue
		}
		for _, required := range requireHumanFor {
			required = normalize(required)
			if required == "" {
				continue
			}
			if signal == required || strings.Contains(signal, required) || strings.Contains(required, signal) {
				matched = true
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func withinDoDBoundary(approval *approvalmodel.Approval, goal *workspace.Goal, dod *workspace.DoD) bool {
	if approval == nil {
		return false
	}
	query := normalize(strings.Join([]string{approval.Reason, approval.Action, stringFromMap(approval.Context, "kind"), stringFromMap(approval.Context, "risk")}, " "))
	if containsAnyText(query, []string{"outside_dod", "out_of_scope", "scope_change", "expand_scope"}) {
		return false
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "scope")), []string{"outside_dod", "out_of_scope"}) {
		return false
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "dodStatus")), []string{"mismatch", "outside"}) {
		return false
	}
	_ = goal
	_ = dod
	return true
}
