package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCWILedgerSchemaCurrent identifies the canonical CWI identity fields
// persisted on RuntimeCoverageItemFact. The coverage row remains the one CWI
// ledger; this schema does not introduce a parallel registry or owner.
const RuntimeCWILedgerSchemaCurrent = "workorch.runtime_cwi_ledger.v1"

// RuntimeCWIBindingLineageLimit and RuntimeCWIReopenLineageLimit bound the
// per-CWI audit trail. They keep reopening/resealing recoverable without
// turning the canonical coverage row into an unbounded history store.
const (
	RuntimeCWIBindingLineageLimit           = 32
	RuntimeCWIBindingRetirementLineageLimit = 16
	RuntimeCWIReopenLineageLimit            = 16
)

// RuntimeCWIConvergenceState records the SM-2 authority state for one CWI.
// It is intentionally distinct from RuntimeCoverageItemState: the latter
// records accepted coverage credit, while this value controls whether a CWI
// may receive a current semantic binding.
type RuntimeCWIConvergenceState string

const (
	RuntimeCWIConvergenceStateUnbound  RuntimeCWIConvergenceState = "unbound"
	RuntimeCWIConvergenceStatePlanning RuntimeCWIConvergenceState = "planning"
	RuntimeCWIConvergenceStateBound    RuntimeCWIConvergenceState = "bound"
	RuntimeCWIConvergenceStateSettled  RuntimeCWIConvergenceState = "settled"
	RuntimeCWIConvergenceStateReopened RuntimeCWIConvergenceState = "reopened"
)

// RuntimeCWIBindingRef is the compact identity of the exact proof binding
// currently authorized for a CWI. The full proof remains in the sealed focus
// and reconciliation contract; this value only prevents the aggregate from
// losing its canonical identity across semantic generations.
type RuntimeCWIBindingRef struct {
	BindingID            string
	BindingHash          string
	FocusID              string
	SemanticManifestHash string
}

func (r RuntimeCWIBindingRef) Normalized() RuntimeCWIBindingRef {
	r.BindingID = strings.TrimSpace(r.BindingID)
	r.BindingHash = strings.TrimSpace(r.BindingHash)
	r.FocusID = strings.TrimSpace(r.FocusID)
	r.SemanticManifestHash = strings.TrimSpace(r.SemanticManifestHash)
	return r
}

func (r RuntimeCWIBindingRef) Clone() RuntimeCWIBindingRef { return r.Normalized() }

func (r RuntimeCWIBindingRef) Validate() error {
	r = r.Normalized()
	if r.BindingID == "" || r.BindingHash == "" || r.FocusID == "" || r.SemanticManifestHash == "" {
		return fmt.Errorf("runtime CWI binding ref requires binding, focus and semantic generation identity")
	}
	return nil
}

func SameRuntimeCWIBindingRef(left, right RuntimeCWIBindingRef) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.BindingID == right.BindingID && left.BindingHash == right.BindingHash &&
		left.FocusID == right.FocusID && left.SemanticManifestHash == right.SemanticManifestHash
}

// RuntimeCWIBindingRefForCoverageBinding derives a row-local identity from a
// semantic binding. It is pure and cannot grant coverage credit.
func RuntimeCWIBindingRefForCoverageBinding(binding CoverageEvidenceBinding, semanticManifestHash string) RuntimeCWIBindingRef {
	binding = binding.Normalized()
	return RuntimeCWIBindingRef{
		BindingID: binding.ID, BindingHash: binding.Hash, FocusID: binding.FocusID,
		SemanticManifestHash: strings.TrimSpace(semanticManifestHash),
	}.Normalized()
}

// RuntimeCWIReopenLineage records an explicit SM-2 reopening cause while the
// old receipt/evidence remains immutable audit history on the coverage row.
type RuntimeCWIReopenLineage struct {
	PreviousBindingRef RuntimeCWIBindingRef
	TriggerRef         string
	Reason             string
}

func (l RuntimeCWIReopenLineage) Normalized() RuntimeCWIReopenLineage {
	l.PreviousBindingRef = l.PreviousBindingRef.Clone()
	l.TriggerRef = strings.TrimSpace(l.TriggerRef)
	l.Reason = strings.TrimSpace(l.Reason)
	return l
}

