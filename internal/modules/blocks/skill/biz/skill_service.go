package skill

import (
	"context"
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

type Service struct {
	skills repository.SkillRepository
}

func NewService(skills repository.SkillRepository) *Service {
	return &Service{skills: skills}
}

func (s *Service) CreateSkill(ctx context.Context, cmd skillevents.CreateSkillCommand) (*skillmodel.Skill, error) {
	if err := validateSkillCommand(cmd); err != nil {
		return nil, err
	}
	item := skillFromCommand(cmd)
	if err := s.skills.Create(ctx, item); err != nil {
		return nil, err
	}
	return skillmodel.CloneSkill(item), nil
}

func (s *Service) GetSkill(ctx context.Context, id common.SkillID) (*skillmodel.Skill, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	item, err := s.skills.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return validatedRegistrySkill(item)
}

func (s *Service) ListSkills(ctx context.Context, cursorAfterID common.SkillID, limit int) ([]*skillmodel.Skill, common.SkillID, error) {
	limit = normalizedSkillListLimit(limit)
	items, err := s.skills.List(ctx)
	if err != nil {
		return nil, "", err
	}
	validated := make([]*skillmodel.Skill, 0, len(items))
	for _, item := range items {
		value, err := validatedRegistrySkill(item)
		if err != nil {
			return nil, "", err
		}
		validated = append(validated, value)
	}
	sort.Slice(validated, func(i, j int) bool { return validated[i].ID < validated[j].ID })
	page := make([]*skillmodel.Skill, 0, min(limit+1, len(validated)))
	for _, item := range validated {
		if cursorAfterID != "" && item.ID <= cursorAfterID {
			continue
		}
		page = append(page, item)
		if len(page) > limit {
			break
		}
	}
	var nextCursor common.SkillID
	if len(page) > limit {
		nextCursor = page[limit-1].ID
		page = page[:limit]
	}
	return skillmodel.CloneSkills(page), nextCursor, nil
}

func validateSkillCommand(cmd skillevents.CreateSkillCommand) error {
	if cmd.SkillID == "" || strings.TrimSpace(string(cmd.SkillID)) != string(cmd.SkillID) ||
		strings.TrimSpace(cmd.Name) == "" ||
		strings.TrimSpace(cmd.Version) == "" ||
		strings.TrimSpace(cmd.Instruction) == "" {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.TriggerRules) ||
		hasBlank(cmd.ReferenceRefs) ||
		hasBlank(cmd.RequiredToolKinds) ||
		hasBlank(cmd.RequiredMCPServerRefs) ||
		hasBlank(cmd.RequiredCapabilityRefs) ||
		hasBlank(cmd.Constraints) {
		return common.ErrInvalidArgument
	}
	return validateSkillContent(skillContentFromCommand(cmd).Normalized())
}

func skillFromCommand(cmd skillevents.CreateSkillCommand) *skillmodel.Skill {
	content := skillContentFromCommand(cmd).Normalized()
	return &skillmodel.Skill{
		ID:                     common.SkillID(content.ID),
		Name:                   content.Name,
		Version:                content.Version,
		Description:            content.Description,
		TriggerRules:           cloneStrings(content.TriggerRules),
		Instruction:            content.Instruction,
		ReferenceRefs:          cloneStrings(content.ReferenceRefs),
		RequiredToolKinds:      cloneStrings(content.RequiredToolKinds),
		RequiredMCPServerRefs:  cloneStrings(content.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(content.RequiredCapabilityRefs),
		Constraints:            cloneStrings(content.Constraints),
		Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(content),
		},
	}
}

func skillContentFromCommand(cmd skillevents.CreateSkillCommand) workorchskillcontract.Content {
	return workorchskillcontract.Content{
		ID: string(cmd.SkillID), Name: cmd.Name, Version: cmd.Version, Description: cmd.Description,
		TriggerRules: cloneStrings(cmd.TriggerRules), Instruction: cmd.Instruction, ReferenceRefs: cloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds: cloneStrings(cmd.RequiredToolKinds), RequiredMCPServerRefs: cloneStrings(cmd.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(cmd.RequiredCapabilityRefs), Constraints: cloneStrings(cmd.Constraints),
	}
}

func validateSkillContent(content workorchskillcontract.Content) error {
	if content.ID == "" || content.Name == "" || content.Version == "" || content.Instruction == "" ||
		len(content.ID) > modulecommon.SkillIDMaxBytes || len(content.Name) > modulecommon.SkillNameMaxBytes ||
		len(content.Version) > modulecommon.SkillVersionMaxBytes || len(content.Description) > modulecommon.SkillDescriptionMaxBytes ||
		len(content.Instruction) > modulecommon.SkillInstructionMaxBytes || content.ByteSize() > modulecommon.SkillContentMaxBytes {
		return common.ErrInvalidArgument
	}
	for _, values := range [][]string{content.TriggerRules, content.ReferenceRefs, content.RequiredToolKinds, content.RequiredMCPServerRefs, content.RequiredCapabilityRefs, content.Constraints} {
		if len(values) > modulecommon.SkillListValueMaxCount {
			return common.ErrInvalidArgument
		}
		for _, value := range values {
			if value == "" || len(value) > modulecommon.SkillListValueMaxBytes {
				return common.ErrInvalidArgument
			}
		}
	}
	return nil
}

func validatedRegistrySkill(item *skillmodel.Skill) (*skillmodel.Skill, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	content := item.Content().Normalized()
	if err := validateSkillContent(content); err != nil {
		return nil, common.ErrConflict
	}
	provenance := item.Provenance.Normalized()
	if err := provenance.Validate(); err != nil || provenance.Source != workorchskillcontract.SourceWorkOrchRegistry ||
		provenance.Scope != workorchskillcontract.ScopeGlobal || provenance.WorkspaceID != "" ||
		provenance.ContentHash != workorchskillcontract.StableContentHash(content) {
		return nil, common.ErrConflict
	}
	normalized := skillFromContent(content, provenance)
	if !sameStoredSkill(item, normalized) {
		return nil, common.ErrConflict
	}
	return normalized, nil
}

func skillFromContent(content workorchskillcontract.Content, provenance workorchskillcontract.Provenance) *skillmodel.Skill {
	return &skillmodel.Skill{
		ID: common.SkillID(content.ID), Name: content.Name, Version: content.Version, Description: content.Description,
		TriggerRules: cloneStrings(content.TriggerRules), Instruction: content.Instruction, ReferenceRefs: cloneStrings(content.ReferenceRefs),
		RequiredToolKinds: cloneStrings(content.RequiredToolKinds), RequiredMCPServerRefs: cloneStrings(content.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(content.RequiredCapabilityRefs), Constraints: cloneStrings(content.Constraints), Provenance: provenance,
	}
}

func sameStoredSkill(left, right *skillmodel.Skill) bool {
	if left == nil || right == nil || left.ID != right.ID || left.Name != right.Name || left.Version != right.Version ||
		left.Description != right.Description || left.Instruction != right.Instruction || !workorchskillcontract.SameProvenance(left.Provenance, right.Provenance) {
		return false
	}
	for idx, values := range [][]string{left.TriggerRules, left.ReferenceRefs, left.RequiredToolKinds, left.RequiredMCPServerRefs, left.RequiredCapabilityRefs, left.Constraints} {
		rightValues := [][]string{right.TriggerRules, right.ReferenceRefs, right.RequiredToolKinds, right.RequiredMCPServerRefs, right.RequiredCapabilityRefs, right.Constraints}[idx]
		if len(values) != len(rightValues) {
			return false
		}
		for valueIdx := range values {
			if values[valueIdx] != rightValues[valueIdx] {
				return false
			}
		}
	}
	return true
}

func normalizedSkillListLimit(value int) int {
	if value <= 0 {
		return modulecommon.SkillListDefaultLimit
	}
	if value > modulecommon.SkillListMaxLimit {
		return modulecommon.SkillListMaxLimit
	}
	return value
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}
