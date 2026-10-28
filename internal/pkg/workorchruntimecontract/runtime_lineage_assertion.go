package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeLineageAssertionRule 是 Profile 拥有的附加 all-of 门禁，不是证据种类、
// operation、provider fact 或独立覆盖证明。未知或不可用的消费者保持封闭。
type RuntimeLineageAssertionKind string

// RuntimeProofPhase 仅区分 guard 消费时点，不是新的 Runtime 状态机 phase。
type RuntimeProofPhase string

const (
	RuntimeLineageCompletedWorkRounds       RuntimeLineageAssertionKind = "completed_work_rounds"
	RuntimeLineageRepairThenFreshValidation RuntimeLineageAssertionKind = "repair_then_fresh_validation"
	RuntimeLineageOwnerOperationReceipt     RuntimeLineageAssertionKind = "owner_operation_receipt"
	RuntimeLineageDeliveryAcknowledgements  RuntimeLineageAssertionKind = "delivery_acknowledgements"
	RuntimeLineageTerminalOwnerReceipt      RuntimeLineageAssertionKind = "terminal_owner_operation_receipt"
	RuntimeLineageTerminalResultIndex       RuntimeLineageAssertionKind = "terminal_result_evidence_index"
	RuntimeLineageTerminalOwnerSnapshot     RuntimeLineageAssertionKind = "terminal_owner_snapshot"
	RuntimeTerminalProofContractV1                                      = "runtime/terminal-material-proof/v1"
	RuntimeTerminalProofContractV2                                      = "runtime/terminal-material-proof/v2"
	RuntimeCrossSourceExecutionProofV1                                  = "runtime/cross-source-execution-proof/v1"
	RuntimeGoalSourceExecutionProofV2                                   = "runtime/goal-source-execution-proof/v2"
	RuntimeConfirmedBindingRepairProofV1                                = "runtime/confirmed-binding-repair-proof/v1"
	RuntimeProofPhaseExecution              RuntimeProofPhase           = "execution"
	RuntimeProofPhasePostTerminalDelivery   RuntimeProofPhase           = "post_terminal_delivery"
	MaxRuntimeLineageAssertions                                         = 16
)

type RuntimeLineageAssertionRule struct {
	Ref  string
	Kind RuntimeLineageAssertionKind
	// ConsumptionPoint 是封存声明的消费时点，不是调用方选择的 Runtime 目标状态。
	ConsumptionPoint    RuntimeProofPhase
	MinCompletedRounds  int                `json:",omitempty"`
	OwnerModuleID       string             `json:",omitempty"`
	OperationKind       string             `json:",omitempty"`
	OperationVariantID  string             `json:",omitempty"`
	RequiredEffectScope RuntimeEffectScope `json:",omitempty"`
	// Terminal extensions are opt-in; absence preserves every original rule's encoding/hash.
	TerminalContractRef   string `json:",omitempty"`
	ReceiptSourceScope    string `json:",omitempty"`
	ReceiptSourceID       string `json:",omitempty"`
	ValidationSourceScope string `json:",omitempty"`
	ValidationSourceID    string `json:",omitempty"`
	// Nonempty requires an exact user-confirmed candidate-item mapping. It is
	// never a runtime-selected source selector or a substitute for ordinary proof.
	ExecutionSourceContractRef string                        `json:",omitempty"`
	RepairHistoryContractRef   string                        `json:",omitempty"`
	ReceiptChoiceContractRef   string                        `json:",omitempty"`
	ReceiptOperationChoices    [4]RuntimeOperationVariantRef `json:",omitzero"`
}

type RuntimeLineageAssertion struct {
	Rule               RuntimeLineageAssertionRule
	RuleHash           string
	DeclarationRef     string
	CoverageWorkItemID common.CoverageWorkItemID
	SourceScope        string
	SourceID           string
	SourceTextHash     string
}

const RuntimeTerminalReceiptChoiceV1 = "runtime/terminal-receipt-choice/v1"

