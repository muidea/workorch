// Package workorchquerycontract contains owner-neutral, immutable query values
// shared by owner EventHub commands and repository implementations. It defines
// no topic, command/result root, repository behavior, or state transition.
package workorchquerycontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RunResourcePageQuery is the canonical immutable cursor query shared by all
// application read projections scoped to one Workspace and Run.
type RunResourcePageQuery struct {
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	CursorAfterID string
	Limit         int
}

func (q RunResourcePageQuery) Normalized(defaultLimit, maxLimit int) RunResourcePageQuery {
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.RunID = common.RunID(strings.TrimSpace(string(q.RunID)))
	q.CursorAfterID = strings.TrimSpace(q.CursorAfterID)
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	if maxLimit > 0 && q.Limit > maxLimit {
		q.Limit = maxLimit
	}
	return q
}

type MemorySearchQuery struct {
	WorkspaceID common.WorkspaceID
	Query       string
	Tags        []string
	Types       []string
	Vector      []float64
	MinScore    float64
	Limit       int
}

func (q MemorySearchQuery) Clone() MemorySearchQuery {
	q.Tags = append([]string(nil), q.Tags...)
	q.Types = append([]string(nil), q.Types...)
	q.Vector = append([]float64(nil), q.Vector...)
	return q
}

func (q MemorySearchQuery) Normalized() MemorySearchQuery {
	q = q.Clone()
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.Query = strings.TrimSpace(q.Query)
	q.Tags = normalizeStrings(q.Tags)
	q.Types = normalizeStrings(q.Types)
	return q
}

type MemoryRecallTraceQuery struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Stage       string
	Source      string
	Status      string
	Limit       int
}

func (q MemoryRecallTraceQuery) Normalized() MemoryRecallTraceQuery {
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.RunID = common.RunID(strings.TrimSpace(string(q.RunID)))
	q.StepID = common.StepID(strings.TrimSpace(string(q.StepID)))
	q.Stage = strings.TrimSpace(q.Stage)
	q.Source = strings.TrimSpace(q.Source)
	q.Status = strings.TrimSpace(q.Status)
	return q
}

type AutoGovernanceTraceQuery struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	ItemType    string
	ItemID      string
	Status      string
	Limit       int
}

type EventRecordQuery struct {
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Topic          string
	ExcludeTopics  []string
	SubjectType    string
	SubjectID      string
	CursorAfterID  common.EventRecordID
	OccurredAfter  string
	OccurredBefore string
	Limit          int
	IncludePayload bool
}

func (q EventRecordQuery) Clone() EventRecordQuery {
	q.ExcludeTopics = append([]string(nil), q.ExcludeTopics...)
	return q
}

func (q EventRecordQuery) Normalized() EventRecordQuery {
	q = q.Clone()
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.RunID = common.RunID(strings.TrimSpace(string(q.RunID)))
	q.Topic = strings.TrimSpace(q.Topic)
	q.ExcludeTopics = normalizeStrings(q.ExcludeTopics)
	q.SubjectType = strings.TrimSpace(q.SubjectType)
	q.SubjectID = strings.TrimSpace(q.SubjectID)
	q.CursorAfterID = common.EventRecordID(strings.TrimSpace(string(q.CursorAfterID)))
	q.OccurredAfter = strings.TrimSpace(q.OccurredAfter)
	q.OccurredBefore = strings.TrimSpace(q.OccurredBefore)
	return q
}

func normalizeStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(input))
	ret := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}
