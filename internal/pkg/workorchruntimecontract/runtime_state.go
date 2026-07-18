package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeDispatchState string

const (
	RuntimeDispatchStateSealed    RuntimeDispatchState = "sealed"
	RuntimeDispatchStateCommitted RuntimeDispatchState = "committed"
)

type RuntimeDispatchRecord struct {
	ID                    string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	Transaction           AuthorityTransaction
	CandidateHash         string
	Candidate             RuntimeCandidate
	State                 RuntimeDispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the required per-operation claim/fence ledger.
	OperationExecutions []RuntimeOperationExecutionRecord
}

type RuntimeDiagnosticRecord struct {
	EventID             string
	AuthoritySnapshotID string
	CurrentSnapshotID   string
	DispatchID          string
	Reason              string
}

type RuntimeCandidateAttemptState string

const (
	RuntimeCandidateAttemptStatePending  RuntimeCandidateAttemptState = "pending"
	RuntimeCandidateAttemptStateFailed   RuntimeCandidateAttemptState = "failed"
	RuntimeCandidateAttemptStateAccepted RuntimeCandidateAttemptState = "accepted"
)

type RuntimeCandidateAttemptRecord struct {
	AuthorityContractID string
	Transaction         AuthorityTransaction
	Attempt             int
	StablePrefixHash    string
	State               RuntimeCandidateAttemptState
	Failure             *RuntimeCandidateFailure
}

// RuntimeStateRecord 是 Runtime V2 在 Run owner 中持久化的唯一正式状态。
type RuntimeStateRecord struct {
	// SchemaVersion is the persisted Runtime V2 aggregate schema. Production
	// persistence accepts only SchemaVersionCurrent.
	SchemaVersion               uint32
	StateRevision               uint64
	EvidenceSequence            uint64
	Definition                  RuntimeDefinitionContext
	Facts                       RuntimeFacts
	Decision                    RuntimeDecision
	ActiveDispatch              *RuntimeDispatchRecord
	CompletedDispatches         []RuntimeDispatchRecord
	ProcessedEventIDs           []string
	Diagnostics                 []RuntimeDiagnosticRecord
	CandidateAttempts           []RuntimeCandidateAttemptRecord
	GovernanceRequests          []RuntimeGovernanceRequestRecord
	GovernanceResolutions       []RuntimeGovernanceResolutionRecord
	OperationGovernanceBindings []RuntimeOperationGovernanceBinding
	// OperationReceipts are owner-authored terminal receipts persisted before
	// capability owners return execution results (schema v2 / INV-03/INV-04).
	OperationReceipts []RuntimeOperationReceipt
	// Termination is the SM-1 administrative cancel/supersede record (schema v2).
	// completed/failed do not use this field.
	Termination *RuntimeTerminationRecord
}

const (
	RuntimeStateSchemaVersionV2 uint32 = 2
	// RuntimeStateSchemaVersionCurrent is the only production Runtime V2 schema.
	// Every other version fails fast; no in-place migration or semantic inference is allowed.
	RuntimeStateSchemaVersionCurrent uint32 = 2
)

