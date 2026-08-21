package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeRecoverableWaitStatus string

const (
	RuntimeRecoverableWaitWaiting            RuntimeRecoverableWaitStatus = "waiting"
	RuntimeRecoverableWaitProbeClaimed       RuntimeRecoverableWaitStatus = "probe_claimed"
	RuntimeRecoverableWaitConditionSatisfied RuntimeRecoverableWaitStatus = "condition_satisfied"
	RuntimeRecoverableWaitExpired            RuntimeRecoverableWaitStatus = "expired"
	RuntimeRecoverableWaitExhausted          RuntimeRecoverableWaitStatus = "exhausted"
	RuntimeRecoverableWaitRetired            RuntimeRecoverableWaitStatus = "retired"
)

type RuntimeRecoverableWaitProbeKind string

const (
	RuntimeRecoverableWaitProbeProviderRequestSession   RuntimeRecoverableWaitProbeKind = "provider_request_session"
	RuntimeRecoverableWaitProbeProviderCeiling          RuntimeRecoverableWaitProbeKind = "provider_ceiling"
	RuntimeRecoverableWaitProbeOperationFeasibility     RuntimeRecoverableWaitProbeKind = "operation_feasibility"
	RuntimeRecoverableWaitProbeSemanticProofFeasibility RuntimeRecoverableWaitProbeKind = "semantic_proof_feasibility"
	RuntimeRecoverableWaitProbeOperationAdmissibility   RuntimeRecoverableWaitProbeKind = "operation_admissibility"
	RuntimeRecoverableWaitProbeCWIReconciliation        RuntimeRecoverableWaitProbeKind = "cwi_reconciliation"
	RuntimeRecoverableWaitProbeCapabilityAcquisition    RuntimeRecoverableWaitProbeKind = "capability_acquisition"
)

type RuntimeRecoverableWaitReason string

const (
	RuntimeRecoverableWaitReasonProviderCeilingUnavailable RuntimeRecoverableWaitReason = "provider_ceiling_temporarily_unavailable"
)

type RuntimeRecoverableWaitRecord struct {
	WaitID                 string
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	DefinitionRevision     string
	AuthorityTransactionID string
	StateRevisionOpened    uint64
	ReasonCode             RuntimeRecoverableWaitReason
	ProbeKind              RuntimeRecoverableWaitProbeKind
	ProbeOwner             string
	ProbeRequestHash       string
	RecoveryConditionHash  string
	OpenedAt               string
	NextEligibleAt         string
	Deadline               string
	Attempt                int
	MaxAttempts            int
	BudgetDimension        RuntimeBudgetDimension
	BudgetPolicyRevision   string
	BudgetAllocationID     string
	Status                 RuntimeRecoverableWaitStatus
	LastProbeResultHash    string
}

func (r RuntimeRecoverableWaitRecord) Normalized() RuntimeRecoverableWaitRecord {
	r.WaitID = strings.TrimSpace(r.WaitID)
	r.DefinitionRevision = strings.TrimSpace(r.DefinitionRevision)
	r.AuthorityTransactionID = strings.TrimSpace(r.AuthorityTransactionID)
	r.ReasonCode = RuntimeRecoverableWaitReason(strings.TrimSpace(string(r.ReasonCode)))
	r.ProbeKind = RuntimeRecoverableWaitProbeKind(strings.TrimSpace(string(r.ProbeKind)))
	r.ProbeOwner = strings.TrimSpace(r.ProbeOwner)
	r.ProbeRequestHash = strings.TrimSpace(r.ProbeRequestHash)
	r.RecoveryConditionHash = strings.TrimSpace(r.RecoveryConditionHash)
	r.OpenedAt = strings.TrimSpace(r.OpenedAt)
	r.NextEligibleAt = strings.TrimSpace(r.NextEligibleAt)
	r.Deadline = strings.TrimSpace(r.Deadline)
	r.BudgetDimension = RuntimeBudgetDimension(strings.TrimSpace(string(r.BudgetDimension)))
	r.BudgetPolicyRevision = strings.TrimSpace(r.BudgetPolicyRevision)
	r.BudgetAllocationID = strings.TrimSpace(r.BudgetAllocationID)
	r.Status = RuntimeRecoverableWaitStatus(strings.TrimSpace(string(r.Status)))
	r.LastProbeResultHash = strings.TrimSpace(r.LastProbeResultHash)
	return r
}

