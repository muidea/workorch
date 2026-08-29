package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type RuntimeGovernanceResolutionOutcome string

const (
	RuntimeGovernanceResolutionOutcomeContinue RuntimeGovernanceResolutionOutcome = "continue"
	RuntimeGovernanceResolutionOutcomeFail     RuntimeGovernanceResolutionOutcome = "fail"
)

// RuntimeGovernanceResolution 是交回 Runtime authority owner 的具体治理结果。
type RuntimeGovernanceResolution struct {
	ResolutionID        common.ResolutionID
	TicketID            common.TicketID
	ApprovalID          common.ApprovalID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	DispatchID          string
	DependencyKind      string
	SubjectRef          string
	Outcome             RuntimeGovernanceResolutionOutcome
	Summary             string
	EvidenceRefs        []string
}

func (r RuntimeGovernanceResolution) Normalized() RuntimeGovernanceResolution {
	r.ResolutionID = common.ResolutionID(strings.TrimSpace(string(r.ResolutionID)))
	r.TicketID = common.TicketID(strings.TrimSpace(string(r.TicketID)))
	r.ApprovalID = common.ApprovalID(strings.TrimSpace(string(r.ApprovalID)))
	r.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(r.WorkspaceID)))
	r.RunID = common.RunID(strings.TrimSpace(string(r.RunID)))
	r.AuthoritySnapshotID = strings.TrimSpace(r.AuthoritySnapshotID)
	r.DispatchID = strings.TrimSpace(r.DispatchID)
	r.DependencyKind = strings.TrimSpace(r.DependencyKind)
	r.SubjectRef = strings.TrimSpace(r.SubjectRef)
	r.Outcome = RuntimeGovernanceResolutionOutcome(strings.TrimSpace(string(r.Outcome)))
	r.Summary = strings.TrimSpace(r.Summary)
	r.EvidenceRefs = normalizeStrings(r.EvidenceRefs)
	return r
}

func (r RuntimeGovernanceResolution) Validate() error {
	r = r.Normalized()
	if r.ResolutionID == "" || !validRuntimeGovernanceOwnerIdentity(r.TicketID, r.ApprovalID) || r.WorkspaceID == "" || r.RunID == "" || r.AuthoritySnapshotID == "" || r.DispatchID == "" {
		return fmt.Errorf("runtime governance resolution requires identity, scope, snapshot and dispatch")
	}
	if r.DependencyKind == "" || r.Summary == "" {
		return fmt.Errorf("runtime governance resolution requires dependency kind and summary")
	}
	switch r.Outcome {
	case RuntimeGovernanceResolutionOutcomeContinue, RuntimeGovernanceResolutionOutcomeFail:
		return nil
	default:
		return fmt.Errorf("runtime governance resolution has invalid outcome %q", r.Outcome)
	}
}

func (r RuntimeGovernanceResolution) EventID() string {
	r = r.Normalized()
	return "governance-resolution:" + string(r.ResolutionID) + ":" + r.AuthoritySnapshotID
}

// StableRuntimeGovernanceResolutionHash seals the complete normalized owner
// resolution. The hash is persisted independently by the resolution ledger,
// the operation grant and the accepted governance evidence so recovery can
// prove that all three projections came from exactly the same owner result.
func StableRuntimeGovernanceResolutionHash(resolution RuntimeGovernanceResolution) string {
	resolution = resolution.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-governance-resolution/v1")
	writeHashString(h, string(resolution.ResolutionID))
	writeHashString(h, string(resolution.TicketID))
	writeHashString(h, string(resolution.ApprovalID))
	writeHashString(h, string(resolution.WorkspaceID))
	writeHashString(h, string(resolution.RunID))
	writeHashString(h, resolution.AuthoritySnapshotID)
	writeHashString(h, resolution.DispatchID)
	writeHashString(h, resolution.DependencyKind)
	writeHashString(h, resolution.SubjectRef)
	writeHashString(h, string(resolution.Outcome))
	writeHashString(h, resolution.Summary)
	writeHashStrings(h, resolution.EvidenceRefs)
	return hex.EncodeToString(h.Sum(nil))
}

type RuntimeGovernanceResolutionRecord struct {
	ResolutionID        common.ResolutionID
	TicketID            common.TicketID
	ApprovalID          common.ApprovalID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	DispatchID          string
	DependencyKind      string
	SubjectRef          string
	Outcome             RuntimeGovernanceResolutionOutcome
	Summary             string
	EvidenceRefs        []string
	ResolutionHash      string
}

func RuntimeGovernanceResolutionRecordFromResolution(resolution RuntimeGovernanceResolution) RuntimeGovernanceResolutionRecord {
	resolution = resolution.Normalized()
	return RuntimeGovernanceResolutionRecord{
		ResolutionID:        resolution.ResolutionID,
		TicketID:            resolution.TicketID,
		ApprovalID:          resolution.ApprovalID,
		WorkspaceID:         resolution.WorkspaceID,
		RunID:               resolution.RunID,
		AuthoritySnapshotID: resolution.AuthoritySnapshotID,
		DispatchID:          resolution.DispatchID,
		DependencyKind:      resolution.DependencyKind,
		SubjectRef:          resolution.SubjectRef,
		Outcome:             resolution.Outcome,
		Summary:             resolution.Summary,
		EvidenceRefs:        append([]string(nil), resolution.EvidenceRefs...),
		ResolutionHash:      StableRuntimeGovernanceResolutionHash(resolution),
	}
}

func (r RuntimeGovernanceResolutionRecord) Resolution() RuntimeGovernanceResolution {
	return RuntimeGovernanceResolution{
		ResolutionID:        r.ResolutionID,
		TicketID:            r.TicketID,
		ApprovalID:          r.ApprovalID,
		WorkspaceID:         r.WorkspaceID,
		RunID:               r.RunID,
		AuthoritySnapshotID: r.AuthoritySnapshotID,
		DispatchID:          r.DispatchID,
		DependencyKind:      r.DependencyKind,
		SubjectRef:          r.SubjectRef,
		Outcome:             r.Outcome,
		Summary:             r.Summary,
		EvidenceRefs:        append([]string(nil), r.EvidenceRefs...),
	}.Normalized()
}

