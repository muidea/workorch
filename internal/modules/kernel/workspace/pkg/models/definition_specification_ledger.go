package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// The fixed ledger capacity is intentional: exhausted epochs fail closed;
// deleting historical attempts must never grant another physical call.
const DefinitionSpecificationEpochLimit = 8
const DefinitionSpecificationRequestLimit = 16
const DefinitionSpecificationAttemptLimit = 2

type DefinitionSpecificationPhase string

const (
	DefinitionSpecificationProposal DefinitionSpecificationPhase = "proposal"
	DefinitionSpecificationCritic   DefinitionSpecificationPhase = "critic"
)

type DefinitionSpecificationRequestState string

const (
	DefinitionSpecificationReserved  DefinitionSpecificationRequestState = "reserved"
	DefinitionSpecificationCompleted DefinitionSpecificationRequestState = "completed"
)

// This identity names one possible physical call, not an admitted Definition.
// CandidateHash is the hash of the owner-sealed pre-specification input.
type DefinitionSpecificationRequestIdentity struct {
	WorkspaceID          common.WorkspaceID
	CandidateID          string
	CandidateRevision    uint64
	CandidateHash        string
	CompilationInputHash string
	Phase                DefinitionSpecificationPhase
	ReviewKind           string
	LogicalRequestID     string
	ProposalHash         string
}

