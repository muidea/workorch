package skill

import (
	"reflect"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

// ValidateContent checks already-normalized material. Normalization remains an
// explicit create-time operation; observations must never repair stored data.
func ValidateContent(content workorchskillcontract.Content) error {
	if content.ID == "" || content.Name == "" || content.Version == "" || content.Instruction == "" ||
		!utf8.ValidString(content.ID) || !utf8.ValidString(content.Name) || !utf8.ValidString(content.Version) ||
		!utf8.ValidString(content.Description) || !utf8.ValidString(content.Instruction) ||
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
			if value == "" || !utf8.ValidString(value) || len(value) > modulecommon.SkillListValueMaxBytes {
				return common.ErrInvalidArgument
			}
		}
	}
	return nil
}

// ValidateRegistrySnapshotSkill validates the full observed record, not merely
// a normalized hash. No SkillHub fallback, migration or owner write is allowed.
func ValidateRegistrySnapshotSkill(item *Skill) error {
	if item == nil {
		return common.ErrConflict
	}
	content := item.Content()
	if ValidateContent(content) != nil || !reflect.DeepEqual(content, content.Normalized()) {
		return common.ErrConflict
	}
	p := item.Provenance
	if p != p.Normalized() || p.Validate() != nil || p.Source != workorchskillcontract.SourceWorkOrchRegistry ||
		p.Scope != workorchskillcontract.ScopeGlobal || p.WorkspaceID != "" || p.ContentHash != workorchskillcontract.StableContentHash(content) {
		return common.ErrConflict
	}
	return nil
}
