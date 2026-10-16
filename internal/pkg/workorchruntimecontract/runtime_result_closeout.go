package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeResultCloseout is the Orchestrator-authored structured terminal outcome.
// Result owner only persists it; it must not re-judge accepted/rejected authority.
type RuntimeResultCloseout struct {
	OriginalDiagnostic             *RuntimeOriginalDiagnostic `json:",omitempty"`
	SchemaVersion                  uint32
	CloseoutID                     string
	StartupInputHash               string
	DefinitionRevision             string
	RuntimeStateRevision           uint64
	TerminalOutcome                TerminalOutcome
	RequirementOutcomes            []RuntimeRequirementOutcome
	SourceObligations              *RuntimeSourceObligationInventory `json:",omitempty"`
	AcceptedEvidenceRefs           []string
	DeliverableRefs                []string
	OwnerRevisionSet               RuntimeOwnerRevisionSet
	TerminalSnapshotProofs         []RuntimeTerminalSnapshotProof `json:",omitempty"`
	KnownLimitations               []string
	CloseoutHash                   string
	SatisfiedCoverageWorkItemIDs   []common.CoverageWorkItemID
	UnsatisfiedCoverageWorkItemIDs []common.CoverageWorkItemID
	Gaps                           []RuntimeAssessmentGap
	MissingEvidenceKinds           []string
	TargetRefs                     []string
	EvidenceRefs                   []string
	ArtifactRefs                   []common.ArtifactID
	Disposition                    AssessmentDisposition
	FailureClass                   FailureClass
	FailureCode                    string
	FailureSignature               string
	PredecessorRunID               common.RunID
	PredecessorResultID            common.ResultID
	OperationOutcomes              []RuntimeOperationCloseout
	RecoveryPolicy                 RuntimeRecoveryPolicy
	RecoveryLedger                 RuntimeRecoveryLedger
}

// RuntimeOperationCloseout is the terminal projection of one sealed operation
// transaction. It is derived from the execution ledger and persisted owner
// receipt; Result owner persists it without re-judging Runtime authority.
type RuntimeOperationCloseout struct {
	DispatchID          string
	OperationID         string
	ExecutionKey        string
	Fence               uint64
	ExpectedEffectScope RuntimeEffectScope
	ObservedEffectScope RuntimeEffectScope
	Outcome             RuntimeOperationOutcome
	ExecutionState      RuntimeOperationExecutionState
	ReceiptRef          string
	ReceiptHash         string
}

const RuntimeResultCloseoutSchemaVersionCurrent uint32 = 1

// Result feedback is sealed by Orchestrator when an aggregate is initialized.
// Only the current contract may drive or render a Result. Older archives are
// rejected without mutation or fallback; same-version acknowledgement loss
// must reproduce the identical material from the persisted aggregate.
const RuntimeResultFeedbackVersionCurrent uint32 = 5

func ValidateRuntimeResultFeedbackVersion(version uint32) error {
	if version != RuntimeResultFeedbackVersionCurrent {
		return fmt.Errorf("unsupported runtime result feedback version %d; require current version %d", version, RuntimeResultFeedbackVersionCurrent)
	}
	return nil
}

