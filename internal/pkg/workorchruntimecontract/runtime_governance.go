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
