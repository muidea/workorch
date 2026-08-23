package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeProviderBudgetSettlementStatus string

const (
	RuntimeProviderBudgetSettlementConfirmed RuntimeProviderBudgetSettlementStatus = "confirmed"
	RuntimeProviderBudgetSettlementReleased  RuntimeProviderBudgetSettlementStatus = "released"
	RuntimeProviderBudgetSettlementUnknown   RuntimeProviderBudgetSettlementStatus = "unknown"
)

// RuntimeProviderBudgetSettlement is the Orchestrator-owner-private result for
// all reservations authorized by one physical LLM request. It is derived only
// from the exact LLM audit fact (or from the absence of any trusted fact) and
// is applied in the same aggregate CAS as the request-session transition.
type RuntimeProviderBudgetSettlement struct {
	LogicalRequestID string
	PhysicalCallID   string
	Status           RuntimeProviderBudgetSettlementStatus
	Usage            *llmaudit.ProviderUsage
	OwnerFactRef     string
	OwnerFactHash    string
	SettlementHash   string
}

func (s RuntimeProviderBudgetSettlement) Clone() RuntimeProviderBudgetSettlement {
	s.LogicalRequestID = strings.TrimSpace(s.LogicalRequestID)
	s.PhysicalCallID = strings.TrimSpace(s.PhysicalCallID)
	s.Status = RuntimeProviderBudgetSettlementStatus(strings.TrimSpace(string(s.Status)))
	s.OwnerFactRef = strings.TrimSpace(s.OwnerFactRef)
	s.OwnerFactHash = strings.TrimSpace(s.OwnerFactHash)
	s.SettlementHash = strings.TrimSpace(s.SettlementHash)
	if s.Usage != nil {
		usage := *s.Usage
		s.Usage = &usage
	}
	return s
}

func (s RuntimeProviderBudgetSettlement) Validate() error {
	s = s.Clone()
	if s.LogicalRequestID == "" || s.PhysicalCallID == "" || s.OwnerFactRef == "" || s.OwnerFactHash == "" || s.SettlementHash == "" {
		return fmt.Errorf("runtime provider budget settlement identity is incomplete")
	}
	switch s.Status {
	case RuntimeProviderBudgetSettlementConfirmed:
	case RuntimeProviderBudgetSettlementReleased, RuntimeProviderBudgetSettlementUnknown:
		if s.Usage != nil {
			return fmt.Errorf("unsubmitted or unknown provider settlement cannot carry usage")
		}
	default:
		return fmt.Errorf("runtime provider budget settlement status is invalid")
	}
	if s.Usage != nil {
		values := []int{s.Usage.InputTokens, s.Usage.OutputTokens, s.Usage.TotalTokens, s.Usage.CacheReadTokens, s.Usage.CacheWriteTokens, s.Usage.CacheCreationTokens, s.Usage.CacheableInputTokens, s.Usage.NonCacheableInputTokens}
		for _, value := range values {
			if value < 0 {
				return fmt.Errorf("runtime provider budget settlement usage cannot be negative")
			}
		}
	}
	if s.SettlementHash != stableRuntimeProviderBudgetSettlementHash(s) {
		return fmt.Errorf("runtime provider budget settlement hash mismatch")
	}
	return nil
}

func NewRuntimeProviderBudgetUnknownSettlement(logicalRequestID, physicalCallID string) (RuntimeProviderBudgetSettlement, error) {
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	physicalCallID = strings.TrimSpace(physicalCallID)
	ownerFactHash := stableRuntimeProviderOwnerFactHash("provider-outcome-unknown", logicalRequestID, physicalCallID)
	ownerFactRef := "orchestrator/provider-outcome-unknown/" + physicalCallID + "/" + ownerFactHash
	return newRuntimeProviderBudgetSettlement(logicalRequestID, physicalCallID, RuntimeProviderBudgetSettlementUnknown, nil, ownerFactRef, ownerFactHash)
}