func (l RuntimeCWIReopenLineage) Clone() RuntimeCWIReopenLineage { return l.Normalized() }

func (l RuntimeCWIReopenLineage) Validate() error {
	l = l.Normalized()
	if err := l.PreviousBindingRef.Validate(); err != nil {
		return fmt.Errorf("runtime CWI reopen lineage previous binding: %w", err)
	}
	if l.TriggerRef == "" || l.Reason == "" {
		return fmt.Errorf("runtime CWI reopen lineage requires trigger and reason")
	}
	return nil
}

// RuntimeCWIBindingRetirementLineage records an explicit SM-2 retirement of
// a binding before any owner receipt, effect, or evidence was accepted. It
// carries only immutable identity and cause; eligibility is still derived by
// the owner-local reducer and aggregate validator.
type RuntimeCWIBindingRetirementLineage struct {
	RetiredBindingRef RuntimeCWIBindingRef
	CausationRef      string
	Reason            string
}

func (l RuntimeCWIBindingRetirementLineage) Normalized() RuntimeCWIBindingRetirementLineage {
	l.RetiredBindingRef = l.RetiredBindingRef.Clone()
	l.CausationRef = strings.TrimSpace(l.CausationRef)
	l.Reason = strings.TrimSpace(l.Reason)
	return l
}

func (l RuntimeCWIBindingRetirementLineage) Clone() RuntimeCWIBindingRetirementLineage {
	return l.Normalized()
}

func (l RuntimeCWIBindingRetirementLineage) Validate() error {
	l = l.Normalized()
	if err := l.RetiredBindingRef.Validate(); err != nil {
		return fmt.Errorf("runtime CWI binding retirement lineage binding: %w", err)
	}
	if l.CausationRef == "" || l.Reason == "" {
		return fmt.Errorf("runtime CWI binding retirement lineage requires causation and reason")
	}
	return nil
}

func normalizeRuntimeCWIBindingLineage(values []RuntimeCWIBindingRef) []RuntimeCWIBindingRef {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCWIBindingRef, len(values))
	for index := range values {
		ret[index] = values[index].Clone()
	}
	return ret
}

func normalizeRuntimeCWIReopenLineage(values []RuntimeCWIReopenLineage) []RuntimeCWIReopenLineage {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCWIReopenLineage, len(values))
	for index := range values {
		ret[index] = values[index].Clone()
	}
	return ret
}

func normalizeRuntimeCWIBindingRetirementLineage(values []RuntimeCWIBindingRetirementLineage) []RuntimeCWIBindingRetirementLineage {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCWIBindingRetirementLineage, len(values))
	for index := range values {
		ret[index] = values[index].Clone()
	}
	return ret
}

func normalizeRuntimeCoverageItemFact(value RuntimeCoverageItemFact) RuntimeCoverageItemFact {
	value.CWIConvergenceState = RuntimeCWIConvergenceState(strings.TrimSpace(string(value.CWIConvergenceState)))
	value.FocusID = strings.TrimSpace(value.FocusID)
	if value.CurrentBindingRef != nil {
		binding := value.CurrentBindingRef.Clone()
		value.CurrentBindingRef = &binding
	}
	value.BindingLineage = normalizeRuntimeCWIBindingLineage(value.BindingLineage)
	value.BindingRetirementLineage = normalizeRuntimeCWIBindingRetirementLineage(value.BindingRetirementLineage)
	value.ReopenLineage = normalizeRuntimeCWIReopenLineage(value.ReopenLineage)
	return value
}