func (r RuntimeStateRecord) Validate() error {
	if r.SchemaVersion != RuntimeStateSchemaVersionCurrent {
		return fmt.Errorf("unsupported runtime state schema version %d", r.SchemaVersion)
	}
	if r.Facts.WorkspaceID == "" || r.Facts.RunID == "" {
		return fmt.Errorf("runtime state requires workspace and run scope")
	}
	if err := r.Facts.Validate(); err != nil {
		return err
	}
	if err := r.Definition.Validate(); err != nil {
		return err
	}
	if r.Definition.Normalized().Revision != r.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime state definition revision mismatch")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if r.Decision.AuthoritySnapshot.WorkspaceID != r.Facts.WorkspaceID || r.Decision.AuthoritySnapshot.RunID != r.Facts.RunID {
		return fmt.Errorf("runtime state decision scope mismatch")
	}
	if r.ActiveDispatch != nil {
		if err := r.ActiveDispatch.ValidateWithActivation(r.Decision, r.Facts.Activation); err != nil {
			return fmt.Errorf("runtime state active dispatch: %w", err)
		}
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceResolutionRecords(r.GovernanceResolutions); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestRecords(r.GovernanceRequests); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceBindings(r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceipts(r.OperationReceipts); err != nil {
		return err
	}
	for _, request := range r.GovernanceRequests {
		matched := r.ActiveDispatch != nil && r.ActiveDispatch.ID == request.DispatchID &&
			r.ActiveDispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && r.ActiveDispatch.Candidate.Governance != nil
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if dispatch.ID == request.DispatchID && dispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && dispatch.Candidate.Governance != nil {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("runtime governance request has no governed dispatch")
		}
	}
	for _, rawBinding := range r.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *RuntimeDispatchRecord
		for idx := range r.CompletedDispatches {
			if r.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := r.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		var execution *RuntimeOperationExecutionRecord
		for idx := range source.OperationExecutions {
			if source.OperationExecutions[idx].OperationID == binding.OperationID {
				value := source.OperationExecutions[idx].Normalized()
				execution = &value
				break
			}
		}
		if execution == nil || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
			execution.OwnerReceiptHash != binding.GovernanceReceiptHash || execution.State != OperationExecutionOwnerCommitted {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := RuntimeOperationReceiptForExecutionKeyFence(r.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || receipt.Outcome != RuntimeOperationOutcomeGovernanceRequired || StableRuntimeOperationReceiptHash(receipt) != binding.GovernanceReceiptHash {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == binding.TicketID && request.DispatchID == binding.GovernanceDispatchID &&
				request.AuthoritySnapshotID == binding.GovernanceSnapshotID && request.DependencyKind == binding.DependencyKind && request.SubjectRef == binding.SubjectRef {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching ticket request")
		}
	}
	for _, attempt := range r.CandidateAttempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := r.ActiveDispatch != nil && r.ActiveDispatch.Transaction == attempt.Transaction
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted runtime candidate attempt has no sealed dispatch")
		}
	}
	for _, resolution := range r.GovernanceResolutions {
		matched := false
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID {
				bound = true
				break
			}
		}
		for _, dispatch := range r.CompletedDispatches {
			if dispatch.ID == resolution.DispatchID && dispatch.Transaction.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				dispatch.State == RuntimeDispatchStateCommitted && dispatch.Candidate.Governance != nil {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime governance resolution has no completed governed dispatch")
		}
	}
	if r.Termination != nil {
		if err := r.Termination.Validate(); err != nil {
			return fmt.Errorf("runtime state termination: %w", err)
		}
	}
	if r.ActiveDispatch != nil {
		if err := ValidateRuntimeOperationExecutionRecords(r.ActiveDispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state active dispatch executions: %w", err)
		}
	}
	for idx, dispatch := range r.CompletedDispatches {
		if err := ValidateRuntimeOperationExecutionRecords(dispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state completed dispatch[%d] executions: %w", idx, err)
		}
	}
	return nil
}

func ValidateRuntimeCandidateAttemptRecords(values []RuntimeCandidateAttemptRecord) error {
	seen := map[string]struct{}{}
	bySnapshot := map[string][]RuntimeCandidateAttemptRecord{}
	for _, value := range values {
		if value.Attempt < 0 || value.Attempt > 1 || strings.TrimSpace(value.Transaction.AuthoritySnapshotID) == "" || strings.TrimSpace(value.StablePrefixHash) == "" {
			return fmt.Errorf("runtime candidate attempt requires sealed transaction, stable prefix and attempt 0 or 1")
		}
		key := value.Transaction.AuthoritySnapshotID + "|" + fmt.Sprintf("%d", value.Attempt)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("runtime candidate attempt %s is duplicated", key)
		}
		seen[key] = struct{}{}
		bySnapshot[value.Transaction.AuthoritySnapshotID] = append(bySnapshot[value.Transaction.AuthoritySnapshotID], value)
		switch value.State {
		case RuntimeCandidateAttemptStatePending, RuntimeCandidateAttemptStateAccepted:
			if value.Failure != nil {
				return fmt.Errorf("runtime candidate attempt %s state %s cannot carry failure", key, value.State)
			}
		case RuntimeCandidateAttemptStateFailed:
			if value.Failure == nil || value.Failure.Class == "" || strings.TrimSpace(value.Failure.Reason) == "" {
				return fmt.Errorf("failed runtime candidate attempt %s requires typed failure", key)
			}
		default:
			return fmt.Errorf("runtime candidate attempt %s has invalid state %q", key, value.State)
		}
	}
	for snapshotID, attempts := range bySnapshot {
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if hasSecond && hasFirst && (first.State != RuntimeCandidateAttemptStateFailed || first.Transaction != second.Transaction || first.StablePrefixHash != second.StablePrefixHash) {
			return fmt.Errorf("runtime candidate snapshot %s attempt 1 is not a same-contract retry", snapshotID)
		}
		if hasFirst && first.State == RuntimeCandidateAttemptStateAccepted && hasSecond {
			return fmt.Errorf("runtime candidate snapshot %s retried after accepted attempt 0", snapshotID)
		}
	}
	return nil
}

// ValidateRuntimeCandidateAttemptRecordsForScope validates every persisted record
// against its explicit stable authority contract identity.
func ValidateRuntimeCandidateAttemptRecordsForScope(workspaceID common.WorkspaceID, runID common.RunID, values []RuntimeCandidateAttemptRecord) error {
	if err := ValidateRuntimeCandidateAttemptRecords(values); err != nil {
		return err
	}
	byContract := map[string][]RuntimeCandidateAttemptRecord{}
	for _, value := range values {
		if strings.TrimSpace(value.AuthorityContractID) == "" {
			return fmt.Errorf("runtime candidate attempt requires authority contract id")
		}
		contractID := CandidateAttemptAuthorityContractID(workspaceID, runID, value)
		byContract[contractID] = append(byContract[contractID], value)
		if value.AuthorityContractID != contractID || value.AuthorityContractID != StableAuthorityContractID(workspaceID, runID, value.Transaction) {
			return fmt.Errorf("runtime candidate authority contract id mismatch")
		}
	}
	for contractID, attempts := range byContract {
		attemptCounts := [2]int{}
		for _, attempt := range attempts {
			attemptCounts[attempt.Attempt]++
		}
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if len(attempts) > 2 || attemptCounts[0] > 1 || attemptCounts[1] > 1 || (hasSecond && (!hasFirst || first.State != RuntimeCandidateAttemptStateFailed || !SameAuthorityContract(first.Transaction, second.Transaction) || first.StablePrefixHash != second.StablePrefixHash)) {
			return fmt.Errorf("runtime candidate authority contract %s exceeds the sealed attempt 0/1 transaction", contractID)
		}
		if hasFirst && first.State == RuntimeCandidateAttemptStateAccepted && hasSecond {
			return fmt.Errorf("runtime candidate authority contract %s retried after accepted attempt 0", contractID)
		}
	}
	return nil
}

// StableAuthorityContractID identifies the candidate retry contract without
// reducer bookkeeping identity.
func StableAuthorityContractID(workspaceID common.WorkspaceID, runID common.RunID, transaction AuthorityTransaction) string {
	h := sha256.New()
	writeHashString(h, string(workspaceID))
	writeHashString(h, string(runID))
	writeHashString(h, transaction.FocusID)
	writeHashString(h, string(transaction.Action))
	writeHashString(h, transaction.RequiredOutputHash)
	writeHashString(h, transaction.DefinitionRevision)
	writeHashString(h, transaction.EvidenceRevision)
	return "authority-contract-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func SameAuthorityContract(left AuthorityTransaction, right AuthorityTransaction) bool {
	return left.FocusID == right.FocusID &&
		left.Action == right.Action &&
		left.RequiredOutputHash == right.RequiredOutputHash &&
		left.DefinitionRevision == right.DefinitionRevision &&
		left.EvidenceRevision == right.EvidenceRevision
}

func CandidateAttemptAuthorityContractID(workspaceID common.WorkspaceID, runID common.RunID, record RuntimeCandidateAttemptRecord) string {
	return strings.TrimSpace(record.AuthorityContractID)
}

func runtimeCandidateAttemptByNumber(values []RuntimeCandidateAttemptRecord, attempt int) (RuntimeCandidateAttemptRecord, bool) {
	for _, value := range values {
		if value.Attempt == attempt {
			return value, true
		}
	}
	return RuntimeCandidateAttemptRecord{}, false
}

func (r RuntimeStateRecord) Clone() RuntimeStateRecord {
	ret := r
	ret.Definition = r.Definition.Clone()
	ret.Facts = r.Facts.Normalized()
	ret.Decision = cloneRuntimeDecision(r.Decision)
	if r.ActiveDispatch != nil {
		value := r.ActiveDispatch.Clone()
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]RuntimeDispatchRecord, len(r.CompletedDispatches))
	for idx := range r.CompletedDispatches {
		ret.CompletedDispatches[idx] = r.CompletedDispatches[idx].Clone()
	}
	ret.ProcessedEventIDs = append([]string(nil), r.ProcessedEventIDs...)
	ret.Diagnostics = append([]RuntimeDiagnosticRecord(nil), r.Diagnostics...)
	ret.CandidateAttempts = make([]RuntimeCandidateAttemptRecord, len(r.CandidateAttempts))
	for idx, attempt := range r.CandidateAttempts {
		ret.CandidateAttempts[idx] = attempt
		if attempt.Failure != nil {
			failure := *attempt.Failure
			ret.CandidateAttempts[idx].Failure = &failure
		}
	}
	ret.GovernanceRequests = append([]RuntimeGovernanceRequestRecord(nil), r.GovernanceRequests...)
	ret.GovernanceResolutions = append([]RuntimeGovernanceResolutionRecord(nil), r.GovernanceResolutions...)
	ret.OperationGovernanceBindings = make([]RuntimeOperationGovernanceBinding, len(r.OperationGovernanceBindings))
	for idx := range r.OperationGovernanceBindings {
		ret.OperationGovernanceBindings[idx] = r.OperationGovernanceBindings[idx].Clone()
	}
	ret.OperationReceipts = NormalizeRuntimeOperationReceipts(r.OperationReceipts)
	if r.Termination != nil {
		term := r.Termination.Clone()
		ret.Termination = &term
	}
	return ret
}

