package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

// RuntimeActivationContext 是一次 Runtime Run 在 application activation 阶段
// 已解析完成的 typed 运行输入。它保存 provider inference selection 与
// provider-facing 所需的稳定内容快照，不使用 map/JSON bridge 表达组件合同。
// ProviderSelection 是 Run owner 封存的不可变事实，不进入 C0/C1/C2 stable prefix。
type RuntimeActivationContext struct {
	ProviderSelection  ProviderInferenceSelection
	DialogLanguage     string
	DefaultSoul        *RuntimeSoulContext
	StageSoulBindings  []RuntimeStageSoulBinding
	Skills             []RuntimeSkillContext
	SkillContextBudget *RuntimeSkillContextBudget
	MCPServerIDs       []common.MCPServerID
	MCPCapabilities    []RuntimeMCPCapabilityContext
	// OperationPolicies are typed effect/replay facts published by capability
	// owners during activation. Runtime consumes this owner-neutral surface and
	// never derives authority from MCP names, risk labels, or protocol kinds.
	OperationPolicies []RuntimeOperationPolicyFact
	Memories          []RuntimeMemoryContext
	MemorySelection   *workorchmemorycontract.RecallSelectionEvidence
	EvidencePolicies  []RuntimeEvidencePolicy
	ExecutionContract RuntimeExecutionContract
	Continuation      RuntimeContinuationContext
}

type RuntimeSoulContext struct {
	ID                          common.SoulID
	Name                        string
	Description                 string
	BusinessDomains             []string
	Instructions                string
	AllowedSkillRefs            []string
	AllowedMCPServerRefs        []string
	AllowedCapabilityRefs       []string
	AllowedCapabilityCategories []string
	AllowedToolKinds            []string
}

type RuntimeStageSoulBinding struct {
	Stage  string
	Source string
	Soul   RuntimeSoulContext
}

type RuntimeSkillContext struct {
	ID                       string
	Name                     string
	Version                  string
	Description              string
	TriggerRules             []string
	Instruction              string
	ReferenceRefs            []string
	RequiredToolKinds        []string
	RequiredMCPServerRefs    []string
	RequiredCapabilityRefs   []string
	Constraints              []string
	Provenance               workorchskillcontract.Provenance
	OriginalInstructionBytes int
	InstructionTruncated     bool
}

type RuntimeMCPCapabilityContext struct {
	ID       common.MCPCapabilityID
	ServerID common.MCPServerID
	// Kind is the registry wire kind (e.g. "tool"). OperationKind is the Runtime
	// operation kind (e.g. "mcp/tool_call"). Empty OperationKind is derived from Kind.
	Kind          string
	OperationKind string
	Name          string
	DisplayName   string
	Description   string
	Category      string
	Tags          []string
	UsageHint     string
	InputSummary  string
	OutputSummary string
	StageHints    []string
	RiskLevel     string
	// EffectScope and IdempotencyClass are explicit capability-owner metadata sealed
	// at activation. Empty/invalid input normalizes to unknown; Runtime never
	// derives either value from RiskLevel or a tool name.
	EffectScope      string
	IdempotencyClass string
	URI              string
	Enabled          bool
	ApprovalPolicy   string
	Status           string
}

type RuntimeMemoryContext struct {
	ID          common.MemoryID
	MemoryType  string
	Summary     string
	Content     string
	Tags        []string
	Importance  int
	Confidence  int
	Visibility  string
	Provenance  string
	VectorScore float64
}

