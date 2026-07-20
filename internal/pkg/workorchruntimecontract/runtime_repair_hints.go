package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeRepairHint is a domain-neutral guidance item derived from sealed runtime facts.
// It must not encode language stacks, test frameworks, MCP tool schemas, or product playbooks.
type RuntimeRepairHint struct {
	Code    string
	Message string
}

// DeriveRuntimeRepairHints returns bounded, domain-neutral next-step hints for candidate repair.
// Hints are computed only from evidence ledger, missing required kinds, and failure facts.
func DeriveRuntimeRepairHints(facts RuntimeFacts) []RuntimeRepairHint {
	facts = facts.Normalized()
	ret := make([]RuntimeRepairHint, 0, 4)
	// INV-05: repair directive is the sole next-action authority. Do not emit hints
	// that tell the model to produce only missing kinds when a directive is active.
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID {
		directive := facts.RepairDirective.Normalized()
		ret = append(ret, RuntimeRepairHint{
			Code: "obey_repair_directive",
			Message: fmt.Sprintf(
				"requiredAction=%s; requiredEffectScopes=%s; requiredEvidenceKinds=%s; produce only operations compatible with this directive",
				directive.RequiredAction,
				strings.Join(RuntimeEffectScopesAsStrings(directive.RequiredEffectScopes), ","),
				strings.Join(directive.RequiredEvidenceKinds, ","),
			),
		})
	} else {
		missing := MissingRequiredEvidenceKinds(facts)
		if len(missing) > 0 {
			ret = append(ret, RuntimeRepairHint{
				Code:    "produce_missing_evidence",
				Message: fmt.Sprintf("current focus is missing required evidence kinds: %s; produce only the missing kinds", strings.Join(missing, ", ")),
			})
			if containsStringValue(missing, EvidenceKindValidation) && !containsStringValue(missing, EvidenceKindStateChange) {
				ret = append(ret, RuntimeRepairHint{
					Code:    "prefer_validation_over_state_change",
					Message: "accepted state_change evidence already exists for this focus; prefer produce_validation instead of another unrelated state_change",
				})
			}
		}
	}
	if facts.Failure != nil {
		class := strings.TrimSpace(string(facts.Failure.Class))
		code := strings.TrimSpace(facts.Failure.Code)
		message := strings.TrimSpace(facts.Failure.Message)
		lowerCode := strings.ToLower(code)
		lowerMessage := strings.ToLower(message)
		switch {
		case class == string(FailureClassCandidateExhausted) ||
			strings.Contains(lowerMessage, "does not accept parameter") ||
			strings.Contains(lowerMessage, "requires parameter") ||
			strings.Contains(lowerCode, "contract"):
			ret = append(ret, RuntimeRepairHint{
				Code:    "fix_operation_parameters",
				Message: "previous candidate violated an operation parameter contract; rewrite parameters to the allowed typed fields for the requested kind",
			})
		case code == "validation_target_missing" ||
			strings.Contains(lowerCode, "preflight") ||
			strings.Contains(lowerMessage, "validation preflight failed") ||
			strings.Contains(lowerMessage, "validation target is missing"):
			ret = append(ret, RuntimeRepairHint{
				Code:    "create_validation_target",
				Message: "validation target is missing; create the required validation asset before retrying the same validation command, or choose an already executable validation command",
			})
		case strings.Contains(lowerCode, "conflict") ||
			strings.Contains(lowerMessage, "conflict") ||
			strings.Contains(lowerMessage, "not found") ||
			strings.Contains(lowerMessage, "mismatch"):
			ret = append(ret, RuntimeRepairHint{
				Code:    "reread_before_retry",
				Message: "latest state-changing operation conflicted with current target state; re-read current target content and realign operation inputs before retrying",
			})
		}
		if repeated := countActiveRejectedFailureSignature(facts, facts.Failure.Signature); repeated >= 1 {
			ret = append(ret, RuntimeRepairHint{
				Code:    "avoid_same_failed_shape",
				Message: fmt.Sprintf("failure signature %q has %d active rejection(s) on this focus after the latest accepted evidence; do not repeat the same failed operation shape", strings.TrimSpace(facts.Failure.Signature), repeated),
			})
		}
	}
	return dedupeRuntimeRepairHints(ret)
}

