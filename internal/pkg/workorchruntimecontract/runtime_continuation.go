package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeContinuationContext 是 successor/resume Run 可继承的最小 typed 历史事实。
// 它只保留 owner identity、恢复动作、失败摘要和 evidence/artifact refs，禁止承载
// execution graph、provider payload、patch/content 正文或任意自由结构 map。
type RuntimeContinuationContext struct {
	DefinitionChangeSupersedesPrior bool
	GovernanceHistory               []RuntimeGovernanceHistoryEntry
	Replan                          *RuntimeReplanContinuation
	RejectedResult                  *RuntimeRejectedResultContinuation
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

type RuntimeReplanContinuation struct {
	PredecessorRunID  common.RunID
	TicketID          common.TicketID
	TicketStepID      common.StepID
	ResolutionID      common.ResolutionID
	ResolutionType    string
	RecoveryAction    string
	Reason            string
	ResolutionSummary string
	Failure           *RuntimeContinuationFailure
}

type RuntimeRejectedResultContinuation struct {
	PredecessorRunID    common.RunID
	ResultID            common.ResultID
	ResultState         common.ResultState
	SourceType          string
	SourceRef           string
	Summary             string
	GoalOutcome         string
	DefinitionRevision  string
	Disposition         AssessmentDisposition
	Failure             *RuntimeContinuationFailure
	SatisfiedDoDItems   []RuntimeContinuationItem
	UnsatisfiedDoDItems []RuntimeContinuationItem
	CoverageGaps        []RuntimeContinuationCoverageGap
	RequiredEvidence    []string
	EvidenceRefs        []string
	ArtifactRefs        []common.ArtifactID
	GateFailureReason   string
	RejectionReason     string
	SourceSummary       string
}

type RuntimeContinuationItem struct {
	CoverageWorkItemID common.CoverageWorkItemID
	Item               string
	State              string
	Reason             string
	EvidenceRefs       []string
}

type RuntimeContinuationCoverageGap struct {
	Scope                string
	ID                   string
	CoverageWorkItemIDs  []common.CoverageWorkItemID
	MissingEvidenceKinds []string
	TargetRefs           []string
	FailureClass         FailureClass
	FailureCode          string
	FailureSignature     string
	Deliverable          string
	Name                 string
	Path                 string
	Goal                 string
	GoalItem             string
	Item                 string
	Requirement          string
	State                string
	EvidenceRefs         []string
	ArtifactRefs         []common.ArtifactID
}

func (c RuntimeContinuationContext) Normalized() RuntimeContinuationContext {
	if len(c.GovernanceHistory) > 0 {
		ret := make([]RuntimeGovernanceHistoryEntry, 0, len(c.GovernanceHistory))
		for _, item := range c.GovernanceHistory {
			item = item.Normalized()
			if item.TicketID != "" && item.RunID != "" && item.ResolutionID != "" {
				ret = append(ret, item)
			}
		}
		c.GovernanceHistory = nilIfEmpty(ret)
	}
	if c.Replan != nil {
		value := c.Replan.Normalized()
		if c.DefinitionChangeSupersedesPrior || value.PredecessorRunID == "" {
			c.Replan = nil
		} else {
			c.Replan = &value
		}
	}
	if c.RejectedResult != nil {
		value := c.RejectedResult.Normalized()
		if c.DefinitionChangeSupersedesPrior || value.PredecessorRunID == "" || value.ResultID == "" {
			c.RejectedResult = nil
		} else {
			c.RejectedResult = &value
		}
	}
	return c
}

func (c RuntimeContinuationContext) Validate() error {
	c = c.Normalized()
	for _, item := range c.GovernanceHistory {
		if item.TicketID == "" || item.RunID == "" || item.ResolutionID == "" {
			return fmt.Errorf("runtime governance continuation requires ticket, run and resolution identity")
		}
	}
	if c.Replan != nil && (c.Replan.PredecessorRunID == "" || c.Replan.TicketID == "" || c.Replan.ResolutionID == "") {
		return fmt.Errorf("runtime replan continuation requires predecessor, ticket and resolution identity")
	}
	if c.RejectedResult != nil {
		if c.RejectedResult.PredecessorRunID == "" || c.RejectedResult.ResultID == "" || c.RejectedResult.ResultState != common.ResultStateRejected {
			return fmt.Errorf("runtime rejected-result continuation requires rejected predecessor result identity")
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
	if c.Replan != nil {
		value := c.Replan.Clone()
		ret.Replan = &value
	}
	if c.RejectedResult != nil {
		value := c.RejectedResult.Clone()
		ret.RejectedResult = &value
	}
	return ret
}

func SameRuntimeContinuationContext(left RuntimeContinuationContext, right RuntimeContinuationContext) bool {
	left, right = left.Normalized(), right.Normalized()
	if left.DefinitionChangeSupersedesPrior != right.DefinitionChangeSupersedesPrior || len(left.GovernanceHistory) != len(right.GovernanceHistory) ||
		!sameRuntimeReplanContinuation(left.Replan, right.Replan) || !sameRuntimeRejectedResultContinuation(left.RejectedResult, right.RejectedResult) {
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

func (r RuntimeReplanContinuation) Normalized() RuntimeReplanContinuation {
	r.ResolutionType = strings.TrimSpace(r.ResolutionType)
	r.RecoveryAction = strings.TrimSpace(r.RecoveryAction)
	r.Reason = strings.TrimSpace(r.Reason)
	r.ResolutionSummary = strings.TrimSpace(r.ResolutionSummary)
	if r.Failure != nil {
		value := r.Failure.Normalized()
		if value.empty() {
			r.Failure = nil
		} else {
			r.Failure = &value
		}
	}
	return r
}

func (r RuntimeReplanContinuation) Clone() RuntimeReplanContinuation {
	r = r.Normalized()
	if r.Failure != nil {
		value := r.Failure.Clone()
		r.Failure = &value
	}
	return r
}

func (r RuntimeRejectedResultContinuation) Normalized() RuntimeRejectedResultContinuation {
	r.SourceType = strings.TrimSpace(r.SourceType)
	r.SourceRef = strings.TrimSpace(r.SourceRef)
	r.Summary = strings.TrimSpace(r.Summary)
	r.GoalOutcome = strings.TrimSpace(r.GoalOutcome)
	r.DefinitionRevision = strings.TrimSpace(r.DefinitionRevision)
	r.Disposition = r.Disposition.Normalized()
	if r.Failure != nil {
		failure := r.Failure.Normalized()
		if failure.empty() {
			r.Failure = nil
		} else {
			r.Failure = &failure
		}
	}
	r.SatisfiedDoDItems = normalizeRuntimeContinuationItems(r.SatisfiedDoDItems)
	r.UnsatisfiedDoDItems = normalizeRuntimeContinuationItems(r.UnsatisfiedDoDItems)
	r.CoverageGaps = normalizeRuntimeContinuationCoverageGaps(r.CoverageGaps)
	r.RequiredEvidence = normalizeStrings(r.RequiredEvidence)
	r.EvidenceRefs = normalizeStrings(r.EvidenceRefs)
	r.ArtifactRefs = normalizeRuntimeArtifactIDs(r.ArtifactRefs)
	r.GateFailureReason = strings.TrimSpace(r.GateFailureReason)
	r.RejectionReason = strings.TrimSpace(r.RejectionReason)
	r.SourceSummary = strings.TrimSpace(r.SourceSummary)
	return r
}

func (r RuntimeRejectedResultContinuation) Clone() RuntimeRejectedResultContinuation {
	r = r.Normalized()
	r.SatisfiedDoDItems = cloneRuntimeContinuationItems(r.SatisfiedDoDItems)
	r.UnsatisfiedDoDItems = cloneRuntimeContinuationItems(r.UnsatisfiedDoDItems)
	r.CoverageGaps = cloneRuntimeContinuationCoverageGaps(r.CoverageGaps)
	r.RequiredEvidence = append([]string(nil), r.RequiredEvidence...)
	r.EvidenceRefs = append([]string(nil), r.EvidenceRefs...)
	r.ArtifactRefs = append([]common.ArtifactID(nil), r.ArtifactRefs...)
	return r
}

func normalizeRuntimeContinuationItems(values []RuntimeContinuationItem) []RuntimeContinuationItem {
	ret := make([]RuntimeContinuationItem, 0, len(values))
	for _, value := range values {
		value.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(value.CoverageWorkItemID)))
		value.Item = strings.TrimSpace(value.Item)
		value.State = strings.TrimSpace(value.State)
		value.Reason = strings.TrimSpace(value.Reason)
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		if value.Item != "" || value.CoverageWorkItemID != "" {
			ret = append(ret, value)
		}
	}
	return nilIfEmpty(ret)
}

func cloneRuntimeContinuationItems(values []RuntimeContinuationItem) []RuntimeContinuationItem {
	ret := append([]RuntimeContinuationItem(nil), values...)
	for idx := range ret {
		ret[idx].EvidenceRefs = append([]string(nil), values[idx].EvidenceRefs...)
	}
	return ret
}

func normalizeRuntimeContinuationCoverageGaps(values []RuntimeContinuationCoverageGap) []RuntimeContinuationCoverageGap {
	ret := make([]RuntimeContinuationCoverageGap, 0, len(values))
	for _, value := range values {
		value.Scope = strings.TrimSpace(value.Scope)
		value.ID = strings.TrimSpace(value.ID)
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
		value.MissingEvidenceKinds = normalizeStrings(value.MissingEvidenceKinds)
		value.TargetRefs = normalizeStrings(value.TargetRefs)
		value.FailureClass = FailureClass(strings.TrimSpace(string(value.FailureClass)))
		value.FailureCode = strings.TrimSpace(value.FailureCode)
		value.FailureSignature = strings.TrimSpace(value.FailureSignature)
		value.Deliverable = strings.TrimSpace(value.Deliverable)
		value.Name = strings.TrimSpace(value.Name)
		value.Path = strings.TrimSpace(value.Path)
		value.Goal = strings.TrimSpace(value.Goal)
		value.GoalItem = strings.TrimSpace(value.GoalItem)
		value.Item = strings.TrimSpace(value.Item)
		value.Requirement = strings.TrimSpace(value.Requirement)
		value.State = strings.TrimSpace(value.State)
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.ArtifactRefs = normalizeRuntimeArtifactIDs(value.ArtifactRefs)
		if value.Scope != "" && (value.ID != "" || len(value.CoverageWorkItemIDs) > 0 || value.Deliverable != "" || value.Name != "" || value.Path != "" || value.Goal != "" || value.GoalItem != "" || value.Item != "" || value.Requirement != "") {
			ret = append(ret, value)
		}
	}
	return nilIfEmpty(ret)
}

func cloneRuntimeContinuationCoverageGaps(values []RuntimeContinuationCoverageGap) []RuntimeContinuationCoverageGap {
	ret := append([]RuntimeContinuationCoverageGap(nil), values...)
	for idx := range ret {
		ret[idx].EvidenceRefs = append([]string(nil), values[idx].EvidenceRefs...)
		ret[idx].ArtifactRefs = append([]common.ArtifactID(nil), values[idx].ArtifactRefs...)
	}
	return ret
}

func normalizeRuntimeArtifactIDs(values []common.ArtifactID) []common.ArtifactID {
	refs := make([]string, 0, len(values))
	for _, value := range values {
		refs = append(refs, string(value))
	}
	refs = normalizeStrings(refs)
	ret := make([]common.ArtifactID, 0, len(refs))
	for _, value := range refs {
		ret = append(ret, common.ArtifactID(value))
	}
	return nilIfEmpty(ret)
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

func sameRuntimeReplanContinuation(left *RuntimeReplanContinuation, right *RuntimeReplanContinuation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.PredecessorRunID == r.PredecessorRunID && l.TicketID == r.TicketID && l.TicketStepID == r.TicketStepID && l.ResolutionID == r.ResolutionID &&
		l.ResolutionType == r.ResolutionType && l.RecoveryAction == r.RecoveryAction && l.Reason == r.Reason && l.ResolutionSummary == r.ResolutionSummary &&
		sameRuntimeContinuationFailure(l.Failure, r.Failure)
}

func sameRuntimeRejectedResultContinuation(left *RuntimeRejectedResultContinuation, right *RuntimeRejectedResultContinuation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	if l.PredecessorRunID != r.PredecessorRunID || l.ResultID != r.ResultID || l.ResultState != r.ResultState || l.SourceType != r.SourceType || l.SourceRef != r.SourceRef ||
		l.Summary != r.Summary || l.GoalOutcome != r.GoalOutcome || l.GateFailureReason != r.GateFailureReason || l.RejectionReason != r.RejectionReason || l.SourceSummary != r.SourceSummary ||
		len(l.SatisfiedDoDItems) != len(r.SatisfiedDoDItems) || len(l.UnsatisfiedDoDItems) != len(r.UnsatisfiedDoDItems) || len(l.CoverageGaps) != len(r.CoverageGaps) ||
		!sameStrings(l.RequiredEvidence, r.RequiredEvidence) || !sameStrings(l.EvidenceRefs, r.EvidenceRefs) || !sameRuntimeArtifactIDs(l.ArtifactRefs, r.ArtifactRefs) {
		return false
	}
	for idx := range l.SatisfiedDoDItems {
		if !sameRuntimeContinuationItem(l.SatisfiedDoDItems[idx], r.SatisfiedDoDItems[idx]) {
			return false
		}
	}
	for idx := range l.UnsatisfiedDoDItems {
		if !sameRuntimeContinuationItem(l.UnsatisfiedDoDItems[idx], r.UnsatisfiedDoDItems[idx]) {
			return false
		}
	}
	for idx := range l.CoverageGaps {
		if !sameRuntimeContinuationCoverageGap(l.CoverageGaps[idx], r.CoverageGaps[idx]) {
			return false
		}
	}
	return true
}

func sameRuntimeContinuationItem(left RuntimeContinuationItem, right RuntimeContinuationItem) bool {
	return left.Item == right.Item && left.State == right.State && left.Reason == right.Reason && sameStrings(left.EvidenceRefs, right.EvidenceRefs)
}

func sameRuntimeContinuationCoverageGap(left RuntimeContinuationCoverageGap, right RuntimeContinuationCoverageGap) bool {
	return left.Scope == right.Scope && left.ID == right.ID && left.Deliverable == right.Deliverable && left.Name == right.Name && left.Path == right.Path &&
		left.Goal == right.Goal && left.GoalItem == right.GoalItem && left.Item == right.Item && left.Requirement == right.Requirement && left.State == right.State &&
		sameStrings(left.EvidenceRefs, right.EvidenceRefs) && sameRuntimeArtifactIDs(left.ArtifactRefs, right.ArtifactRefs)
}

func sameRuntimeArtifactIDs(left []common.ArtifactID, right []common.ArtifactID) bool {
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
