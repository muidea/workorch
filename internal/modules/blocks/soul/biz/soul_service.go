package soul

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	souls repository.SoulRepository
}

func NewService(souls repository.SoulRepository) *Service {
	return &Service{souls: souls}
}

func (s *Service) CreateSoul(ctx context.Context, cmd soulevents.CreateSoulCommand) (*soulmodel.Soul, error) {
	return s.createSoul(ctx, cmd, soulmodel.SoulSourceUserManaged)
}

func (s *Service) createSoul(ctx context.Context, cmd soulevents.CreateSoulCommand, provenance string) (*soulmodel.Soul, error) {
	item, err := normalizedSoulFromCommand(cmd)
	if err != nil {
		return nil, err
	}
	item.Revision = 1
	item.SourceProvenance = strings.TrimSpace(provenance)
	if !soulmodel.ValidSoulSourceProvenance(item.SourceProvenance) {
		return nil, common.ErrInvalidArgument
	}
	item.ContentHash = soulmodel.StableSoulContentHash(item)
	if err := s.souls.Create(ctx, item); err != nil {
		return nil, err
	}
	return soulmodel.CloneSoul(item), nil
}

func (s *Service) UpdateSoul(ctx context.Context, cmd soulevents.CreateSoulCommand) (*soulmodel.Soul, error) {
	item, err := normalizedSoulFromCommand(cmd)
	if err != nil {
		return nil, err
	}
	current, err := s.GetSoul(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	item.Revision = current.Revision
	item.SourceProvenance = current.SourceProvenance
	item.ContentHash = soulmodel.StableSoulContentHash(item)
	if item.ContentHash == current.ContentHash {
		return current, nil
	}
	item.Revision++
	if err := s.souls.Update(ctx, item); err != nil {
		return nil, err
	}
	return soulmodel.CloneSoul(item), nil
}

func (s *Service) DeleteSoul(ctx context.Context, id common.SoulID) error {
	if err := validateSoulID(id); err != nil {
		return err
	}
	return s.souls.Delete(ctx, id)
}

func (s *Service) GetSoul(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	if err := validateSoulID(id); err != nil {
		return nil, err
	}
	item, err := s.souls.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.ID != id {
		return nil, fmt.Errorf("%w: stored Soul identity mismatch", common.ErrConflict)
	}
	return normalizedStoredSoul(item)
}

func (s *Service) ListSouls(ctx context.Context, cursorAfterID common.SoulID, limit int) ([]*soulmodel.Soul, common.SoulID, error) {
	if cursorAfterID != "" {
		if err := validateSoulID(cursorAfterID); err != nil {
			return nil, "", err
		}
	}
	limit = normalizedSoulListLimit(limit)
	items, err := s.souls.List(ctx)
	if err != nil {
		return nil, "", err
	}
	validated := make([]*soulmodel.Soul, 0, len(items))
	seen := make(map[common.SoulID]struct{}, len(items))
	for _, item := range items {
		value, err := normalizedStoredSoul(item)
		if err != nil {
			return nil, "", err
		}
		if _, duplicate := seen[value.ID]; duplicate {
			return nil, "", fmt.Errorf("%w: duplicate stored Soul identity", common.ErrConflict)
		}
		seen[value.ID] = struct{}{}
		validated = append(validated, value)
	}
	sort.Slice(validated, func(i, j int) bool { return validated[i].ID < validated[j].ID })
	page := make([]*soulmodel.Soul, 0, min(limit+1, len(validated)))
	for _, item := range validated {
		if cursorAfterID != "" && item.ID <= cursorAfterID {
			continue
		}
		page = append(page, item)
		if len(page) > limit {
			break
		}
	}
	var nextCursor common.SoulID
	if len(page) > limit {
		nextCursor = page[limit-1].ID
		page = page[:limit]
	}
	return soulmodel.CloneSouls(page), nextCursor, nil
}

// ValidateCapabilitySelection validates only this owner's five canonical
// selection allowlists. Resource existence, MCP server/capability ownership,
// effect, risk and approval are authoritative facts of their respective
// capability owners and are deliberately not inferred here.
func (s *Service) ValidateCapabilitySelection(ctx context.Context, cmd soulevents.ValidateCapabilitySelectionCommand) error {
	if err := validateSoulID(cmd.SoulID); err != nil {
		return err
	}
	selected, err := normalizedCapabilitySelection(cmd)
	if err != nil {
		return err
	}
	item, err := s.GetSoul(ctx, cmd.SoulID)
	if err != nil {
		return err
	}
	if value, ok := firstDisallowedSelection(selected.SkillRefs, item.AllowedSkillRefs); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "skill", value)
	}
	if value, ok := firstDisallowedSelection(selected.MCPServerRefs, item.AllowedMCPServerRefs); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "MCP server", value)
	}
	if value, ok := firstDisallowedSelection(selected.CapabilityRefs, item.AllowedCapabilityRefs); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "MCP capability", value)
	}
	if value, ok := firstDisallowedSelection(selected.CapabilityCategories, item.AllowedCapabilityCategories); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "MCP capability category", value)
	}
	if value, ok := firstDisallowedSelection(selected.ToolKinds, item.AllowedToolKinds); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "tool kind", value)
	}
	return nil
}