// MissingRequiredEvidenceKinds returns the required evidence kinds not yet
// attributed to the current focus by the Runtime V2 authority rule.
func MissingRequiredEvidenceKinds(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	accepted := make(map[string]struct{}, len(facts.Evidence.Accepted))
	for _, evidence := range facts.Evidence.Accepted {
		if EvidenceAppliesToCurrentFocus(facts, evidence) {
			accepted[strings.TrimSpace(evidence.Kind)] = struct{}{}
		}
	}
	missing := make([]string, 0, len(facts.Focus.RequiredEvidenceKinds))
	for _, kind := range facts.Focus.RequiredEvidenceKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, ok := accepted[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	return missing
}

func countActiveRejectedFailureSignature(facts RuntimeFacts, signature string) int {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return 0
	}
	latestAccepted := latestAcceptedEvidenceRevisionForRepairHints(facts)
	count := 0
	for _, evidence := range facts.Evidence.Rejected {
		if !evidenceAppliesToFocus(facts.Focus, evidence) {
			continue
		}
		if strings.TrimSpace(evidence.FailureSignature) != signature {
			continue
		}
		if latestAccepted != "" && !evidenceRevisionAfter(evidence.Revision, latestAccepted) {
			continue
		}
		count++
	}
	return count
}

// latestAcceptedEvidenceRevisionForRepairHints is a pure evidence-summary
// helper used only for repair-hint scoring. Candidate admission derives its
// persisted lineage inside the Orchestrator owner.
func latestAcceptedEvidenceRevisionForRepairHints(facts RuntimeFacts) string {
	latest := ""
	for _, evidence := range facts.Evidence.Accepted {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		if latest == "" || evidenceRevisionAfter(evidence.Revision, latest) {
			latest = evidence.Revision
		}
	}
	return latest
}

// EvidenceAppliesToCurrentFocus is the authority rule for attributing evidence to the
// active coverage focus. Evidence without CoverageWorkItemIDs only applies when the
// current focus is the last remaining focus (no RemainingFocuses).
func EvidenceAppliesToCurrentFocus(facts RuntimeFacts, evidence EvidenceFact) bool {
	facts = facts.Normalized()
	if len(evidence.CoverageWorkItemIDs) == 0 {
		return len(facts.RemainingFocuses) == 0
	}
	current := make(map[common.CoverageWorkItemID]struct{}, len(facts.Focus.CoverageWorkItemIDs))
	for _, id := range facts.Focus.CoverageWorkItemIDs {
		current[id] = struct{}{}
	}
	for _, id := range evidence.CoverageWorkItemIDs {
		if _, ok := current[id]; ok {
			return true
		}
	}
	return false
}

// evidenceAppliesToFocus is a focus-local membership check used by repair hints and
// failure-signature accounting. Empty CoverageWorkItemIDs are treated as focus-local
// for hint visibility; missing-evidence closure must use EvidenceAppliesToCurrentFocus.
func evidenceAppliesToFocus(focus CoverageFocus, evidence EvidenceFact) bool {
	if len(evidence.CoverageWorkItemIDs) == 0 {
		return true
	}
	allowed := make(map[common.CoverageWorkItemID]struct{}, len(focus.CoverageWorkItemIDs))
	for _, id := range focus.CoverageWorkItemIDs {
		allowed[id] = struct{}{}
	}
	for _, id := range evidence.CoverageWorkItemIDs {
		if _, ok := allowed[id]; ok {
			return true
		}
	}
	return false
}

func containsStringValue(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func dedupeRuntimeRepairHints(items []RuntimeRepairHint) []RuntimeRepairHint {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	ret := make([]RuntimeRepairHint, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Code) + "|" + strings.TrimSpace(item.Message)
		if key == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, RuntimeRepairHint{Code: strings.TrimSpace(item.Code), Message: strings.TrimSpace(item.Message)})
	}
	return ret
}