func (i DefinitionSpecificationRequestIdentity) Normalized() DefinitionSpecificationRequestIdentity {
	i.CandidateID = strings.TrimSpace(i.CandidateID)
	i.CandidateHash = strings.TrimSpace(i.CandidateHash)
	i.CompilationInputHash = strings.TrimSpace(i.CompilationInputHash)
	i.Phase = DefinitionSpecificationPhase(strings.TrimSpace(string(i.Phase)))
	i.ReviewKind = strings.TrimSpace(i.ReviewKind)
	i.LogicalRequestID = strings.TrimSpace(i.LogicalRequestID)
	i.ProposalHash = strings.TrimSpace(i.ProposalHash)
	return i
}
func (i DefinitionSpecificationRequestIdentity) Validate() error {
	i = i.Normalized()
	if i.WorkspaceID == "" || i.CandidateID == "" || i.CandidateRevision == 0 || i.CandidateHash == "" || i.CompilationInputHash == "" || i.LogicalRequestID == "" || len(i.CandidateID) > 128 || len(i.CandidateHash) > 256 || len(i.CompilationInputHash) > 256 || len(i.LogicalRequestID) > 128 || len(i.ProposalHash) > 256 {
		return fmt.Errorf("specification request identity is incomplete or oversized")
	}
	switch i.Phase {
	case DefinitionSpecificationProposal:
		if i.ReviewKind != "" || i.ProposalHash != "" {
			return fmt.Errorf("proposal cannot claim a critic or previous proof")
		}
	case DefinitionSpecificationCritic:
		if !GoalSpecificationReviewKind(i.ReviewKind) || i.ProposalHash == "" {
			return fmt.Errorf("critic requires exact proposal and review kind")
		}
	default:
		return fmt.Errorf("unknown specification request phase")
	}
	return nil
}
func GoalSpecificationReviewKind(kind string) bool {
	for _, expected := range goalReviewKinds {
		if expected == kind {
			return true
		}
	}
	return false
}
func (i DefinitionSpecificationRequestIdentity) PermitID() string {
	i = i.Normalized()
	// Each field is length-delimited, including the candidate revision.
	fields := []string{string(i.WorkspaceID), i.CandidateID, fmt.Sprintf("%d", i.CandidateRevision), i.CandidateHash, i.CompilationInputHash, string(i.Phase), i.ReviewKind, i.LogicalRequestID, i.ProposalHash}
	h := sha256.New()
	for _, field := range fields {
		_, _ = fmt.Fprintf(h, "%08x%s", len(field), field)
	}
	return "workspace-spec-permit-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func DefinitionSpecificationCompilationInputHash(candidateHash string, resources []GoalSpecificationResource, dimensions []GoalSpecificationDimension) string {
	seed := GoalSpecification{Version: GoalSpecificationVersionCurrent, Resources: resources, Dimensions: dimensions}
	seed = seed.Normalized()
	h := sha256.New()
	writeDefinitionHash(h, "workspace-specification-compilation-input-v1", strings.TrimSpace(candidateHash), fmt.Sprintf("%d", len(seed.Resources)))
	for _, resource := range seed.Resources {
		writeDefinitionHash(h, resource.ID, resource.Ref, resource.SnapshotHash, fmt.Sprintf("%t", resource.Required), fmt.Sprintf("%t", resource.Available))
	}
	writeDefinitionHash(h, fmt.Sprintf("%d", len(seed.Dimensions)))
	for _, dimension := range seed.Dimensions {
		writeDefinitionHash(h, dimension.ID, fmt.Sprintf("%t", dimension.Required), fmt.Sprintf("%t", dimension.Activated), fmt.Sprintf("%d", len(dimension.ResourceIDs)))
		writeDefinitionHash(h, dimension.ResourceIDs...)
	}
	return "workspace-spec-input-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ValidateDefinitionSpecificationInputs checks the bounded typed Begin facts;
// it does not claim to judge natural-language completeness or provider output.
func ValidateDefinitionSpecificationInputs(resources []GoalSpecificationResource, dimensions []GoalSpecificationDimension) error {
	seed := GoalSpecification{Version: GoalSpecificationVersionCurrent, Resources: resources, Dimensions: dimensions}.Normalized()
	if len(seed.Resources) > 128 || len(seed.Dimensions) > 64 {
		return fmt.Errorf("specification compilation input exceeds bounds")
	}
	seen := map[string]bool{}
	resourceIDs := map[string]bool{}
	for _, resource := range seed.Resources {
		if resource.ID == "" || len(resource.ID) > 128 || seen[resource.ID] || resource.Ref == "" || len(resource.Ref) > 1024 || len(resource.SnapshotHash) > 256 {
			return fmt.Errorf("specification compilation resource is invalid")
		}
		seen[resource.ID] = true
		resourceIDs[resource.ID] = true
	}
	for _, dimension := range seed.Dimensions {
		if dimension.ID == "" || len(dimension.ID) > 128 || seen[dimension.ID] || len(dimension.ResourceIDs) > 128 || !specRefsExist(dimension.ResourceIDs, func(ref string) bool { return resourceIDs[ref] }) {
			return fmt.Errorf("specification compilation dimension is invalid")
		}
		seen[dimension.ID] = true
	}
	return nil
}

func sameSpecificationResources(a, b []GoalSpecificationResource) bool {
	return len(a) == len(b) && (len(a) == 0 || reflect.DeepEqual(a, b))
}
func sameSpecificationDimensions(a, b []GoalSpecificationDimension) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Required != b[i].Required || a[i].Activated != b[i].Activated || len(a[i].ResourceIDs) != len(b[i].ResourceIDs) || (len(a[i].ResourceIDs) > 0 && !reflect.DeepEqual(a[i].ResourceIDs, b[i].ResourceIDs)) {
			return false
		}
	}
	return true
}

// Completion contains only a typed bounded parse outcome or a short error
// class, never provider output text, prompt, secret or executable authority.
type DefinitionSpecificationRequestCompletion struct {
	Proposal    *GoalSpecification
	Review      *GoalSpecificationCompletenessReview
	FailureCode string
}

