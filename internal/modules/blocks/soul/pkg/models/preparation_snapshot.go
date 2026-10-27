package soul

import (
	"strings"
	"unicode/utf8"

	soulcommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidPreparationSoulID(id common.SoulID) bool {
	v := string(id)
	return v != "" && strings.TrimSpace(v) == v && v != "." && v != ".." && !strings.ContainsAny(v, `/\`) && utf8.ValidString(v) && len(v) <= soulcommon.SoulIDMaxBytes
}

// ValidPreparationSnapshot checks a bounded detached record, not readiness.
// ContentHash alone does not bind revision, provenance or list partitions.
func ValidPreparationSnapshot(item *Soul, id common.SoulID) bool {
	if item == nil || item.ID != id || !ValidPreparationSoulID(id) || item.Revision == 0 || !ValidSoulSourceProvenance(item.SourceProvenance) {
		return false
	}
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.SoulMD) == "" || !utf8.ValidString(item.Name) || !utf8.ValidString(item.Description) || !utf8.ValidString(item.SoulMD) || len(item.Name) > soulcommon.SoulNameMaxBytes || len(item.Description) > soulcommon.SoulDescriptionMaxBytes || len(item.SoulMD) > soulcommon.SoulInstructionMaxBytes {
		return false
	}
	total := len(item.ID) + len(item.Name) + len(item.Description) + len(item.SoulMD)
	for _, values := range [][]string{item.BusinessDomains, item.AllowedSkillRefs, item.AllowedMCPServerRefs, item.AllowedCapabilityRefs, item.AllowedCapabilityCategories, item.AllowedToolKinds} {
		if len(values) > soulcommon.SoulListValueMaxCount {
			return false
		}
		for _, v := range values {
			if v == "" || !utf8.ValidString(v) || len(v) > soulcommon.SoulListValueMaxBytes {
				return false
			}
			total += len(v)
		}
	}
	return total <= soulcommon.SoulContentMaxBytes && item.ContentHash == StableSoulContentHash(item)
}
