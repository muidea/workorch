package workorchruntimecontract

import (
	"fmt"
	"strings"
)

const RuntimeDefinitionChangeImpactVersionCurrent = "workorch.definition_change_impact"

type RuntimeDefinitionChangeImpact struct {
	Version                        string
	Changed                        bool
	Policy                         string
	RequiredPreExecutionAssessment bool
	GoalChange                     RuntimeDefinitionChangeSet
	DoDChange                      RuntimeDefinitionChangeSet
	StrategyRequirements           []string
}

type RuntimeDefinitionChangeSet struct {
	EffectiveDefinitionID string
	LatestDefinitionID    string
	PreviousDefinitionIDs []string
	AllDefinitionIDs      []string
	DefinitionRevisions   []RuntimeDefinitionRevision
	ChangedInPlace        bool
	AllEntriesConstrain   bool
}

type RuntimeDefinitionRevision struct {
	ID                     string
	RevisionCount          int
	Source                 string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDefinitionID   string
	LastEventID            string
	LastOccurredAt         string
}

func (i RuntimeDefinitionChangeImpact) Normalized() RuntimeDefinitionChangeImpact {
	i.Version = strings.TrimSpace(i.Version)
	i.Policy = strings.TrimSpace(i.Policy)
	i.GoalChange = i.GoalChange.normalized()
	i.DoDChange = i.DoDChange.normalized()
	i.StrategyRequirements = orderedStrings(i.StrategyRequirements)
	return i
}

func (i RuntimeDefinitionChangeImpact) Validate() error {
	i = i.Normalized()
	if i.Version != RuntimeDefinitionChangeImpactVersionCurrent || !i.Changed {
		return fmt.Errorf("runtime definition change impact requires v1 changed contract")
	}
	if i.GoalChange.EffectiveDefinitionID == "" || i.DoDChange.EffectiveDefinitionID == "" {
		return fmt.Errorf("runtime definition change impact requires effective Goal and DoD identity")
	}
	return nil
}

func (s RuntimeDefinitionChangeSet) normalized() RuntimeDefinitionChangeSet {
	s.EffectiveDefinitionID = strings.TrimSpace(s.EffectiveDefinitionID)
	s.LatestDefinitionID = strings.TrimSpace(s.LatestDefinitionID)
	s.PreviousDefinitionIDs = orderedStrings(s.PreviousDefinitionIDs)
	s.AllDefinitionIDs = orderedStrings(s.AllDefinitionIDs)
	revisions := make([]RuntimeDefinitionRevision, 0, len(s.DefinitionRevisions))
	seen := map[string]struct{}{}
	for _, revision := range s.DefinitionRevisions {
		revision.ID = strings.TrimSpace(revision.ID)
		revision.Source = strings.TrimSpace(revision.Source)
		revision.DefinitionRevision = strings.TrimSpace(revision.DefinitionRevision)
		revision.PreviousDefinitionHash = strings.TrimSpace(revision.PreviousDefinitionHash)
		revision.PreviousDefinitionID = strings.TrimSpace(revision.PreviousDefinitionID)
		revision.LastEventID = strings.TrimSpace(revision.LastEventID)
		revision.LastOccurredAt = strings.TrimSpace(revision.LastOccurredAt)
		if revision.ID == "" {
			continue
		}
		if _, ok := seen[revision.ID]; ok {
			continue
		}
		seen[revision.ID] = struct{}{}
		revisions = append(revisions, revision)
	}
	s.DefinitionRevisions = nilIfEmpty(revisions)
	return s
}

func SameRuntimeDefinitionChangeImpact(left *RuntimeDefinitionChangeImpact, right *RuntimeDefinitionChangeImpact) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.Version == r.Version && l.Changed == r.Changed && l.Policy == r.Policy &&
		l.RequiredPreExecutionAssessment == r.RequiredPreExecutionAssessment && sameRuntimeDefinitionChangeSet(l.GoalChange, r.GoalChange) &&
		sameRuntimeDefinitionChangeSet(l.DoDChange, r.DoDChange) && sameStrings(l.StrategyRequirements, r.StrategyRequirements)
}

func sameRuntimeDefinitionChangeSet(left RuntimeDefinitionChangeSet, right RuntimeDefinitionChangeSet) bool {
	if left.EffectiveDefinitionID != right.EffectiveDefinitionID || left.LatestDefinitionID != right.LatestDefinitionID ||
		left.ChangedInPlace != right.ChangedInPlace || left.AllEntriesConstrain != right.AllEntriesConstrain ||
		!sameOrderedStrings(left.PreviousDefinitionIDs, right.PreviousDefinitionIDs) || !sameOrderedStrings(left.AllDefinitionIDs, right.AllDefinitionIDs) ||
		len(left.DefinitionRevisions) != len(right.DefinitionRevisions) {
		return false
	}
	for idx := range left.DefinitionRevisions {
		if left.DefinitionRevisions[idx] != right.DefinitionRevisions[idx] {
			return false
		}
	}
	return true
}

func orderedStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return nilIfEmpty(ret)
}

func sameOrderedStrings(left []string, right []string) bool {
	left, right = orderedStrings(left), orderedStrings(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