func cloneRuntimeDecision(decision RuntimeDecision) RuntimeDecision {
	ret := decision
	ret.Focus = decision.Focus.Normalized()
	ret.RequiredOutput = decision.RequiredOutput.Normalized()
	ret.Prerequisites = append([]RuntimePrerequisite(nil), decision.Prerequisites...)
	for idx := range ret.Prerequisites {
		ret.Prerequisites[idx].RequiredCapabilities = append([]string(nil), decision.Prerequisites[idx].RequiredCapabilities...)
	}
	if decision.Terminal != nil {
		value := *decision.Terminal
		ret.Terminal = &value
	}
	return ret
}

func (d RuntimeDispatchRecord) Validate(decision RuntimeDecision) error {
	return d.ValidateWithActivation(decision, RuntimeActivationContext{})
}

func (d RuntimeDispatchRecord) ValidateWithActivation(decision RuntimeDecision, activation RuntimeActivationContext) error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" {
		return fmt.Errorf("runtime dispatch requires id, scope and candidate hash")
	}
	if err := ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("runtime dispatch scope mismatch")
	}
	if err := d.Candidate.ValidateWithActivation(decision, activation); err != nil {
		return err
	}
	if d.CandidateHash != StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("runtime dispatch candidate hash mismatch")
	}
	if d.State != RuntimeDispatchStateSealed && d.State != RuntimeDispatchStateCommitted {
		return fmt.Errorf("runtime dispatch has invalid state %q", d.State)
	}
	available := runtimeCandidateOperationIDs(d.Candidate)
	seen := map[string]struct{}{}
	for _, operationID := range d.CompletedOperationIDs {
		operationID = strings.TrimSpace(operationID)
		if _, ok := available[operationID]; !ok {
			return fmt.Errorf("runtime dispatch completed operation %q is not present in candidate", operationID)
		}
		if _, ok := seen[operationID]; ok {
			return fmt.Errorf("runtime dispatch completed operation %q is duplicated", operationID)
		}
		seen[operationID] = struct{}{}
	}
	if err := ValidateRuntimeOperationExecutionRecords(d.OperationExecutions); err != nil {
		return err
	}
	for _, exec := range NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions) {
		if _, ok := available[exec.OperationID]; !ok {
			return fmt.Errorf("runtime dispatch operation execution %q is not present in candidate", exec.OperationID)
		}
	}
	return nil
}