func (c RuntimeActivationContext) Normalized() RuntimeActivationContext {
	c.ProviderSelection = c.ProviderSelection.Normalized()
	c.DialogLanguage = strings.TrimSpace(c.DialogLanguage)
	if c.DefaultSoul != nil {
		value := c.DefaultSoul.Normalized()
		if value.ID == "" {
			c.DefaultSoul = nil
		} else {
			c.DefaultSoul = &value
		}
	}
	c.StageSoulBindings = normalizeRuntimeStageSoulBindings(c.StageSoulBindings)
	c.Skills = normalizeRuntimeSkills(c.Skills)
	if c.SkillContextBudget != nil {
		value := c.SkillContextBudget.Normalized()
		c.SkillContextBudget = &value
	}
	c.MCPServerIDs = normalizeMCPServerIDs(c.MCPServerIDs)
	c.MCPCapabilities = normalizeRuntimeMCPCapabilities(c.MCPCapabilities)
	c.OperationPolicies = NormalizeRuntimeOperationPolicyFacts(c.OperationPolicies)
	c.Memories = normalizeRuntimeMemories(c.Memories)
	if c.MemorySelection != nil {
		value := c.MemorySelection.Normalized()
		c.MemorySelection = &value
	}
	c.EvidencePolicies = NormalizeRuntimeEvidencePolicies(c.EvidencePolicies)
	c.ExecutionContract = c.ExecutionContract.Normalized()
	c.Continuation = c.Continuation.Normalized()
	return c
}

func (c RuntimeActivationContext) Validate() error {
	if err := ValidateRuntimeOperationPolicyFacts(c.OperationPolicies); err != nil {
		return fmt.Errorf("runtime activation operation policies: %w", err)
	}
	if err := ValidateRuntimeSkillContexts(c.Skills, c.SkillContextBudget); err != nil {
		return fmt.Errorf("runtime activation skills: %w", err)
	}
	c = c.Normalized()
	if err := c.ProviderSelection.Validate(); err != nil {
		return fmt.Errorf("runtime activation provider selection: %w", err)
	}
	for _, binding := range c.StageSoulBindings {
		if binding.Stage == "" || binding.Soul.ID == "" {
			return fmt.Errorf("runtime activation stage soul binding requires stage and soul")
		}
	}
	for _, capability := range c.MCPCapabilities {
		if capability.ID == "" || capability.ServerID == "" || capability.Kind == "" || capability.Name == "" {
			return fmt.Errorf("runtime activation MCP capability requires id, server, kind and name")
		}
	}
	for _, memory := range c.Memories {
		if memory.ID == "" || (strings.TrimSpace(memory.Summary) == "" && strings.TrimSpace(memory.Content) == "") {
			return fmt.Errorf("runtime activation memory requires id and content")
		}
	}
	if len(c.Memories) > 0 && c.MemorySelection == nil {
		return fmt.Errorf("runtime activation memories require recall selection evidence")
	}
	if c.MemorySelection != nil {
		if err := c.MemorySelection.Validate(); err != nil {
			return fmt.Errorf("runtime activation memory selection: %w", err)
		}
		selected := make(map[common.MemoryID]struct{}, len(c.MemorySelection.SelectedMemoryIDs))
		for _, id := range c.MemorySelection.SelectedMemoryIDs {
			selected[id] = struct{}{}
		}
		if len(selected) != len(c.Memories) {
			return fmt.Errorf("runtime activation memory selection differs from sealed memories")
		}
		for _, memory := range c.Memories {
			if _, ok := selected[memory.ID]; !ok {
				return fmt.Errorf("runtime activation memory %s is absent from selection evidence", memory.ID)
			}
		}
	}
	if err := ValidateRuntimeEvidencePolicies(c.EvidencePolicies); err != nil {
		return err
	}
	if err := c.ExecutionContract.Validate(); err != nil {
		return err
	}
	if err := c.Continuation.Validate(); err != nil {
		return err
	}
	return nil
}

func (c RuntimeActivationContext) Clone() RuntimeActivationContext {
	return c.Normalized()
}