func (r RuntimeRecoverableWaitRecord) Clone() RuntimeRecoverableWaitRecord { return r.Normalized() }

func (r RuntimeRecoverableWaitRecord) Validate(ledger RuntimeExecutionBudgetLedger) error {
	r = r.Normalized()
	if r.WaitID == "" || r.WorkspaceID == "" || r.RunID == "" || r.DefinitionRevision == "" || r.AuthorityTransactionID == "" ||
		r.StateRevisionOpened == 0 || r.ProbeRequestHash == "" || r.RecoveryConditionHash == "" || r.Attempt < 0 || r.MaxAttempts <= 0 || r.Attempt > r.MaxAttempts ||
		r.BudgetPolicyRevision == "" || r.BudgetAllocationID == "" {
		return fmt.Errorf("runtime recoverable wait is incomplete")
	}
	if r.WaitID != StableRuntimeRecoverableWaitID(r) {
		return fmt.Errorf("runtime recoverable wait identity mismatch")
	}
	if r.ReasonCode != RuntimeRecoverableWaitReasonProviderCeilingUnavailable || r.ProbeKind != RuntimeRecoverableWaitProbeProviderCeiling || r.ProbeOwner != RuntimeModuleLLM {
		return fmt.Errorf("runtime recoverable wait reason/probe/owner combination is unsupported")
	}
	opened, openedErr := time.Parse(time.RFC3339Nano, r.OpenedAt)
	next, nextErr := time.Parse(time.RFC3339Nano, r.NextEligibleAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, r.Deadline)
	if openedErr != nil || nextErr != nil || deadlineErr != nil || next.Before(opened) || deadline.Before(next) {
		return fmt.Errorf("runtime recoverable wait time bounds are invalid")
	}
	if r.BudgetDimension != RuntimeBudgetDimensionRecoveryProbe || r.BudgetPolicyRevision != ledger.Policy.PolicyRevision {
		return fmt.Errorf("runtime recoverable wait budget policy binding is invalid")
	}
	allocation, found := RuntimeBudgetAllocationForID(ledger.Allocations, r.BudgetAllocationID)
	if !found || allocation.Dimension != r.BudgetDimension || allocation.LogicalConsumptionID != r.WaitID || allocation.ScopeRef != r.ProbeRequestHash {
		return fmt.Errorf("runtime recoverable wait budget allocation binding is invalid")
	}
	reconciliations := make([]RuntimeBudgetReconciliationRecord, 0, 1)
	for _, record := range ledger.Reconciliations {
		record = record.Normalized()
		if record.AllocationID != r.BudgetAllocationID {
			continue
		}
		if record.ProbeKind != string(r.ProbeKind) || record.ProbeOwner != r.ProbeOwner || record.ProbeRequestHash != r.ProbeRequestHash || record.ExpectedOwnerFence != r.RecoveryConditionHash {
			return fmt.Errorf("runtime recoverable wait budget reconciliation binding is invalid")
		}
		reconciliations = append(reconciliations, record)
	}
	if len(reconciliations) > 1 {
		return fmt.Errorf("runtime recoverable wait has multiple budget reconciliations")
	}
	var reconciliation *RuntimeBudgetReconciliationRecord
	if len(reconciliations) == 1 {
		reconciliation = &reconciliations[0]
	}
	switch r.Status {
	case RuntimeRecoverableWaitWaiting, RuntimeRecoverableWaitProbeClaimed:
		if r.LastProbeResultHash != "" {
			return fmt.Errorf("active runtime recoverable wait cannot carry a probe result")
		}
		if allocation.State != RuntimeBudgetAllocationReserved && allocation.State != RuntimeBudgetAllocationUnknown {
			return fmt.Errorf("active runtime recoverable wait requires a reserved or unknown probe allocation")
		}
		if allocation.State == RuntimeBudgetAllocationReserved && reconciliation != nil {
			return fmt.Errorf("reserved runtime recoverable wait cannot carry budget reconciliation")
		}
		if allocation.State == RuntimeBudgetAllocationUnknown && (reconciliation == nil || (reconciliation.Status != RuntimeBudgetReconciliationPending && reconciliation.Status != RuntimeBudgetReconciliationStillUnknown)) {
			return fmt.Errorf("unknown active runtime recoverable wait requires an active budget reconciliation")
		}
	case RuntimeRecoverableWaitConditionSatisfied, RuntimeRecoverableWaitRetired:
		if r.Attempt == 0 || r.LastProbeResultHash == "" || allocation.State != RuntimeBudgetAllocationConfirmed {
			return fmt.Errorf("satisfied runtime recoverable wait requires a confirmed probe")
		}
		if reconciliation != nil && (reconciliation.Status != RuntimeBudgetReconciliationConfirmed || reconciliation.OwnerResultHash != r.LastProbeResultHash) {
			return fmt.Errorf("satisfied runtime recoverable wait has inconsistent budget reconciliation")
		}
	case RuntimeRecoverableWaitExpired:
		switch allocation.State {
		case RuntimeBudgetAllocationReleased:
			if r.Attempt != 0 || r.LastProbeResultHash != "" || reconciliation != nil {
				return fmt.Errorf("unprobed expired runtime recoverable wait has inconsistent terminal facts")
			}
		case RuntimeBudgetAllocationConfirmed:
			if r.Attempt == 0 || r.LastProbeResultHash == "" || (reconciliation != nil && (reconciliation.Status != RuntimeBudgetReconciliationConfirmed || reconciliation.OwnerResultHash != r.LastProbeResultHash)) {
				return fmt.Errorf("confirmed expired runtime recoverable wait has inconsistent terminal facts")
			}
		case RuntimeBudgetAllocationExhausted:
			if r.Attempt != 0 || r.LastProbeResultHash != "" || reconciliation == nil || reconciliation.Status != RuntimeBudgetReconciliationFailed {
				return fmt.Errorf("exhausted expired runtime recoverable wait requires failed budget reconciliation")
			}
		default:
			return fmt.Errorf("expired runtime recoverable wait has nonterminal probe allocation")
		}
	case RuntimeRecoverableWaitExhausted:
		if allocation.State != RuntimeBudgetAllocationExhausted || r.Attempt != 0 || r.LastProbeResultHash != "" || reconciliation == nil || reconciliation.Status != RuntimeBudgetReconciliationFailed {
			return fmt.Errorf("exhausted runtime recoverable wait requires failed budget reconciliation")
		}
	default:
		return fmt.Errorf("runtime recoverable wait status is invalid")
	}
	return nil
}