func (d RuntimeDispatchRecord) Clone() RuntimeDispatchRecord {
	ret := d
	ret.Candidate = cloneRuntimeCandidate(d.Candidate)
	ret.CompletedOperationIDs = append([]string(nil), d.CompletedOperationIDs...)
	ret.OperationExecutions = NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions)
	if len(ret.OperationExecutions) > 0 {
		execs := make([]RuntimeOperationExecutionRecord, len(ret.OperationExecutions))
		copy(execs, ret.OperationExecutions)
		ret.OperationExecutions = execs
	}
	return ret
}

func cloneRuntimeCandidate(candidate RuntimeCandidate) RuntimeCandidate {
	ret := RuntimeCandidate{}
	if candidate.StateChange != nil {
		value := StateChangeCandidate{Operations: make([]StateChangingOperation, len(candidate.StateChange.Operations))}
		for idx, operation := range candidate.StateChange.Operations {
			value.Operations[idx] = StateChangingOperation{Operation: cloneRuntimeOperation(operation.Operation)}
		}
		ret.StateChange = &value
	}
	if candidate.Validation != nil {
		value := ValidationCandidate{Operations: make([]ValidationOperation, len(candidate.Validation.Operations))}
		for idx, operation := range candidate.Validation.Operations {
			value.Operations[idx] = ValidationOperation{Operation: cloneRuntimeOperation(operation.Operation)}
		}
		ret.Validation = &value
	}
	if candidate.Assessment != nil {
		value := candidate.Assessment.Normalized()
		ret.Assessment = &value
	}
	if candidate.Governance != nil {
		value := *candidate.Governance
		ret.Governance = &value
	}
	return ret
}

func cloneRuntimeOperation(operation RuntimeOperation) RuntimeOperation {
	ret := operation.Normalized()
	ret.Parameters = make([]OperationParameter, len(operation.Parameters))
	for idx, parameter := range operation.Parameters {
		ret.Parameters[idx] = parameter
		ret.Parameters[idx].StringList = append([]string(nil), parameter.StringList...)
		if parameter.String != nil {
			value := *parameter.String
			ret.Parameters[idx].String = &value
		}
		if parameter.Integer != nil {
			value := *parameter.Integer
			ret.Parameters[idx].Integer = &value
		}
		if parameter.Boolean != nil {
			value := *parameter.Boolean
			ret.Parameters[idx].Boolean = &value
		}
	}
	return ret
}

func runtimeCandidateOperationIDs(candidate RuntimeCandidate) map[string]struct{} {
	ret := map[string]struct{}{}
	switch {
	case candidate.StateChange != nil:
		for _, operation := range candidate.StateChange.Operations {
			ret[operation.Operation.Normalized().ID] = struct{}{}
		}
	case candidate.Validation != nil:
		for _, operation := range candidate.Validation.Operations {
			ret[operation.Operation.Normalized().ID] = struct{}{}
		}
	}
	return ret
}
