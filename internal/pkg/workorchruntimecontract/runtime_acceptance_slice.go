package workorchruntimecontract

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeAcceptanceSlicePolicySchemaCurrent = "workorch.acceptance_slice_policy"
	RuntimeAcceptanceSliceCompletionAll       = "all_required_items_accepted"
	RuntimeAcceptanceSliceResultAllowed       = "slice_result_allowed"
	RuntimeCoverageItemStateOpen              = "open"
	RuntimeCoverageItemStateSatisfied         = "satisfied"
	RuntimeCoverageItemStateUnsatisfied       = "unsatisfied"
	RuntimeAcceptanceSliceMaxWorkItemsMin     = 1
	RuntimeAcceptanceSliceMaxWorkItemsMax     = 32
	RuntimeAcceptanceSliceDefaultMaxItems     = 1
	RuntimeAcceptanceFocusMaxCount            = 128
	// RuntimeOwnerInventoryItemsMaxCount bounds a complete capability-owner
	// feasibility inventory, including File target selection and operation
	// admissibility. It is deliberately independent from
	// MaxCandidateContextItems: owner inventory establishes an SM-2 fact,
	// whereas the latter bounds one candidate request's contextual work-item
	// projection.
	RuntimeOwnerInventoryItemsMaxCount   = 32
	RuntimeCandidateContextItemsMaxCount = 32
)

// RuntimeAcceptanceSlicePolicy is the typed activation policy for grouping coverage
// work items into bounded acceptance slices. Kernel does not interpret business titles.
type RuntimeAcceptanceSlicePolicy struct {
	SchemaVersion            string
	MaxWorkItems             int
	MaxFocuses               int
	MaxCandidateContextItems int
	CompletionPolicy         string
	ResultPolicy             string
	Groups                   []RuntimeAcceptanceSliceGroupPolicy
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
	TargetText  string
	Order       int
	// SourceObligationChecks 保留共享工作上的独立来源判据，不授予 credit。
	SourceObligationChecks  []RuntimeSourceObligationCheck     `json:",omitempty"`
	GoalExecutionAssertions []RuntimeLineageAssertion          `json:",omitempty"`
	HistoricalAcceptance    *RuntimeHistoricalAcceptanceSource `json:",omitempty"`
	// PrerequisiteCoverageWorkItemIDs constrain semantic-plan admission only;
	// they never reopen an already satisfied CWI.
	PrerequisiteCoverageWorkItemIDs []common.CoverageWorkItemID
	// ReopenDependencies are downstream relations applied after accepted
	// evidence; they are not semantic-plan prerequisites.
	ReopenDependencies []RuntimeCoverageReopenDependency
}

// RuntimeCoverageReopenDependency is the compiled coverage-ID form of a
// profile dependency. It is data only: the Orchestrator decides when an
// accepted evidence fact satisfies its trigger and performs the transition.
type RuntimeCoverageReopenDependency struct {
	CoverageWorkItemID   common.CoverageWorkItemID
	TriggerEvidenceKinds []string
}

type RuntimeCoverageItemState string

type RuntimeCoverageItemFact struct {
	ID                              common.CoverageWorkItemID
	SliceID                         string
	SourceScope                     string
	SourceID                        string
	TargetText                      string
	State                           RuntimeCoverageItemState
	EvidenceRefs                    []string
	LastAssessmentID                string
	PrerequisiteCoverageWorkItemIDs []common.CoverageWorkItemID
	ReopenDependencies              []RuntimeCoverageReopenDependency
	ReopenedByEvidenceRefs          []string
	// CWIConvergenceState, CurrentBindingRef and FocusID form the canonical
	// persisted CWI ledger. They are owned by the Orchestrator SM-2 reducer;
	// capability owners only return observations and receipts.
	CWIConvergenceState RuntimeCWIConvergenceState
	CurrentBindingRef   *RuntimeCWIBindingRef
	BindingLineage      []RuntimeCWIBindingRef
	// BindingRetirementLineage records the only legal pre-effect removal of a
	// current binding. It is append-only SM-2 audit, not a second binding owner.
	BindingRetirementLineage   []RuntimeCWIBindingRetirementLineage
	FocusID                    string
	ReopenLineage              []RuntimeCWIReopenLineage
	HistoricalBehavior         *RuntimeHistoricalBehaviorCompletion `json:",omitempty"`
	HistoricalAcceptanceProofs []RuntimeHistoricalAcceptanceProof   `json:",omitempty"`
}