func (r RuntimeGovernanceResolutionRecord) Normalized() RuntimeGovernanceResolutionRecord {
	resolutionHash := strings.TrimSpace(r.ResolutionHash)
	ret := RuntimeGovernanceResolutionRecordFromResolution(r.Resolution())
	ret.ResolutionHash = resolutionHash
	return ret
}

// Clone deliberately preserves the raw persisted representation. Validation,
// not cloning, rejects malformed or non-normalized recovery state.
func (r RuntimeGovernanceResolutionRecord) Clone() RuntimeGovernanceResolutionRecord {
	ret := r
	ret.EvidenceRefs = append([]string(nil), r.EvidenceRefs...)
	return ret
}

func CloneRuntimeGovernanceResolutionRecords(values []RuntimeGovernanceResolutionRecord) []RuntimeGovernanceResolutionRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeGovernanceResolutionRecord, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func SameRuntimeGovernanceResolutionRecord(left, right RuntimeGovernanceResolutionRecord) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ResolutionID == right.ResolutionID && left.TicketID == right.TicketID && left.ApprovalID == right.ApprovalID &&
		left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID && left.AuthoritySnapshotID == right.AuthoritySnapshotID &&
		left.DispatchID == right.DispatchID && left.DependencyKind == right.DependencyKind && left.SubjectRef == right.SubjectRef &&
		left.Outcome == right.Outcome && left.Summary == right.Summary && left.ResolutionHash == right.ResolutionHash &&
		sameStrings(left.EvidenceRefs, right.EvidenceRefs)
}

type RuntimeGovernanceRequestRecord struct {
	TicketID            common.TicketID
	ApprovalID          common.ApprovalID
	AuthoritySnapshotID string
	DispatchID          string
	DependencyKind      string
	SubjectRef          string
	RequestedAction     string
	RiskLevel           string
	AuthorizationHash   string
}

func (r RuntimeGovernanceRequestRecord) Normalized() RuntimeGovernanceRequestRecord {
	r.TicketID = common.TicketID(strings.TrimSpace(string(r.TicketID)))
	r.ApprovalID = common.ApprovalID(strings.TrimSpace(string(r.ApprovalID)))
	r.AuthoritySnapshotID = strings.TrimSpace(r.AuthoritySnapshotID)
	r.DispatchID = strings.TrimSpace(r.DispatchID)
	r.DependencyKind = strings.TrimSpace(r.DependencyKind)
	r.SubjectRef = strings.TrimSpace(r.SubjectRef)
	r.RequestedAction = strings.TrimSpace(r.RequestedAction)
	r.RiskLevel = strings.TrimSpace(r.RiskLevel)
	r.AuthorizationHash = strings.TrimSpace(r.AuthorizationHash)
	return r
}

func (r RuntimeGovernanceRequestRecord) Clone() RuntimeGovernanceRequestRecord {
	return r.Normalized()
}

// RuntimeOperationGovernanceBinding preserves the identity of an operation that
// was suspended before any side effect because its owner required governance. It
// is deliberately separate from the governance dispatch: the former owns the
// ExecutionKey/fence, while the latter owns the Decision/Approval ticket.
type RuntimeOperationGovernanceBindingState string

const (
	RuntimeOperationGovernanceBindingSuspended   RuntimeOperationGovernanceBindingState = "suspended"
	RuntimeOperationGovernanceBindingTicketBound RuntimeOperationGovernanceBindingState = "ticket_bound"
	RuntimeOperationGovernanceBindingGranted     RuntimeOperationGovernanceBindingState = "granted"
	RuntimeOperationGovernanceBindingRejected    RuntimeOperationGovernanceBindingState = "rejected"
)

type RuntimeOperationGovernanceBinding struct {
	TicketID               common.TicketID
	ApprovalID             common.ApprovalID
	GovernanceDispatchID   string
	GovernanceSnapshotID   string
	OperationDispatchID    string
	OperationID            string
	ExecutionKey           string
	Fence                  uint64
	ExpectedEffectScope    RuntimeEffectScope
	IdempotencyClass       RuntimeOperationIdempotencyClass
	ExecutionAuthorityHash string
	DependencyKind         string
	SubjectRef             string
	RequestedAction        string
	RiskLevel              string
	AuthorizationHash      string
	GovernanceReceiptHash  string
	State                  RuntimeOperationGovernanceBindingState
	Grant                  *RuntimeOperationGovernanceGrant
}

type RuntimeOperationGovernanceGrant struct {
	ResolutionID    common.ResolutionID
	ResolutionHash  string
	TicketID        common.TicketID
	ApprovalID      common.ApprovalID
	ExecutionKey    string
	GovernanceFence uint64
	ResumeFence     uint64
	// ResumeDispatchID binds this grant to the one SM-3 continuation
	// transaction created in the same Orchestrator CAS.
	ResumeDispatchID      string
	DependencyKind        string
	SubjectRef            string
	RequestedAction       string
	RiskLevel             string
	AuthorizationHash     string
	GovernanceReceiptHash string
	EvidenceRefs          []string
}

func (g RuntimeOperationGovernanceGrant) Normalized() RuntimeOperationGovernanceGrant {
	g.ResolutionHash = strings.TrimSpace(g.ResolutionHash)
	g.ExecutionKey = strings.TrimSpace(g.ExecutionKey)
	g.ResumeDispatchID = strings.TrimSpace(g.ResumeDispatchID)
	g.DependencyKind = strings.TrimSpace(g.DependencyKind)
	g.SubjectRef = strings.TrimSpace(g.SubjectRef)
	g.RequestedAction = strings.TrimSpace(g.RequestedAction)
	g.RiskLevel = strings.TrimSpace(g.RiskLevel)
	g.AuthorizationHash = strings.TrimSpace(g.AuthorizationHash)
	g.GovernanceReceiptHash = strings.TrimSpace(g.GovernanceReceiptHash)
	g.EvidenceRefs = normalizeStrings(g.EvidenceRefs)
	return g
}