// InitializeRuntimeCWILedgerForActivation assigns exactly one persisted CWI
// row to each fresh-bootstrap focus. It is activation construction only; it
// must never be used to infer or repair a stored aggregate.
func InitializeRuntimeCWILedgerForActivation(facts RuntimeFacts) (RuntimeFacts, error) {
	facts = facts.Normalized()
	if facts.CWILedgerSchemaVersion != "" && facts.CWILedgerSchemaVersion != RuntimeCWILedgerSchemaCurrent {
		return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger has unsupported schema %q", facts.CWILedgerSchemaVersion)
	}
	for _, focus := range runtimeCWIFocusQueue(facts) {
		if len(focus.CoverageEvidenceBindings) != 0 {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation focus %q must not contain sealed coverage evidence bindings", focus.ID)
		}
		if len(focus.CoverageWorkItemIDs) != 1 {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation focus %q must contain exactly one coverage item", focus.ID)
		}
	}
	coverage := make(map[common.CoverageWorkItemID]struct{}, len(facts.Coverage))
	for _, item := range facts.Coverage {
		if item.ID == "" {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation has empty coverage item")
		}
		if _, duplicate := coverage[item.ID]; duplicate {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation duplicates coverage item %q", item.ID)
		}
		coverage[item.ID] = struct{}{}
	}
	focusByCoverage := make(map[common.CoverageWorkItemID]string, len(coverage))
	for _, focus := range runtimeCWIFocusQueue(facts) {
		if focus.ID == "" {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation has empty focus")
		}
		for _, id := range focus.CoverageWorkItemIDs {
			if _, found := coverage[id]; !found {
				return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger focus %q references unknown coverage item %q", focus.ID, id)
			}
			if prior, duplicate := focusByCoverage[id]; duplicate {
				return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation places coverage item %q in both focus %q and %q", id, prior, focus.ID)
			}
			focusByCoverage[id] = focus.ID
		}
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		focusID, found := focusByCoverage[item.ID]
		if !found {
			return RuntimeFacts{}, fmt.Errorf("runtime CWI ledger activation has no focus for coverage item %q", item.ID)
		}
		item.CWIConvergenceState = RuntimeCWIConvergenceStateUnbound
		item.FocusID = focusID
		item.CurrentBindingRef = nil
		item.BindingLineage = nil
		item.BindingRetirementLineage = nil
		item.ReopenLineage = nil
	}
	facts.CWILedgerSchemaVersion = RuntimeCWILedgerSchemaCurrent
	facts.Coverage = NormalizeRuntimeCoverageFacts(facts.Coverage)
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		return RuntimeFacts{}, err
	}
	return facts, nil
}

// ValidateRuntimeStateCWILedger classifies the persisted canonical CWI
// authority boundary before a Runtime aggregate can be driven. It is pure:
// no owner I/O, migration, replay, or recovery selection is permitted here.
// A current-schema defect is deliberately typed so callers can fail closed
// without mistaking it for a normal Runtime drive failure.
func ValidateRuntimeStateCWILedger(facts RuntimeFacts, ledger RuntimeCWIReconciliationLedger) error {
	if strings.TrimSpace(facts.CWILedgerSchemaVersion) != RuntimeCWILedgerSchemaCurrent {
		return NewRuntimeStateCWILedgerIncompatibilityError(fmt.Errorf("runtime state requires current CWI ledger schema"))
	}
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		return NewRuntimeStateCWILedgerIncompatibilityError(err)
	}
	if err := ValidateRuntimeCWIFocusBindings(facts); err != nil {
		return NewRuntimeStateCWILedgerIncompatibilityError(err)
	}
	if err := ValidateRuntimeCWIReconciliationLedger(facts, ledger); err != nil {
		return NewRuntimeStateCWILedgerIncompatibilityError(err)
	}
	return nil
}