// NewRuntimeProviderBudgetSettlementFromAudit turns the LLM owner's exact
// request audit into confirmed/released settlement authority. Request bytes are
// deliberately ignored: ProviderSubmitted is the only submission fact.
func NewRuntimeProviderBudgetSettlementFromAudit(workspaceID, runID, logicalRequestID, physicalCallID string, audit *runtimecontract.ProviderRequestAuditView, usage *llmaudit.ProviderUsage) (RuntimeProviderBudgetSettlement, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	physicalCallID = strings.TrimSpace(physicalCallID)
	if audit == nil {
		return NewRuntimeProviderBudgetUnknownSettlement(logicalRequestID, physicalCallID)
	}
	if audit.RequestID != physicalCallID || audit.LogicalRequestID != logicalRequestID || audit.PhysicalCallID != physicalCallID ||
		audit.WorkspaceID != workspaceID || audit.RunID != runID || audit.Status != strings.TrimSpace(audit.Status) || audit.Status == "" ||
		audit.DurationMs < 0 || audit.RequestBytes < 0 || audit.ResponseBytes < 0 {
		return RuntimeProviderBudgetSettlement{}, fmt.Errorf("LLM provider audit does not match the submitted runtime request")
	}
	if err := audit.PhysicalResponseMode.Validate(); err != nil {
		return RuntimeProviderBudgetSettlement{}, err
	}
	if audit.PhysicalResponseMode != audit.PhysicalResponseMode.Normalized() || audit.ContextBudget.Empty() || audit.ContextBudget.ProviderSubmitted != audit.ProviderSubmitted {
		return RuntimeProviderBudgetSettlement{}, fmt.Errorf("LLM provider audit has an invalid physical submission fact")
	}
	status := RuntimeProviderBudgetSettlementConfirmed
	if !audit.ProviderSubmitted {
		if audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || audit.ResponseBytes != 0 || usage != nil {
			return RuntimeProviderBudgetSettlement{}, fmt.Errorf("unsubmitted LLM provider audit cannot carry physical response usage")
		}
		status = RuntimeProviderBudgetSettlementReleased
	}
	payload, err := json.Marshal(struct {
		Audit runtimecontract.ProviderRequestAuditView `json:"audit"`
		Usage *llmaudit.ProviderUsage                  `json:"usage,omitempty"`
	}{Audit: *audit, Usage: usage})
	if err != nil {
		return RuntimeProviderBudgetSettlement{}, err
	}
	ownerFactHash := "llm-provider-audit-" + stableRuntimeProviderBytesHash(payload)
	ownerFactRef := "llm/provider-request-audit/" + physicalCallID + "/" + ownerFactHash
	return newRuntimeProviderBudgetSettlement(logicalRequestID, physicalCallID, status, usage, ownerFactRef, ownerFactHash)
}

func newRuntimeProviderBudgetSettlement(logicalRequestID, physicalCallID string, status RuntimeProviderBudgetSettlementStatus, usage *llmaudit.ProviderUsage, ownerFactRef, ownerFactHash string) (RuntimeProviderBudgetSettlement, error) {
	settlement := RuntimeProviderBudgetSettlement{
		LogicalRequestID: strings.TrimSpace(logicalRequestID), PhysicalCallID: strings.TrimSpace(physicalCallID),
		Status: status, Usage: usage, OwnerFactRef: strings.TrimSpace(ownerFactRef), OwnerFactHash: strings.TrimSpace(ownerFactHash),
	}
	settlement.SettlementHash = stableRuntimeProviderBudgetSettlementHash(settlement)
	if err := settlement.Validate(); err != nil {
		return RuntimeProviderBudgetSettlement{}, err
	}
	return settlement.Clone(), nil
}