func (c DefinitionSpecificationRequestCompletion) Clone() DefinitionSpecificationRequestCompletion {
	if c.Proposal != nil {
		s := c.Proposal.Clone()
		c.Proposal = &s
	}
	if c.Review != nil {
		r := *c.Review
		r.EvidenceRefs = append([]string(nil), r.EvidenceRefs...)
		r.RequirementIDs = append([]string(nil), r.RequirementIDs...)
		c.Review = &r
	}
	return c
}
func (c DefinitionSpecificationRequestCompletion) Normalized() DefinitionSpecificationRequestCompletion {
	c = c.Clone()
	c.FailureCode = strings.TrimSpace(c.FailureCode)
	if c.Proposal != nil {
		s := c.Proposal.Normalized()
		c.Proposal = &s
	}
	if c.Review != nil {
		r := *c.Review
		r.Kind = strings.TrimSpace(r.Kind)
		r.Status = strings.TrimSpace(r.Status)
		r.SpecificationHash = strings.TrimSpace(r.SpecificationHash)
		r.Reason = strings.TrimSpace(r.Reason)
		r.EvidenceRefs = normalizedSpecRefs(r.EvidenceRefs)
		r.RequirementIDs = normalizedSpecRefs(r.RequirementIDs)
		c.Review = &r
	}
	return c
}
func (c DefinitionSpecificationRequestCompletion) ValidateFor(id DefinitionSpecificationRequestIdentity, candidate DefinitionAdmissionCandidate, resources []GoalSpecificationResource, dimensions []GoalSpecificationDimension, proposal *GoalSpecification) error {
	c = c.Normalized()
	id = id.Normalized()
	if c.FailureCode != "" {
		if c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96 {
			return fmt.Errorf("failure contains payload or exceeds bound")
		}
		for _, r := range c.FailureCode {
			if (r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9') {
				return fmt.Errorf("failure code must be opaque lowercase identifier")
			}
		}
		return nil
	}
	switch id.Phase {
	case DefinitionSpecificationProposal:
		if c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions) {
			return fmt.Errorf("proposal does not bind exact sealed compilation input")
		}
		seed := c.Proposal.Clone()
		hash := seed.ContentHash()
		for _, kind := range goalReviewKinds {
			seed.Reviews = append(seed.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: hash})
		}
		return seed.Validate(candidate.GoalInput, candidate.DoDInputs)
	case DefinitionSpecificationCritic:
		if c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash {
			return fmt.Errorf("critic does not bind owner proposal")
		}
		seed := proposal.Clone()
		seed.Reviews = nil
		for _, kind := range goalReviewKinds {
			r := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: id.ProposalHash}
			if kind == c.Review.Kind {
				r = *c.Review
			}
			seed.Reviews = append(seed.Reviews, r)
		}
		return seed.Validate(candidate.GoalInput, candidate.DoDInputs)
	default:
		return fmt.Errorf("unsupported specification phase")
	}
}

type DefinitionSpecificationRequestRecord struct {
	Identity   DefinitionSpecificationRequestIdentity
	PermitID   string
	State      DefinitionSpecificationRequestState
	Completion *DefinitionSpecificationRequestCompletion
}

func (r DefinitionSpecificationRequestRecord) Clone() DefinitionSpecificationRequestRecord {
	if r.Completion != nil {
		c := r.Completion.Clone()
		r.Completion = &c
	}
	return r
}

type DefinitionSpecificationCompilationRef struct {
	InputCandidateHash   string
	CompilationInputHash string
	ProposalPermitID     string
	ProposalHash         string
	CriticPermitIDs      [6]string
}

func (r DefinitionSpecificationCompilationRef) Normalized() DefinitionSpecificationCompilationRef {
	r.InputCandidateHash = strings.TrimSpace(r.InputCandidateHash)
	r.CompilationInputHash = strings.TrimSpace(r.CompilationInputHash)
	r.ProposalPermitID = strings.TrimSpace(r.ProposalPermitID)
	r.ProposalHash = strings.TrimSpace(r.ProposalHash)
	for i := range r.CriticPermitIDs {
		r.CriticPermitIDs[i] = strings.TrimSpace(r.CriticPermitIDs[i])
	}
	return r
}
func (r DefinitionSpecificationCompilationRef) Validate() error {
	r = r.Normalized()
	if r.InputCandidateHash == "" || r.CompilationInputHash == "" || r.ProposalPermitID == "" || r.ProposalHash == "" || len(r.InputCandidateHash) > 256 || len(r.CompilationInputHash) > 256 || len(r.ProposalPermitID) > 256 || len(r.ProposalHash) > 256 {
		return fmt.Errorf("compilation ref is incomplete or oversized")
	}
	seen := map[string]bool{r.ProposalPermitID: true}
	for _, permit := range r.CriticPermitIDs {
		if permit == "" || len(permit) > 256 || seen[permit] {
			return fmt.Errorf("compilation ref lacks six unique critics")
		}
		seen[permit] = true
	}
	return nil
}