func (r RuntimeLineageAssertionRule) Validate() error {
	choices := r.ReceiptOperationChoices
	if r.ReceiptChoiceContractRef != "" || choices != ([4]RuntimeOperationVariantRef{}) {
		if r.ReceiptChoiceContractRef != RuntimeTerminalReceiptChoiceV1 || r.Kind != RuntimeLineageTerminalOwnerReceipt || r.OperationKind != "" || r.OperationVariantID != "" {
			return fmt.Errorf("unsupported terminal receipt choice contract")
		}
		count := 0
		seen := map[RuntimeOperationVariantRef]bool{}
		empty := false
		for _, choice := range choices {
			if choice.Kind == "" {
				if choice.VariantID != "" {
					return fmt.Errorf("receipt choice has variant without kind")
				}
				empty = true
				continue
			}
			if empty || seen[choice] {
				return fmt.Errorf("receipt choice inventory is ambiguous")
			}
			seen[choice] = true
			count++
			exact := r
			exact.ReceiptChoiceContractRef = ""
			exact.ReceiptOperationChoices = [4]RuntimeOperationVariantRef{}
			exact.OperationKind = choice.Kind
			exact.OperationVariantID = choice.VariantID
			if err := exact.Validate(); err != nil {
				return err
			}
		}
		if count < 2 {
			return fmt.Errorf("receipt choice requires two to four exact alternatives")
		}
		return nil
	}

	if r.RepairHistoryContractRef != "" && (r.RepairHistoryContractRef != RuntimeConfirmedBindingRepairProofV1 || r.Kind != RuntimeLineageRepairThenFreshValidation || r.ConsumptionPoint != RuntimeProofPhaseExecution) {
		return fmt.Errorf("confirmed-binding repair assertion requires supported execution repair consumer")
	}
	if r.ExecutionSourceContractRef != "" && ((r.ExecutionSourceContractRef != RuntimeCrossSourceExecutionProofV1 && r.ExecutionSourceContractRef != RuntimeGoalSourceExecutionProofV2) || r.ConsumptionPoint != RuntimeProofPhaseExecution || (r.Kind != RuntimeLineageCompletedWorkRounds && r.Kind != RuntimeLineageRepairThenFreshValidation)) {
		return fmt.Errorf("cross-source execution assertion requires supported version and work/repair consumer")
	}
	if r.Ref == "" || strings.TrimSpace(r.Ref) != r.Ref || len(r.Ref) > 512 {
		return fmt.Errorf("lineage assertion requires exact bounded rule ref")
	}
	if r.ConsumptionPoint != RuntimeProofPhaseExecution && r.ConsumptionPoint != RuntimeProofPhasePostTerminalDelivery {
		return fmt.Errorf("lineage assertion has unsupported consumption point %q", r.ConsumptionPoint)
	}
	terminalMaterial := r.Kind == RuntimeLineageTerminalOwnerReceipt || r.Kind == RuntimeLineageTerminalResultIndex || r.Kind == RuntimeLineageTerminalOwnerSnapshot
	if terminalMaterial {
		version := RuntimeTerminalProofContractV1
		if r.Kind == RuntimeLineageTerminalOwnerSnapshot {
			version = RuntimeTerminalProofContractV2
		}
		if r.TerminalContractRef != version || r.ConsumptionPoint != RuntimeProofPhasePostTerminalDelivery {
			return fmt.Errorf("terminal material assertion requires exact versioned contract and post-terminal consumption")
		}
	} else if r.TerminalContractRef != "" || r.ReceiptSourceScope != "" || r.ReceiptSourceID != "" {
		return fmt.Errorf("non-terminal-material assertion cannot carry terminal extensions")
	}
	if r.Kind != RuntimeLineageTerminalOwnerSnapshot && (r.ValidationSourceScope != "" || r.ValidationSourceID != "") {
		return fmt.Errorf("only terminal snapshot assertions can carry validation source selectors")
	}
	switch r.Kind {
	case RuntimeLineageCompletedWorkRounds:
		if r.ConsumptionPoint != RuntimeProofPhaseExecution || r.MinCompletedRounds < 1 || r.MinCompletedRounds > MaxRuntimeImplementationWorkRounds || r.OwnerModuleID != "" || r.OperationKind != "" || r.OperationVariantID != "" || r.RequiredEffectScope != "" {
			return fmt.Errorf("completed work assertion requires bounded rounds and execution phase only")
		}
	case RuntimeLineageRepairThenFreshValidation:
		if r.ConsumptionPoint != RuntimeProofPhaseExecution || r.MinCompletedRounds != 0 || r.OwnerModuleID != "" || r.OperationKind != "" || r.OperationVariantID != "" || r.RequiredEffectScope != "" {
			return fmt.Errorf("repair assertion requires execution phase only")
		}
	case RuntimeLineageOwnerOperationReceipt:
		descriptor, known := RuntimeOperationDescriptorForKind(r.OperationKind)
		if r.ConsumptionPoint != RuntimeProofPhaseExecution || r.MinCompletedRounds != 0 || !known || descriptor.OwnerModuleID != r.OwnerModuleID || !descriptor.RequiresOwnerReceipt || CanonicalRuntimeBlockKind(r.OperationKind) != r.OperationKind || !validSealedRuntimeEffectScope(r.RequiredEffectScope) || !runtimeLineageReceiptInventoryMatches(r, descriptor) {
			return fmt.Errorf("owner receipt assertion requires original canonical owner/kind/variant/effect inventory")
		}
	case RuntimeLineageTerminalOwnerReceipt:
		descriptor, known := RuntimeOperationDescriptorForKind(r.OperationKind)
		if r.MinCompletedRounds != 0 || !known || descriptor.OwnerModuleID != r.OwnerModuleID || !descriptor.RequiresOwnerReceipt || CanonicalRuntimeBlockKind(r.OperationKind) != r.OperationKind || !validSealedRuntimeEffectScope(r.RequiredEffectScope) || !runtimeLineageReceiptInventoryMatches(r, descriptor) || !runtimeTerminalReceiptSourceValid(r.ReceiptSourceScope, r.ReceiptSourceID) {
			return fmt.Errorf("terminal owner receipt requires exact original source and canonical owner/kind/variant/effect inventory")
		}
	case RuntimeLineageTerminalResultIndex:
		if r.MinCompletedRounds != 0 || r.OwnerModuleID != "" || r.OperationKind != "" || r.OperationVariantID != "" || r.RequiredEffectScope != "" || r.ReceiptSourceScope != "" || r.ReceiptSourceID != "" {
			return fmt.Errorf("terminal Result index assertion cannot carry operation selectors")
		}
	case RuntimeLineageTerminalOwnerSnapshot:
		descriptor, known := RuntimeOperationDescriptorForKind(r.OperationKind)
		contract, found := RuntimeOperationContractForKind(r.OperationKind)
		supported := false
		for _, variant := range contract.Variants {
			supported = supported || (variant.ID == r.OperationVariantID && variant.TerminalSnapshotProtocolRef == RuntimeTerminalOwnerSnapshotProtocol)
		}
		if r.MinCompletedRounds != 0 || !known || !found || !supported || descriptor.OwnerModuleID != r.OwnerModuleID || !descriptor.RequiresOwnerReceipt || CanonicalRuntimeBlockKind(r.OperationKind) != r.OperationKind || !runtimeLineageReceiptInventoryMatches(r, descriptor) || !runtimeTerminalReceiptSourceValid(r.ReceiptSourceScope, r.ReceiptSourceID) || !runtimeTerminalReceiptSourceValid(r.ValidationSourceScope, r.ValidationSourceID) {
			return fmt.Errorf("terminal snapshot requires supported owner protocol and exact original receipt/validation sources")
		}
	case RuntimeLineageDeliveryAcknowledgements:
		if r.ConsumptionPoint != RuntimeProofPhasePostTerminalDelivery || r.MinCompletedRounds != 0 || r.OwnerModuleID != "" || r.OperationKind != "" || r.OperationVariantID != "" || r.RequiredEffectScope != "" {
			return fmt.Errorf("delivery assertion requires post-terminal phase only")
		}
	default:
		return fmt.Errorf("unsupported lineage assertion kind %q", r.Kind)
	}
	return nil
}