func (r RuntimeRecoverableWaitRecord) Active() bool {
	switch r.Normalized().Status {
	case RuntimeRecoverableWaitWaiting, RuntimeRecoverableWaitProbeClaimed:
		return true
	default:
		return false
	}
}

func StableRuntimeRecoverableWaitID(record RuntimeRecoverableWaitRecord) string {
	r := record.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-recoverable-wait")
	writeHashString(h, string(r.WorkspaceID))
	writeHashString(h, string(r.RunID))
	writeHashString(h, r.DefinitionRevision)
	writeHashString(h, r.AuthorityTransactionID)
	writeHashString(h, string(r.ReasonCode))
	writeHashString(h, string(r.ProbeKind))
	writeHashString(h, r.ProbeOwner)
	writeHashString(h, r.ProbeRequestHash)
	writeHashString(h, r.RecoveryConditionHash)
	return "runtime-wait-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeProviderCeilingProbeRequestHash(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID, prerequisiteID string, selection ProviderInferenceSelection, minimumBytes int) string {
	selection = selection.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-provider-ceiling-probe")
	writeHashString(h, string(workspaceID))
	writeHashString(h, string(runID))
	writeHashString(h, strings.TrimSpace(authoritySnapshotID))
	writeHashString(h, strings.TrimSpace(prerequisiteID))
	writeHashString(h, selection.Target.ProviderID)
	writeHashString(h, selection.Target.ModelID)
	writeHashString(h, selection.ReasoningEffort)
	writeHashString(h, fmt.Sprintf("%d", minimumBytes))
	return "runtime-provider-ceiling-probe-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeProviderCeilingRecoveryConditionHash(definitionRevision, authorityTransactionID, prerequisiteID string) string {
	h := sha256.New()
	writeHashString(h, "runtime-provider-ceiling-condition")
	writeHashString(h, strings.TrimSpace(definitionRevision))
	writeHashString(h, strings.TrimSpace(authorityTransactionID))
	writeHashString(h, strings.TrimSpace(prerequisiteID))
	return "runtime-provider-ceiling-condition-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeRecoverableWaitProbeUnknownHash(waitID, probeRequestHash string) string {
	h := sha256.New()
	writeHashString(h, "runtime-recoverable-wait-probe-unknown")
	writeHashString(h, strings.TrimSpace(waitID))
	writeHashString(h, strings.TrimSpace(probeRequestHash))
	return "runtime-wait-probe-unknown-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeRecoverableWaitProbeObservation struct {
	WaitID           string
	ProbeRequestHash string
	OwnerResultHash  string
	ObservedAt       string
}