func (g RuntimeOperationGovernanceGrant) Validate() error {
	raw := g
	g = g.Normalized()
	if !reflect.DeepEqual(raw, g) {
		return fmt.Errorf("runtime operation governance grant requires canonical fields")
	}
	if g.ResolutionID == "" || !validRuntimeGovernanceOwnerIdentity(g.TicketID, g.ApprovalID) || g.ExecutionKey == "" || g.GovernanceFence == 0 || g.ResumeFence == 0 || g.ResumeDispatchID == "" ||
		g.DependencyKind == "" || g.SubjectRef == "" || g.GovernanceReceiptHash == "" {
		return fmt.Errorf("runtime operation governance grant requires identity, execution, dependency and receipt hash")
	}
	if len(g.ResolutionHash) != sha256.Size*2 {
		return fmt.Errorf("runtime operation governance grant requires sha256 resolution hash")
	}
	if _, err := hex.DecodeString(g.ResolutionHash); err != nil {
		return fmt.Errorf("runtime operation governance grant resolution hash: %w", err)
	}
	if g.ResumeFence != g.GovernanceFence+1 {
		return fmt.Errorf("runtime operation governance grant resume fence must follow governance fence")
	}
	if g.DependencyKind == string(GovernanceApproval) && (g.ApprovalID == "" || g.RequestedAction == "" || g.AuthorizationHash == "") {
		return fmt.Errorf("runtime approval governance grant requires approval identity, requested action and authorization hash")
	}
	return nil
}

func SameRuntimeOperationGovernanceGrant(left RuntimeOperationGovernanceGrant, right RuntimeOperationGovernanceGrant) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.ResolutionID != right.ResolutionID || left.ResolutionHash != right.ResolutionHash || left.TicketID != right.TicketID || left.ApprovalID != right.ApprovalID || left.ExecutionKey != right.ExecutionKey ||
		left.GovernanceFence != right.GovernanceFence || left.ResumeFence != right.ResumeFence || left.ResumeDispatchID != right.ResumeDispatchID || left.DependencyKind != right.DependencyKind ||
		left.SubjectRef != right.SubjectRef || left.RequestedAction != right.RequestedAction || left.RiskLevel != right.RiskLevel ||
		left.AuthorizationHash != right.AuthorizationHash || left.GovernanceReceiptHash != right.GovernanceReceiptHash || len(left.EvidenceRefs) != len(right.EvidenceRefs) {
		return false
	}
	for idx := range left.EvidenceRefs {
		if left.EvidenceRefs[idx] != right.EvidenceRefs[idx] {
			return false
		}
	}
	return true
}

func (b RuntimeOperationGovernanceBinding) Normalized() RuntimeOperationGovernanceBinding {
	b.GovernanceDispatchID = strings.TrimSpace(b.GovernanceDispatchID)
	b.GovernanceSnapshotID = strings.TrimSpace(b.GovernanceSnapshotID)
	b.OperationDispatchID = strings.TrimSpace(b.OperationDispatchID)
	b.OperationID = strings.TrimSpace(b.OperationID)
	b.ExecutionKey = strings.TrimSpace(b.ExecutionKey)
	b.ExpectedEffectScope = NormalizeRuntimeEffectScope(b.ExpectedEffectScope)
	b.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(b.IdempotencyClass)
	b.ExecutionAuthorityHash = strings.TrimSpace(b.ExecutionAuthorityHash)
	b.DependencyKind = strings.TrimSpace(b.DependencyKind)
	b.SubjectRef = strings.TrimSpace(b.SubjectRef)
	b.RequestedAction = strings.TrimSpace(b.RequestedAction)
	b.RiskLevel = strings.TrimSpace(b.RiskLevel)
	b.AuthorizationHash = strings.TrimSpace(b.AuthorizationHash)
	b.GovernanceReceiptHash = strings.TrimSpace(b.GovernanceReceiptHash)
	b.State = RuntimeOperationGovernanceBindingState(strings.TrimSpace(string(b.State)))
	if b.Grant != nil {
		grant := b.Grant.Normalized()
		b.Grant = &grant
	}
	return b
}

func (b RuntimeOperationGovernanceBinding) Clone() RuntimeOperationGovernanceBinding {
	ret := b
	if b.Grant != nil {
		grant := *b.Grant
		grant.EvidenceRefs = append([]string(nil), b.Grant.EvidenceRefs...)
		ret.Grant = &grant
	}
	return ret
}

func ValidateRuntimeOperationGovernanceBindings(values []RuntimeOperationGovernanceBinding) error {
	seenExecution := map[string]struct{}{}
	seenOwnerIdentity := map[string]struct{}{}
	for _, raw := range values {
		value := raw.Normalized()
		if !reflect.DeepEqual(raw, value) {
			return fmt.Errorf("operation governance binding requires canonical fields")
		}
		if value.OperationDispatchID == "" || value.OperationID == "" || value.ExecutionKey == "" || value.Fence == 0 ||
			!validSealedRuntimeEffectScope(raw.ExpectedEffectScope) || !validSealedRuntimeOperationIdempotencyClass(raw.IdempotencyClass) ||
			value.ExecutionAuthorityHash == "" || value.DependencyKind == "" || value.SubjectRef == "" || value.GovernanceReceiptHash == "" {
			return fmt.Errorf("operation governance binding requires source operation execution identity and dependency")
		}
		if value.DependencyKind == string(GovernanceApproval) && (value.RequestedAction == "" || value.AuthorizationHash == "") {
			return fmt.Errorf("operation approval governance binding requires requested action and authorization hash")
		}
		executionIdentity := value.ExecutionKey + "|" + fmt.Sprintf("%d", value.Fence)
		if _, duplicate := seenExecution[executionIdentity]; duplicate {
			return fmt.Errorf("operation governance binding execution identity %q is duplicated", executionIdentity)
		}
		seenExecution[executionIdentity] = struct{}{}
		switch value.State {
		case RuntimeOperationGovernanceBindingSuspended:
			if value.TicketID != "" || value.ApprovalID != "" || value.GovernanceDispatchID != "" || value.GovernanceSnapshotID != "" || value.Grant != nil {
				return fmt.Errorf("suspended operation governance binding cannot have owner request or governance dispatch")
			}
		case RuntimeOperationGovernanceBindingTicketBound, RuntimeOperationGovernanceBindingRejected:
			if !validRuntimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID) || value.GovernanceDispatchID == "" || value.GovernanceSnapshotID == "" {
				return fmt.Errorf("operation governance binding state %q requires one owner request and governance dispatch", value.State)
			}
			if value.Grant != nil {
				return fmt.Errorf("operation governance binding state %q cannot carry grant", value.State)
			}
			ownerIdentity := runtimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID)
			if _, duplicate := seenOwnerIdentity[ownerIdentity]; duplicate {
				return fmt.Errorf("operation governance binding owner request %q is duplicated", ownerIdentity)
			}
			seenOwnerIdentity[ownerIdentity] = struct{}{}
		case RuntimeOperationGovernanceBindingGranted:
			if !validRuntimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID) || value.GovernanceDispatchID == "" || value.GovernanceSnapshotID == "" || value.Grant == nil {
				return fmt.Errorf("granted operation governance binding requires owner request, governance dispatch and grant")
			}
			if err := value.Grant.Validate(); err != nil {
				return err
			}
			grant := value.Grant.Normalized()
			if grant.TicketID != value.TicketID || grant.ApprovalID != value.ApprovalID || grant.ExecutionKey != value.ExecutionKey || grant.GovernanceFence != value.Fence ||
				grant.DependencyKind != value.DependencyKind || grant.SubjectRef != value.SubjectRef || grant.RequestedAction != value.RequestedAction ||
				grant.RiskLevel != value.RiskLevel || grant.AuthorizationHash != value.AuthorizationHash || grant.GovernanceReceiptHash != value.GovernanceReceiptHash {
				return fmt.Errorf("operation governance grant does not match binding")
			}
			ownerIdentity := runtimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID)
			if _, duplicate := seenOwnerIdentity[ownerIdentity]; duplicate {
				return fmt.Errorf("operation governance binding owner request %q is duplicated", ownerIdentity)
			}
			seenOwnerIdentity[ownerIdentity] = struct{}{}
		default:
			return fmt.Errorf("operation governance binding has invalid state %q", value.State)
		}
	}
	return nil
}