// ValidateRuntimeCWIBindingRetirementsAgainstExecution prevents a binding
// retirement from rewriting already accepted execution history. It reads only
// the persisted aggregate and does not select a transition or recovery path.
func ValidateRuntimeCWIBindingRetirementsAgainstExecution(facts RuntimeFacts, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt) error {
	retired := make(map[string]common.CoverageWorkItemID)
	retiredByFocus := make(map[string]string)
	for _, raw := range facts.Coverage {
		item := normalizeRuntimeCoverageItemFact(raw)
		for _, lineage := range item.BindingRetirementLineage {
			retired[lineage.RetiredBindingRef.BindingID] = item.ID
			retiredByFocus[lineage.RetiredBindingRef.FocusID] = lineage.RetiredBindingRef.BindingID
		}
	}
	if len(retired) == 0 {
		return nil
	}
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	if active != nil {
		dispatches = append(dispatches, active.Clone())
	}
	for _, dispatch := range completed {
		dispatches = append(dispatches, dispatch.Clone())
	}
	for _, dispatch := range dispatches {
		operations := make([]RuntimeOperation, 0)
		if dispatch.Candidate.StateChange != nil {
			operations = append(operations, stateChangingRuntimeOperations(dispatch.Candidate.StateChange.Operations)...)
		}
		if dispatch.Candidate.Validation != nil {
			operations = append(operations, validationRuntimeOperations(dispatch.Candidate.Validation.Operations)...)
		}
		for _, operation := range operations {
			operation = operation.Normalized()
			retiredCoverageID := common.CoverageWorkItemID("")
			retiredBindingID := ""
			for _, bindingID := range operation.BindingRefs {
				if coverageID, found := retired[bindingID]; found {
					retiredCoverageID = coverageID
					retiredBindingID = bindingID
					break
				}
			}
			execution, found := runtimeOperationExecutionForID(dispatch.OperationExecutions, operation.ID)
			if !found || execution.Fence == 0 {
				continue
			}
			receipt, found := RuntimeOperationReceiptForExecutionKeyFence(receipts, execution.ExecutionKey, execution.Fence)
			if !found || receipt.Outcome != RuntimeOperationOutcomeAccepted {
				continue
			}
			if retiredBindingID == "" && len(operation.BindingRefs) == 0 && (RuntimeEffectIsDurable(receipt.ObservedEffectScope) || receipt.Changed) {
				if bindingID, matchesRetiredFocus := retiredByFocus[dispatch.Transaction.FocusID]; matchesRetiredFocus {
					return fmt.Errorf("runtime binding %q retirement is ambiguous after accepted durable execution %q omitted its sealed binding identity", bindingID, execution.ExecutionKey)
				}
				continue
			}
			if retiredBindingID == "" {
				continue
			}
			if RuntimeEffectIsDurable(receipt.ObservedEffectScope) || receipt.Changed {
				return fmt.Errorf("runtime CWI %q binding %q was retired after accepted durable execution %q", retiredCoverageID, retiredBindingID, execution.ExecutionKey)
			}
		}
	}
	for _, evidence := range facts.Evidence.Accepted {
		for _, bindingID := range evidence.BindingRefs {
			if coverageID, found := retired[bindingID]; found {
				return fmt.Errorf("runtime CWI %q binding %q was retired after accepted evidence %q", coverageID, bindingID, evidence.ID)
			}
		}
	}
	return nil
}

func runtimeOperationExecutionForID(values []RuntimeOperationExecutionRecord, operationID string) (RuntimeOperationExecutionRecord, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, value := range values {
		value = value.Normalized()
		if value.OperationID == operationID {
			return value, true
		}
	}
	return RuntimeOperationExecutionRecord{}, false
}