func normalizedSoulFromCommand(cmd soulevents.CreateSoulCommand) (*soulmodel.Soul, error) {
	return normalizeAndValidateSoul(&soulmodel.Soul{
		ID:                          cmd.SoulID,
		Name:                        cmd.Name,
		Description:                 cmd.Description,
		BusinessDomains:             append([]string(nil), cmd.BusinessDomains...),
		SoulMD:                      cmd.SoulMD,
		AllowedSkillRefs:            append([]string(nil), cmd.AllowedSkillRefs...),
		AllowedMCPServerRefs:        append([]string(nil), cmd.AllowedMCPServerRefs...),
		AllowedCapabilityRefs:       append([]string(nil), cmd.AllowedCapabilityRefs...),
		AllowedCapabilityCategories: append([]string(nil), cmd.AllowedCapabilityCategories...),
		AllowedToolKinds:            append([]string(nil), cmd.AllowedToolKinds...),
	})
}

func normalizedStoredSoul(item *soulmodel.Soul) (*soulmodel.Soul, error) {
	value, err := normalizeAndValidateSoul(item)
	if err != nil {
		return nil, fmt.Errorf("%w: stored Soul violates owner invariant", common.ErrConflict)
	}
	if value.Revision == 0 && value.ContentHash == "" && value.SourceProvenance == "" {
		value.Revision = 1
		value.SourceProvenance = soulmodel.SoulSourceLegacyOwned
		value.ContentHash = soulmodel.StableSoulContentHash(value)
	}
	if value.Revision == 0 || !soulmodel.ValidSoulSourceProvenance(value.SourceProvenance) || value.ContentHash != soulmodel.StableSoulContentHash(value) {
		return nil, fmt.Errorf("%w: stored Soul revision/content identity mismatch", common.ErrConflict)
	}
	return value, nil
}

func normalizeAndValidateSoul(item *soulmodel.Soul) (*soulmodel.Soul, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateSoulID(item.ID); err != nil {
		return nil, err
	}
	value := soulmodel.CloneSoul(item)
	value.Name = strings.TrimSpace(value.Name)
	value.Description = strings.TrimSpace(value.Description)
	value.SoulMD = strings.TrimSpace(value.SoulMD)
	if value.Name == "" || value.SoulMD == "" || !utf8.ValidString(value.Name) || !utf8.ValidString(value.Description) || !utf8.ValidString(value.SoulMD) {
		return nil, common.ErrInvalidArgument
	}
	value.SoulMD += "\n"
	var err error
	if value.BusinessDomains, err = normalizeSoulValues(value.BusinessDomains, false); err != nil {
		return nil, fmt.Errorf("%w: invalid businessDomains", common.ErrInvalidArgument)
	}
	if value.AllowedSkillRefs, err = normalizeSoulValues(value.AllowedSkillRefs, true); err != nil {
		return nil, fmt.Errorf("%w: invalid allowedSkillRefs", common.ErrInvalidArgument)
	}
	if value.AllowedMCPServerRefs, err = normalizeSoulValues(value.AllowedMCPServerRefs, true); err != nil {
		return nil, fmt.Errorf("%w: invalid allowedMCPServerRefs", common.ErrInvalidArgument)
	}
	if value.AllowedCapabilityRefs, err = normalizeSoulValues(value.AllowedCapabilityRefs, true); err != nil {
		return nil, fmt.Errorf("%w: invalid allowedCapabilityRefs", common.ErrInvalidArgument)
	}
	if value.AllowedCapabilityCategories, err = normalizeSoulValues(value.AllowedCapabilityCategories, true); err != nil {
		return nil, fmt.Errorf("%w: invalid allowedCapabilityCategories", common.ErrInvalidArgument)
	}
	if value.AllowedToolKinds, err = normalizeSoulValues(value.AllowedToolKinds, true); err != nil {
		return nil, fmt.Errorf("%w: invalid allowedToolKinds", common.ErrInvalidArgument)
	}
	if len(value.Name) > modulecommon.SoulNameMaxBytes || len(value.Description) > modulecommon.SoulDescriptionMaxBytes || len(value.SoulMD) > modulecommon.SoulInstructionMaxBytes || soulContentBytes(value) > modulecommon.SoulContentMaxBytes {
		return nil, common.ErrInvalidArgument
	}
	return value, nil
}