func SameRuntimeActivationContext(left RuntimeActivationContext, right RuntimeActivationContext) bool {
	left = left.Normalized()
	right = right.Normalized()
	if !left.ProviderSelection.Same(right.ProviderSelection) || left.DialogLanguage != right.DialogLanguage || !sameRuntimeSoulPointers(left.DefaultSoul, right.DefaultSoul) ||
		len(left.StageSoulBindings) != len(right.StageSoulBindings) || len(left.Skills) != len(right.Skills) ||
		len(left.MCPServerIDs) != len(right.MCPServerIDs) || len(left.MCPCapabilities) != len(right.MCPCapabilities) || len(left.OperationPolicies) != len(right.OperationPolicies) || len(left.Memories) != len(right.Memories) ||
		!sameRuntimeSkillContextBudget(left.SkillContextBudget, right.SkillContextBudget) ||
		!sameRuntimeMemorySelection(left.MemorySelection, right.MemorySelection) ||
		!SameRuntimeEvidencePolicies(left.EvidencePolicies, right.EvidencePolicies) {
		return false
	}
	if !SameRuntimeExecutionContract(left.ExecutionContract, right.ExecutionContract) {
		return false
	}
	if !SameRuntimeContinuationContext(left.Continuation, right.Continuation) {
		return false
	}
	for idx := range left.StageSoulBindings {
		if left.StageSoulBindings[idx].Stage != right.StageSoulBindings[idx].Stage ||
			left.StageSoulBindings[idx].Source != right.StageSoulBindings[idx].Source ||
			!sameRuntimeSoul(left.StageSoulBindings[idx].Soul, right.StageSoulBindings[idx].Soul) {
			return false
		}
	}
	for idx := range left.Skills {
		if !sameRuntimeSkill(left.Skills[idx], right.Skills[idx]) {
			return false
		}
	}
	for idx := range left.MCPServerIDs {
		if left.MCPServerIDs[idx] != right.MCPServerIDs[idx] {
			return false
		}
	}
	for idx := range left.MCPCapabilities {
		if !sameRuntimeMCPCapability(left.MCPCapabilities[idx], right.MCPCapabilities[idx]) {
			return false
		}
	}
	for idx := range left.OperationPolicies {
		if !sameRuntimeOperationPolicyFact(left.OperationPolicies[idx], right.OperationPolicies[idx]) {
			return false
		}
	}
	for idx := range left.Memories {
		if !sameRuntimeMemory(left.Memories[idx], right.Memories[idx]) {
			return false
		}
	}
	return true
}

func (s RuntimeSoulContext) Normalized() RuntimeSoulContext {
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	s.BusinessDomains = normalizeStrings(s.BusinessDomains)
	s.Instructions = strings.TrimSpace(s.Instructions)
	s.AllowedSkillRefs = normalizeStrings(s.AllowedSkillRefs)
	s.AllowedMCPServerRefs = normalizeStrings(s.AllowedMCPServerRefs)
	s.AllowedCapabilityRefs = normalizeStrings(s.AllowedCapabilityRefs)
	s.AllowedCapabilityCategories = normalizeStrings(s.AllowedCapabilityCategories)
	s.AllowedToolKinds = normalizeStrings(s.AllowedToolKinds)
	return s
}