// ValidateRuntimeCWILedger verifies the CWI authority projection contained in
// RuntimeFacts. It inspects persisted aggregate values only: it performs no
// owner I/O, no recovery selection and no transition.
func ValidateRuntimeCWILedger(facts RuntimeFacts) error {
	if strings.TrimSpace(facts.CWILedgerSchemaVersion) != RuntimeCWILedgerSchemaCurrent {
		return fmt.Errorf("runtime CWI ledger requires current schema")
	}
	coverage := make(map[common.CoverageWorkItemID]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, raw := range facts.Coverage {
		item := normalizeRuntimeCoverageItemFact(raw)
		if item.ID == "" {
			return fmt.Errorf("runtime CWI ledger has empty coverage item")
		}
		if _, duplicate := coverage[item.ID]; duplicate {
			return fmt.Errorf("runtime CWI ledger duplicates coverage item %q", item.ID)
		}
		coverage[item.ID] = item
	}
	if len(coverage) == 0 {
		return fmt.Errorf("runtime CWI ledger requires coverage items")
	}

	focusByCoverage := make(map[common.CoverageWorkItemID]CoverageFocus, len(coverage))
	bindingByID := make(map[string]CoverageEvidenceBinding, len(coverage))
	seenFocusIDs := make(map[string]struct{}, 1+len(facts.RemainingFocuses)+len(facts.CompletedFocuses))
	for _, rawFocus := range runtimeCWIFocusQueue(facts) {
		focus := rawFocus.Normalized()
		if focus.ID == "" {
			return fmt.Errorf("runtime CWI ledger has empty focus")
		}
		if len(focus.CoverageWorkItemIDs) != 1 {
			return fmt.Errorf("runtime CWI focus %q must contain exactly one coverage item", focus.ID)
		}
		if _, duplicate := seenFocusIDs[focus.ID]; duplicate {
			return fmt.Errorf("runtime CWI ledger duplicates focus %q", focus.ID)
		}
		seenFocusIDs[focus.ID] = struct{}{}
		if strings.HasPrefix(focus.ID, "semantic-pending/") {
			if len(focus.CoverageWorkItemIDs) != 1 || focus.ID != "semantic-pending/"+string(focus.CoverageWorkItemIDs[0]) {
				return fmt.Errorf("runtime CWI pending focus %q does not use the canonical CWI identity", focus.ID)
			}
			if len(focus.CoverageEvidenceBindings) != 0 {
				return fmt.Errorf("runtime CWI pending focus %q retains a sealed binding", focus.ID)
			}
		}
		for _, id := range focus.CoverageWorkItemIDs {
			if _, found := coverage[id]; !found {
				return fmt.Errorf("runtime CWI focus %q references unknown coverage item %q", focus.ID, id)
			}
			if prior, duplicate := focusByCoverage[id]; duplicate {
				return fmt.Errorf("runtime CWI coverage item %q appears in both focus %q and %q", id, prior.ID, focus.ID)
			}
			focusByCoverage[id] = focus
		}
		for _, binding := range focus.CoverageEvidenceBindings {
			binding = binding.Normalized()
			if _, duplicate := bindingByID[binding.ID]; duplicate {
				return fmt.Errorf("runtime CWI binding %q appears in more than one focus", binding.ID)
			}
			bindingByID[binding.ID] = binding
		}
	}
	if len(focusByCoverage) != len(coverage) {
		return fmt.Errorf("runtime CWI ledger focus projection does not cover every coverage item")
	}

	seenCurrentBindings := make(map[string]common.CoverageWorkItemID, len(coverage))
	for id, item := range coverage {
		focus := focusByCoverage[id]
		if item.FocusID == "" || item.FocusID != focus.ID {
			return fmt.Errorf("runtime CWI coverage item %q focus identity does not match queue projection", id)
		}
		if err := validateRuntimeCWIItem(item, focus, bindingByID, seenCurrentBindings); err != nil {
			return fmt.Errorf("runtime CWI coverage item %q: %w", id, err)
		}
	}

	contract := facts.ReconciliationContract.Normalized()
	if contract.SchemaVersion == "" {
		if len(seenCurrentBindings) != 0 {
			return fmt.Errorf("runtime CWI ledger has current bindings without a reconciliation contract")
		}
		return nil
	}
	if err := contract.Validate(); err != nil {
		return fmt.Errorf("runtime CWI ledger reconciliation contract: %w", err)
	}
	if len(contract.Bindings) != len(seenCurrentBindings) {
		return fmt.Errorf("runtime CWI ledger reconciliation contract does not cover current bindings")
	}
	for _, binding := range contract.Bindings {
		item, found := coverage[binding.CoverageWorkItemID]
		if !found || item.CurrentBindingRef == nil {
			return fmt.Errorf("runtime CWI reconciliation binding %q has no current coverage row", binding.BindingID)
		}
		current := item.CurrentBindingRef.Normalized()
		if current.BindingID != binding.BindingID || current.BindingHash != binding.BindingHash {
			return fmt.Errorf("runtime CWI reconciliation binding %q differs from current coverage binding", binding.BindingID)
		}
	}
	return nil
}

// ValidateRuntimeCWIFocusBindings validates the full proof payload only for
// focus projections that actually carry a sealed binding. The canonical CWI
// ledger separately requires a binding for bound/settled rows and forbids one
// for unbound/planning/reopened rows, so an activation focus can remain
// intentionally unbound until SM-2 seals the semantic contract.
func ValidateRuntimeCWIFocusBindings(facts RuntimeFacts) error {
	facts = facts.Normalized()
	for _, focus := range runtimeCWIFocusQueue(facts) {
		focus = focus.Normalized()
		if len(focus.CoverageEvidenceBindings) == 0 {
			continue
		}
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, facts.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime CWI focus %s: %w", focus.ID, err)
		}
	}
	return nil
}