// ValidateRuntimeOperationGovernanceExecutionBindings proves the bidirectional
// lineage between every granted governance binding and the one execution at its
// resume fence. Callers pass executions from both active and completed
// dispatches so a completed continuation remains valid while duplicates and
// orphan governance markers fail closed.
func ValidateRuntimeOperationGovernanceExecutionBindings(bindings []RuntimeOperationGovernanceBinding, executions []RuntimeOperationExecutionRecord) error {
	byIdentity := make(map[string]RuntimeOperationExecutionRecord, len(executions))
	for _, rawExecution := range executions {
		execution := rawExecution.Normalized()
		identity := fmt.Sprintf("%s|%s|%d", execution.OperationID, execution.ExecutionKey, execution.Fence)
		if previous, found := byIdentity[identity]; found {
			if !sameRuntimeOperationExecutionRecord(previous, execution) {
				return fmt.Errorf("operation governance execution identity %q has inconsistent dispatch projections", identity)
			}
			continue
		}
		byIdentity[identity] = execution
	}
	for _, rawBinding := range bindings {
		binding := rawBinding.Normalized()
		if binding.State != RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
			continue
		}
		grant := binding.Grant.Normalized()
		identity := fmt.Sprintf("%s|%s|%d", binding.OperationID, binding.ExecutionKey, grant.ResumeFence)
		execution, found := byIdentity[identity]
		if !found {
			return fmt.Errorf("operation governance grant has no resume execution")
		}
		if execution.ExpectedEffectScope != binding.ExpectedEffectScope || execution.IdempotencyClass != binding.IdempotencyClass ||
			execution.AuthorityHash != binding.ExecutionAuthorityHash || !RuntimeOperationClaimMarkerIsGovernance(execution.ClaimedAt) {
			return fmt.Errorf("operation governance resume execution does not match granted authority")
		}
	}
	for _, execution := range byIdentity {
		if !RuntimeOperationClaimMarkerIsGovernance(execution.ClaimedAt) {
			continue
		}
		matched := 0
		for _, rawBinding := range bindings {
			binding := rawBinding.Normalized()
			if binding.State != RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
				continue
			}
			grant := binding.Grant.Normalized()
			if binding.OperationID == execution.OperationID && binding.ExecutionKey == execution.ExecutionKey && grant.ResumeFence == execution.Fence &&
				binding.ExpectedEffectScope == execution.ExpectedEffectScope && binding.IdempotencyClass == execution.IdempotencyClass &&
				binding.ExecutionAuthorityHash == execution.AuthorityHash {
				matched++
			}
		}
		if matched != 1 {
			return fmt.Errorf("operation governance claim marker requires exactly one granted binding, got %d", matched)
		}
	}
	return nil
}

// ValidateRuntimeDispatchExecutionKeyLineage proves that every execution key
// was minted by its current dispatch or was carried forward by one exact,
// persisted governance-continuation lineage. Unlike the flattened governance
// validator above, this check deliberately retains dispatch identity and
// history order.
type runtimeDispatchHistoryEntry struct {
	dispatch  RuntimeDispatchRecord
	completed bool
}