func normalizeRuntimeStageSoulBindings(values []RuntimeStageSoulBinding) []RuntimeStageSoulBinding {
	byStage := map[string]RuntimeStageSoulBinding{}
	for _, value := range values {
		value.Stage = strings.TrimSpace(value.Stage)
		value.Source = strings.TrimSpace(value.Source)
		value.Soul = value.Soul.Normalized()
		if value.Stage != "" && value.Soul.ID != "" {
			byStage[value.Stage] = value
		}
	}
	stages := make([]string, 0, len(byStage))
	for stage := range byStage {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	ret := make([]RuntimeStageSoulBinding, 0, len(stages))
	for _, stage := range stages {
		ret = append(ret, byStage[stage])
	}
	return nilIfEmpty(ret)
}

func normalizeRuntimeSkills(values []RuntimeSkillContext) []RuntimeSkillContext {
	byID := map[string]RuntimeSkillContext{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Name = strings.TrimSpace(value.Name)
		value.Version = strings.TrimSpace(value.Version)
		value.Description = strings.TrimSpace(value.Description)
		value.TriggerRules = normalizeStrings(value.TriggerRules)
		value.Instruction = strings.TrimSpace(value.Instruction)
		value.ReferenceRefs = normalizeStrings(value.ReferenceRefs)
		value.RequiredToolKinds = normalizeStrings(value.RequiredToolKinds)
		value.RequiredMCPServerRefs = normalizeStrings(value.RequiredMCPServerRefs)
		value.RequiredCapabilityRefs = normalizeStrings(value.RequiredCapabilityRefs)
		value.Constraints = normalizeStrings(value.Constraints)
		value.Provenance = value.Provenance.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeMCPServerIDs(values []common.MCPServerID) []common.MCPServerID {
	stringsValue := make([]string, 0, len(values))
	for _, value := range values {
		stringsValue = append(stringsValue, string(value))
	}
	stringsValue = normalizeStrings(stringsValue)
	ret := make([]common.MCPServerID, 0, len(stringsValue))
	for _, value := range stringsValue {
		ret = append(ret, common.MCPServerID(value))
	}
	return nilIfEmpty(ret)
}

func normalizeRuntimeMCPCapabilities(values []RuntimeMCPCapabilityContext) []RuntimeMCPCapabilityContext {
	byID := map[string]RuntimeMCPCapabilityContext{}
	for _, value := range values {
		value.Kind = strings.TrimSpace(value.Kind)
		value.OperationKind = strings.TrimSpace(value.OperationKind)
		if value.OperationKind == "" {
			value.OperationKind = normalizeMCPRegistryKindToOperationKind(value.Kind)
		}
		value.Name = strings.TrimSpace(value.Name)
		value.DisplayName = strings.TrimSpace(value.DisplayName)
		value.Description = strings.TrimSpace(value.Description)
		value.Category = strings.TrimSpace(value.Category)
		value.Tags = normalizeStrings(value.Tags)
		value.UsageHint = strings.TrimSpace(value.UsageHint)
		value.InputSummary = strings.TrimSpace(value.InputSummary)
		value.OutputSummary = strings.TrimSpace(value.OutputSummary)
		value.StageHints = normalizeStrings(value.StageHints)
		value.RiskLevel = strings.TrimSpace(value.RiskLevel)
		value.EffectScope = string(NormalizeRuntimeEffectScope(RuntimeEffectScope(value.EffectScope)))
		value.IdempotencyClass = string(NormalizeRuntimeOperationIdempotencyClass(RuntimeOperationIdempotencyClass(value.IdempotencyClass)))
		value.URI = strings.TrimSpace(value.URI)
		value.ApprovalPolicy = strings.TrimSpace(value.ApprovalPolicy)
		value.Status = strings.TrimSpace(value.Status)
		if value.ID != "" {
			byID[string(value.ID)] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeRuntimeMemories(values []RuntimeMemoryContext) []RuntimeMemoryContext {
	byID := map[string]RuntimeMemoryContext{}
	for _, value := range values {
		value.MemoryType = strings.TrimSpace(value.MemoryType)
		value.Summary = strings.TrimSpace(value.Summary)
		value.Content = strings.TrimSpace(value.Content)
		value.Tags = normalizeStrings(value.Tags)
		value.Visibility = strings.TrimSpace(value.Visibility)
		value.Provenance = strings.TrimSpace(value.Provenance)
		if value.ID != "" {
			byID[string(value.ID)] = value
		}
	}
	return sortedMapValues(byID)
}

func sortedMapValues[T any](values map[string]T) []T {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]T, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, values[key])
	}
	return nilIfEmpty(ret)
}

func nilIfEmpty[T any](values []T) []T {
	if len(values) == 0 {
		return nil
	}
	return values
}

func sameRuntimeSoulPointers(left *RuntimeSoulContext, right *RuntimeSoulContext) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return sameRuntimeSoul(*left, *right)
}

func sameRuntimeSoul(left RuntimeSoulContext, right RuntimeSoulContext) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.Name == right.Name && left.Description == right.Description && left.Instructions == right.Instructions &&
		sameStrings(left.BusinessDomains, right.BusinessDomains) && sameStrings(left.AllowedSkillRefs, right.AllowedSkillRefs) &&
		sameStrings(left.AllowedMCPServerRefs, right.AllowedMCPServerRefs) && sameStrings(left.AllowedCapabilityRefs, right.AllowedCapabilityRefs) &&
		sameStrings(left.AllowedCapabilityCategories, right.AllowedCapabilityCategories) && sameStrings(left.AllowedToolKinds, right.AllowedToolKinds)
}

func sameRuntimeSkill(left RuntimeSkillContext, right RuntimeSkillContext) bool {
	leftValues := normalizeRuntimeSkills([]RuntimeSkillContext{left})
	rightValues := normalizeRuntimeSkills([]RuntimeSkillContext{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	return left.ID == right.ID && left.Name == right.Name && left.Version == right.Version && left.Description == right.Description &&
		left.Instruction == right.Instruction && workorchskillcontract.SameProvenance(left.Provenance, right.Provenance) &&
		left.OriginalInstructionBytes == right.OriginalInstructionBytes && left.InstructionTruncated == right.InstructionTruncated &&
		sameStrings(left.TriggerRules, right.TriggerRules) && sameStrings(left.ReferenceRefs, right.ReferenceRefs) && sameStrings(left.RequiredToolKinds, right.RequiredToolKinds) &&
		sameStrings(left.RequiredMCPServerRefs, right.RequiredMCPServerRefs) && sameStrings(left.RequiredCapabilityRefs, right.RequiredCapabilityRefs) && sameStrings(left.Constraints, right.Constraints)
}

func sameRuntimeMCPCapability(left RuntimeMCPCapabilityContext, right RuntimeMCPCapabilityContext) bool {
	leftValues := normalizeRuntimeMCPCapabilities([]RuntimeMCPCapabilityContext{left})
	rightValues := normalizeRuntimeMCPCapabilities([]RuntimeMCPCapabilityContext{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	return left.ID == right.ID && left.ServerID == right.ServerID && left.Kind == right.Kind && left.OperationKind == right.OperationKind && left.Name == right.Name && left.DisplayName == right.DisplayName &&
		left.Description == right.Description && left.Category == right.Category && left.UsageHint == right.UsageHint && left.InputSummary == right.InputSummary &&
		left.OutputSummary == right.OutputSummary && left.RiskLevel == right.RiskLevel && left.EffectScope == right.EffectScope && left.IdempotencyClass == right.IdempotencyClass && left.URI == right.URI && left.Enabled == right.Enabled &&
		left.ApprovalPolicy == right.ApprovalPolicy && left.Status == right.Status && sameStrings(left.Tags, right.Tags) && sameStrings(left.StageHints, right.StageHints)
}

func sameRuntimeMemory(left RuntimeMemoryContext, right RuntimeMemoryContext) bool {
	leftValues := normalizeRuntimeMemories([]RuntimeMemoryContext{left})
	rightValues := normalizeRuntimeMemories([]RuntimeMemoryContext{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	return left.ID == right.ID && left.MemoryType == right.MemoryType && left.Summary == right.Summary && left.Content == right.Content &&
		left.Importance == right.Importance && left.Confidence == right.Confidence && left.Visibility == right.Visibility && left.Provenance == right.Provenance &&
		left.VectorScore == right.VectorScore && sameStrings(left.Tags, right.Tags)
}

func sameRuntimeMemorySelection(left, right *workorchmemorycontract.RecallSelectionEvidence) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return workorchmemorycontract.SameRecallSelectionEvidence(*left, *right)
}

// normalizeMCPRegistryKindToOperationKind maps registry wire kinds to Runtime operation kinds.
// Registry "tool" -> mcp/tool_call; already-canonical kinds pass through.
func normalizeMCPRegistryKindToOperationKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ""
	}
	// Prefer known Runtime operation kinds / aliases from the descriptor inventory.
	canonical := CanonicalRuntimeBlockKind(kind)
	if _, ok := RuntimeOperationDescriptorForKind(canonical); ok {
		return canonical
	}
	switch strings.ToLower(kind) {
	case "tool", "tools":
		return StepKindMCPToolCall
	case "resource", "resources":
		return StepKindMCPResourceRead
	case "prompt", "prompts":
		return StepKindMCPPromptGet
	default:
		return kind
	}
}
