package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeAcceptanceSlicePolicySchemaV1  = "workorch.acceptance_slice_policy.v1"
	RuntimeAcceptanceSliceCompletionAll   = "all_required_items_accepted"
	RuntimeAcceptanceSliceResultAllowed   = "slice_result_allowed"
	RuntimeCoverageItemStateOpen          = "open"
	RuntimeCoverageItemStateSatisfied     = "satisfied"
	RuntimeCoverageItemStateUnsatisfied   = "unsatisfied"
	RuntimeAcceptanceSliceMaxWorkItemsMin = 1
	RuntimeAcceptanceSliceMaxWorkItemsMax = 32
	RuntimeAcceptanceSliceDefaultMaxItems = 1
	RuntimeAcceptanceFocusMaxCount        = 64
)

// RuntimeAcceptanceSlicePolicy is the typed activation policy for grouping coverage
// work items into bounded acceptance slices. Kernel does not interpret business titles.
type RuntimeAcceptanceSlicePolicy struct {
	SchemaVersion    string
	MaxWorkItems     int
	CompletionPolicy string
	ResultPolicy     string
	Groups           []RuntimeAcceptanceSliceGroupPolicy
}

type RuntimeAcceptanceSliceGroupPolicy struct {
	ID           string
	Title        string
	Scope        string
	Priority     int
	MaxWorkItems int
	Match        RuntimeAcceptanceSliceMatch
}

type RuntimeAcceptanceSliceMatch struct {
	EvidenceRoles        []string
	MissingEvidenceKinds []string
	SourceRequirementIDs []string
	SourceScopes         []string
	All                  bool
}

// RuntimeAcceptanceSlice is one compiled bounded slice in the focus queue.
type RuntimeAcceptanceSlice struct {
	ID                    string
	Title                 string
	Scope                 string
	Priority              int
	CoverageWorkItemIDs   []common.CoverageWorkItemID
	RequiredEvidenceKinds []string
	CompletionPolicy      string
	ResultPolicy          string
}

// RuntimeCoverageItem is the stable item-level coverage identity independent of slice size.
type RuntimeCoverageItem struct {
	ID          common.CoverageWorkItemID
	SliceID     string
	SourceScope string
	SourceID    string
	Order       int
}

type RuntimeCoverageItemState string

type RuntimeCoverageItemFact struct {
	ID               common.CoverageWorkItemID
	SliceID          string
	State            RuntimeCoverageItemState
	EvidenceRefs     []string
	LastAssessmentID string
}

// InitialRuntimeCoverageFacts materializes the item closure ledger from the
// sealed bootstrap. Legacy bootstraps without CoverageItems retain one item per
// focus membership so that an active Run can be migrated without replaying it.
func InitialRuntimeCoverageFacts(items []RuntimeCoverageItem, focuses []CoverageFocus) []RuntimeCoverageItemFact {
	items = NormalizeRuntimeCoverageItems(items)
	if len(items) == 0 {
		for _, focus := range focuses {
			for _, id := range focus.Normalized().CoverageWorkItemIDs {
				items = append(items, RuntimeCoverageItem{ID: id, SliceID: focus.SliceID})
			}
		}
	}
	ret := make([]RuntimeCoverageItemFact, 0, len(items))
	seen := map[common.CoverageWorkItemID]struct{}{}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ret = append(ret, RuntimeCoverageItemFact{ID: item.ID, SliceID: item.SliceID, State: RuntimeCoverageItemStateOpen})
	}
	return NormalizeRuntimeCoverageFacts(ret)
}

func NormalizeRuntimeCoverageFacts(values []RuntimeCoverageItemFact) []RuntimeCoverageItemFact {
	ret := make([]RuntimeCoverageItemFact, 0, len(values))
	seen := map[common.CoverageWorkItemID]struct{}{}
	for _, value := range values {
		value.ID = common.CoverageWorkItemID(strings.TrimSpace(string(value.ID)))
		value.SliceID = strings.TrimSpace(value.SliceID)
		value.State = RuntimeCoverageItemState(strings.TrimSpace(string(value.State)))
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.LastAssessmentID = strings.TrimSpace(value.LastAssessmentID)
		if value.ID == "" || value.State == "" {
			continue
		}
		switch value.State {
		case RuntimeCoverageItemStateOpen, RuntimeCoverageItemStateSatisfied, RuntimeCoverageItemStateUnsatisfied:
		default:
			continue
		}
		if _, ok := seen[value.ID]; ok {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return string(ret[i].ID) < string(ret[j].ID) })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (p RuntimeAcceptanceSlicePolicy) Normalized() RuntimeAcceptanceSlicePolicy {
	p.SchemaVersion = strings.TrimSpace(p.SchemaVersion)
	p.CompletionPolicy = strings.TrimSpace(p.CompletionPolicy)
	p.ResultPolicy = strings.TrimSpace(p.ResultPolicy)
	if p.CompletionPolicy == "" {
		p.CompletionPolicy = RuntimeAcceptanceSliceCompletionAll
	}
	if p.ResultPolicy == "" {
		p.ResultPolicy = RuntimeAcceptanceSliceResultAllowed
	}
	if p.MaxWorkItems <= 0 {
		p.MaxWorkItems = RuntimeAcceptanceSliceDefaultMaxItems
	}
	groups := make([]RuntimeAcceptanceSliceGroupPolicy, 0, len(p.Groups))
	for _, group := range p.Groups {
		group.ID = strings.TrimSpace(group.ID)
		group.Title = strings.TrimSpace(group.Title)
		group.Scope = strings.TrimSpace(group.Scope)
		group.Match.EvidenceRoles = normalizeStrings(group.Match.EvidenceRoles)
		group.Match.MissingEvidenceKinds = normalizeStrings(group.Match.MissingEvidenceKinds)
		group.Match.SourceRequirementIDs = normalizeStrings(group.Match.SourceRequirementIDs)
		group.Match.SourceScopes = normalizeStrings(group.Match.SourceScopes)
		if group.ID == "" {
			continue
		}
		if group.MaxWorkItems < 0 {
			group.MaxWorkItems = 0
		}
		groups = append(groups, group)
	}
	// Stable group order: priority ascending, then declaration order (stable sort).
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Priority != groups[j].Priority {
			return groups[i].Priority < groups[j].Priority
		}
		return false
	})
	p.Groups = groups
	return p
}