func ValidateRuntimeDispatchExecutionKeyLineage(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	bindings []RuntimeOperationGovernanceBinding,
) error {
	history := make([]runtimeDispatchHistoryEntry, 0, len(completed)+1)
	positions := make(map[string]int, len(completed)+1)
	for idx, dispatch := range completed {
		history = append(history, runtimeDispatchHistoryEntry{dispatch: dispatch, completed: true})
		positions[dispatch.ID] = idx
	}
	if active != nil {
		positions[active.ID] = len(history)
		history = append(history, runtimeDispatchHistoryEntry{dispatch: *active})
	}
	matchedResumeBindings := make(map[int]int, len(bindings))
	for dispatchIndex, entry := range history {
		dispatch := entry.dispatch
		resumeBindingIndexes := make([]int, 0, 1)
		for bindingIndex, rawBinding := range bindings {
			if runtimeGovernanceBindingResumesDispatch(rawBinding, dispatch, history, positions, dispatchIndex) {
				resumeBindingIndexes = append(resumeBindingIndexes, bindingIndex)
				matchedResumeBindings[bindingIndex]++
			}
		}
		if len(resumeBindingIndexes) > 1 {
			return fmt.Errorf("runtime dispatch %q matches multiple governance continuation grants", dispatch.ID)
		}
		for _, rawExecution := range dispatch.OperationExecutions {
			execution := rawExecution.Normalized()
			originID, err := RuntimeOperationExecutionKeyOrigin(workspaceID, runID, execution.ExecutionKey, execution.OperationID)
			if err != nil {
				return fmt.Errorf("runtime dispatch %q operation %q execution key: %w", dispatch.ID, execution.OperationID, err)
			}
			if originID == dispatch.ID {
				if len(resumeBindingIndexes) == 1 {
					binding := bindings[resumeBindingIndexes[0]].Normalized()
					if execution.OperationID == binding.OperationID || !runtimeGovernancePendingSiblingRekeyMatches(binding, dispatch, execution, history, positions) {
						return fmt.Errorf("runtime dispatch %q operation %q governance pending-sibling authority mismatch", dispatch.ID, execution.OperationID)
					}
				}
				continue
			}
			originIndex, found := positions[originID]
			if !found || originIndex >= dispatchIndex || !history[originIndex].completed {
				return fmt.Errorf("runtime dispatch %q operation %q has no earlier canonical execution-key origin", dispatch.ID, execution.OperationID)
			}
			origin := history[originIndex].dispatch
			originExecution, found := runtimeDispatchExecutionForOperation(origin, execution.OperationID)
			if !found || originExecution.ExecutionKey != StableRuntimeOperationExecutionKey(workspaceID, runID, origin.ID, execution.OperationID) ||
				originExecution.OperationHash != execution.OperationHash || originExecution.AuthorityHash != execution.AuthorityHash ||
				origin.CandidateHash != dispatch.CandidateHash || StableRuntimeValidationRecipeSnapshotHash(origin.ValidationRecipes) != StableRuntimeValidationRecipeSnapshotHash(dispatch.ValidationRecipes) {
				return fmt.Errorf("runtime dispatch %q operation %q execution-key origin authority mismatch", dispatch.ID, execution.OperationID)
			}
			if len(resumeBindingIndexes) != 1 {
				return fmt.Errorf("runtime dispatch %q operation %q non-current execution key has no exact governance continuation dispatch", dispatch.ID, execution.OperationID)
			}
			binding := bindings[resumeBindingIndexes[0]].Normalized()
			grant := binding.Grant.Normalized()
			if binding.OperationID == execution.OperationID && binding.ExecutionKey == execution.ExecutionKey &&
				grant.ResumeFence == execution.Fence && execution.ExpectedEffectScope == binding.ExpectedEffectScope &&
				execution.IdempotencyClass == binding.IdempotencyClass && execution.AuthorityHash == binding.ExecutionAuthorityHash &&
				RuntimeOperationClaimMarkerIsGovernance(execution.ClaimedAt) {
				continue
			}
			// Once the destination itself is proven to be this exact continuation,
			// an already-completed sibling may be projected unchanged.
			if runtimeGovernanceCompletedSiblingCloneMatches(binding, dispatch, execution, history, positions) {
				continue
			}
			return fmt.Errorf("runtime dispatch %q operation %q is neither the governance target nor an exact completed sibling clone", dispatch.ID, execution.OperationID)
		}
	}
	for bindingIndex, rawBinding := range bindings {
		binding := rawBinding.Normalized()
		if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil && matchedResumeBindings[bindingIndex] != 1 {
			return fmt.Errorf("operation governance grant requires exactly one resume dispatch, got %d", matchedResumeBindings[bindingIndex])
		}
	}
	return nil
}

func runtimeGovernancePendingSiblingRekeyMatches(binding RuntimeOperationGovernanceBinding, dispatch RuntimeDispatchRecord, execution RuntimeOperationExecutionRecord, history []runtimeDispatchHistoryEntry, positions map[string]int) bool {
	sourceIndex, found := positions[binding.OperationDispatchID]
	if !found || sourceIndex < 0 || sourceIndex >= len(history) || !history[sourceIndex].completed || history[sourceIndex].dispatch.State != RuntimeDispatchStateCommitted {
		return false
	}
	sourceExecution, found := runtimeDispatchExecutionForOperation(history[sourceIndex].dispatch, execution.OperationID)
	if !found || sourceExecution.State != OperationExecutionSealed {
		return false
	}
	expected := sourceExecution.Clone()
	expected.ExecutionKey = StableRuntimeOperationExecutionKey(dispatch.WorkspaceID, dispatch.RunID, dispatch.ID, execution.OperationID)
	expected.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(expected)
	return sameRuntimeOperationExecutionImmutableAuthority(expected, execution)
}

func runtimeGovernanceBindingResumesDispatch(binding RuntimeOperationGovernanceBinding, dispatch RuntimeDispatchRecord, history []runtimeDispatchHistoryEntry, positions map[string]int, dispatchIndex int) bool {
	binding = binding.Normalized()
	if binding.State != RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
		return false
	}
	grant := binding.Grant.Normalized()
	if grant.ResumeDispatchID != dispatch.ID {
		return false
	}
	sourceIndex, found := positions[binding.OperationDispatchID]
	if !found || sourceIndex >= dispatchIndex || !history[sourceIndex].completed || history[sourceIndex].dispatch.State != RuntimeDispatchStateCommitted {
		return false
	}
	governanceIndex, found := positions[binding.GovernanceDispatchID]
	if !found || governanceIndex <= sourceIndex || governanceIndex >= dispatchIndex || !history[governanceIndex].completed {
		return false
	}
	governanceDispatch := history[governanceIndex].dispatch
	if governanceDispatch.ID != binding.GovernanceDispatchID || governanceDispatch.State != RuntimeDispatchStateCommitted ||
		governanceDispatch.Candidate.Governance == nil || governanceDispatch.Transaction.AuthoritySnapshotID != binding.GovernanceSnapshotID {
		return false
	}
	source := history[sourceIndex].dispatch
	if source.CandidateHash != dispatch.CandidateHash || StableRuntimeValidationRecipeSnapshotHash(source.ValidationRecipes) != StableRuntimeValidationRecipeSnapshotHash(dispatch.ValidationRecipes) {
		return false
	}
	sourceExecution, found := runtimeDispatchExecutionForOperation(source, binding.OperationID)
	if !found || sourceExecution.ExecutionKey != binding.ExecutionKey || sourceExecution.Fence != binding.Fence ||
		sourceExecution.ExpectedEffectScope != binding.ExpectedEffectScope || sourceExecution.IdempotencyClass != binding.IdempotencyClass ||
		sourceExecution.AuthorityHash != binding.ExecutionAuthorityHash || sourceExecution.OwnerReceiptHash != binding.GovernanceReceiptHash ||
		sourceExecution.State != OperationExecutionOwnerCommitted {
		return false
	}
	resumeExecution, found := runtimeDispatchExecutionForOperation(dispatch, binding.OperationID)
	return found && grant.ExecutionKey == resumeExecution.ExecutionKey && grant.ResumeFence == resumeExecution.Fence &&
		resumeExecution.ExpectedEffectScope == binding.ExpectedEffectScope && resumeExecution.IdempotencyClass == binding.IdempotencyClass &&
		resumeExecution.AuthorityHash == binding.ExecutionAuthorityHash && RuntimeOperationClaimMarkerIsGovernance(resumeExecution.ClaimedAt)
}