// InitialRuntimeCoverageFacts materializes the item closure ledger from the
// explicit sealed bootstrap inventory.
func InitialRuntimeCoverageFacts(items []RuntimeCoverageItem) []RuntimeCoverageItemFact {
	items = NormalizeRuntimeCoverageItems(items)
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
		ret = append(ret, RuntimeCoverageItemFact{
			ID: item.ID, SliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID,
			TargetText: item.TargetText, State: RuntimeCoverageItemStateOpen,
			PrerequisiteCoverageWorkItemIDs: cloneRuntimeCoveragePrerequisiteIDs(item.PrerequisiteCoverageWorkItemIDs),
			ReopenDependencies:              cloneRuntimeCoverageReopenDependencies(item.ReopenDependencies),
		})
	}
	return NormalizeRuntimeCoverageFacts(ret)
}

func NormalizeRuntimeCoverageFacts(values []RuntimeCoverageItemFact) []RuntimeCoverageItemFact {
	ret := make([]RuntimeCoverageItemFact, 0, len(values))
	seen := map[common.CoverageWorkItemID]struct{}{}
	for _, value := range values {
		value.ID = common.CoverageWorkItemID(strings.TrimSpace(string(value.ID)))
		value.SliceID = strings.TrimSpace(value.SliceID)
		value.SourceScope = strings.TrimSpace(value.SourceScope)
		value.SourceID = strings.TrimSpace(value.SourceID)
		value.TargetText = strings.TrimSpace(value.TargetText)
		value.State = RuntimeCoverageItemState(strings.TrimSpace(string(value.State)))
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.LastAssessmentID = strings.TrimSpace(value.LastAssessmentID)
		value.PrerequisiteCoverageWorkItemIDs = normalizeRuntimeCoveragePrerequisiteIDs(value.PrerequisiteCoverageWorkItemIDs)
		value.ReopenDependencies = normalizeRuntimeCoverageReopenDependencies(value.ReopenDependencies)
		value.ReopenedByEvidenceRefs = normalizeStrings(value.ReopenedByEvidenceRefs)
		value = normalizeRuntimeCoverageItemFact(value)
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
	if p.SchemaVersion != RuntimeAcceptanceSlicePolicySchemaCurrent {
		return fmt.Errorf("acceptance slice policy has unsupported schema %q", p.SchemaVersion)
	}
	if p.MaxWorkItems < RuntimeAcceptanceSliceMaxWorkItemsMin || p.MaxWorkItems > RuntimeAcceptanceSliceMaxWorkItemsMax {
		return fmt.Errorf("acceptance slice policy maxWorkItems must be in %d..%d", RuntimeAcceptanceSliceMaxWorkItemsMin, RuntimeAcceptanceSliceMaxWorkItemsMax)
	}
	if p.MaxFocuses < 1 || p.MaxFocuses > RuntimeAcceptanceFocusMaxCount {
		return fmt.Errorf("acceptance slice policy maxFocuses must be in 1..%d", RuntimeAcceptanceFocusMaxCount)
	}
	if p.MaxCandidateContextItems < 1 || p.MaxCandidateContextItems > RuntimeCandidateContextItemsMaxCount {
		return fmt.Errorf("acceptance slice policy maxCandidateContextItems must be in 1..%d", RuntimeCandidateContextItemsMaxCount)
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
	c.HistoricalAcceptance = cloneRuntimeHistoricalAcceptanceSource(c.HistoricalAcceptance)
	c.ID = common.CoverageWorkItemID(strings.TrimSpace(string(c.ID)))
	c.SliceID = strings.TrimSpace(c.SliceID)
	c.SourceScope = strings.TrimSpace(c.SourceScope)
	c.SourceID = strings.TrimSpace(c.SourceID)
	c.TargetText = strings.TrimSpace(c.TargetText)
	c.SourceObligationChecks = CloneRuntimeSourceObligationChecks(c.SourceObligationChecks)
	c.GoalExecutionAssertions = slices.Clone(c.GoalExecutionAssertions)
	c.PrerequisiteCoverageWorkItemIDs = normalizeRuntimeCoveragePrerequisiteIDs(c.PrerequisiteCoverageWorkItemIDs)
	c.ReopenDependencies = normalizeRuntimeCoverageReopenDependencies(c.ReopenDependencies)
	return c
}

func normalizeRuntimeCoveragePrerequisiteIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	return normalizeCoverageWorkItemIDs(values)
}

func normalizeRuntimeCoverageReopenDependencies(values []RuntimeCoverageReopenDependency) []RuntimeCoverageReopenDependency {
	byID := map[common.CoverageWorkItemID]RuntimeCoverageReopenDependency{}
	for _, value := range values {
		value.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(value.CoverageWorkItemID)))
		value.TriggerEvidenceKinds = normalizeStrings(value.TriggerEvidenceKinds)
		if prior, ok := byID[value.CoverageWorkItemID]; ok {
			value.TriggerEvidenceKinds = normalizeStrings(append(prior.TriggerEvidenceKinds, value.TriggerEvidenceKinds...))
		}
		byID[value.CoverageWorkItemID] = value
	}
	keys := make([]string, 0, len(byID))
	for id := range byID {
		keys = append(keys, string(id))
	}
	sort.Strings(keys)
	ret := make([]RuntimeCoverageReopenDependency, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[common.CoverageWorkItemID(key)])
	}
	return nilIfEmpty(ret)
}