// Epoch is anchored to formal owner Goal/DoD revisions (not candidate or
// StartupInput revisions). The snapshot is strictly pre-seal and never appears
// in DefinitionAdmissionCandidates until the normal admission intent arrives.
type DefinitionSpecificationEpoch struct {
	GoalRevision         string
	DoDRevision          string
	Snapshot             DefinitionAdmissionCandidate
	Resources            []GoalSpecificationResource
	Dimensions           []GoalSpecificationDimension
	CompilationInputHash string
	TemplateHash         string
	ConfigurationHash    string
	WorkspaceRevision    uint64
	Count                uint8
	Records              [DefinitionSpecificationRequestLimit]DefinitionSpecificationRequestRecord
}

func (e DefinitionSpecificationEpoch) Clone() DefinitionSpecificationEpoch {
	e.Snapshot = e.Snapshot.Clone()
	seed := GoalSpecification{Resources: e.Resources, Dimensions: e.Dimensions}.Clone()
	e.Resources, e.Dimensions = seed.Resources, seed.Dimensions
	for i := range e.Records {
		e.Records[i] = e.Records[i].Clone()
	}
	return e
}
func (e DefinitionSpecificationEpoch) Validate(workspaceID common.WorkspaceID) error {
	if e.GoalRevision == "" || e.DoDRevision == "" || e.TemplateHash == "" || e.ConfigurationHash == "" || e.WorkspaceRevision == 0 || e.Count > DefinitionSpecificationRequestLimit || len(e.GoalRevision) > 160 || len(e.DoDRevision) > 160 || len(e.TemplateHash) > 256 || len(e.ConfigurationHash) > 256 {
		return fmt.Errorf("specification epoch missing owner identity or over budget")
	}
	if e.Snapshot.WorkspaceID != workspaceID || e.Snapshot.Specification != nil || e.Snapshot.CompilationRef != nil || e.Snapshot.CandidateHash == "" || e.Snapshot.WorkspaceTemplateSnapshot.SnapshotHash != e.TemplateHash || e.Snapshot.Validate() != nil || !strings.HasSuffix(e.Snapshot.GoalInput.SourceIdentity, "@"+e.GoalRevision) || len(e.Snapshot.DoDInputs) == 0 || !strings.HasSuffix(e.Snapshot.DoDInputs[0].SourceIdentity, "@"+e.DoDRevision) {
		return fmt.Errorf("specification epoch snapshot invalid")
	}
	seed := GoalSpecification{Resources: e.Resources, Dimensions: e.Dimensions}.Normalized()
	if !sameSpecificationResources(e.Resources, seed.Resources) || !sameSpecificationDimensions(e.Dimensions, seed.Dimensions) || ValidateDefinitionSpecificationInputs(e.Resources, e.Dimensions) != nil || e.CompilationInputHash != DefinitionSpecificationCompilationInputHash(e.Snapshot.CandidateHash, e.Resources, e.Dimensions) {
		return fmt.Errorf("specification epoch compilation input invalid")
	}
	seenPermits := map[string]bool{}
	seenIDs := map[string]bool{}
	attempts := map[string]int{}
	proposals := map[string]*GoalSpecification{}
	for n := 0; n < int(e.Count); n++ {
		record := e.Records[n]
		id := record.Identity.Normalized()
		if record.Identity != id || id.Validate() != nil || id.WorkspaceID != workspaceID || id.CandidateID != e.Snapshot.CandidateID || id.CandidateRevision != e.Snapshot.CandidateRevision || id.CandidateHash != e.Snapshot.CandidateHash || id.CompilationInputHash != e.CompilationInputHash || record.PermitID != id.PermitID() || seenPermits[record.PermitID] || seenIDs[id.LogicalRequestID] {
			return fmt.Errorf("specification request identity forged or duplicated")
		}
		seenPermits[record.PermitID] = true
		seenIDs[id.LogicalRequestID] = true
		attemptKey := string(id.Phase) + "\x00" + id.ReviewKind
		attempts[attemptKey]++
		if attempts[attemptKey] > DefinitionSpecificationAttemptLimit {
			return fmt.Errorf("specification phase attempt budget exceeded")
		}
		var proposal *GoalSpecification
		if id.Phase == DefinitionSpecificationCritic {
			proposal = proposals[id.ProposalHash]
			if proposal == nil {
				return fmt.Errorf("critic lacks earlier completed owner proposal")
			}
		}
		switch record.State {
		case DefinitionSpecificationReserved:
			if record.Completion != nil {
				return fmt.Errorf("reserved specification request already completed")
			}
		case DefinitionSpecificationCompleted:
			if record.Completion == nil || record.Completion.ValidateFor(id, e.Snapshot, e.Resources, e.Dimensions, proposal) != nil {
				return fmt.Errorf("completed specification request has invalid typed outcome")
			}
			if record.Completion.Proposal != nil {
				proposals[record.Completion.Proposal.ContentHash()] = record.Completion.Proposal
			}
		default:
			return fmt.Errorf("specification request has unknown state")
		}
	}
	for n := int(e.Count); n < len(e.Records); n++ {
		if e.Records[n].Identity != (DefinitionSpecificationRequestIdentity{}) || e.Records[n].PermitID != "" || e.Records[n].State != "" || e.Records[n].Completion != nil {
			return fmt.Errorf("specification epoch contains undeclared request slot")
		}
	}
	return nil
}

