package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultMaxRepairAttemptsPerFocus        = 10
	DefaultMaxValidationOnlyRetriesPerFocus = 1
	MaxRepairAttemptsPerFocusLimit          = 100
	MaxValidationOnlyRetriesPerFocusLimit   = 10
)

// RuntimeConvergencePolicy is sealed at activation and used for repair/slice budgets.
// Candidate attempts remain fixed at 0/1 and are not part of this policy.
type RuntimeConvergencePolicy struct {
	MaxRepairAttemptsPerFocus        int
	MaxValidationOnlyRetriesPerFocus int
	MaxAcceptanceSliceWorkItems      int
	MaxFocuses                       int
	MaxCandidateContextItems         int
}

// RuntimeRepairLedger tracks same-focus repair progress independently from candidate attempts.
type RuntimeRepairLedger struct {
	FocusID               string
	SliceID               string
	ActiveGap             *RuntimeAssessmentGap
	RepairAttempts        int
	ValidationOnlyRetries int
	NoProgressCount       int
	SignatureCounts       []RuntimeFailureSignatureCount
	// NoProgressOperationShapes records accepted operation structures that did
	// not yet resolve the active repair gap. An SM-2 owner-fact reopen may carry
	// only these shapes into the successor semantic focus for the same CWI; all
	// attempts, gap text and directives remain focus-local. It is volatile SM-3
	// context; callers in Orchestrator decide when a receipt belongs here.
	NoProgressOperationShapes []RuntimeRejectedOperationShape
	LastCycleID               string
	LastProgressRevision      string
	LastProgressAt            string
	Exhausted                 bool
}

type RuntimeFailureSignatureCount struct {
	Signature string
	Count     int
}

func (p RuntimeConvergencePolicy) Normalized() RuntimeConvergencePolicy {
	return p
}

func (p RuntimeConvergencePolicy) Validate() error {
	p = p.Normalized()
	if p.MaxRepairAttemptsPerFocus < 1 || p.MaxRepairAttemptsPerFocus > MaxRepairAttemptsPerFocusLimit {
		return fmt.Errorf("MaxRepairAttemptsPerFocus must be in 1..%d", MaxRepairAttemptsPerFocusLimit)
	}
	if p.MaxValidationOnlyRetriesPerFocus < 0 || p.MaxValidationOnlyRetriesPerFocus > MaxValidationOnlyRetriesPerFocusLimit {
		return fmt.Errorf("MaxValidationOnlyRetriesPerFocus must be in 0..%d", MaxValidationOnlyRetriesPerFocusLimit)
	}
	if p.MaxAcceptanceSliceWorkItems < RuntimeAcceptanceSliceMaxWorkItemsMin || p.MaxAcceptanceSliceWorkItems > RuntimeAcceptanceSliceMaxWorkItemsMax {
		return fmt.Errorf("MaxAcceptanceSliceWorkItems must be in %d..%d", RuntimeAcceptanceSliceMaxWorkItemsMin, RuntimeAcceptanceSliceMaxWorkItemsMax)
	}
	if p.MaxFocuses < 1 || p.MaxFocuses > RuntimeAcceptanceFocusMaxCount {
		return fmt.Errorf("MaxFocuses must be in 1..%d", RuntimeAcceptanceFocusMaxCount)
	}
	if p.MaxCandidateContextItems < 1 || p.MaxCandidateContextItems > RuntimeCandidateContextItemsMaxCount {
		return fmt.Errorf("MaxCandidateContextItems must be in 1..%d", RuntimeCandidateContextItemsMaxCount)
	}
	return nil
}

func (l RuntimeRepairLedger) Normalized() RuntimeRepairLedger {
	l.FocusID = strings.TrimSpace(l.FocusID)
	l.SliceID = strings.TrimSpace(l.SliceID)
	l.LastCycleID = strings.TrimSpace(l.LastCycleID)
	l.LastProgressRevision = strings.TrimSpace(l.LastProgressRevision)
	l.LastProgressAt = strings.TrimSpace(l.LastProgressAt)
	if l.ActiveGap != nil {
		gap := l.ActiveGap.Normalized()
		l.ActiveGap = &gap
	}
	if len(l.SignatureCounts) > 0 {
		counts := make([]RuntimeFailureSignatureCount, 0, len(l.SignatureCounts))
		for _, item := range l.SignatureCounts {
			item.Signature = strings.TrimSpace(item.Signature)
			if item.Signature == "" || item.Count <= 0 {
				continue
			}
			counts = append(counts, item)
		}
		sort.SliceStable(counts, func(i, j int) bool { return counts[i].Signature < counts[j].Signature })
		l.SignatureCounts = counts
	}
	l.NoProgressOperationShapes = NormalizeRuntimeRejectedOperationShapes(l.NoProgressOperationShapes)
	return l
}

// StableAssessmentFailureSignature hashes the owner-observable assessment
// identity without timestamps or evidence IDs. LLM claimed missing kinds,
// claimed targets and raw failure wording are diagnostic lineage and must not
// alter repair budgets or next-action routing.
func StableAssessmentFailureSignature(focusID string, disposition AssessmentDisposition, gap *RuntimeAssessmentGap) string {
	focusID = strings.TrimSpace(focusID)
	disposition = disposition.Normalized()
	parts := []string{focusID, string(disposition)}
	if gap != nil {
		g := gap.Normalized()
		coverage := make([]string, 0, len(g.CoverageWorkItemIDs))
		for _, id := range g.CoverageWorkItemIDs {
			coverage = append(coverage, string(id))
		}
		parts = append(parts, strings.Join(coverage, ","))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// StableRepairCycleID is deterministic for the same run/focus/signature/source fact.
func StableRepairCycleID(runID, focusID, failureSignature, sourceFactID string) string {
	payload := strings.Join([]string{
		strings.TrimSpace(runID),
		strings.TrimSpace(focusID),
		strings.TrimSpace(failureSignature),
		strings.TrimSpace(sourceFactID),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func DefaultRuntimeConvergencePolicy() RuntimeConvergencePolicy {
	return RuntimeConvergencePolicy{
		MaxRepairAttemptsPerFocus:        DefaultMaxRepairAttemptsPerFocus,
		MaxValidationOnlyRetriesPerFocus: DefaultMaxValidationOnlyRetriesPerFocus,
		MaxAcceptanceSliceWorkItems:      RuntimeAcceptanceSliceDefaultMaxItems,
		MaxFocuses:                       RuntimeAcceptanceFocusMaxCount,
		MaxCandidateContextItems:         RuntimeCandidateContextItemsMaxCount,
	}.Normalized()
}
