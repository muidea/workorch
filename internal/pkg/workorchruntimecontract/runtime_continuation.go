package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeContinuationContext carries bounded governance history for the
// current activation. It is diagnostic input only and cannot select coverage,
// actions, operations, repair or successor lineage.
type RuntimeContinuationContext struct {
	DefinitionChangeSupersedesPrior bool
	GovernanceHistory               []RuntimeGovernanceHistoryEntry
}

type RuntimeGovernanceHistoryEntry struct {
	TicketID          common.TicketID
	RunID             common.RunID
	StepID            common.StepID
	DecisionKind      string
	Reason            string
	Question          string
	ResolutionID      common.ResolutionID
	ResolutionType    string
	RecoveryAction    string
	ResolutionSummary string
	Failure           *RuntimeContinuationFailure
}

type RuntimeContinuationFailure struct {
	Class          string
	Signature      string
	TargetRefs     []string
	FailedStepID   common.StepID
	FailedStepKind string
	Summary        string
	RecoveryPolicy string
}

func (c RuntimeContinuationContext) Normalized() RuntimeContinuationContext {
	ret := make([]RuntimeGovernanceHistoryEntry, 0, len(c.GovernanceHistory))
	for _, item := range c.GovernanceHistory {
		item = item.Normalized()
		if item.TicketID != "" && item.RunID != "" && item.ResolutionID != "" {
			ret = append(ret, item)
		}
	}
	c.GovernanceHistory = nilIfEmpty(ret)
	return c
}

func (c RuntimeContinuationContext) Validate() error {
	c = c.Normalized()
	for _, item := range c.GovernanceHistory {
		if item.TicketID == "" || item.RunID == "" || item.ResolutionID == "" {
			return fmt.Errorf("runtime governance continuation requires ticket, run and resolution identity")
		}
	}
	return nil
}

func (c RuntimeContinuationContext) Clone() RuntimeContinuationContext {
	c = c.Normalized()
	ret := c
	ret.GovernanceHistory = make([]RuntimeGovernanceHistoryEntry, len(c.GovernanceHistory))
	for idx := range c.GovernanceHistory {
		ret.GovernanceHistory[idx] = c.GovernanceHistory[idx].Clone()
	}
	return ret
}

func SameRuntimeContinuationContext(left RuntimeContinuationContext, right RuntimeContinuationContext) bool {
	left, right = left.Normalized(), right.Normalized()
	if left.DefinitionChangeSupersedesPrior != right.DefinitionChangeSupersedesPrior || len(left.GovernanceHistory) != len(right.GovernanceHistory) {
		return false
	}
	for idx := range left.GovernanceHistory {
		if !sameRuntimeGovernanceHistoryEntry(left.GovernanceHistory[idx], right.GovernanceHistory[idx]) {
			return false
		}
	}
	return true
}

func (e RuntimeGovernanceHistoryEntry) Normalized() RuntimeGovernanceHistoryEntry {
	e.DecisionKind = strings.TrimSpace(e.DecisionKind)
	e.Reason = strings.TrimSpace(e.Reason)
	e.Question = strings.TrimSpace(e.Question)
	e.ResolutionType = strings.TrimSpace(e.ResolutionType)
	e.RecoveryAction = strings.TrimSpace(e.RecoveryAction)
	e.ResolutionSummary = strings.TrimSpace(e.ResolutionSummary)
	if e.Failure != nil {
		value := e.Failure.Normalized()
		if value.empty() {
			e.Failure = nil
		} else {
			e.Failure = &value
		}
	}
	return e
}

func (e RuntimeGovernanceHistoryEntry) Clone() RuntimeGovernanceHistoryEntry {
	e = e.Normalized()
	if e.Failure != nil {
		value := e.Failure.Clone()
		e.Failure = &value
	}
	return e
}

func (f RuntimeContinuationFailure) Normalized() RuntimeContinuationFailure {
	f.Class = strings.TrimSpace(f.Class)
	f.Signature = strings.TrimSpace(f.Signature)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.FailedStepKind = strings.TrimSpace(f.FailedStepKind)
	f.Summary = strings.TrimSpace(f.Summary)
	f.RecoveryPolicy = strings.TrimSpace(f.RecoveryPolicy)
	return f
}

func (f RuntimeContinuationFailure) Clone() RuntimeContinuationFailure {
	f = f.Normalized()
	f.TargetRefs = append([]string(nil), f.TargetRefs...)
	return f
}

func (f RuntimeContinuationFailure) empty() bool {
	return f.Class == "" && f.Signature == "" && len(f.TargetRefs) == 0 && f.FailedStepID == "" && f.FailedStepKind == "" && f.Summary == "" && f.RecoveryPolicy == ""
}

func sameRuntimeGovernanceHistoryEntry(left RuntimeGovernanceHistoryEntry, right RuntimeGovernanceHistoryEntry) bool {
	left, right = left.Normalized(), right.Normalized()
	return left.TicketID == right.TicketID && left.RunID == right.RunID && left.StepID == right.StepID && left.DecisionKind == right.DecisionKind &&
		left.Reason == right.Reason && left.Question == right.Question && left.ResolutionID == right.ResolutionID && left.ResolutionType == right.ResolutionType &&
		left.RecoveryAction == right.RecoveryAction && left.ResolutionSummary == right.ResolutionSummary && sameRuntimeContinuationFailure(left.Failure, right.Failure)
}

func sameRuntimeContinuationFailure(left *RuntimeContinuationFailure, right *RuntimeContinuationFailure) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.Class == r.Class && l.Signature == r.Signature && l.FailedStepID == r.FailedStepID && l.FailedStepKind == r.FailedStepKind &&
		l.Summary == r.Summary && l.RecoveryPolicy == r.RecoveryPolicy && sameStrings(l.TargetRefs, r.TargetRefs)
}