// ValidateRuntimeCoverageItemDependencies verifies the fully compiled coverage
// graph.  It is a pure contract helper used by activation and bootstrap
// validation; it does not choose focus ordering or execute a state transition.
func ValidateRuntimeCoverageItemDependencies(items []RuntimeCoverageItem) error {
	items = NormalizeRuntimeCoverageItems(items)
	if err := validateRuntimeCoveragePrerequisiteDependencies(items); err != nil {
		return err
	}
	byID := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	edges := make(map[common.CoverageWorkItemID][]common.CoverageWorkItemID, len(items))
	for _, item := range items {
		for _, dependency := range item.ReopenDependencies {
			if dependency.CoverageWorkItemID == "" || len(dependency.TriggerEvidenceKinds) == 0 {
				return fmt.Errorf("runtime coverage item %s has incomplete reopen dependency", item.ID)
			}
			if dependency.CoverageWorkItemID == item.ID {
				return fmt.Errorf("runtime coverage item %s cannot reopen itself", item.ID)
			}
			if _, ok := byID[dependency.CoverageWorkItemID]; !ok {
				return fmt.Errorf("runtime coverage item %s reopens undeclared coverage item %s", item.ID, dependency.CoverageWorkItemID)
			}
			edges[item.ID] = append(edges[item.ID], dependency.CoverageWorkItemID)
		}
	}
	return validateRuntimeCoverageDependencyGraph(edges, "reopen")
}

func validateRuntimeCoveragePrerequisiteDependencies(items []RuntimeCoverageItem) error {
	byID := make(map[common.CoverageWorkItemID]struct{}, len(items))
	for _, item := range items {
		byID[item.ID] = struct{}{}
	}
	edges := make(map[common.CoverageWorkItemID][]common.CoverageWorkItemID, len(items))
	for _, item := range items {
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			if prerequisiteID == "" {
				return fmt.Errorf("runtime coverage item %s has an empty prerequisite dependency", item.ID)
			}
			if prerequisiteID == item.ID {
				return fmt.Errorf("runtime coverage item %s cannot depend on itself", item.ID)
			}
			if _, ok := byID[prerequisiteID]; !ok {
				return fmt.Errorf("runtime coverage item %s depends on undeclared coverage item %s", item.ID, prerequisiteID)
			}
			edges[item.ID] = append(edges[item.ID], prerequisiteID)
		}
	}
	return validateRuntimeCoverageDependencyGraph(edges, "prerequisite")
}

func validateRuntimeCoverageDependencyGraph(edges map[common.CoverageWorkItemID][]common.CoverageWorkItemID, relation string) error {
	visiting := map[common.CoverageWorkItemID]bool{}
	visited := map[common.CoverageWorkItemID]bool{}
	var visit func(common.CoverageWorkItemID) error
	visit = func(id common.CoverageWorkItemID) error {
		if visiting[id] {
			return fmt.Errorf("runtime coverage %s dependencies contain a cycle at %s", relation, id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range edges[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range edges {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func cloneRuntimeCoverageReopenDependencies(values []RuntimeCoverageReopenDependency) []RuntimeCoverageReopenDependency {
	values = normalizeRuntimeCoverageReopenDependencies(values)
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCoverageReopenDependency, len(values))
	for idx := range values {
		ret[idx] = values[idx]
		ret[idx].TriggerEvidenceKinds = append([]string(nil), values[idx].TriggerEvidenceKinds...)
	}
	return ret
}

func cloneRuntimeCoveragePrerequisiteIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	values = normalizeRuntimeCoveragePrerequisiteIDs(values)
	if len(values) == 0 {
		return nil
	}
	return append([]common.CoverageWorkItemID(nil), values...)
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