// runtimeLineageReceiptInventoryMatches 只接受封闭 operation 库存声明的 variant/effect。
// 动态 effect 的 kind 没有静态可证明的库存值，必须保持封闭，不能用规则替代 owner policy。
func runtimeLineageReceiptInventoryMatches(rule RuntimeLineageAssertionRule, descriptor RuntimeOperationDescriptor) bool {
	contract, found := RuntimeOperationContractForKind(rule.OperationKind)
	if !found || contract.Kind != rule.OperationKind {
		return false
	}
	if len(contract.Variants) == 0 {
		return rule.OperationVariantID == "" && descriptor.DefaultEffectScope != RuntimeEffectUnknown && rule.RequiredEffectScope == descriptor.DefaultEffectScope
	}
	matches := 0
	for _, variant := range contract.Variants {
		if variant.ID != rule.OperationVariantID {
			continue
		}
		effect := variant.EffectScope
		if effect == "" {
			effect = descriptor.DefaultEffectScope
		}
		if validSealedRuntimeEffectScope(effect) && effect == rule.RequiredEffectScope {
			matches++
		}
	}
	return rule.OperationVariantID != "" && matches == 1
}

func StableRuntimeLineageAssertionRuleHash(rule RuntimeLineageAssertionRule) string {
	// 保持原 JSON 与域前缀编码，只复用本次 marshal 缓冲，不缓存规则或校验结果。
	const prefix = "runtime-lineage-rule/v1\x00"
	payload, _ := json.Marshal(rule)
	bodyLength := len(payload)
	if cap(payload)-bodyLength < len(prefix) {
		payload = append([]byte(prefix), payload...)
	} else {
		payload = append(payload, prefix...)
		copy(payload[len(prefix):], payload[:bodyLength])
		copy(payload, prefix)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func RuntimeLineageSourceTextHash(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func ValidateRuntimeLineageAssertionRules(rules []RuntimeLineageAssertionRule) error {
	if len(rules) > MaxRuntimeLineageAssertions {
		return fmt.Errorf("lineage assertion rules exceed bounded count")
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		if seen[rule.Ref] {
			return fmt.Errorf("duplicate lineage rule %q", rule.Ref)
		}
		seen[rule.Ref] = true
	}
	return nil
}

func CompileRuntimeLineageAssertions(rules []RuntimeLineageAssertionRule, declarationRef string, item RuntimeCoverageItem) ([]RuntimeLineageAssertion, error) {
	if err := ValidateRuntimeLineageAssertionRules(rules); err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	if declarationRef == "" || item.ID == "" || item.SourceScope == "" || item.SourceID == "" || item.TargetText == "" {
		return nil, fmt.Errorf("lineage assertion requires complete original CWI source")
	}
	ret := make([]RuntimeLineageAssertion, 0, len(rules))
	for _, rule := range rules {
		ret = append(ret, RuntimeLineageAssertion{Rule: rule, RuleHash: StableRuntimeLineageAssertionRuleHash(rule), DeclarationRef: declarationRef, CoverageWorkItemID: item.ID, SourceScope: item.SourceScope, SourceID: item.SourceID, SourceTextHash: RuntimeLineageSourceTextHash(item.TargetText)})
	}
	return ret, nil
}

// CompileRuntimeCWILineageAssertions retains primary source declarations AND
// the independently sealed Goal consumers anchored at this exact original CWI.
func CompileRuntimeCWILineageAssertions(rules []RuntimeLineageAssertionRule, declarationRef string, item RuntimeCoverageItem) ([]RuntimeLineageAssertion, error) {
	ret, err := CompileRuntimeLineageAssertions(rules, declarationRef, item)
	if err != nil {
		return nil, err
	}
	ret = append(ret, item.GoalExecutionAssertions...)
	if err := ValidateRuntimeLineageAssertions(ret, item.ID, declarationRef); err != nil {
		return nil, err
	}
	return ret, nil
}

func ValidateRuntimeLineageAssertions(values []RuntimeLineageAssertion, coverageID common.CoverageWorkItemID, declarationRef string) error {
	if len(values) > MaxRuntimeLineageAssertions {
		return fmt.Errorf("lineage assertions exceed bounded count")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if err := value.Rule.Validate(); err != nil {
			return err
		}
		if seen[value.Rule.Ref] || value.RuleHash != StableRuntimeLineageAssertionRuleHash(value.Rule) || value.CoverageWorkItemID != coverageID || value.DeclarationRef != declarationRef || value.SourceScope == "" || value.SourceID == "" || len(value.SourceTextHash) != 64 {
			return fmt.Errorf("lineage assertion source/rule identity mismatch")
		}
		decoded, err := hex.DecodeString(value.SourceTextHash)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != value.SourceTextHash {
			return fmt.Errorf("lineage assertion source text hash is not canonical SHA256")
		}
		seen[value.Rule.Ref] = true
	}
	return nil
}

func mergeRuntimeLineageAssertionRules(base, added []RuntimeLineageAssertionRule) ([]RuntimeLineageAssertionRule, error) {
	ret := slices.Clone(base)
	for _, rule := range added {
		found := false
		for _, prior := range ret {
			if prior.Ref != rule.Ref {
				continue
			}
			if prior != rule {
				return nil, fmt.Errorf("conflicting lineage assertion rule %q", rule.Ref)
			}
			found = true
		}
		if !found {
			ret = append(ret, rule)
		}
	}
	if err := ValidateRuntimeLineageAssertionRules(ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func runtimeProfilesDeclareLineageAssertions(profiles []RuntimeProfileContract) bool {
	for _, profile := range profiles {
		for _, requirement := range profile.EvidenceRequirements {
			if len(requirement.LineageAssertions) != 0 {
				return true
			}
		}
	}
	return false
}

// ValidateRuntimeDefinitionLineageAssertions 从原 startup Profile 独立反编声明，
// 不把两个协调改写的自 hash 视为原始 authority。
func ValidateRuntimeDefinitionLineageAssertions(focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract) error {
	for _, focus := range focuses {
		policy, err := CompileRuntimeSourceEvidencePolicy(focus.AssessmentScope, focus.AssessmentID, profiles, nil)
		if err != nil {
			return err
		}
		if !slices.Equal(focus.LineageAssertions, policy.LineageAssertions) {
			return fmt.Errorf("definition lineage rules differ from original sealed Profile")
		}
	}
	return nil
}

func runtimeTerminalReceiptOperationMatches(rule RuntimeLineageAssertionRule, kind, variant string) bool {
	if rule.ReceiptChoiceContractRef == "" {
		return kind == rule.OperationKind && variant == rule.OperationVariantID
	}
	if rule.ReceiptChoiceContractRef != RuntimeTerminalReceiptChoiceV1 {
		return false
	}
	for _, choice := range rule.ReceiptOperationChoices {
		if choice.Kind == kind && choice.VariantID == variant && kind != "" {
			return true
		}
	}
	return false
}