func runtimeDispatchExecutionForOperation(dispatch RuntimeDispatchRecord, operationID string) (RuntimeOperationExecutionRecord, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, execution := range dispatch.OperationExecutions {
		value := execution.Normalized()
		if value.OperationID == operationID {
			return value, true
		}
	}
	return RuntimeOperationExecutionRecord{}, false
}

func runtimeGovernanceCompletedSiblingCloneMatches(binding RuntimeOperationGovernanceBinding, current RuntimeDispatchRecord, execution RuntimeOperationExecutionRecord, history []runtimeDispatchHistoryEntry, positions map[string]int) bool {
	sourceIndex, found := positions[binding.OperationDispatchID]
	if !found || sourceIndex < 0 || sourceIndex >= len(history) || !history[sourceIndex].completed || history[sourceIndex].dispatch.State != RuntimeDispatchStateCommitted {
		return false
	}
	source := history[sourceIndex].dispatch
	if source.CandidateHash != current.CandidateHash ||
		StableRuntimeValidationRecipeSnapshotHash(source.ValidationRecipes) != StableRuntimeValidationRecipeSnapshotHash(current.ValidationRecipes) ||
		!containsString(source.CompletedOperationIDs, execution.OperationID) {
		return false
	}
	previous, found := runtimeDispatchExecutionForOperation(source, execution.OperationID)
	return found && sameRuntimeOperationExecutionRecord(previous, execution)
}

// ValidateRuntimeOperationGovernanceGrantLineage binds physical execution
// authority to the exact durable continue resolution and accepted governance
// evidence that created it. A persisted grant cannot invent a resolution ID or
// substitute its evidence references.
func ValidateRuntimeOperationGovernanceGrantLineage(bindings []RuntimeOperationGovernanceBinding, resolutions []RuntimeGovernanceResolutionRecord, accepted, rejected []EvidenceFact) error {
	for _, rawBinding := range bindings {
		binding := rawBinding.Normalized()
		if binding.State != RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
			continue
		}
		grant := binding.Grant.Normalized()
		resolutionMatches := 0
		var matchedResolution RuntimeGovernanceResolutionRecord
		for _, rawResolution := range resolutions {
			resolution := rawResolution.Normalized()
			if resolution.ResolutionID == grant.ResolutionID && resolution.TicketID == binding.TicketID && resolution.ApprovalID == binding.ApprovalID &&
				resolution.AuthoritySnapshotID == binding.GovernanceSnapshotID && resolution.DispatchID == binding.GovernanceDispatchID &&
				resolution.DependencyKind == binding.DependencyKind && resolution.SubjectRef == binding.SubjectRef &&
				resolution.Outcome == RuntimeGovernanceResolutionOutcomeContinue && resolution.ResolutionHash == grant.ResolutionHash &&
				sameStrings(resolution.EvidenceRefs, grant.EvidenceRefs) {
				resolutionMatches++
				matchedResolution = resolution
			}
		}
		if resolutionMatches != 1 {
			return fmt.Errorf("operation governance grant requires exactly one continue resolution, got %d", resolutionMatches)
		}
		wantEvidenceID := common.EvidenceRecordID("governance-resolution:" + string(grant.ResolutionID) + ":" + binding.GovernanceSnapshotID)
		wantSource := "governance-resolution/" + string(grant.ResolutionID)
		evidenceMatches := 0
		for _, rawFact := range accepted {
			fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
			if ok && fact.ID == wantEvidenceID && fact.SourceRef == wantSource &&
				runtimeGovernanceEvidenceMatchesResolution(fact, matchedResolution) {
				evidenceMatches++
			}
		}
		if evidenceMatches != 1 {
			return fmt.Errorf("operation governance grant requires exactly one accepted resolution evidence, got %d", evidenceMatches)
		}
	}
	for _, rawResolution := range resolutions {
		resolution := rawResolution.Normalized()
		if resolution.Outcome == RuntimeGovernanceResolutionOutcomeFail {
			rejectedMatches := 0
			for _, rawFact := range rejected {
				fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
				if ok && runtimeGovernanceRejectedEvidenceMatchesResolution(fact, resolution) {
					rejectedMatches++
				}
			}
			acceptedIdentityMatches := 0
			for _, rawFact := range accepted {
				fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
				if ok && runtimeGovernanceEvidenceHasResolutionIdentity(fact, resolution) {
					acceptedIdentityMatches++
				}
			}
			if rejectedMatches != 1 || acceptedIdentityMatches != 0 {
				return fmt.Errorf("runtime governance fail resolution requires exactly one rejected and no accepted resolution evidence")
			}
			continue
		}
		if resolution.Outcome != RuntimeGovernanceResolutionOutcomeContinue {
			continue
		}
		evidenceMatches := 0
		for _, rawFact := range accepted {
			fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
			if ok && runtimeGovernanceEvidenceMatchesResolution(fact, resolution) {
				evidenceMatches++
			}
		}
		if evidenceMatches != 1 {
			return fmt.Errorf("runtime governance continue resolution requires exactly one accepted resolution evidence, got %d", evidenceMatches)
		}
		for _, rawFact := range rejected {
			fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
			if ok && runtimeGovernanceEvidenceHasResolutionIdentity(fact, resolution) {
				return fmt.Errorf("runtime governance continue resolution cannot have rejected resolution evidence")
			}
		}
		operationBindings := 0
		grantedMatches := 0
		for _, rawBinding := range bindings {
			binding := rawBinding.Normalized()
			if binding.TicketID != resolution.TicketID || binding.ApprovalID != resolution.ApprovalID ||
				binding.GovernanceSnapshotID != strings.TrimSpace(resolution.AuthoritySnapshotID) || binding.GovernanceDispatchID != strings.TrimSpace(resolution.DispatchID) {
				continue
			}
			operationBindings++
			if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil {
				grant := binding.Grant.Normalized()
				if grant.ResolutionID == resolution.ResolutionID && grant.ResolutionHash == resolution.ResolutionHash &&
					grant.DependencyKind == resolution.DependencyKind && grant.SubjectRef == resolution.SubjectRef &&
					sameStrings(grant.EvidenceRefs, resolution.EvidenceRefs) {
					grantedMatches++
				}
			}
		}
		if operationBindings != 0 && (operationBindings != 1 || grantedMatches != 1) {
			return fmt.Errorf("operation governance continue resolution requires exactly one granted binding")
		}
	}
	for _, rawFact := range accepted {
		fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
		if !ok {
			if strings.TrimSpace(rawFact.Kind) == EvidenceKindGovernance {
				return fmt.Errorf("accepted governance evidence is malformed")
			}
			continue
		}
		if fact.Kind != EvidenceKindGovernance {
			continue
		}
		resolutionMatches := 0
		for _, rawResolution := range resolutions {
			resolution := rawResolution.Normalized()
			if resolution.Outcome == RuntimeGovernanceResolutionOutcomeContinue && runtimeGovernanceEvidenceMatchesResolution(fact, resolution) {
				resolutionMatches++
			}
		}
		if resolutionMatches != 1 {
			return fmt.Errorf("accepted governance evidence requires exactly one continue resolution, got %d", resolutionMatches)
		}
	}
	for _, rawFact := range rejected {
		fact, ok := normalizedRuntimeGovernanceEvidenceFact(rawFact)
		if !ok {
			if strings.TrimSpace(rawFact.Kind) == EvidenceKindGovernance {
				return fmt.Errorf("rejected governance evidence is malformed")
			}
			continue
		}
		if fact.Kind != EvidenceKindGovernance {
			continue
		}
		resolutionMatches := 0
		for _, rawResolution := range resolutions {
			resolution := rawResolution.Normalized()
			if resolution.Outcome == RuntimeGovernanceResolutionOutcomeFail && runtimeGovernanceRejectedEvidenceMatchesResolution(fact, resolution) {
				resolutionMatches++
			}
		}
		if resolutionMatches != 1 {
			return fmt.Errorf("rejected governance evidence requires exactly one fail resolution, got %d", resolutionMatches)
		}
	}
	return nil
}