func validateRuntimeCWIItem(item RuntimeCoverageItemFact, focus CoverageFocus, bindingByID map[string]CoverageEvidenceBinding, seenCurrentBindings map[string]common.CoverageWorkItemID) error {
	item = normalizeRuntimeCoverageItemFact(item)
	switch item.CWIConvergenceState {
	case RuntimeCWIConvergenceStateUnbound, RuntimeCWIConvergenceStatePlanning, RuntimeCWIConvergenceStateBound, RuntimeCWIConvergenceStateSettled, RuntimeCWIConvergenceStateReopened:
	default:
		return fmt.Errorf("has unsupported convergence state %q", item.CWIConvergenceState)
	}
	if len(item.BindingLineage) > RuntimeCWIBindingLineageLimit {
		return fmt.Errorf("binding lineage exceeds bounded limit %d", RuntimeCWIBindingLineageLimit)
	}
	if len(item.ReopenLineage) > RuntimeCWIReopenLineageLimit {
		return fmt.Errorf("reopen lineage exceeds bounded limit %d", RuntimeCWIReopenLineageLimit)
	}
	if len(item.BindingRetirementLineage) > RuntimeCWIBindingRetirementLineageLimit {
		return fmt.Errorf("binding retirement lineage exceeds bounded limit %d", RuntimeCWIBindingRetirementLineageLimit)
	}
	lineage := make(map[string]RuntimeCWIBindingRef, len(item.BindingLineage))
	for _, binding := range item.BindingLineage {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("binding lineage: %w", err)
		}
		if _, duplicate := lineage[binding.BindingID]; duplicate {
			return fmt.Errorf("duplicates binding lineage %q", binding.BindingID)
		}
		lineage[binding.BindingID] = binding
	}
	seenReopenedBindings := make(map[string]struct{}, len(item.ReopenLineage))
	for _, reopen := range item.ReopenLineage {
		if err := reopen.Validate(); err != nil {
			return fmt.Errorf("reopen lineage: %w", err)
		}
		if _, found := lineage[reopen.PreviousBindingRef.BindingID]; !found {
			return fmt.Errorf("reopen lineage references a binding outside binding lineage")
		}
		if _, duplicate := seenReopenedBindings[reopen.PreviousBindingRef.BindingID]; duplicate {
			return fmt.Errorf("duplicates reopen lineage for binding %q", reopen.PreviousBindingRef.BindingID)
		}
		seenReopenedBindings[reopen.PreviousBindingRef.BindingID] = struct{}{}
	}
	seenRetiredBindings := make(map[string]struct{}, len(item.BindingRetirementLineage))
	for _, retirement := range item.BindingRetirementLineage {
		if err := retirement.Validate(); err != nil {
			return fmt.Errorf("binding retirement lineage: %w", err)
		}
		bindingID := retirement.RetiredBindingRef.BindingID
		historical, found := lineage[bindingID]
		if !found || !SameRuntimeCWIBindingRef(historical, retirement.RetiredBindingRef) {
			return fmt.Errorf("binding retirement lineage references a binding outside binding lineage")
		}
		if _, duplicate := seenRetiredBindings[bindingID]; duplicate {
			return fmt.Errorf("duplicates binding retirement lineage for binding %q", bindingID)
		}
		if _, reopened := seenReopenedBindings[bindingID]; reopened {
			return fmt.Errorf("binding %q cannot be both retired and reopened", bindingID)
		}
		seenRetiredBindings[bindingID] = struct{}{}
	}

	if item.CurrentBindingRef != nil {
		current := item.CurrentBindingRef.Normalized()
		if err := current.Validate(); err != nil {
			return err
		}
		if current.FocusID != focus.ID {
			return fmt.Errorf("current binding focus %q differs from queue focus %q", current.FocusID, focus.ID)
		}
		binding, found := bindingByID[current.BindingID]
		if !found || binding.CoverageWorkItemID != item.ID || binding.Hash != current.BindingHash || binding.FocusID != current.FocusID {
			return fmt.Errorf("current binding does not match sealed focus binding")
		}
		if prior, duplicate := seenCurrentBindings[current.BindingID]; duplicate && prior != item.ID {
			return fmt.Errorf("current binding %q is shared by coverage items %q and %q", current.BindingID, prior, item.ID)
		}
		seenCurrentBindings[current.BindingID] = item.ID
		if historical, found := lineage[current.BindingID]; !found || !SameRuntimeCWIBindingRef(historical, current) {
			return fmt.Errorf("current binding is absent from binding lineage")
		}
		if _, retired := seenRetiredBindings[current.BindingID]; retired {
			return fmt.Errorf("current binding is already retired")
		}
	} else if len(item.BindingLineage) != 0 {
		latest := item.BindingLineage[len(item.BindingLineage)-1]
		_, retired := seenRetiredBindings[latest.BindingID]
		_, reopened := seenReopenedBindings[latest.BindingID]
		if !retired && !reopened {
			return fmt.Errorf("cleared binding %q has no retirement or reopen lineage", latest.BindingID)
		}
	}
	if len(item.BindingLineage) > 1 {
		for _, historical := range item.BindingLineage[:len(item.BindingLineage)-1] {
			_, retired := seenRetiredBindings[historical.BindingID]
			_, reopened := seenReopenedBindings[historical.BindingID]
			if !retired && !reopened {
				return fmt.Errorf("successor binding %q has no retirement or reopen lineage for predecessor %q", item.BindingLineage[len(item.BindingLineage)-1].BindingID, historical.BindingID)
			}
		}
	}
	_, focusHasBinding := bindingByIDForCoverage(focus, item.ID)

	switch item.CWIConvergenceState {
	case RuntimeCWIConvergenceStateUnbound, RuntimeCWIConvergenceStatePlanning:
		if item.CurrentBindingRef != nil || focusHasBinding {
			return fmt.Errorf("unbound/planning CWI cannot retain a current binding")
		}
		if item.State == RuntimeCoverageItemStateSatisfied {
			return fmt.Errorf("unbound/planning CWI cannot retain satisfied coverage")
		}
	case RuntimeCWIConvergenceStateBound:
		if item.CurrentBindingRef == nil {
			return fmt.Errorf("bound CWI requires a current binding")
		}
		if item.State == RuntimeCoverageItemStateSatisfied {
			return fmt.Errorf("bound CWI cannot retain satisfied coverage")
		}
	case RuntimeCWIConvergenceStateSettled:
		if item.CurrentBindingRef == nil || item.State != RuntimeCoverageItemStateSatisfied {
			return fmt.Errorf("settled CWI requires current binding and satisfied coverage")
		}
	case RuntimeCWIConvergenceStateReopened:
		if item.CurrentBindingRef != nil || focusHasBinding || len(item.ReopenLineage) == 0 || item.State == RuntimeCoverageItemStateSatisfied {
			return fmt.Errorf("reopened CWI requires prior lineage, no current binding and open coverage")
		}
	}
	return nil
}