func (o RuntimeRecoverableWaitProbeObservation) Normalized() RuntimeRecoverableWaitProbeObservation {
	o.WaitID = strings.TrimSpace(o.WaitID)
	o.ProbeRequestHash = strings.TrimSpace(o.ProbeRequestHash)
	o.OwnerResultHash = strings.TrimSpace(o.OwnerResultHash)
	o.ObservedAt = strings.TrimSpace(o.ObservedAt)
	return o
}

func (o RuntimeRecoverableWaitProbeObservation) Validate() error {
	o = o.Normalized()
	if o.WaitID == "" || o.ProbeRequestHash == "" || o.OwnerResultHash == "" {
		return fmt.Errorf("runtime recoverable wait probe observation is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, o.ObservedAt); err != nil {
		return fmt.Errorf("runtime recoverable wait probe observation time is invalid")
	}
	return nil
}

type RuntimeRecoverableWaitView struct {
	WaitID                   string                          `json:"waitId"`
	ReasonCode               RuntimeRecoverableWaitReason    `json:"reasonCode"`
	Status                   RuntimeRecoverableWaitStatus    `json:"status"`
	ProbeKind                RuntimeRecoverableWaitProbeKind `json:"probeKind"`
	ProbeOwner               string                          `json:"probeOwner"`
	RecoveryConditionSummary string                          `json:"recoveryConditionSummary"`
	OpenedAt                 string                          `json:"openedAt"`
	NextEligibleAt           string                          `json:"nextEligibleAt"`
	Deadline                 string                          `json:"deadline"`
	Attempt                  int                             `json:"attempt"`
	MaxAttempts              int                             `json:"maxAttempts"`
	BudgetDimension          RuntimeBudgetDimension          `json:"budgetDimension"`
	BudgetAllocationID       string                          `json:"budgetAllocationId"`
	StateRevision            uint64                          `json:"stateRevision"`
}

func (v RuntimeRecoverableWaitView) Clone() RuntimeRecoverableWaitView { return v }

func RuntimeRecoverableWaitProjection(record RuntimeRecoverableWaitRecord, stateRevision uint64) (RuntimeRecoverableWaitView, error) {
	r := record.Normalized()
	if stateRevision == 0 || !r.Active() {
		return RuntimeRecoverableWaitView{}, fmt.Errorf("runtime recoverable wait projection requires an active wait")
	}
	return RuntimeRecoverableWaitView{
		WaitID: r.WaitID, ReasonCode: r.ReasonCode, Status: r.Status, ProbeKind: r.ProbeKind, ProbeOwner: r.ProbeOwner,
		RecoveryConditionSummary: string(r.ProbeKind) + ":" + string(r.ReasonCode), OpenedAt: r.OpenedAt,
		NextEligibleAt: r.NextEligibleAt, Deadline: r.Deadline, Attempt: r.Attempt, MaxAttempts: r.MaxAttempts,
		BudgetDimension: r.BudgetDimension, BudgetAllocationID: r.BudgetAllocationID, StateRevision: stateRevision,
	}, nil
}