func (c RuntimeResultCloseout) Normalized() RuntimeResultCloseout {
	c.OriginalDiagnostic = c.OriginalDiagnostic.CloneOriginal()
	c.CloseoutID = strings.TrimSpace(c.CloseoutID)
	c.StartupInputHash = strings.TrimSpace(c.StartupInputHash)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.TerminalOutcome = c.TerminalOutcome.Normalized()
	if c.SourceObligations != nil {
		inventory := c.SourceObligations.Clone()
		c.SourceObligations = &inventory
	}
	c.RequirementOutcomes = append([]RuntimeRequirementOutcome(nil), c.RequirementOutcomes...)
	for idx := range c.RequirementOutcomes {
		c.RequirementOutcomes[idx] = c.RequirementOutcomes[idx].Normalized()
	}
	sort.Slice(c.RequirementOutcomes, func(i, j int) bool {
		return c.RequirementOutcomes[i].CoverageWorkItemID < c.RequirementOutcomes[j].CoverageWorkItemID
	})
	c.AcceptedEvidenceRefs = normalizeStrings(c.AcceptedEvidenceRefs)
	c.DeliverableRefs = normalizeStrings(c.DeliverableRefs)
	c.OwnerRevisionSet = c.OwnerRevisionSet.Normalized()
	c.TerminalSnapshotProofs = cloneRuntimeStateRawSlice(c.TerminalSnapshotProofs, func(p RuntimeTerminalSnapshotProof) RuntimeTerminalSnapshotProof { return p.CloneRaw() })
	c.KnownLimitations = normalizeStrings(c.KnownLimitations)
	c.CloseoutHash = strings.TrimSpace(c.CloseoutHash)
	c.SatisfiedCoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.SatisfiedCoverageWorkItemIDs)
	c.UnsatisfiedCoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.UnsatisfiedCoverageWorkItemIDs)
	if len(c.Gaps) > 0 {
		gaps := make([]RuntimeAssessmentGap, 0, len(c.Gaps))
		for _, gap := range c.Gaps {
			gap = gap.Normalized()
			if len(gap.CoverageWorkItemIDs) == 0 && gap.Reason == "" {
				continue
			}
			gaps = append(gaps, gap)
		}
		if len(gaps) == 0 {
			c.Gaps = nil
		} else {
			c.Gaps = gaps
		}
	}
	c.MissingEvidenceKinds = normalizeStrings(c.MissingEvidenceKinds)
	c.TargetRefs = normalizeStrings(c.TargetRefs)
	c.EvidenceRefs = normalizeStrings(c.EvidenceRefs)
	if len(c.ArtifactRefs) > 0 {
		refs := make([]common.ArtifactID, 0, len(c.ArtifactRefs))
		for _, ref := range c.ArtifactRefs {
			ref = common.ArtifactID(strings.TrimSpace(string(ref)))
			if ref != "" {
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			c.ArtifactRefs = nil
		} else {
			c.ArtifactRefs = refs
		}
	}
	c.Disposition = c.Disposition.Normalized()
	c.FailureClass = FailureClass(strings.TrimSpace(string(c.FailureClass)))
	c.FailureCode = strings.TrimSpace(c.FailureCode)
	c.FailureSignature = strings.TrimSpace(c.FailureSignature)
	c.PredecessorRunID = common.RunID(strings.TrimSpace(string(c.PredecessorRunID)))
	c.PredecessorResultID = common.ResultID(strings.TrimSpace(string(c.PredecessorResultID)))
	if len(c.OperationOutcomes) > 0 {
		outcomes := make([]RuntimeOperationCloseout, 0, len(c.OperationOutcomes))
		for _, outcome := range c.OperationOutcomes {
			outcome = outcome.Normalized()
			if outcome.DispatchID == "" || outcome.OperationID == "" {
				continue
			}
			outcomes = append(outcomes, outcome)
		}
		sort.SliceStable(outcomes, func(i, j int) bool {
			if outcomes[i].DispatchID != outcomes[j].DispatchID {
				return outcomes[i].DispatchID < outcomes[j].DispatchID
			}
			return outcomes[i].OperationID < outcomes[j].OperationID
		})
		if len(outcomes) == 0 {
			c.OperationOutcomes = nil
		} else {
			c.OperationOutcomes = outcomes
		}
	}
	return c
}