func bindingByIDForCoverage(focus CoverageFocus, coverageID common.CoverageWorkItemID) (CoverageEvidenceBinding, bool) {
	for _, binding := range focus.Normalized().CoverageEvidenceBindings {
		if binding.CoverageWorkItemID == coverageID {
			return binding.Clone(), true
		}
	}
	return CoverageEvidenceBinding{}, false
}

func runtimeCWIFocusQueue(facts RuntimeFacts) []CoverageFocus {
	ret := make([]CoverageFocus, 0, 1+len(facts.RemainingFocuses)+len(facts.CompletedFocuses))
	ret = append(ret, facts.Focus)
	ret = append(ret, facts.RemainingFocuses...)
	ret = append(ret, facts.CompletedFocuses...)
	return ret
}

// RuntimeCWIsEligibleForPlanning returns the exact rows SM-2 may present to a
// new semantic generation. It is a pure projection and deliberately excludes
// settled CWI unless an explicit owner-local reopen changed their state. A
// dependent is admitted only after every declared prerequisite has current
// settled credit. Definition order remains authoritative; slice priority and
// lexical CWI identity cannot bypass admission.
func RuntimeCWIsEligibleForPlanning(facts RuntimeFacts) []common.CoverageWorkItemID {
	byID := make(map[common.CoverageWorkItemID]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		item.ID = common.CoverageWorkItemID(strings.TrimSpace(string(item.ID)))
		item.State = RuntimeCoverageItemState(strings.TrimSpace(string(item.State)))
		item = normalizeRuntimeCoverageItemFact(item)
		byID[item.ID] = item
	}
	ret := make([]common.CoverageWorkItemID, 0, len(facts.Coverage))
	for _, item := range facts.Coverage {
		item.ID = common.CoverageWorkItemID(strings.TrimSpace(string(item.ID)))
		item.State = RuntimeCoverageItemState(strings.TrimSpace(string(item.State)))
		item = normalizeRuntimeCoverageItemFact(item)
		switch item.CWIConvergenceState {
		case RuntimeCWIConvergenceStateUnbound, RuntimeCWIConvergenceStatePlanning, RuntimeCWIConvergenceStateReopened:
		default:
			continue
		}
		prerequisitesSettled := true
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			prerequisite, found := byID[prerequisiteID]
			if !found || prerequisite.CWIConvergenceState != RuntimeCWIConvergenceStateSettled ||
				prerequisite.State != RuntimeCoverageItemStateSatisfied || prerequisite.CurrentBindingRef == nil {
				prerequisitesSettled = false
				break
			}
		}
		if prerequisitesSettled {
			ret = append(ret, item.ID)
		}
	}
	order := make(map[common.CoverageWorkItemID]int, len(facts.DefinitionSeed.CoverageItems))
	for index, item := range facts.DefinitionSeed.CoverageItems {
		value := item.Order
		if value <= 0 {
			value = index + 1
		}
		order[item.ID] = value
	}
	sort.SliceStable(ret, func(i, j int) bool {
		left, leftFound := order[ret[i]]
		right, rightFound := order[ret[j]]
		if leftFound != rightFound {
			return leftFound
		}
		if leftFound && left != right {
			return left < right
		}
		return string(ret[i]) < string(ret[j])
	})
	return ret
}