type DefinitionSpecificationRequestLedger struct {
	Count  uint8
	Epochs [DefinitionSpecificationEpochLimit]DefinitionSpecificationEpoch
}

func (l DefinitionSpecificationRequestLedger) Clone() DefinitionSpecificationRequestLedger {
	for i := range l.Epochs {
		l.Epochs[i] = l.Epochs[i].Clone()
	}
	return l
}
func (l DefinitionSpecificationRequestLedger) Validate(workspaceID common.WorkspaceID) error {
	if workspaceID == "" || l.Count == 0 || l.Count > DefinitionSpecificationEpochLimit {
		return fmt.Errorf("specification ledger epoch capacity invalid")
	}
	seen := map[string]bool{}
	for i := 0; i < int(l.Count); i++ {
		e := l.Epochs[i]
		if err := e.Validate(workspaceID); err != nil {
			return err
		}
		key := e.GoalRevision + "\x00" + e.DoDRevision
		if seen[key] {
			return fmt.Errorf("specification epoch reopened")
		}
		seen[key] = true
	}
	for i := int(l.Count); i < len(l.Epochs); i++ {
		if !reflect.DeepEqual(l.Epochs[i].Snapshot, DefinitionAdmissionCandidate{}) || l.Epochs[i].GoalRevision != "" || l.Epochs[i].DoDRevision != "" || l.Epochs[i].TemplateHash != "" || l.Epochs[i].CompilationInputHash != "" || len(l.Epochs[i].Resources) != 0 || len(l.Epochs[i].Dimensions) != 0 || l.Epochs[i].ConfigurationHash != "" || l.Epochs[i].WorkspaceRevision != 0 || l.Epochs[i].Count != 0 {
			return fmt.Errorf("specification ledger contains undeclared epoch")
		}
		for _, record := range l.Epochs[i].Records {
			if record.Identity != (DefinitionSpecificationRequestIdentity{}) || record.PermitID != "" || record.State != "" || record.Completion != nil {
				return fmt.Errorf("specification ledger contains undeclared epoch requests")
			}
		}
	}
	return nil
}