func stableRuntimeProviderBudgetSettlementHash(settlement RuntimeProviderBudgetSettlement) string {
	s := settlement.Clone()
	payload, _ := json.Marshal(struct {
		LogicalRequestID string                                `json:"logicalRequestId"`
		PhysicalCallID   string                                `json:"physicalCallId"`
		Status           RuntimeProviderBudgetSettlementStatus `json:"status"`
		Usage            *llmaudit.ProviderUsage               `json:"usage,omitempty"`
		OwnerFactRef     string                                `json:"ownerFactRef"`
		OwnerFactHash    string                                `json:"ownerFactHash"`
	}{s.LogicalRequestID, s.PhysicalCallID, s.Status, s.Usage, s.OwnerFactRef, s.OwnerFactHash})
	return "runtime-provider-budget-settlement-" + stableRuntimeProviderBytesHash(payload)
}

func stableRuntimeProviderOwnerFactHash(parts ...string) string {
	return "runtime-provider-owner-fact-" + stableRuntimeProviderBytesHash([]byte(strings.Join(parts, "\x00")))
}

func stableRuntimeProviderBytesHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])[:20]
}

// reserveCandidateRequestExecutionBudget is part of the same SM-3 CAS as the
// provider-submitted marker.  A successful return authorizes exactly one
// physical provider call; an error therefore leaves the caller with zero
// external-call authority.
func reserveCandidateRequestExecutionBudget(aggregate *RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, sequence int, physicalCallID string) error {
	if aggregate == nil || session.ExpectedDocument == nil {
		return fmt.Errorf("runtime candidate request has no expected-document budget identity")
	}
	specs, err := runtimecontract.RuntimeProviderBudgetAllocationSpecs(
		aggregate.ExecutionBudget,
		session.DocumentKind,
		session.ExpectedDocument.CorrectionAttempt,
		sequence,
		session.LogicalRequestID,
		physicalCallID,
	)
	if err != nil {
		return err
	}
	ledger, _, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, specs)
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

// applyCandidateRequestBudgetSettlement is the sole Orchestrator-private
// reducer for provider reservation settlement. All matching dimensions are
// transformed on the local aggregate copy; the caller commits that copy once
// together with its request-session/recovery transition.
func applyCandidateRequestBudgetSettlement(aggregate *RuntimeAggregate, settlement *RuntimeProviderBudgetSettlement) (bool, error) {
	if aggregate == nil || settlement == nil {
		return false, nil
	}
	fact := settlement.Clone()
	if err := fact.Validate(); err != nil {
		return false, err
	}
	ledger := aggregate.ExecutionBudget.Clone()
	matched := 0
	changed := false
	for _, allocation := range append([]runtimecontract.RuntimeBudgetAllocation(nil), ledger.Allocations...) {
		allocation = allocation.Normalized()
		if allocation.LogicalConsumptionID != fact.LogicalRequestID || allocation.ScopeRef != fact.PhysicalCallID {
			continue
		}
		matched++
		if fact.Status == RuntimeProviderBudgetSettlementReleased {
			var released bool
			var err error
			ledger, released, err = runtimecontract.ReleaseRuntimeBudgetAllocation(ledger, allocation.AllocationID, fact.OwnerFactRef)
			if err != nil {
				return false, err
			}
			changed = changed || released
			continue
		}
		confirmed, unknown := int64(1), int64(0)
		if fact.Status == RuntimeProviderBudgetSettlementUnknown {
			confirmed, unknown = 0, allocation.ReservedAmount
		} else {
			switch allocation.Dimension {
			case runtimecontract.RuntimeBudgetDimensionProviderInputToken:
				if fact.Usage == nil {
					confirmed, unknown = 0, allocation.ReservedAmount
				} else {
					confirmed = int64(fact.Usage.InputTokens)
				}
			case runtimecontract.RuntimeBudgetDimensionProviderOutputToken:
				if fact.Usage == nil {
					confirmed, unknown = 0, allocation.ReservedAmount
				} else {
					confirmed = int64(fact.Usage.OutputTokens)
				}
			}
		}
		var joined bool
		var err error
		ledger, _, joined, err = runtimecontract.JoinRuntimeBudgetConsumption(ledger, runtimecontract.RuntimeBudgetConsumptionJoin{
			AllocationID: allocation.AllocationID, Owner: runtimecontract.RuntimeModuleLLM, PhysicalConsumptionID: fact.PhysicalCallID,
			ConfirmedAmount: confirmed, UnknownAmount: unknown, OwnerResultRef: fact.OwnerFactRef, OwnerResultHash: fact.OwnerFactHash,
		})
		if err != nil {
			return false, err
		}
		changed = changed || joined
	}
	if matched == 0 {
		return false, fmt.Errorf("runtime provider budget settlement has no allocation")
	}
	aggregate.ExecutionBudget = ledger
	return changed, nil
}