// ValidateRuntimeCWIReconciliationLedger binds the active SM-4 request to
// the current SM-2 contract. Historical generations are intentionally not
// compared with the current binding set: they are immutable audit evidence of
// a superseded generation.
func ValidateRuntimeCWIReconciliationLedger(facts RuntimeFacts, ledger RuntimeCWIReconciliationLedger) error {
	if strings.TrimSpace(facts.CWILedgerSchemaVersion) != RuntimeCWILedgerSchemaCurrent {
		return fmt.Errorf("runtime CWI reconciliation requires current CWI ledger schema")
	}
	if err := ledger.Validate(); err != nil {
		return err
	}
	facts = facts.Normalized()
	for _, historical := range ledger.History {
		historical = historical.Normalized()
		if historical.Request.WorkspaceID != facts.WorkspaceID || historical.Request.RunID != facts.RunID {
			return fmt.Errorf("runtime CWI reconciliation history scope does not match runtime facts")
		}
	}
	if ledger.Request == nil {
		return nil
	}
	if ledger.Request.WorkspaceID != facts.WorkspaceID || ledger.Request.RunID != facts.RunID {
		return fmt.Errorf("runtime CWI reconciliation active request scope does not match runtime facts")
	}
	if facts.ReconciliationContract.SchemaVersion == "" {
		return fmt.Errorf("runtime CWI reconciliation has an active request without a current contract")
	}
	if err := ledger.Request.ValidateAgainstContract(facts.ReconciliationContract); err != nil {
		return fmt.Errorf("runtime CWI reconciliation active request: %w", err)
	}
	return nil
}