func (p RuntimeAcceptanceSlicePolicy) Validate() error {
	p = p.Normalized()
	if p.SchemaVersion == "" {
		// legacy missing schema is allowed only for normalization paths; new activation must set v1.
		return nil
	}
	if p.SchemaVersion != RuntimeAcceptanceSlicePolicySchemaV1 {
		return fmt.Errorf("acceptance slice policy has unsupported schema %q", p.SchemaVersion)
	}
	if p.MaxWorkItems < RuntimeAcceptanceSliceMaxWorkItemsMin || p.MaxWorkItems > RuntimeAcceptanceSliceMaxWorkItemsMax {
		return fmt.Errorf("acceptance slice policy maxWorkItems must be in %d..%d", RuntimeAcceptanceSliceMaxWorkItemsMin, RuntimeAcceptanceSliceMaxWorkItemsMax)
	}
	if p.CompletionPolicy != RuntimeAcceptanceSliceCompletionAll {
		return fmt.Errorf("acceptance slice policy has unsupported completionPolicy %q", p.CompletionPolicy)
	}
	if p.ResultPolicy != RuntimeAcceptanceSliceResultAllowed {
		return fmt.Errorf("acceptance slice policy has unsupported resultPolicy %q", p.ResultPolicy)
	}
	for _, group := range p.Groups {
		if group.ID == "" {
			return fmt.Errorf("acceptance slice group requires id")
		}
		if group.MaxWorkItems > 0 && (group.MaxWorkItems < RuntimeAcceptanceSliceMaxWorkItemsMin || group.MaxWorkItems > p.MaxWorkItems) {
			return fmt.Errorf("acceptance slice group %q maxWorkItems must be in %d..%d", group.ID, RuntimeAcceptanceSliceMaxWorkItemsMin, p.MaxWorkItems)
		}
		if group.Match.All {
			if len(group.Match.EvidenceRoles)+len(group.Match.MissingEvidenceKinds)+len(group.Match.SourceRequirementIDs)+len(group.Match.SourceScopes) != 0 {
				return fmt.Errorf("acceptance slice group %q match.all must be the sole condition", group.ID)
			}
		}
	}
	return nil
}

func (s RuntimeAcceptanceSlice) Normalized() RuntimeAcceptanceSlice {
	s.ID = strings.TrimSpace(s.ID)
	s.Title = strings.TrimSpace(s.Title)
	s.Scope = strings.TrimSpace(s.Scope)
	s.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(s.CoverageWorkItemIDs)
	s.RequiredEvidenceKinds = normalizeStrings(s.RequiredEvidenceKinds)
	s.CompletionPolicy = strings.TrimSpace(s.CompletionPolicy)
	s.ResultPolicy = strings.TrimSpace(s.ResultPolicy)
	return s
}

func (c RuntimeCoverageItem) Normalized() RuntimeCoverageItem {
	c.ID = common.CoverageWorkItemID(strings.TrimSpace(string(c.ID)))
	c.SliceID = strings.TrimSpace(c.SliceID)
	c.SourceScope = strings.TrimSpace(c.SourceScope)
	c.SourceID = strings.TrimSpace(c.SourceID)
	return c
}

func NormalizeRuntimeAcceptanceSlices(values []RuntimeAcceptanceSlice) []RuntimeAcceptanceSlice {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAcceptanceSlice, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID == "" || len(value.CoverageWorkItemIDs) == 0 {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func NormalizeRuntimeCoverageItems(values []RuntimeCoverageItem) []RuntimeCoverageItem {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCoverageItem, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID == "" {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// EffectiveMaxWorkItems returns the group cap clamped by the global policy max.
func (p RuntimeAcceptanceSlicePolicy) EffectiveMaxWorkItems(group RuntimeAcceptanceSliceGroupPolicy) int {
	p = p.Normalized()
	maxItems := p.MaxWorkItems
	if group.MaxWorkItems > 0 && group.MaxWorkItems < maxItems {
		maxItems = group.MaxWorkItems
	}
	if maxItems < RuntimeAcceptanceSliceMaxWorkItemsMin {
		maxItems = RuntimeAcceptanceSliceMaxWorkItemsMin
	}
	if maxItems > RuntimeAcceptanceSliceMaxWorkItemsMax {
		maxItems = RuntimeAcceptanceSliceMaxWorkItemsMax
	}
	return maxItems
}