func runtimeOperationBudgetScopeRef(executionKey string, fence uint64) string {
	return fmt.Sprintf("%s/%d", strings.TrimSpace(executionKey), fence)
}

func reserveRuntimeOperationExecutionBudget(aggregate *RuntimeAggregate, execution runtimecontract.RuntimeOperationExecutionRecord, nextFence uint64) error {
	if aggregate == nil || strings.TrimSpace(execution.ExecutionKey) == "" || nextFence == 0 {
		return fmt.Errorf("runtime operation budget identity is incomplete")
	}
	ledger, _, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension:            runtimecontract.RuntimeBudgetDimensionCapabilityOperation,
		LogicalConsumptionID: execution.ExecutionKey,
		ScopeRef:             runtimeOperationBudgetScopeRef(execution.ExecutionKey, nextFence),
		ReservedAmount:       1,
	}})
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

func joinRuntimeOperationExecutionBudget(aggregate *RuntimeAggregate, operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, receipt *runtimecontract.RuntimeOperationReceipt, outcomeUnknown bool) (bool, error) {
	if aggregate == nil || strings.TrimSpace(execution.ExecutionKey) == "" || execution.Fence == 0 {
		return false, fmt.Errorf("runtime operation budget identity is incomplete")
	}
	scopeRef := runtimeOperationBudgetScopeRef(execution.ExecutionKey, execution.Fence)
	var allocation runtimecontract.RuntimeBudgetAllocation
	for _, candidate := range aggregate.ExecutionBudget.Allocations {
		candidate = candidate.Normalized()
		if candidate.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation && candidate.LogicalConsumptionID == execution.ExecutionKey && candidate.ScopeRef == scopeRef {
			allocation = candidate
			break
		}
	}
	if allocation.AllocationID == "" {
		return false, fmt.Errorf("runtime operation execution has no matching budget allocation")
	}
	owner := strings.TrimSpace(runtimecontract.RuntimeModuleForOperationKind(operation.Kind))
	if owner == "" {
		return false, fmt.Errorf("runtime operation budget owner is unknown")
	}
	resultRef := "runtime-receipt/" + execution.ExecutionKey
	resultHash := "runtime-operation-outcome-unknown/" + scopeRef
	confirmed, unknown := int64(1), int64(0)
	if outcomeUnknown {
		confirmed, unknown = 0, 1
	} else {
		if receipt == nil {
			return false, fmt.Errorf("runtime operation budget confirmation requires receipt")
		}
		resultHash = runtimecontract.StableRuntimeOperationReceiptHash(*receipt)
	}
	ledger, _, changed, err := runtimecontract.JoinRuntimeBudgetConsumption(aggregate.ExecutionBudget, runtimecontract.RuntimeBudgetConsumptionJoin{
		AllocationID: allocation.AllocationID, Owner: owner, PhysicalConsumptionID: scopeRef,
		ConfirmedAmount: confirmed, UnknownAmount: unknown, OwnerResultRef: resultRef, OwnerResultHash: resultHash,
	})
	if err != nil {
		return false, err
	}
	aggregate.ExecutionBudget = ledger
	return changed, nil
}
