package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeGovernanceResolutionOutcome string

const (
	RuntimeGovernanceResolutionOutcomeContinue RuntimeGovernanceResolutionOutcome = "continue"
	RuntimeGovernanceResolutionOutcomeFail     RuntimeGovernanceResolutionOutcome = "fail"
)

// RuntimeGovernanceResolution 是交回 Runtime V2 authority owner 的具体治理结果。
type RuntimeGovernanceResolution struct {
	ResolutionID        common.ResolutionID
	TicketID            common.TicketID
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
	if r.ResolutionID == "" || r.TicketID == "" || r.WorkspaceID == "" || r.RunID == "" || r.AuthoritySnapshotID == "" || r.DispatchID == "" {
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

type RuntimeGovernanceResolutionRecord struct {
	ResolutionID        common.ResolutionID
	TicketID            common.TicketID
	AuthoritySnapshotID string
	DispatchID          string
	Outcome             RuntimeGovernanceResolutionOutcome
}

type RuntimeGovernanceRequestRecord struct {
	TicketID            common.TicketID
	AuthoritySnapshotID string
	DispatchID          string
	DependencyKind      string
	SubjectRef          string
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
	TicketID              common.TicketID
	GovernanceDispatchID  string
	GovernanceSnapshotID  string
	OperationDispatchID   string
	OperationID           string
	ExecutionKey          string
	Fence                 uint64
	ExpectedEffectScope   RuntimeEffectScope
	DependencyKind        string
	SubjectRef            string
	GovernanceReceiptHash string
	State                 RuntimeOperationGovernanceBindingState
	Grant                 *RuntimeOperationGovernanceGrant
}

type RuntimeOperationGovernanceGrant struct {
	ResolutionID          common.ResolutionID
	TicketID              common.TicketID
	ExecutionKey          string
	GovernanceFence       uint64
	ResumeFence           uint64
	DependencyKind        string
	SubjectRef            string
	GovernanceReceiptHash string
	EvidenceRefs          []string
}

func (g RuntimeOperationGovernanceGrant) Normalized() RuntimeOperationGovernanceGrant {
	g.ExecutionKey = strings.TrimSpace(g.ExecutionKey)
	g.DependencyKind = strings.TrimSpace(g.DependencyKind)
	g.SubjectRef = strings.TrimSpace(g.SubjectRef)
	g.GovernanceReceiptHash = strings.TrimSpace(g.GovernanceReceiptHash)
	g.EvidenceRefs = normalizeStrings(g.EvidenceRefs)
	return g
}

func (g RuntimeOperationGovernanceGrant) Validate() error {
	g = g.Normalized()
	if g.ResolutionID == "" || g.TicketID == "" || g.ExecutionKey == "" || g.GovernanceFence == 0 || g.ResumeFence == 0 ||
		g.DependencyKind == "" || g.SubjectRef == "" || g.GovernanceReceiptHash == "" {
		return fmt.Errorf("runtime operation governance grant requires identity, execution, dependency and receipt hash")
	}
	if g.ResumeFence != g.GovernanceFence+1 {
		return fmt.Errorf("runtime operation governance grant resume fence must follow governance fence")
	}
	return nil
}

func SameRuntimeOperationGovernanceGrant(left RuntimeOperationGovernanceGrant, right RuntimeOperationGovernanceGrant) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.ResolutionID != right.ResolutionID || left.TicketID != right.TicketID || left.ExecutionKey != right.ExecutionKey ||
		left.GovernanceFence != right.GovernanceFence || left.ResumeFence != right.ResumeFence || left.DependencyKind != right.DependencyKind ||
		left.SubjectRef != right.SubjectRef || left.GovernanceReceiptHash != right.GovernanceReceiptHash || len(left.EvidenceRefs) != len(right.EvidenceRefs) {
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
	b.DependencyKind = strings.TrimSpace(b.DependencyKind)
	b.SubjectRef = strings.TrimSpace(b.SubjectRef)
	b.GovernanceReceiptHash = strings.TrimSpace(b.GovernanceReceiptHash)
	b.State = RuntimeOperationGovernanceBindingState(strings.TrimSpace(string(b.State)))
	if b.Grant != nil {
		grant := b.Grant.Normalized()
		b.Grant = &grant
	}
	return b
}

func (b RuntimeOperationGovernanceBinding) Clone() RuntimeOperationGovernanceBinding {
	ret := b.Normalized()
	if ret.Grant != nil {
		grant := ret.Grant.Normalized()
		grant.EvidenceRefs = append([]string(nil), grant.EvidenceRefs...)
		ret.Grant = &grant
	}
	return ret
}

func ValidateRuntimeOperationGovernanceBindings(values []RuntimeOperationGovernanceBinding) error {
	seenExecution := map[string]struct{}{}
	seenTicket := map[common.TicketID]struct{}{}
	for _, raw := range values {
		value := raw.Normalized()
		if value.OperationDispatchID == "" || value.OperationID == "" || value.ExecutionKey == "" || value.Fence == 0 ||
			value.ExpectedEffectScope == RuntimeEffectUnknown || value.DependencyKind == "" || value.SubjectRef == "" || value.GovernanceReceiptHash == "" {
			return fmt.Errorf("operation governance binding requires source operation execution identity and dependency")
		}
		executionIdentity := value.ExecutionKey + "|" + fmt.Sprintf("%d", value.Fence)
		if _, duplicate := seenExecution[executionIdentity]; duplicate {
			return fmt.Errorf("operation governance binding execution identity %q is duplicated", executionIdentity)
		}
		seenExecution[executionIdentity] = struct{}{}
		switch value.State {
		case RuntimeOperationGovernanceBindingSuspended:
			if value.TicketID != "" || value.GovernanceDispatchID != "" || value.GovernanceSnapshotID != "" || value.Grant != nil {
				return fmt.Errorf("suspended operation governance binding cannot have ticket or governance dispatch")
			}
		case RuntimeOperationGovernanceBindingTicketBound, RuntimeOperationGovernanceBindingRejected:
			if value.TicketID == "" || value.GovernanceDispatchID == "" || value.GovernanceSnapshotID == "" {
				return fmt.Errorf("operation governance binding state %q requires ticket and governance dispatch", value.State)
			}
			if value.Grant != nil {
				return fmt.Errorf("operation governance binding state %q cannot carry grant", value.State)
			}
			if _, duplicate := seenTicket[value.TicketID]; duplicate {
				return fmt.Errorf("operation governance binding ticket %q is duplicated", value.TicketID)
			}
			seenTicket[value.TicketID] = struct{}{}
		case RuntimeOperationGovernanceBindingGranted:
			if value.TicketID == "" || value.GovernanceDispatchID == "" || value.GovernanceSnapshotID == "" || value.Grant == nil {
				return fmt.Errorf("granted operation governance binding requires ticket, governance dispatch and grant")
			}
			if err := value.Grant.Validate(); err != nil {
				return err
			}
			grant := value.Grant.Normalized()
			if grant.TicketID != value.TicketID || grant.ExecutionKey != value.ExecutionKey || grant.GovernanceFence != value.Fence ||
				grant.DependencyKind != value.DependencyKind || grant.SubjectRef != value.SubjectRef || grant.GovernanceReceiptHash != value.GovernanceReceiptHash {
				return fmt.Errorf("operation governance grant does not match binding")
			}
			if _, duplicate := seenTicket[value.TicketID]; duplicate {
				return fmt.Errorf("operation governance binding ticket %q is duplicated", value.TicketID)
			}
			seenTicket[value.TicketID] = struct{}{}
		default:
			return fmt.Errorf("operation governance binding has invalid state %q", value.State)
		}
	}
	return nil
}

func ValidateRuntimeGovernanceRequestRecords(values []RuntimeGovernanceRequestRecord) error {
	seenTickets := map[common.TicketID]struct{}{}
	seenDispatches := map[string]struct{}{}
	for _, value := range values {
		value.AuthoritySnapshotID = strings.TrimSpace(value.AuthoritySnapshotID)
		value.DispatchID = strings.TrimSpace(value.DispatchID)
		value.DependencyKind = strings.TrimSpace(value.DependencyKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		if value.TicketID == "" || value.AuthoritySnapshotID == "" || value.DispatchID == "" || value.DependencyKind == "" {
			return fmt.Errorf("runtime governance request record requires ticket, snapshot, dispatch and dependency")
		}
		if _, ok := seenTickets[value.TicketID]; ok {
			return fmt.Errorf("runtime governance request ticket %s is duplicated", value.TicketID)
		}
		key := value.AuthoritySnapshotID + "|" + value.DispatchID
		if _, ok := seenDispatches[key]; ok {
			return fmt.Errorf("runtime governance request dispatch %s is duplicated", key)
		}
		seenTickets[value.TicketID] = struct{}{}
		seenDispatches[key] = struct{}{}
	}
	return nil
}

func ValidateRuntimeGovernanceResolutionRecords(values []RuntimeGovernanceResolutionRecord) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		key := string(value.ResolutionID) + "|" + strings.TrimSpace(value.AuthoritySnapshotID)
		if value.ResolutionID == "" || value.TicketID == "" || strings.TrimSpace(value.AuthoritySnapshotID) == "" || strings.TrimSpace(value.DispatchID) == "" {
			return fmt.Errorf("runtime governance resolution record requires identity, snapshot and dispatch")
		}
		if value.Outcome != RuntimeGovernanceResolutionOutcomeContinue && value.Outcome != RuntimeGovernanceResolutionOutcomeFail {
			return fmt.Errorf("runtime governance resolution record %s has invalid outcome %q", key, value.Outcome)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("runtime governance resolution record %s is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