func validateSoulID(id common.SoulID) error {
	value := string(id)
	if value == "" || value != strings.TrimSpace(value) || value == "." || value == ".." || strings.ContainsAny(value, `/\`) || len(value) > modulecommon.SoulIDMaxBytes || !utf8.ValidString(value) {
		return fmt.Errorf("%w: invalid soulId", common.ErrInvalidArgument)
	}
	return nil
}

func normalizeSoulValues(values []string, allowWildcard bool) ([]string, error) {
	if len(values) > modulecommon.SoulListValueMaxCount {
		return nil, common.ErrInvalidArgument
	}
	set := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || len(value) > modulecommon.SoulListValueMaxBytes || !utf8.ValidString(value) {
			return nil, common.ErrInvalidArgument
		}
		set[value] = struct{}{}
	}
	if allowWildcard {
		if _, wildcard := set["*"]; wildcard && len(set) != 1 {
			return nil, common.ErrInvalidArgument
		}
	}
	ret := make([]string, 0, len(set))
	for value := range set {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func normalizedCapabilitySelection(cmd soulevents.ValidateCapabilitySelectionCommand) (soulevents.ValidateCapabilitySelectionCommand, error) {
	ret := cmd
	selections := []struct {
		name   string
		source []string
		target *[]string
	}{
		{name: "skill", source: cmd.SkillRefs, target: &ret.SkillRefs},
		{name: "MCP server", source: cmd.MCPServerRefs, target: &ret.MCPServerRefs},
		{name: "MCP capability", source: cmd.CapabilityRefs, target: &ret.CapabilityRefs},
		{name: "MCP capability category", source: cmd.CapabilityCategories, target: &ret.CapabilityCategories},
		{name: "tool kind", source: cmd.ToolKinds, target: &ret.ToolKinds},
	}
	for _, selection := range selections {
		for _, value := range selection.source {
			if strings.TrimSpace(value) == "" {
				return soulevents.ValidateCapabilitySelectionCommand{}, fmt.Errorf("%w: selected %s is blank", common.ErrInvalidArgument, selection.name)
			}
		}
		values, err := normalizeSoulValues(selection.source, false)
		if err != nil {
			return soulevents.ValidateCapabilitySelectionCommand{}, fmt.Errorf("%w: invalid selected %s", common.ErrInvalidArgument, selection.name)
		}
		*selection.target = values
	}
	return ret, nil
}

func firstDisallowedSelection(selected []string, allowed []string) (string, bool) {
	if slices.Contains(allowed, "*") {
		return "", false
	}
	for _, value := range selected {
		if !slices.Contains(allowed, value) {
			return value, true
		}
	}
	return "", false
}

func invalidCapabilitySelectionError(soulID common.SoulID, capability string, value string) error {
	return fmt.Errorf("%w: soul %s does not allow %s %q", common.ErrInvalidArgument, soulID, capability, value)
}

func normalizedSoulListLimit(value int) int {
	if value <= 0 {
		return modulecommon.SoulListDefaultLimit
	}
	if value > modulecommon.SoulListMaxLimit {
		return modulecommon.SoulListMaxLimit
	}
	return value
}

func soulContentBytes(item *soulmodel.Soul) int {
	if item == nil {
		return 0
	}
	total := len(item.ID) + len(item.Name) + len(item.Description) + len(item.SoulMD)
	for _, values := range [][]string{item.BusinessDomains, item.AllowedSkillRefs, item.AllowedMCPServerRefs, item.AllowedCapabilityRefs, item.AllowedCapabilityCategories, item.AllowedToolKinds} {
		for _, value := range values {
			total += len(value)
		}
	}
	return total
}
