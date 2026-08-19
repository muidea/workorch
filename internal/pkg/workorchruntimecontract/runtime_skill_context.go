package workorchruntimecontract

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const (
	RuntimeSkillContextMaxSkills             = 5
	RuntimeSkillInstructionMaxBytesPerSkill  = 8 * 1024
	RuntimeSkillInstructionMaxTotalBytes     = 24 * 1024
	RuntimeSkillContextMetadataMaxTotalBytes = 16 * 1024
)

// RuntimeSkillContextBudget is sealed with the selected Skill snapshots. It
// records the fixed, owner-neutral context bounds applied before any provider
// request is assembled; it is C3 input and never part of the stable prefix.
type RuntimeSkillContextBudget struct {
	MaxSkills                   int
	MaxInstructionBytesPerSkill int
	MaxInstructionBytesTotal    int
	MaxMetadataBytesTotal       int
	OriginalInstructionBytes    int
	IncludedInstructionBytes    int
	MetadataBytes               int
	TruncatedSkills             int
}

func (b RuntimeSkillContextBudget) Normalized() RuntimeSkillContextBudget {
	return b
}

// SealRuntimeSkillContexts validates typed provenance and deterministically
// bounds instruction material. Metadata is rejected when it exceeds its
// independent ceiling; only instructions may be truncated.
func SealRuntimeSkillContexts(values []RuntimeSkillContext) ([]RuntimeSkillContext, *RuntimeSkillContextBudget, error) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value.ID)
		if id == "" {
			return nil, nil, fmt.Errorf("runtime Skill id is required")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, nil, fmt.Errorf("runtime Skill %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	values = normalizeRuntimeSkills(values)
	if len(values) > RuntimeSkillContextMaxSkills {
		return nil, nil, fmt.Errorf("runtime Skill count %d exceeds %d", len(values), RuntimeSkillContextMaxSkills)
	}

	budget := &RuntimeSkillContextBudget{
		MaxSkills: RuntimeSkillContextMaxSkills, MaxInstructionBytesPerSkill: RuntimeSkillInstructionMaxBytesPerSkill,
		MaxInstructionBytesTotal: RuntimeSkillInstructionMaxTotalBytes, MaxMetadataBytesTotal: RuntimeSkillContextMetadataMaxTotalBytes,
	}
	desired := make([]int, len(values))
	allocations := make([]int, len(values))
	for idx := range values {
		value := &values[idx]
		if value.ID == "" || value.Instruction == "" || !utf8.ValidString(value.Instruction) {
			return nil, nil, fmt.Errorf("runtime Skill %q requires a valid instruction", value.ID)
		}
		if err := value.Provenance.Validate(); err != nil {
			return nil, nil, fmt.Errorf("runtime Skill %q provenance: %w", value.ID, err)
		}
		if value.Provenance.ContentHash != workorchskillcontract.StableContentHash(runtimeSkillContent(*value)) {
			return nil, nil, fmt.Errorf("runtime Skill %q provenance content hash mismatch", value.ID)
		}
		budget.MetadataBytes += runtimeSkillMetadataBytes(*value)
		originalBytes := len(value.Instruction)
		budget.OriginalInstructionBytes += originalBytes
		desired[idx] = min(originalBytes, RuntimeSkillInstructionMaxBytesPerSkill)
	}
	if budget.MetadataBytes > RuntimeSkillContextMetadataMaxTotalBytes {
		return nil, nil, fmt.Errorf("runtime Skill metadata %d exceeds %d bytes", budget.MetadataBytes, RuntimeSkillContextMetadataMaxTotalBytes)
	}

	fairShare := RuntimeSkillInstructionMaxTotalBytes / len(values)
	for idx := range desired {
		allocations[idx] = min(desired[idx], fairShare)
		budget.IncludedInstructionBytes += allocations[idx]
	}
	remaining := RuntimeSkillInstructionMaxTotalBytes - budget.IncludedInstructionBytes
	for remaining > 0 {
		progressed := false
		for idx := range allocations {
			if allocations[idx] >= desired[idx] || remaining == 0 {
				continue
			}
			allocations[idx]++
			remaining--
			progressed = true
		}
		if !progressed {
			break
		}
	}

	budget.IncludedInstructionBytes = 0
	for idx := range values {
		value := &values[idx]
		originalBytes := len(value.Instruction)
		value.Instruction = truncateRuntimeSkillInstruction(value.Instruction, allocations[idx])
		value.OriginalInstructionBytes = originalBytes
		value.InstructionTruncated = len(value.Instruction) < originalBytes
		budget.IncludedInstructionBytes += len(value.Instruction)
		if value.InstructionTruncated {
			budget.TruncatedSkills++
		}
	}
	if err := ValidateRuntimeSkillContexts(values, budget); err != nil {
		return nil, nil, err
	}
	return values, budget, nil
}

func ValidateRuntimeSkillContexts(values []RuntimeSkillContext, budget *RuntimeSkillContextBudget) error {
	if len(values) == 0 {
		if budget != nil {
			return fmt.Errorf("runtime Skill budget requires selected Skills")
		}
		return nil
	}
	if len(values) > RuntimeSkillContextMaxSkills {
		return fmt.Errorf("runtime Skill count %d exceeds %d", len(values), RuntimeSkillContextMaxSkills)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value.ID)
		if id == "" {
			return fmt.Errorf("runtime Skill id is required")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("runtime Skill %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	values = normalizeRuntimeSkills(values)
	if budget == nil {
		return fmt.Errorf("runtime Skills require a sealed context budget")
	}
	b := budget.Normalized()
	if b.MaxSkills != RuntimeSkillContextMaxSkills || b.MaxInstructionBytesPerSkill != RuntimeSkillInstructionMaxBytesPerSkill ||
		b.MaxInstructionBytesTotal != RuntimeSkillInstructionMaxTotalBytes || b.MaxMetadataBytesTotal != RuntimeSkillContextMetadataMaxTotalBytes ||
		len(values) > b.MaxSkills {
		return fmt.Errorf("runtime Skill context budget policy drift")
	}
	originalBytes := 0
	includedBytes := 0
	metadataBytes := 0
	truncatedSkills := 0
	for _, value := range values {
		if value.ID == "" || value.Instruction == "" || !utf8.ValidString(value.Instruction) || value.OriginalInstructionBytes <= 0 {
			return fmt.Errorf("runtime Skill %q has invalid sealed instruction", value.ID)
		}
		if err := value.Provenance.Validate(); err != nil {
			return fmt.Errorf("runtime Skill %q provenance: %w", value.ID, err)
		}
		if !value.InstructionTruncated && value.Provenance.ContentHash != workorchskillcontract.StableContentHash(runtimeSkillContent(value)) {
			return fmt.Errorf("runtime Skill %q provenance content hash mismatch", value.ID)
		}
		included := len(value.Instruction)
		if included > b.MaxInstructionBytesPerSkill || value.OriginalInstructionBytes < included ||
			value.InstructionTruncated != (value.OriginalInstructionBytes > included) {
			return fmt.Errorf("runtime Skill %q instruction budget mismatch", value.ID)
		}
		originalBytes += value.OriginalInstructionBytes
		includedBytes += included
		metadataBytes += runtimeSkillMetadataBytes(value)
		if value.InstructionTruncated {
			truncatedSkills++
		}
	}
	if includedBytes > b.MaxInstructionBytesTotal || metadataBytes > b.MaxMetadataBytesTotal ||
		b.OriginalInstructionBytes != originalBytes || b.IncludedInstructionBytes != includedBytes ||
		b.MetadataBytes != metadataBytes || b.TruncatedSkills != truncatedSkills {
		return fmt.Errorf("runtime Skill context budget totals mismatch")
	}
	return nil
}

func sameRuntimeSkillContextBudget(left, right *RuntimeSkillContextBudget) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Normalized() == right.Normalized()
}

func runtimeSkillMetadataBytes(value RuntimeSkillContext) int {
	total := len(value.ID) + len(value.Name) + len(value.Version) + len(value.Description)
	for _, values := range [][]string{value.TriggerRules, value.ReferenceRefs, value.RequiredToolKinds, value.RequiredMCPServerRefs, value.RequiredCapabilityRefs, value.Constraints} {
		for _, item := range values {
			total += len(item)
		}
	}
	provenance := value.Provenance.Normalized()
	total += len(provenance.Source) + len(provenance.Scope) + len(provenance.WorkspaceID) + len(provenance.ContentHash)
	return total
}

func runtimeSkillContent(value RuntimeSkillContext) workorchskillcontract.Content {
	return workorchskillcontract.Content{
		ID: value.ID, Name: value.Name, Version: value.Version, Description: value.Description,
		TriggerRules: append([]string(nil), value.TriggerRules...), Instruction: value.Instruction,
		ReferenceRefs: append([]string(nil), value.ReferenceRefs...), RequiredToolKinds: append([]string(nil), value.RequiredToolKinds...),
		RequiredMCPServerRefs: append([]string(nil), value.RequiredMCPServerRefs...), RequiredCapabilityRefs: append([]string(nil), value.RequiredCapabilityRefs...),
		Constraints: append([]string(nil), value.Constraints...),
	}
}

func truncateRuntimeSkillInstruction(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	limit := maxBytes
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return strings.TrimSpace(value[:limit])
}