func (c RuntimeResultCloseout) Validate() error {
	c = c.Normalized()
	if c.OriginalDiagnostic != nil {
		if err := c.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("runtime result closeout original diagnostic: %w", err)
		}
	}
	if c.SchemaVersion != RuntimeResultCloseoutSchemaVersionCurrent {
		return fmt.Errorf("unsupported runtime result closeout schema version %d", c.SchemaVersion)
	}
	if c.CloseoutID != "" {
		if err := c.validateFormalCloseout(); err != nil {
			return err
		}
	}
	if err := c.RecoveryPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime result closeout recovery policy: %w", err)
	}
	if err := c.RecoveryLedger.Validate(c.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime result closeout recovery ledger: %w", err)
	}
	seen := map[string]struct{}{}
	for _, outcome := range c.OperationOutcomes {
		if err := outcome.Validate(); err != nil {
			return err
		}
		key := outcome.DispatchID + "|" + outcome.OperationID
		if _, ok := seen[key]; ok {
			return fmt.Errorf("runtime operation closeout %s is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (c RuntimeResultCloseout) WithCloseoutHash() RuntimeResultCloseout {
	c = c.Normalized()
	c.CloseoutHash = ""
	c.CloseoutHash = stableFormalValueHash("runtime-result-closeout", c)
	return c
}

func (c RuntimeResultCloseout) validateFormalCloseout() error {
	if c.CloseoutID == "" || c.StartupInputHash == "" || c.DefinitionRevision == "" || c.RuntimeStateRevision == 0 {
		return fmt.Errorf("formal runtime closeout identity is incomplete")
	}
	if err := c.TerminalOutcome.Validate(); err != nil {
		return err
	}
	if err := c.OwnerRevisionSet.Validate(); err != nil {
		return err
	}
	if len(c.TerminalSnapshotProofs) > MaxRuntimeLineageAssertions {
		return fmt.Errorf("terminal snapshot proof inventory exceeds bound")
	}
	for _, proof := range c.TerminalSnapshotProofs {
		if err := proof.ValidateRaw(); err != nil {
			return err
		}
		if c.TerminalOutcome.Kind != TerminalOutcomeKindCompleted || proof.CurrentDependencies.DefinitionRevision != c.DefinitionRevision || proof.ObservedAtStateRevision+1 != c.RuntimeStateRevision {
			return fmt.Errorf("terminal snapshot proof differs from original material generation")
		}
	}
	if c.OwnerRevisionSet.DefinitionRevision != c.DefinitionRevision {
		return fmt.Errorf("formal runtime closeout owner revisions are stale")
	}
	seen := map[common.CoverageWorkItemID]struct{}{}
	accepted := make([]string, 0)
	for _, outcome := range c.RequirementOutcomes {
		if err := outcome.Validate(); err != nil {
			return err
		}
		if outcome.DefinitionRevision != c.DefinitionRevision || outcome.OwnerRevisionSetHash != c.OwnerRevisionSet.SetHash {
			return fmt.Errorf("runtime requirement outcome does not match closeout authority")
		}
		if _, duplicate := seen[outcome.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime requirement outcome %q is duplicated", outcome.CoverageWorkItemID)
		}
		seen[outcome.CoverageWorkItemID] = struct{}{}
		accepted = append(accepted, outcome.AcceptedEvidenceRefs...)
		if c.TerminalOutcome.Kind == TerminalOutcomeKindCompleted && (outcome.CoverageState != RuntimeRequirementCovered || outcome.AssessmentState != RuntimeRequirementSatisfied || len(outcome.AcceptedEvidenceRefs) == 0) {
			return fmt.Errorf("successful runtime closeout has incomplete requirement %q", outcome.CoverageWorkItemID)
		}
		for _, source := range outcome.SourceObligations {
			if c.TerminalOutcome.Kind == TerminalOutcomeKindCompleted && (source.CoverageState != RuntimeRequirementCovered || source.AssessmentState != RuntimeRequirementSatisfied || len(source.AcceptedEvidenceRefs) == 0) {
				return fmt.Errorf("successful runtime closeout has incomplete source obligation %q", source.Check.ObligationID)
			}
		}
	}
	if len(c.RequirementOutcomes) == 0 {
		return fmt.Errorf("formal runtime closeout requires requirement outcomes")
	}
	if err := validateRuntimeCloseoutSourceObligations(c); err != nil {
		return err
	}
	if len(accepted) > 0 && len(c.OwnerRevisionSet.Entries) == 0 {
		return fmt.Errorf("formal runtime closeout accepted evidence requires owner revision entries")
	}
	if strings.Join(normalizeStrings(accepted), "\x00") != strings.Join(c.AcceptedEvidenceRefs, "\x00") {
		return fmt.Errorf("formal runtime closeout accepted evidence union mismatch")
	}
	if c.CloseoutHash == "" || c.CloseoutHash != c.WithCloseoutHash().CloseoutHash {
		return fmt.Errorf("formal runtime closeout hash mismatch")
	}
	return nil
}

func (o RuntimeOperationCloseout) Normalized() RuntimeOperationCloseout {
	o.DispatchID = strings.TrimSpace(o.DispatchID)
	o.OperationID = strings.TrimSpace(o.OperationID)
	o.ExecutionKey = strings.TrimSpace(o.ExecutionKey)
	o.ExpectedEffectScope = NormalizeRuntimeEffectScope(o.ExpectedEffectScope)
	o.ObservedEffectScope = NormalizeRuntimeEffectScope(o.ObservedEffectScope)
	o.Outcome = o.Outcome.Normalized()
	o.ExecutionState = o.ExecutionState.Normalized()
	o.ReceiptRef = strings.TrimSpace(o.ReceiptRef)
	o.ReceiptHash = strings.TrimSpace(o.ReceiptHash)
	return o
}

func (o RuntimeOperationCloseout) Validate() error {
	o = o.Normalized()
	if o.DispatchID == "" || o.OperationID == "" || o.ExecutionKey == "" {
		return fmt.Errorf("runtime operation closeout requires dispatch, operation and execution key")
	}
	if (o.ReceiptRef != "" || o.ReceiptHash != "" || o.Outcome != "") && o.Fence == 0 {
		return fmt.Errorf("runtime operation closeout receipt/outcome requires fence")
	}
	if !ValidRuntimeEffectScope(o.ExpectedEffectScope) || !ValidRuntimeOperationExecutionState(o.ExecutionState) {
		return fmt.Errorf("runtime operation closeout has invalid expected effect or execution state")
	}
	if o.Outcome == "" {
		return nil
	}
	if !ValidRuntimeOperationOutcome(o.Outcome) || !ValidRuntimeEffectScope(o.ObservedEffectScope) {
		return fmt.Errorf("runtime operation closeout has invalid owner outcome")
	}
	if o.ReceiptRef == "" || o.ReceiptHash == "" {
		return fmt.Errorf("runtime operation closeout owner outcome requires receipt ref/hash")
	}
	return nil
}