func normalizedRuntimeGovernanceEvidenceFact(raw EvidenceFact) (EvidenceFact, bool) {
	values := normalizeEvidenceFacts([]EvidenceFact{raw})
	if len(values) != 1 || !reflect.DeepEqual(raw, values[0]) {
		return EvidenceFact{}, false
	}
	return values[0], true
}

// RuntimeGovernanceEvidenceFactForResolution is the sole canonical governance
// evidence projection. Revision is supplied by the Orchestrator's global
// evidence ledger; every other EvidenceFact field is determined here so a
// governance result cannot acquire operation or coverage credit by persistence
// mutation.
func RuntimeGovernanceEvidenceFactForResolution(resolution RuntimeGovernanceResolution, revision string) (EvidenceFact, error) {
	resolution = resolution.Normalized()
	revision = strings.TrimSpace(revision)
	if err := resolution.Validate(); err != nil {
		return EvidenceFact{}, err
	}
	if revision == "" {
		return EvidenceFact{}, fmt.Errorf("runtime governance evidence requires revision")
	}
	fact := EvidenceFact{
		ID:                  common.EvidenceRecordID(resolution.EventID()),
		Kind:                EvidenceKindGovernance,
		SubjectRef:          resolution.SubjectRef,
		SourceRef:           "governance-resolution/" + string(resolution.ResolutionID),
		AuthoritySnapshotID: resolution.AuthoritySnapshotID,
		DispatchID:          resolution.DispatchID,
		Revision:            revision,
		Summary:             resolution.Summary,
		ContentHash:         StableRuntimeGovernanceResolutionHash(resolution),
		EvidenceRefs:        append([]string(nil), resolution.EvidenceRefs...),
	}
	if resolution.Outcome == RuntimeGovernanceResolutionOutcomeFail {
		fact.FailureClass = FailureClassGovernanceRejected
		fact.FailureMessage = resolution.Summary
	}
	return fact, nil
}

// RuntimeGovernanceEvidenceFactMatchesResolution compares the complete raw
// canonical fact. In particular OperationID, target/binding/CWI credit,
// validation observations and quality metadata must remain their zero values.
func RuntimeGovernanceEvidenceFactMatchesResolution(fact EvidenceFact, resolution RuntimeGovernanceResolution) bool {
	expected, err := RuntimeGovernanceEvidenceFactForResolution(resolution, fact.Revision)
	return err == nil && reflect.DeepEqual(fact, expected)
}

func runtimeGovernanceEvidenceMatchesResolution(fact EvidenceFact, resolution RuntimeGovernanceResolutionRecord) bool {
	resolution = resolution.Normalized()
	return resolution.Outcome == RuntimeGovernanceResolutionOutcomeContinue &&
		RuntimeGovernanceEvidenceFactMatchesResolution(fact, resolution.Resolution())
}

func runtimeGovernanceRejectedEvidenceMatchesResolution(fact EvidenceFact, resolution RuntimeGovernanceResolutionRecord) bool {
	resolution = resolution.Normalized()
	return resolution.Outcome == RuntimeGovernanceResolutionOutcomeFail &&
		RuntimeGovernanceEvidenceFactMatchesResolution(fact, resolution.Resolution())
}

func runtimeGovernanceEvidenceHasResolutionIdentity(fact EvidenceFact, resolution RuntimeGovernanceResolutionRecord) bool {
	resolution = resolution.Normalized()
	wantEvidenceID := common.EvidenceRecordID("governance-resolution:" + string(resolution.ResolutionID) + ":" + resolution.AuthoritySnapshotID)
	wantSource := "governance-resolution/" + string(resolution.ResolutionID)
	return fact.ID == wantEvidenceID && fact.Kind == EvidenceKindGovernance && fact.SubjectRef == resolution.SubjectRef && fact.SourceRef == wantSource &&
		fact.AuthoritySnapshotID == resolution.AuthoritySnapshotID && fact.DispatchID == resolution.DispatchID
}

