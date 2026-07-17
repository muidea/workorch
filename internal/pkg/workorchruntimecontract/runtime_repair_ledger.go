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
	LastCycleID           string
	LastProgressRevision  string
	LastProgressAt        string
	Exhausted             bool
}

type RuntimeFailureSignatureCount struct {
	Signature string
	Count     int
}

func (p RuntimeConvergencePolicy) Normalized() RuntimeConvergencePolicy {
	if p.MaxRepairAttemptsPerFocus <= 0 {
		p.MaxRepairAttemptsPerFocus = DefaultMaxRepairAttemptsPerFocus
	}
	if p.MaxValidationOnlyRetriesPerFocus < 0 {
		p.MaxValidationOnlyRetriesPerFocus = DefaultMaxValidationOnlyRetriesPerFocus
	}
	if p.MaxAcceptanceSliceWorkItems <= 0 {
		p.MaxAcceptanceSliceWorkItems = RuntimeAcceptanceSliceDefaultMaxItems
	}
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
	return l
}

// StableAssessmentFailureSignature hashes disposition/gap identity without timestamps or evidence IDs.
func StableAssessmentFailureSignature(focusID string, disposition AssessmentDisposition, gap *RuntimeAssessmentGap) string {
	focusID = strings.TrimSpace(focusID)
	disposition = disposition.Normalized()
	parts := []string{focusID, string(disposition)}
	if gap != nil {
		g := gap.Normalized()
		parts = append(parts, string(g.FailureClass), g.FailureCode)
		parts = append(parts, strings.Join(append([]string(nil), g.MissingEvidenceKinds...), ","))
		parts = append(parts, strings.Join(append([]string(nil), g.TargetRefs...), ","))
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

// BeginRepairCycle records a new same-focus repair cycle when the cycle ID is new.
// Returns the updated ledger and whether the cycle was newly counted.
func (l RuntimeRepairLedger) BeginRepairCycle(focusID, sliceID, cycleID, signature string, policy RuntimeConvergencePolicy) (RuntimeRepairLedger, bool, error) {
	l = l.Normalized()
	policy = policy.Normalized()
	focusID = strings.TrimSpace(focusID)
	sliceID = strings.TrimSpace(sliceID)
	cycleID = strings.TrimSpace(cycleID)
	signature = strings.TrimSpace(signature)
	if focusID == "" || cycleID == "" || signature == "" {
		return l, false, fmt.Errorf("repair cycle requires focus, cycle id and signature")
	}
	// Focus change resets per-focus counters.
	if l.FocusID != "" && l.FocusID != focusID {
		l = RuntimeRepairLedger{FocusID: focusID, SliceID: sliceID}
	} else {
		l.FocusID = focusID
		if sliceID != "" {
			l.SliceID = sliceID
		}
	}
	if l.LastCycleID == cycleID {
		return l, false, nil
	}
	if l.RepairAttempts >= policy.MaxRepairAttemptsPerFocus {
		l.Exhausted = true
		return l, false, nil
	}
	l.LastCycleID = cycleID
	l.RepairAttempts++
	l.SignatureCounts = incrementSignatureCount(l.SignatureCounts, signature)
	return l, true, nil
}

func (l RuntimeRepairLedger) MarkValidationOnlyRetry(policy RuntimeConvergencePolicy) RuntimeRepairLedger {
	l = l.Normalized()
	policy = policy.Normalized()
	l.ValidationOnlyRetries++
	if l.ValidationOnlyRetries > policy.MaxValidationOnlyRetriesPerFocus {
		// caller decides terminal/governance; ledger only tracks the count.
	}
	return l
}

func (l RuntimeRepairLedger) NoteProgress(revision string) RuntimeRepairLedger {
	l = l.Normalized()
	l.NoProgressCount = 0
	if strings.TrimSpace(revision) != "" {
		l.LastProgressRevision = strings.TrimSpace(revision)
	}
	return l
}

func (l RuntimeRepairLedger) NoteNoProgress() RuntimeRepairLedger {
	l = l.Normalized()
	l.NoProgressCount++
	return l
}

func incrementSignatureCount(values []RuntimeFailureSignatureCount, signature string) []RuntimeFailureSignatureCount {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return values
	}
	for idx := range values {
		if values[idx].Signature == signature {
			values[idx].Count++
			return values
		}
	}
	return append(append([]RuntimeFailureSignatureCount(nil), values...), RuntimeFailureSignatureCount{Signature: signature, Count: 1})
}

func DefaultRuntimeConvergencePolicy() RuntimeConvergencePolicy {
	return RuntimeConvergencePolicy{
		MaxRepairAttemptsPerFocus:        DefaultMaxRepairAttemptsPerFocus,
		MaxValidationOnlyRetriesPerFocus: DefaultMaxValidationOnlyRetriesPerFocus,
		MaxAcceptanceSliceWorkItems:      RuntimeAcceptanceSliceDefaultMaxItems,
	}.Normalized()
}