func sameRuntimeOperationExecutionRecord(left, right RuntimeOperationExecutionRecord) bool {
	left = left.Normalized()
	right = right.Normalized()
	if !sameRuntimeOperationExecutionImmutableAuthority(left, right) || left.State != right.State || left.Fence != right.Fence || left.OwnerReceiptRef != right.OwnerReceiptRef ||
		left.OwnerReceiptHash != right.OwnerReceiptHash || left.EvidenceFactID != right.EvidenceFactID ||
		left.EvidenceFactHash != right.EvidenceFactHash || left.ClaimedAt != right.ClaimedAt {
		return false
	}
	return true
}

func sameRuntimeOperationExecutionImmutableAuthority(left, right RuntimeOperationExecutionRecord) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.OperationID != right.OperationID || left.OperationHash != right.OperationHash || left.ExecutionKey != right.ExecutionKey || left.ExpectedEffectScope != right.ExpectedEffectScope ||
		left.IdempotencyClass != right.IdempotencyClass || left.AuthorityHash != right.AuthorityHash ||
		left.AdmissibilitySnapshotID != right.AdmissibilitySnapshotID || left.AcquisitionBinding != right.AcquisitionBinding {
		return false
	}
	if left.ReplayProof == nil || right.ReplayProof == nil {
		return left.ReplayProof == nil && right.ReplayProof == nil
	}
	leftProof := left.ReplayProof.Normalized()
	rightProof := right.ReplayProof.Normalized()
	return leftProof.OwnerModuleID == rightProof.OwnerModuleID && leftProof.OperationID == rightProof.OperationID &&
		leftProof.BeforeHash == rightProof.BeforeHash && leftProof.AfterHash == rightProof.AfterHash &&
		leftProof.AdmissibilitySnapshotID == rightProof.AdmissibilitySnapshotID && leftProof.AcquisitionBinding == rightProof.AcquisitionBinding &&
		sameRuntimeStringSet(leftProof.TargetRefs, rightProof.TargetRefs)
}

func ValidateRuntimeGovernanceRequestRecords(values []RuntimeGovernanceRequestRecord) error {
	seenOwnerIdentities := map[string]struct{}{}
	seenDispatches := map[string]struct{}{}
	for _, raw := range values {
		value := raw.Normalized()
		if !reflect.DeepEqual(raw, value) {
			return fmt.Errorf("runtime governance request record requires canonical fields")
		}
		if !validRuntimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID) || value.AuthoritySnapshotID == "" || value.DispatchID == "" || value.DependencyKind == "" {
			return fmt.Errorf("runtime governance request record requires owner identity, snapshot, dispatch and dependency")
		}
		if value.DependencyKind == string(GovernanceApproval) && (value.ApprovalID == "" || value.SubjectRef == "" || value.RequestedAction == "" || value.AuthorizationHash == "") {
			return fmt.Errorf("runtime approval governance request record requires approval identity, subject, requested action and authorization hash")
		}
		ownerIdentity := runtimeGovernanceOwnerIdentity(value.TicketID, value.ApprovalID)
		if _, ok := seenOwnerIdentities[ownerIdentity]; ok {
			return fmt.Errorf("runtime governance owner request %s is duplicated", ownerIdentity)
		}
		key := value.AuthoritySnapshotID + "|" + value.DispatchID
		if _, ok := seenDispatches[key]; ok {
			return fmt.Errorf("runtime governance request dispatch %s is duplicated", key)
		}
		seenOwnerIdentities[ownerIdentity] = struct{}{}
		seenDispatches[key] = struct{}{}
	}
	return nil
}

// ValidateRuntimeGovernanceRequestDispatchLineage binds every persisted owner
// request to the complete external dependency of one exact sealed governance
// dispatch. Dispatch identity alone cannot authorize a drifted action, risk or
// authorization context after restart.
func ValidateRuntimeGovernanceRequestDispatchLineage(values []RuntimeGovernanceRequestRecord, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord) error {
	if err := ValidateRuntimeGovernanceRequestRecords(values); err != nil {
		return err
	}
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, request := range values {
		request = request.Normalized()
		matched := 0
		for _, dispatch := range dispatches {
			if dispatch.ID != request.DispatchID || dispatch.Transaction.AuthoritySnapshotID != request.AuthoritySnapshotID {
				continue
			}
			if dispatch.Candidate.Governance == nil {
				return fmt.Errorf("runtime governance request dispatch %q has no governance candidate", request.DispatchID)
			}
			dependency := dispatch.Candidate.Governance.ExternalDependency.Normalized()
			if dependency.Kind != request.DependencyKind || dependency.SubjectRef != request.SubjectRef ||
				dependency.RequestedAction != request.RequestedAction || dependency.RiskLevel != request.RiskLevel ||
				StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext) != request.AuthorizationHash {
				return fmt.Errorf("runtime governance request does not match sealed external dependency for dispatch %q", request.DispatchID)
			}
			matched++
		}
		if matched != 1 {
			return fmt.Errorf("runtime governance request requires exactly one sealed dispatch, got %d", matched)
		}
	}
	return nil
}

func ValidateRuntimeGovernanceResolutionRecords(values []RuntimeGovernanceResolutionRecord) error {
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := raw.Normalized()
		if !reflect.DeepEqual(raw, value) {
			return fmt.Errorf("runtime governance resolution record requires canonical fields")
		}
		resolution := value.Resolution()
		key := string(value.ResolutionID) + "|" + value.AuthoritySnapshotID
		if err := resolution.Validate(); err != nil {
			return fmt.Errorf("runtime governance resolution record %s: %w", key, err)
		}
		wantHash := StableRuntimeGovernanceResolutionHash(resolution)
		if value.ResolutionHash == "" || value.ResolutionHash != wantHash {
			return fmt.Errorf("runtime governance resolution record %s has invalid resolution hash", key)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("runtime governance resolution record %s is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validRuntimeGovernanceOwnerIdentity(ticketID common.TicketID, approvalID common.ApprovalID) bool {
	return (ticketID != "") != (approvalID != "")
}

func runtimeGovernanceOwnerIdentity(ticketID common.TicketID, approvalID common.ApprovalID) string {
	if ticketID != "" {
		return "ticket:" + string(ticketID)
	}
	if approvalID != "" {
		return "approval:" + string(approvalID)
	}
	return ""
}

// StableRuntimeAuthorizationContextHash binds an owner-authored, typed
// authorization target to a governance request without exposing mutable maps.
func StableRuntimeAuthorizationContextHash(context workorcheventpayload.Payload) string {
	sum := sha256.Sum256([]byte(context.CanonicalString()))
	return hex.EncodeToString(sum[:])
}
