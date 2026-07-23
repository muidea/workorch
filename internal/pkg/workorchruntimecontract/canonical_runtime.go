package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeContractVersionV2 = "v2"

type ConvergenceState string

const (
	ConvergenceStateNeedPrerequisite ConvergenceState = "need_prerequisite"
	ConvergenceStateNeedStateChange  ConvergenceState = "need_state_change"
	ConvergenceStateNeedValidation   ConvergenceState = "need_validation"
	ConvergenceStateNeedAssessment   ConvergenceState = "need_assessment"
	ConvergenceStateNeedGovernance   ConvergenceState = "need_governance"
	ConvergenceStateComplete         ConvergenceState = "complete"
	ConvergenceStateFailed           ConvergenceState = "failed"
)

type RuntimeAction string

const (
	RuntimeActionAcquirePrerequisite RuntimeAction = "acquire_prerequisite"
	RuntimeActionProduceStateChange  RuntimeAction = "produce_state_change"
	RuntimeActionProduceValidation   RuntimeAction = "produce_validation"
	RuntimeActionProduceAssessment   RuntimeAction = "produce_assessment"
	RuntimeActionRequestGovernance   RuntimeAction = "request_governance"
	RuntimeActionComplete            RuntimeAction = "complete"
	RuntimeActionFail                RuntimeAction = "fail"
)

type RequiredOutputKind string

const (
	RequiredOutputKindNone         RequiredOutputKind = ""
	RequiredOutputKindPrerequisite RequiredOutputKind = "prerequisite"
	RequiredOutputKindStateChange  RequiredOutputKind = "state_change"
	RequiredOutputKindValidation   RequiredOutputKind = "validation"
	RequiredOutputKindAssessment   RequiredOutputKind = "assessment"
	RequiredOutputKindGovernance   RequiredOutputKind = "governance"
)

type CoverageFocus struct {
	ID                     string
	SliceID                string
	AssessmentScope        string
	AssessmentID           string
	AssessmentText         string
	CoverageWorkItemIDs    []common.CoverageWorkItemID
	RequiredEvidenceKinds  []string
	TargetRefs             []string
	EvidenceRole           string
	TargetRole             string
	TargetDiscoverySources []string
	// TargetSelectorRef is an opaque, profile-declared selector identity. The
	// owning capability validates and applies it; Runtime V2 only seals and
	// compares the binding.
	TargetSelectorRef                 string
	RequireTargetGrounding            bool
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []RuntimeOperationVariantRef
	RejectedStepKinds                 []string
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	RequireCoverageTargets            bool
	RequireAssessment                 bool
}

type EvidenceLedger struct {
	Accepted []EvidenceFact
	Rejected []EvidenceFact
}

type RuntimeAssessmentFact struct {
	ID                  string
	FocusID             string
	SliceID             string
	Scope               string
	RequirementID       string
	RequirementText     string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Disposition         AssessmentDisposition
	Items               []RuntimeAssessmentItemFact
	Gap                 *RuntimeAssessmentGap
	Summary             string
	EvidenceRefs        []string
	ArtifactRefs        []string
}

type EvidenceFact struct {
	ID                  common.EvidenceRecordID
	Kind                string
	SubjectRef          string
	SourceRef           string
	AuthoritySnapshotID string
	DispatchID          string
	OperationID         string
	Revision            string
	FailureClass        FailureClass
	FailureCode         string
	FailureSignature    string
	FailureMessage      string
	Summary             string
	ContentHash         string
	ExitCode            *int64
	EvidenceRefs        []string
	// TargetRefs are the sealed operation targets that produced this fact. They
	// preserve owner-validated target lineage for SM-2 comparisons; the contract
	// package does not use them to choose a transition.
	TargetRefs          []string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Quality             common.EvidenceQuality
	ZeroWork            bool
	EvidenceReason      string
	EvidenceTags        []string
	Validation          *RuntimeValidationObservation
}

type RuntimeOperationEvidence struct {
	Summary        string
	ContentHash    string
	ExitCode       *int64
	EvidenceRefs   []string
	Quality        common.EvidenceQuality
	ZeroWork       bool
	EvidenceReason string
	EvidenceTags   []string
	Validation     *RuntimeValidationObservation
}

// RuntimeValidationObservation is an owner-reported, domain-neutral summary of
// validation work. Capability owners may derive it from tool-specific output,
// while Orchestrator only consumes the typed quality and check identities.
type RuntimeValidationObservation struct {
	ExecutedChecks []string
	PassedChecks   []string
	FailedChecks   []string
	SkippedChecks  []string
	OutputPreview  string
}

func (o RuntimeValidationObservation) Normalized() RuntimeValidationObservation {
	o.ExecutedChecks = normalizeStrings(o.ExecutedChecks)
	o.PassedChecks = normalizeStrings(o.PassedChecks)
	o.FailedChecks = normalizeStrings(o.FailedChecks)
	o.SkippedChecks = normalizeStrings(o.SkippedChecks)
	o.OutputPreview = strings.TrimSpace(o.OutputPreview)
	return o
}

func (o RuntimeValidationObservation) Clone() RuntimeValidationObservation {
	o = o.Normalized()
	o.ExecutedChecks = append([]string(nil), o.ExecutedChecks...)
	o.PassedChecks = append([]string(nil), o.PassedChecks...)
	o.FailedChecks = append([]string(nil), o.FailedChecks...)
	o.SkippedChecks = append([]string(nil), o.SkippedChecks...)
	return o
}

type FailureClass string

const (
	FailureClassBuildFailure                 FailureClass = "build_failure"
	FailureClassCoverageAssessmentIncomplete FailureClass = "coverage_assessment_incomplete"
	FailureClassGovernanceDependency         FailureClass = "governance_dependency"
	FailureClassRuntimeContractMisalignment  FailureClass = "runtime_internal_contract_misalignment"
	FailureClassExecutionFailure             FailureClass = "execution_failure"
	FailureClassNoProgressExhausted          FailureClass = "no_progress_exhausted"
	FailureClassCandidateExhausted           FailureClass = "candidate_retry_exhausted"
	FailureClassCandidateOutputExhausted     FailureClass = "candidate_output_continuation_exhausted"
	FailureClassPrerequisiteRefreshExhausted FailureClass = "prerequisite_refresh_exhausted"
	FailureClassGovernanceRejected           FailureClass = "governance_rejected"
)

type RuntimeOperationExecutionStatus string

const (
	RuntimeOperationExecutionStatusAccepted RuntimeOperationExecutionStatus = "accepted"
	RuntimeOperationExecutionStatusRejected RuntimeOperationExecutionStatus = "rejected"
)

type FailureFact struct {
	Class                FailureClass
	Code                 string
	Signature            string
	Message              string
	TargetRefs           []string
	TargetObservation    *RuntimeFailureTargetObservation
	ValidationDiagnostic *ValidationFailureDiagnostic
}

func (f FailureFact) Normalized() FailureFact {
	f.Class = FailureClass(strings.TrimSpace(string(f.Class)))
	f.Code = strings.TrimSpace(f.Code)
	f.Signature = strings.TrimSpace(f.Signature)
	f.Message = strings.TrimSpace(f.Message)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	if f.TargetObservation != nil {
		observation := f.TargetObservation.Normalized()
		f.TargetObservation = &observation
	}
	if f.ValidationDiagnostic != nil {
		diagnostic := f.ValidationDiagnostic.Normalized()
		f.ValidationDiagnostic = &diagnostic
	}
	return f
}

func (f FailureFact) Clone() FailureFact {
	f = f.Normalized()
	f.TargetRefs = append([]string(nil), f.TargetRefs...)
	if f.TargetObservation != nil {
		observation := f.TargetObservation.Normalized()
		observation.TargetRefs = append([]string(nil), observation.TargetRefs...)
		f.TargetObservation = &observation
	}
	if f.ValidationDiagnostic != nil {
		diagnostic := f.ValidationDiagnostic.Clone()
		f.ValidationDiagnostic = &diagnostic
	}
	return f
}

// RuntimeFailureTargetObservation is a bounded, owner-authored observation of
// the repair target behind an execution failure. It is data only: the
// Orchestrator decides whether it is compatible with the current Focus and
// may become persisted repair authority.
type RuntimeFailureTargetObservation struct {
	OwnerModuleID    string
	SubjectRef       string
	TargetRole       string
	ObservedRevision string
	TargetRefs       []string
}

func (o RuntimeFailureTargetObservation) Normalized() RuntimeFailureTargetObservation {
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.SubjectRef = strings.TrimSpace(o.SubjectRef)
	o.TargetRole = strings.TrimSpace(o.TargetRole)
	o.ObservedRevision = strings.TrimSpace(o.ObservedRevision)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	return o
}

func (o RuntimeFailureTargetObservation) Validate() error {
	o = o.Normalized()
	if o.OwnerModuleID == "" || o.SubjectRef == "" || o.TargetRole == "" || o.ObservedRevision == "" || len(o.TargetRefs) == 0 {
		return fmt.Errorf("runtime failure target observation requires owner, subject, role, revision and targets")
	}
	return nil
}

// ValidateRuntimeFailureTargetObservationBinding checks only the data binding
// between a failure and its optional owner observation. It is intentionally a
// pure contract helper: it does not decide whether that observation is
// compatible with a Focus or may advance a Runtime aggregate.
func ValidateRuntimeFailureTargetObservationBinding(failure FailureFact) error {
	if failure.TargetObservation == nil {
		return nil
	}
	observation := failure.TargetObservation.Normalized()
	if err := observation.Validate(); err != nil {
		return err
	}
	if !sameStrings(failure.TargetRefs, observation.TargetRefs) {
		return fmt.Errorf("runtime failure target observation targets must match failure targets")
	}
	return nil
}

type PrerequisiteState struct {
	Pending   []RuntimePrerequisite
	Satisfied []RuntimePrerequisite
}

type PrerequisiteKind string

const (
	PrerequisiteKindManifest          PrerequisiteKind = "manifest"
	PrerequisiteKindTargetSnapshot    PrerequisiteKind = "target_snapshot"
	PrerequisiteKindTargetSelection   PrerequisiteKind = "target_selection"
	PrerequisiteKindFailureTarget     PrerequisiteKind = "failure_target"
	PrerequisiteKindCapability        PrerequisiteKind = "capability"
	PrerequisiteKindCapabilityState   PrerequisiteKind = "capability_state"
	PrerequisiteKindValidationScratch PrerequisiteKind = "validation_scratch"
	PrerequisiteKindProviderCeiling   PrerequisiteKind = "provider_ceiling"
	// PrerequisiteKindOperationAdmissibility acquires a bounded owner-authored
	// parameter inventory before candidate generation. It is an SM-2 fact, not
	// a candidate or an execution receipt.
	PrerequisiteKindOperationAdmissibility PrerequisiteKind = "operation_admissibility"
)

type RuntimePrerequisite struct {
	ID                   string
	Kind                 PrerequisiteKind
	OwnerModuleID        string
	SubjectRef           string
	TargetSelectorRef    string
	CapabilityClass      RuntimeCapabilityClass
	RequiredCapabilities []string
	CapabilityStateKind  string
	MinimumBytes         int
	Reason               string
}

type RuntimeRequirement struct {
	Kind                 PrerequisiteKind
	OwnerModuleID        string
	SubjectRef           string
	TargetSelectorRef    string
	CapabilityClass      RuntimeCapabilityClass
	RequiredCapabilities []string
	CapabilityStateKind  string
	MinimumBytes         int
	Reason               string
}

type RuntimeFacts struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DefinitionRevision string
	EvidenceRevision   string
	Focus              CoverageFocus
	RemainingFocuses   []CoverageFocus
	Evidence           EvidenceLedger
	Assessments        []RuntimeAssessmentFact
	Failure            *FailureFact
	Prerequisites      PrerequisiteState
	Capabilities       RuntimeCapabilities
	Resources          RuntimeResourceFacts
	Activation         RuntimeActivationContext
	StateChange        RuntimeStateChangeLedger
	ConvergencePolicy  RuntimeConvergencePolicy
	RecoveryPolicy     RuntimeRecoveryPolicy
	Recovery           RuntimeRecoveryLedger
	Repair             RuntimeRepairLedger
	// RepairDirective is the single persisted next-action authority for the current
	// focus. When set it overrides raw MissingRequiredEvidenceKinds for candidate
	// action selection (INV-05).
	RepairDirective *RuntimeRepairDirective
	// FreshnessReacquireCoverage records focus coverage whose accepted evidence
	// is retained as lineage but whose partial sealed dispatch was invalidated.
	// It is not repair debt: after prerequisite acquisition reducer must issue a
	// fresh state-change candidate before this coverage can close.
	FreshnessReacquireCoverage []common.CoverageWorkItemID
	Coverage                   []RuntimeCoverageItemFact
	Feasibility                *RuntimeFeasibilityFact
	Slices                     []RuntimeAcceptanceSlice
}

// RuntimeFeasibilityFact is the persisted result of the pre-candidate
// capability gate. It is a reducer input, not a drive error or a candidate
// retry signal.
type RuntimeFeasibilityFact struct {
	FocusID                string
	Action                 RuntimeAction
	RequiredOperationKinds []string
	FeasibleOperationKinds []string
	MissingCapabilityKinds []string
	Reason                 string
}

type RuntimeCapabilities struct {
	StateChangeKinds []string
	ValidationKinds  []string
	GovernanceKinds  []string
}

type RuntimeCapabilityClass string

const (
	RuntimeCapabilityClassStateChange RuntimeCapabilityClass = "state_change"
	RuntimeCapabilityClassValidation  RuntimeCapabilityClass = "validation"
	RuntimeCapabilityClassGovernance  RuntimeCapabilityClass = "governance"
)

type RuntimeResourceFacts struct {
	Manifest                 *WorkspaceManifest
	TargetSnapshots          []TargetSnapshot
	TargetSelection          *RuntimeTargetSelection
	FailureTarget            *FailureTarget
	ValidationScratch        *ValidationScratch
	ProviderCeiling          *ProviderCeiling
	CapabilityStates         []RuntimeCapabilityStateSnapshot
	OperationAdmissibilities []RuntimeOperationAdmissibility
	// PrerequisiteObservations bind mutable owner facts to the exact resource
	// revisions they observed. They are additional freshness metadata, not a
	// replacement for the typed facts above.
	PrerequisiteObservations []RuntimePrerequisiteObservation
}

// RuntimeOperationAdmissibility is a bounded, owner-authored parameter
// inventory for candidate rendering and feasibility probes. It intentionally
// describes admissible references rather than an owner-specific work plan.
type RuntimeOperationAdmissibility struct {
	PrerequisiteID string
	SnapshotID     string
	// AcquisitionBinding is an opaque owner-authored link to a capability
	// snapshot acquired in the same owner observation.  It is optional for
	// owners that do not declare a capability-state prerequisite.
	AcquisitionBinding string
	OwnerModuleID      string
	OperationKinds     []string
	TargetRefs         []string
	SelectorRefs       []string
	ConstraintRefs     []string
	ObservedRevision   string
}

func (a RuntimeOperationAdmissibility) Normalized() RuntimeOperationAdmissibility {
	a.PrerequisiteID = strings.TrimSpace(a.PrerequisiteID)
	a.SnapshotID = strings.TrimSpace(a.SnapshotID)
	a.AcquisitionBinding = strings.TrimSpace(a.AcquisitionBinding)
	a.OwnerModuleID = strings.TrimSpace(a.OwnerModuleID)
	a.OperationKinds = canonicalizeOperationKinds(a.OperationKinds)
	a.TargetRefs = normalizeStrings(a.TargetRefs)
	a.SelectorRefs = normalizeStrings(a.SelectorRefs)
	a.ConstraintRefs = normalizeStrings(a.ConstraintRefs)
	a.ObservedRevision = strings.TrimSpace(a.ObservedRevision)
	return a
}

func (a RuntimeOperationAdmissibility) Validate() error {
	a = a.Normalized()
	if a.PrerequisiteID == "" || a.SnapshotID == "" || a.OwnerModuleID == "" || len(a.OperationKinds) == 0 || a.ObservedRevision == "" {
		return fmt.Errorf("runtime operation admissibility requires identity, owner, kinds and revision")
	}
	for _, kind := range a.OperationKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != a.OwnerModuleID {
			return fmt.Errorf("runtime operation admissibility kind %q does not belong to owner %q", kind, a.OwnerModuleID)
		}
	}
	if expected := StableRuntimeOperationAdmissibilitySnapshotID(a); a.SnapshotID != expected {
		return fmt.Errorf("runtime operation admissibility snapshot identity mismatch")
	}
	return nil
}

func StableRuntimeOperationAdmissibilitySnapshotID(value RuntimeOperationAdmissibility) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, value.PrerequisiteID)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.AcquisitionBinding)
	writeHashStrings(h, value.OperationKinds)
	writeHashStrings(h, value.TargetRefs)
	writeHashStrings(h, value.SelectorRefs)
	writeHashStrings(h, value.ConstraintRefs)
	writeHashString(h, value.ObservedRevision)
	return "operation-admissibility-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeCapabilityStateDisposition string

const (
	RuntimeCapabilityStateReady            RuntimeCapabilityStateDisposition = "ready"
	RuntimeCapabilityStateSequenceRequired RuntimeCapabilityStateDisposition = "operation_sequence_required"
	RuntimeCapabilityStateGovernance       RuntimeCapabilityStateDisposition = "governance_required"
	RuntimeCapabilityStateUnavailable      RuntimeCapabilityStateDisposition = "unavailable"
	RuntimeCapabilityStateUnknown          RuntimeCapabilityStateDisposition = "unknown"
)

type RuntimeRequiredOperationVariant struct {
	RequirementID           string
	Kind                    string
	VariantID               string
	DependsOnRequirementIDs []string
	Reason                  string
}

type RuntimeCapabilityStateSnapshot struct {
	RequirementID  string
	SnapshotID     string
	AcquisitionKey string
	// AcquisitionBinding is the opaque binding emitted by the owner for this
	// persisted acquisition. It may equal AcquisitionKey, but consumers must
	// compare it only as an opaque value.
	AcquisitionBinding  string
	OwnerModuleID       string
	CapabilityStateKind string
	SubjectRef          string
	ObservedRevision    string
	// WorkspaceTreeRevision is the File-owner revision observed alongside a
	// capability state that reads workspace contents. It is distinct from the
	// capability owner's repository revision and keeps cross-owner freshness
	// dependencies exact after a durable File effect.
	WorkspaceTreeRevision     string
	ContentHash               string
	Disposition               RuntimeCapabilityStateDisposition
	RequiredOperationSequence []RuntimeRequiredOperationVariant
	QueryReceiptRef           string
	ObservedAt                string
	InvalidatedByReceiptRefs  []string
}

// RuntimeCapabilityStateRef is the owner-neutral identity of one observed
// capability state. It is the only identity used for invalidation.
type RuntimeCapabilityStateRef struct {
	OwnerModuleID       string
	CapabilityStateKind string
	SubjectRef          string
}

// RuntimeCapabilityStateStaleDetail is an owner receipt fact: a sealed state
// observation no longer matches the state observed immediately before effect.
type RuntimeCapabilityStateStaleDetail struct {
	StateRef         RuntimeCapabilityStateRef
	SnapshotID       string
	ExpectedHash     string
	ObservedRevision string
	ObservedHash     string
	Reason           string
}

type WorkspaceManifest struct {
	Revision   string
	TargetRefs []string
}

type SnapshotContentState string

const (
	SnapshotContentStateComplete  SnapshotContentState = "complete"
	SnapshotContentStateMissing   SnapshotContentState = "missing"
	SnapshotContentStateTruncated SnapshotContentState = "truncated"
	SnapshotContentStateBinary    SnapshotContentState = "binary"
	SnapshotContentStateRedacted  SnapshotContentState = "redacted"
)

type TargetSnapshot struct {
	TargetRef    string
	Content      string
	ContentHash  string
	ContentState SnapshotContentState
	Size         int64
	LineCount    int
	ObservedAt   string
}

// RuntimeTargetSelection is a bounded, read-only owner observation for a
// direct-evidence focus without explicit target refs. It never encodes an
// operation, action, progress decision, or a guessed path: the owner reports
// a selected existing target, an empty-workspace creation scope, or an
// unresolved inventory fact, and SM-2 decides what to do with that fact.
type RuntimeTargetSelectionDisposition string

const (
	RuntimeTargetSelectionSelected RuntimeTargetSelectionDisposition = "selected"
	// RuntimeTargetSelectionCreate authorizes a creation candidate within an
	// owner-observed empty workspace. It deliberately carries no path: the
	// candidate proposes a concrete workspace-relative target and the File
	// owner validates it against its operation admissibility inventory.
	RuntimeTargetSelectionCreate RuntimeTargetSelectionDisposition = "create"
	// RuntimeTargetSelectionScope binds a bounded complete File feasibility
	// inventory selected by an owner-declared scope. It is not a concrete repair
	// target or a filename heuristic: a candidate still proposes concrete
	// operations and their owner validates every target against the sealed
	// admissibility inventory.
	RuntimeTargetSelectionScope     RuntimeTargetSelectionDisposition = "scope"
	RuntimeTargetSelectionAbsent    RuntimeTargetSelectionDisposition = "absent"
	RuntimeTargetSelectionAmbiguous RuntimeTargetSelectionDisposition = "ambiguous"
)

type RuntimeTargetSelection struct {
	PrerequisiteID      string
	SnapshotID          string
	OwnerModuleID       string
	TargetRole          string
	SelectorRef         string
	Disposition         RuntimeTargetSelectionDisposition
	TargetRefs          []string
	CandidateTargetRefs []string
	// CandidateCount is observed before the bounded candidate projection. It
	// prevents a max-items=1 context from turning a multi-target workspace into
	// a false unique selection.
	CandidateCount   int
	ObservedRevision string
}

func (s RuntimeTargetSelection) Normalized() RuntimeTargetSelection {
	s.PrerequisiteID = strings.TrimSpace(s.PrerequisiteID)
	s.SnapshotID = strings.TrimSpace(s.SnapshotID)
	s.OwnerModuleID = strings.TrimSpace(s.OwnerModuleID)
	s.TargetRole = strings.TrimSpace(s.TargetRole)
	s.SelectorRef = strings.TrimSpace(s.SelectorRef)
	s.Disposition = RuntimeTargetSelectionDisposition(strings.TrimSpace(string(s.Disposition)))
	s.TargetRefs = normalizeStrings(s.TargetRefs)
	s.CandidateTargetRefs = normalizeStrings(s.CandidateTargetRefs)
	if s.CandidateCount < 0 {
		s.CandidateCount = 0
	}
	s.ObservedRevision = strings.TrimSpace(s.ObservedRevision)
	return s
}

func (s RuntimeTargetSelection) Validate() error {
	s = s.Normalized()
	if s.PrerequisiteID == "" || s.SnapshotID == "" || s.OwnerModuleID == "" || s.TargetRole == "" || s.SelectorRef == "" || s.ObservedRevision == "" {
		return fmt.Errorf("runtime target selection requires identity, owner, role, selector and revision")
	}
	switch s.Disposition {
	case RuntimeTargetSelectionSelected:
		if s.CandidateCount != 1 || len(s.TargetRefs) != 1 || len(s.CandidateTargetRefs) != 1 || s.TargetRefs[0] != s.CandidateTargetRefs[0] {
			return fmt.Errorf("selected runtime target selection requires exactly one matching target")
		}
	case RuntimeTargetSelectionCreate:
		if s.CandidateCount != 0 || len(s.TargetRefs) != 0 || len(s.CandidateTargetRefs) != 0 {
			return fmt.Errorf("create runtime target selection requires an empty owner inventory")
		}
	case RuntimeTargetSelectionScope:
		if s.CandidateCount < 2 || len(s.TargetRefs) != s.CandidateCount || len(s.CandidateTargetRefs) != s.CandidateCount || !sameStrings(s.TargetRefs, s.CandidateTargetRefs) {
			return fmt.Errorf("scope runtime target selection requires a complete bounded target set")
		}
	case RuntimeTargetSelectionAbsent:
		if s.CandidateCount != 0 || len(s.TargetRefs) != 0 || len(s.CandidateTargetRefs) != 0 {
			return fmt.Errorf("absent runtime target selection cannot contain targets")
		}
	case RuntimeTargetSelectionAmbiguous:
		if s.CandidateCount < 2 || len(s.TargetRefs) != 0 || len(s.CandidateTargetRefs) == 0 || len(s.CandidateTargetRefs) > s.CandidateCount {
			return fmt.Errorf("ambiguous runtime target selection requires bounded candidate targets and no selected target")
		}
	default:
		return fmt.Errorf("runtime target selection has invalid disposition %q", s.Disposition)
	}
	if expected := StableRuntimeTargetSelectionSnapshotID(s); s.SnapshotID != expected {
		return fmt.Errorf("runtime target selection snapshot identity mismatch")
	}
	return nil
}

func StableRuntimeTargetSelectionSnapshotID(value RuntimeTargetSelection) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, value.PrerequisiteID)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.TargetRole)
	writeHashString(h, value.SelectorRef)
	writeHashString(h, string(value.Disposition))
	writeHashStrings(h, value.TargetRefs)
	writeHashStrings(h, value.CandidateTargetRefs)
	writeHashString(h, fmt.Sprintf("%d", value.CandidateCount))
	writeHashString(h, value.ObservedRevision)
	return "target-selection-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func (s TargetSnapshot) Complete() bool {
	return s.ContentState == SnapshotContentStateComplete && s.ContentHash != ""
}

type FailureTarget struct {
	Signature         string
	TargetRefs        []string
	TargetObservation *RuntimeFailureTargetObservation
}

type ValidationScratch struct {
	Reference string
	Revision  string
}

// ProviderCeiling seals the provider/model capability budget for one Runtime V2 Run.
// Runtime core only checks identity, positive ceilings, and revision presence.
// Budget derivation remains owned by blocks/llm.
type ProviderCeiling struct {
	Target                   ProviderModelTarget
	ContextWindowTokens      int
	ProviderMaxOutputTokens  int
	EffectiveMaxOutputTokens int
	MaxInputTokens           int
	MaxBytes                 int
	CapabilityRevision       string
	ConfigurationRevision    string
}

func (c ProviderCeiling) Normalized() ProviderCeiling {
	c.Target = c.Target.Normalized()
	c.CapabilityRevision = strings.TrimSpace(c.CapabilityRevision)
	c.ConfigurationRevision = strings.TrimSpace(c.ConfigurationRevision)
	return c
}

func (c ProviderCeiling) Validate() error {
	c = c.Normalized()
	if err := c.Target.Validate(); err != nil {
		return fmt.Errorf("provider ceiling target: %w", err)
	}
	if c.ContextWindowTokens <= 0 || c.ProviderMaxOutputTokens <= 0 || c.EffectiveMaxOutputTokens <= 0 || c.MaxInputTokens <= 0 || c.MaxBytes <= 0 {
		return fmt.Errorf("provider ceiling requires positive token and byte ceilings")
	}
	if c.MaxInputTokens+c.EffectiveMaxOutputTokens > c.ContextWindowTokens {
		return fmt.Errorf("provider ceiling max input plus effective max output exceeds context window")
	}
	if c.CapabilityRevision == "" || c.ConfigurationRevision == "" {
		return fmt.Errorf("provider ceiling requires capability and configuration revision")
	}
	return nil
}

func SameProviderCeiling(left *ProviderCeiling, right *ProviderCeiling) bool {
	if left == nil || right == nil {
		return left == right
	}
	l := left.Normalized()
	r := right.Normalized()
	return l.Target.Same(r.Target) &&
		l.ContextWindowTokens == r.ContextWindowTokens &&
		l.ProviderMaxOutputTokens == r.ProviderMaxOutputTokens &&
		l.EffectiveMaxOutputTokens == r.EffectiveMaxOutputTokens &&
		l.MaxInputTokens == r.MaxInputTokens &&
		l.MaxBytes == r.MaxBytes &&
		l.CapabilityRevision == r.CapabilityRevision &&
		l.ConfigurationRevision == r.ConfigurationRevision
}

type RequiredOutputContract struct {
	Kind                      RequiredOutputKind
	AcceptedKinds             []string
	AcceptedOperationVariants []RuntimeOperationVariantRef
	// AcceptedEffectScopes are the effect scopes that may appear in a sealed candidate.
	// RequiredEffectScopes must be satisfied by at least one operation for state-change.
	AcceptedEffectScopes      []RuntimeEffectScope
	RequiredEffectScopes      []RuntimeEffectScope
	RequiredEvidenceKinds     []string
	CoverageWorkItemIDs       []common.CoverageWorkItemID
	TargetRefs                []string
	PrerequisiteIDs           []string
	RequiredOperationSequence []RuntimeRequiredOperationVariant
}

type AuthoritySnapshot struct {
	ID                        string
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	State                     ConvergenceState
	Action                    RuntimeAction
	FocusID                   string
	RequiredOutputHash        string
	DefinitionRevision        string
	EvidenceRevision          string
	CapabilityStateHash       string
	PrerequisiteFreshnessHash string
	OperationContractRevision string
	Attempt                   int
	RuntimeContractVersion    string
}

type TerminalOutcome struct {
	Code    string
	Summary string
}

type RuntimeDecision struct {
	State             ConvergenceState
	Action            RuntimeAction
	Focus             CoverageFocus
	RequiredOutput    RequiredOutputContract
	Prerequisites     []RuntimePrerequisite
	AuthoritySnapshot AuthoritySnapshot
	Terminal          *TerminalOutcome
}

type AuthorityTransaction struct {
	AuthoritySnapshotID       string
	FocusID                   string
	Action                    RuntimeAction
	RequiredOutputHash        string
	DefinitionRevision        string
	EvidenceRevision          string
	CapabilityStateHash       string
	PrerequisiteFreshnessHash string
	OperationContractRevision string
	Attempt                   int
}

func (f RuntimeFacts) Normalized() RuntimeFacts {
	f.DefinitionRevision = strings.TrimSpace(f.DefinitionRevision)
	f.EvidenceRevision = strings.TrimSpace(f.EvidenceRevision)
	f.Focus = f.Focus.Normalized()
	f.RemainingFocuses = normalizeCoverageFocusQueue(f.RemainingFocuses)
	f.Evidence.Accepted = normalizeEvidenceFacts(f.Evidence.Accepted)
	f.Evidence.Rejected = normalizeEvidenceFacts(f.Evidence.Rejected)
	f.Assessments = normalizeRuntimeAssessmentFacts(f.Assessments)
	f.Prerequisites.Pending = normalizeRuntimePrerequisites(f.Prerequisites.Pending)
	f.Prerequisites.Satisfied = normalizeRuntimePrerequisites(f.Prerequisites.Satisfied)
	f.Capabilities.StateChangeKinds = normalizeStrings(f.Capabilities.StateChangeKinds)
	f.Capabilities.ValidationKinds = normalizeStrings(f.Capabilities.ValidationKinds)
	f.Capabilities.GovernanceKinds = normalizeStrings(f.Capabilities.GovernanceKinds)
	f.Resources = f.Resources.Normalized()
	f.Activation = f.Activation.Normalized()
	f.StateChange = f.StateChange.Normalized()
	f.ConvergencePolicy = f.ConvergencePolicy.Normalized()
	f.RecoveryPolicy = f.RecoveryPolicy.Normalized()
	f.Recovery = f.Recovery.Normalized()
	f.Repair = f.Repair.Normalized()
	f.FreshnessReacquireCoverage = normalizeCoverageWorkItemIDs(f.FreshnessReacquireCoverage)
	f.Coverage = NormalizeRuntimeCoverageFacts(f.Coverage)
	f.Slices = NormalizeRuntimeAcceptanceSlices(f.Slices)
	if f.Feasibility != nil {
		value := *f.Feasibility
		value.FocusID = strings.TrimSpace(value.FocusID)
		value.Action = RuntimeAction(strings.TrimSpace(string(value.Action)))
		value.RequiredOperationKinds = normalizeStrings(value.RequiredOperationKinds)
		value.FeasibleOperationKinds = normalizeStrings(value.FeasibleOperationKinds)
		value.MissingCapabilityKinds = normalizeStrings(value.MissingCapabilityKinds)
		value.Reason = strings.TrimSpace(value.Reason)
		f.Feasibility = &value
	}
	if f.Failure != nil {
		failure := f.Failure.Normalized()
		f.Failure = &failure
	}
	if f.RepairDirective != nil {
		directive := f.RepairDirective.Normalized()
		f.RepairDirective = &directive
	}
	return f
}

func (f RuntimeFacts) Validate() error {
	f = f.Normalized()
	if f.WorkspaceID == "" || f.RunID == "" {
		return fmt.Errorf("runtime facts require workspace and run scope")
	}
	if f.DefinitionRevision == "" || f.EvidenceRevision == "" {
		return fmt.Errorf("runtime facts require definition and evidence revisions")
	}
	if f.Focus.ID == "" {
		return fmt.Errorf("runtime facts require a coverage focus")
	}
	if err := f.Activation.Validate(); err != nil {
		return err
	}
	if err := f.RecoveryPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts recovery policy: %w", err)
	}
	if err := f.Recovery.Validate(f.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime facts recovery ledger: %w", err)
	}
	if f.Resources.TargetSelection != nil {
		if err := f.Resources.TargetSelection.Validate(); err != nil {
			return fmt.Errorf("runtime facts target selection: %w", err)
		}
	}
	if f.Failure != nil {
		if err := ValidateRuntimeFailureTargetObservationBinding(*f.Failure); err != nil {
			return fmt.Errorf("runtime facts failure target observation: %w", err)
		}
	}
	if f.Resources.FailureTarget != nil && f.Resources.FailureTarget.TargetObservation != nil {
		bound := FailureFact{TargetRefs: f.Resources.FailureTarget.TargetRefs, TargetObservation: f.Resources.FailureTarget.TargetObservation}
		if err := ValidateRuntimeFailureTargetObservationBinding(bound); err != nil {
			return fmt.Errorf("runtime facts failure target resource observation: %w", err)
		}
	}
	for _, snapshot := range f.Resources.CapabilityStates {
		if snapshot.RequirementID == "" || snapshot.SnapshotID == "" || snapshot.AcquisitionKey == "" || snapshot.QueryReceiptRef == "" || snapshot.OwnerModuleID == "" || snapshot.CapabilityStateKind == "" || snapshot.SubjectRef == "" || snapshot.ObservedRevision == "" || snapshot.ContentHash == "" {
			return fmt.Errorf("runtime capability-state snapshot requires identity, owner, subject and revision")
		}
		switch snapshot.Disposition {
		case RuntimeCapabilityStateReady:
			if len(snapshot.RequiredOperationSequence) != 0 {
				return fmt.Errorf("ready capability-state snapshot cannot require operations")
			}
		case RuntimeCapabilityStateSequenceRequired:
			if len(snapshot.RequiredOperationSequence) == 0 {
				return fmt.Errorf("capability-state sequence disposition requires operations")
			}
		case RuntimeCapabilityStateGovernance, RuntimeCapabilityStateUnavailable, RuntimeCapabilityStateUnknown:
			if len(snapshot.RequiredOperationSequence) != 0 {
				return fmt.Errorf("non-ready capability-state snapshot cannot carry required operations")
			}
		default:
			return fmt.Errorf("unknown capability-state disposition %q", snapshot.Disposition)
		}
	}
	for _, admissibility := range f.Resources.OperationAdmissibilities {
		if err := admissibility.Validate(); err != nil {
			return fmt.Errorf("runtime operation admissibility: %w", err)
		}
	}
	for _, observation := range f.Resources.PrerequisiteObservations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("runtime prerequisite observation: %w", err)
		}
		matched := false
		for _, prerequisite := range f.Prerequisites.Satisfied {
			if prerequisite.ID == observation.PrerequisiteID && prerequisite.Kind == observation.Kind && prerequisite.OwnerModuleID == observation.OwnerModuleID {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("runtime prerequisite observation %q has no matching satisfied prerequisite", observation.PrerequisiteID)
		}
	}
	seenFocus := map[string]struct{}{f.Focus.ID: {}}
	for _, focus := range f.RemainingFocuses {
		if focus.ID == "" {
			return fmt.Errorf("runtime facts remaining focus requires identity")
		}
		if _, ok := seenFocus[focus.ID]; ok {
			return fmt.Errorf("runtime facts coverage focus %s is duplicated", focus.ID)
		}
		seenFocus[focus.ID] = struct{}{}
	}
	coverage := map[common.CoverageWorkItemID]struct{}{}
	for _, item := range f.Coverage {
		coverage[item.ID] = struct{}{}
	}
	for _, focus := range append([]CoverageFocus{f.Focus}, f.RemainingFocuses...) {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := coverage[id]; !ok {
				return fmt.Errorf("runtime facts focus coverage item %s has no coverage ledger entry", id)
			}
		}
	}
	if f.RepairDirective != nil {
		if err := f.RepairDirective.Validate(); err != nil {
			return fmt.Errorf("runtime facts repair directive: %w", err)
		}
		if f.RepairDirective.FocusID != f.Focus.ID {
			return fmt.Errorf("runtime facts repair directive focus %s does not match current focus %s", f.RepairDirective.FocusID, f.Focus.ID)
		}
	}
	for _, assessment := range f.Assessments {
		if assessment.ID == "" || assessment.FocusID == "" {
			return fmt.Errorf("runtime assessment fact requires identity and focus")
		}
		if _, err := ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap); err != nil {
			return fmt.Errorf("runtime assessment fact %s: %w", assessment.ID, err)
		}
	}
	return nil
}

// AcceptedEvidenceRefsForFocus returns the canonical reference set that an
// assessment may cite for the current focus. Rendering and commit validation
// must use this same helper to avoid two evidence namespaces.
func AcceptedEvidenceRefsForFocus(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	ret := []string{}
	for _, evidence := range facts.Evidence.Accepted {
		if len(facts.Focus.RequiredEvidenceKinds) > 0 && !EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		ret = append(ret, string(evidence.ID), evidence.SourceRef)
		ret = append(ret, evidence.EvidenceRefs...)
	}
	return normalizeStrings(ret)
}

func (f CoverageFocus) Normalized() CoverageFocus {
	f.ID = strings.TrimSpace(f.ID)
	f.SliceID = strings.TrimSpace(f.SliceID)
	f.AssessmentScope = strings.TrimSpace(f.AssessmentScope)
	f.AssessmentID = strings.TrimSpace(f.AssessmentID)
	f.AssessmentText = strings.TrimSpace(f.AssessmentText)
	f.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(f.CoverageWorkItemIDs)
	f.RequiredEvidenceKinds = normalizeStrings(f.RequiredEvidenceKinds)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.EvidenceRole = strings.TrimSpace(f.EvidenceRole)
	f.TargetRole = strings.TrimSpace(f.TargetRole)
	f.TargetDiscoverySources = normalizeStrings(f.TargetDiscoverySources)
	f.TargetSelectorRef = strings.TrimSpace(f.TargetSelectorRef)
	f.AcceptedStepKinds = normalizeStrings(f.AcceptedStepKinds)
	f.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(f.AcceptedOperationVariants)
	f.RejectedStepKinds = normalizeStrings(f.RejectedStepKinds)
	return f
}

// ValidateRuntimeTargetGrounding validates a declarative target contract. It
// deliberately does not inspect a workspace or resolve a target: those are
// owner observations and SM-2 decisions, respectively.
func ValidateRuntimeTargetGrounding(required bool, role string, targetRefs, discoverySources []string, selectorRef string) error {
	if !required {
		return nil
	}
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("target grounding requires a target role")
	}
	targetRefs = normalizeStrings(targetRefs)
	discoverySources = normalizeStrings(discoverySources)
	selectorRef = strings.TrimSpace(selectorRef)
	if len(targetRefs) == 0 && len(discoverySources) == 0 {
		return fmt.Errorf("target grounding requires target refs or bounded discovery sources")
	}
	if len(targetRefs) == 0 && selectorRef == "" {
		return fmt.Errorf("target grounding without explicit target refs requires an owner target selector")
	}
	for _, source := range discoverySources {
		switch source {
		case RuntimeValidationDiscoverySourceWorkspaceManifest,
			RuntimeValidationDiscoverySourceProjectFiles,
			RuntimeValidationDiscoverySourceDeclaredCommands,
			RuntimeValidationDiscoverySourceAvailableCapabilities:
			// Canonical source vocabulary only. A caller cannot make an
			// unbounded or fictional discovery source look grounded.
		default:
			return fmt.Errorf("target grounding declares unknown discovery source %q", source)
		}
	}
	return nil
}

func (r RuntimeResourceFacts) Normalized() RuntimeResourceFacts {
	if r.Manifest != nil {
		manifest := *r.Manifest
		manifest.Revision = strings.TrimSpace(manifest.Revision)
		manifest.TargetRefs = normalizeStrings(manifest.TargetRefs)
		r.Manifest = &manifest
	}
	r.TargetSnapshots = normalizeTargetSnapshots(r.TargetSnapshots)
	if r.TargetSelection != nil {
		selection := r.TargetSelection.Normalized()
		r.TargetSelection = &selection
	}
	if r.FailureTarget != nil {
		target := *r.FailureTarget
		target.Signature = strings.TrimSpace(target.Signature)
		target.TargetRefs = normalizeStrings(target.TargetRefs)
		if target.TargetObservation != nil {
			observation := target.TargetObservation.Normalized()
			target.TargetObservation = &observation
		}
		r.FailureTarget = &target
	}
	if r.ValidationScratch != nil {
		scratch := *r.ValidationScratch
		scratch.Reference = strings.TrimSpace(scratch.Reference)
		scratch.Revision = strings.TrimSpace(scratch.Revision)
		r.ValidationScratch = &scratch
	}
	if r.ProviderCeiling != nil {
		ceiling := r.ProviderCeiling.Normalized()
		r.ProviderCeiling = &ceiling
	}
	r.CapabilityStates = NormalizeRuntimeCapabilityStateSnapshots(r.CapabilityStates)
	r.OperationAdmissibilities = NormalizeRuntimeOperationAdmissibilities(r.OperationAdmissibilities)
	r.PrerequisiteObservations = NormalizeRuntimePrerequisiteObservations(r.PrerequisiteObservations)
	return r
}

func NormalizeRuntimeOperationAdmissibilities(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
	byPrerequisite := make(map[string]RuntimeOperationAdmissibility, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.PrerequisiteID != "" {
			byPrerequisite[value.PrerequisiteID] = value
		}
	}
	keys := make([]string, 0, len(byPrerequisite))
	for key := range byPrerequisite {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeOperationAdmissibility, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byPrerequisite[key])
	}
	return nilIfEmpty(ret)
}

func NormalizeRuntimeCapabilityStateSnapshots(values []RuntimeCapabilityStateSnapshot) []RuntimeCapabilityStateSnapshot {
	byRequirement := make(map[string]RuntimeCapabilityStateSnapshot, len(values))
	for _, value := range values {
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.SnapshotID = strings.TrimSpace(value.SnapshotID)
		value.AcquisitionKey = strings.TrimSpace(value.AcquisitionKey)
		value.AcquisitionBinding = strings.TrimSpace(value.AcquisitionBinding)
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.ObservedRevision = strings.TrimSpace(value.ObservedRevision)
		value.WorkspaceTreeRevision = strings.TrimSpace(value.WorkspaceTreeRevision)
		value.ContentHash = strings.TrimSpace(value.ContentHash)
		value.ObservedAt = strings.TrimSpace(value.ObservedAt)
		value.QueryReceiptRef = strings.TrimSpace(value.QueryReceiptRef)
		value.InvalidatedByReceiptRefs = normalizeStrings(value.InvalidatedByReceiptRefs)
		value.Disposition = RuntimeCapabilityStateDisposition(strings.TrimSpace(string(value.Disposition)))
		value.RequiredOperationSequence = normalizeRequiredOperationVariants(value.RequiredOperationSequence)
		if value.RequirementID != "" {
			byRequirement[value.RequirementID] = value
		}
	}
	keys := make([]string, 0, len(byRequirement))
	for key := range byRequirement {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeCapabilityStateSnapshot, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byRequirement[key])
	}
	return nilIfEmpty(ret)
}

func StableRuntimeCapabilityStateHash(values []RuntimeCapabilityStateSnapshot) string {
	values = NormalizeRuntimeCapabilityStateSnapshots(values)
	h := sha256.New()
	for _, value := range values {
		writeHashString(h, value.RequirementID)
		writeHashString(h, value.SnapshotID)
		writeHashString(h, value.AcquisitionKey)
		writeHashString(h, value.AcquisitionBinding)
		writeHashString(h, value.OwnerModuleID)
		writeHashString(h, value.CapabilityStateKind)
		writeHashString(h, value.SubjectRef)
		writeHashString(h, value.ObservedRevision)
		writeHashString(h, value.WorkspaceTreeRevision)
		writeHashString(h, value.ContentHash)
		writeHashString(h, string(value.Disposition))
		writeHashString(h, value.QueryReceiptRef)
		for _, required := range value.RequiredOperationSequence {
			writeHashString(h, required.RequirementID)
			writeHashString(h, required.Kind)
			writeHashString(h, required.VariantID)
			writeHashStrings(h, required.DependsOnRequirementIDs)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func StableRuntimeCapabilityStateSnapshotID(value RuntimeCapabilityStateSnapshot) string {
	value = NormalizeRuntimeCapabilityStateSnapshots([]RuntimeCapabilityStateSnapshot{value})[0]
	h := sha256.New()
	writeHashString(h, value.AcquisitionKey)
	writeHashString(h, value.AcquisitionBinding)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.CapabilityStateKind)
	writeHashString(h, value.SubjectRef)
	writeHashString(h, value.ObservedRevision)
	writeHashString(h, value.WorkspaceTreeRevision)
	writeHashString(h, value.ContentHash)
	return "capability-state-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func normalizeRequiredOperationVariants(values []RuntimeRequiredOperationVariant) []RuntimeRequiredOperationVariant {
	byID := make(map[string]RuntimeRequiredOperationVariant, len(values))
	for _, value := range values {
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		value.DependsOnRequirementIDs = normalizeStrings(value.DependsOnRequirementIDs)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.RequirementID != "" {
			byID[value.RequirementID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeRequiredOperationVariant, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func (c RequiredOutputContract) Normalized() RequiredOutputContract {
	c.AcceptedKinds = normalizeStrings(c.AcceptedKinds)
	c.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(c.AcceptedOperationVariants)
	c.AcceptedEffectScopes = NormalizeRuntimeEffectScopes(c.AcceptedEffectScopes)
	c.RequiredEffectScopes = NormalizeRuntimeEffectScopes(c.RequiredEffectScopes)
	c.RequiredEvidenceKinds = normalizeStrings(c.RequiredEvidenceKinds)
	c.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.CoverageWorkItemIDs)
	c.TargetRefs = normalizeStrings(c.TargetRefs)
	c.PrerequisiteIDs = normalizeStrings(c.PrerequisiteIDs)
	c.RequiredOperationSequence = normalizeRequiredOperationVariants(c.RequiredOperationSequence)
	return c
}
func (c RequiredOutputContract) Validate() error {
	c = c.Normalized()
	switch c.Kind {
	case RequiredOutputKindNone:
		if len(c.AcceptedKinds)+len(c.AcceptedOperationVariants)+len(c.RequiredEvidenceKinds)+len(c.CoverageWorkItemIDs)+len(c.TargetRefs)+len(c.PrerequisiteIDs)+len(c.RequiredOperationSequence)+len(c.AcceptedEffectScopes)+len(c.RequiredEffectScopes) != 0 {
			return fmt.Errorf("empty required output kind cannot carry output requirements")
		}
	case RequiredOutputKindPrerequisite:
		if len(c.PrerequisiteIDs) == 0 {
			return fmt.Errorf("prerequisite required output requires prerequisite ids")
		}
	case RequiredOutputKindStateChange, RequiredOutputKindValidation:
		if len(c.AcceptedKinds) == 0 {
			return fmt.Errorf("%s required output requires accepted kinds", c.Kind)
		}
		if len(c.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("%s required output requires coverage work items", c.Kind)
		}
		if len(c.RequiredEffectScopes) == 0 || len(c.AcceptedEffectScopes) == 0 {
			return fmt.Errorf("%s required output requires explicit required and accepted effect scopes", c.Kind)
		}
		action, ok := runtimeActionForRequiredOutput(c.Kind)
		if !ok {
			return fmt.Errorf("%s required output has no runtime action", c.Kind)
		}
		if err := ValidateEffectScopesCompatibleWithAction(action, c.RequiredEffectScopes); err != nil {
			return fmt.Errorf("%s required effect scopes: %w", c.Kind, err)
		}
		if err := ValidateEffectScopesCompatibleWithAction(action, c.AcceptedEffectScopes); err != nil {
			return fmt.Errorf("%s accepted effect scopes: %w", c.Kind, err)
		}
	case RequiredOutputKindGovernance:
		if len(c.AcceptedKinds) == 0 {
			return fmt.Errorf("governance required output requires accepted kinds")
		}
	case RequiredOutputKindAssessment:
		if len(c.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("assessment required output requires coverage work items")
		}
	default:
		return fmt.Errorf("unknown required output kind %q", c.Kind)
	}
	return nil
}

func StableRequiredOutputHash(contract RequiredOutputContract) string {
	contract = contract.Normalized()
	h := sha256.New()
	writeHashString(h, string(contract.Kind))
	writeHashStrings(h, contract.AcceptedKinds)
	for _, variant := range contract.AcceptedOperationVariants {
		writeHashString(h, variant.Kind)
		writeHashString(h, variant.VariantID)
	}
	writeHashStrings(h, RuntimeEffectScopesAsStrings(contract.AcceptedEffectScopes))
	writeHashStrings(h, RuntimeEffectScopesAsStrings(contract.RequiredEffectScopes))
	writeHashStrings(h, contract.RequiredEvidenceKinds)
	coverageIDs := make([]string, 0, len(contract.CoverageWorkItemIDs))
	for _, id := range contract.CoverageWorkItemIDs {
		coverageIDs = append(coverageIDs, string(id))
	}
	writeHashStrings(h, coverageIDs)
	writeHashStrings(h, contract.TargetRefs)
	writeHashStrings(h, contract.PrerequisiteIDs)
	for _, requirement := range contract.RequiredOperationSequence {
		writeHashString(h, requirement.RequirementID)
		writeHashString(h, requirement.Kind)
		writeHashString(h, requirement.VariantID)
		writeHashStrings(h, requirement.DependsOnRequirementIDs)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func SealRuntimeDecision(facts RuntimeFacts, decision RuntimeDecision, attempt int) (RuntimeDecision, error) {
	facts = facts.Normalized()
	if err := facts.Validate(); err != nil {
		return RuntimeDecision{}, err
	}
	decision.Focus = decision.Focus.Normalized()
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	decision.Prerequisites = normalizeRuntimePrerequisites(decision.Prerequisites)
	if decision.Terminal != nil {
		terminal := *decision.Terminal
		terminal.Code = strings.TrimSpace(terminal.Code)
		terminal.Summary = strings.TrimSpace(terminal.Summary)
		decision.Terminal = &terminal
	}
	requiredOutputHash := StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot = AuthoritySnapshot{
		WorkspaceID:               facts.WorkspaceID,
		RunID:                     facts.RunID,
		State:                     decision.State,
		Action:                    decision.Action,
		FocusID:                   decision.Focus.ID,
		RequiredOutputHash:        requiredOutputHash,
		DefinitionRevision:        facts.DefinitionRevision,
		EvidenceRevision:          facts.EvidenceRevision,
		CapabilityStateHash:       StableRuntimeCapabilityStateHash(facts.Resources.CapabilityStates),
		PrerequisiteFreshnessHash: StableRuntimePrerequisiteFreshnessHash(facts.Resources.PrerequisiteObservations),
		OperationContractRevision: facts.Activation.ExecutionContract.OperationContractRevision,
		Attempt:                   attempt,
		RuntimeContractVersion:    RuntimeContractVersionV2,
	}
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)
	if err := decision.Validate(); err != nil {
		return RuntimeDecision{}, err
	}
	return decision, nil
}

func (d RuntimeDecision) Validate() error {
	d.Focus = d.Focus.Normalized()
	d.RequiredOutput = d.RequiredOutput.Normalized()
	if d.Focus.ID == "" {
		return fmt.Errorf("runtime decision requires a coverage focus")
	}
	wantAction, wantOutput, terminal := convergenceStateContract(d.State)
	if wantAction == "" {
		return fmt.Errorf("unknown convergence state %q", d.State)
	}
	if d.Action != wantAction {
		return fmt.Errorf("convergence state %s requires action %s, got %s", d.State, wantAction, d.Action)
	}
	if d.RequiredOutput.Kind != wantOutput {
		return fmt.Errorf("convergence state %s requires output kind %s, got %s", d.State, wantOutput, d.RequiredOutput.Kind)
	}
	if err := d.RequiredOutput.Validate(); err != nil {
		return err
	}
	if terminal != (d.Terminal != nil) {
		return fmt.Errorf("convergence state %s terminal outcome mismatch", d.State)
	}
	if d.Terminal != nil && d.Terminal.Code == "" {
		return fmt.Errorf("terminal outcome requires a code")
	}
	if d.State == ConvergenceStateNeedPrerequisite && len(d.Prerequisites) == 0 {
		return fmt.Errorf("prerequisite decision requires pending prerequisites")
	}
	if d.State != ConvergenceStateNeedPrerequisite && len(d.Prerequisites) != 0 {
		return fmt.Errorf("only prerequisite decisions may dispatch prerequisites")
	}
	return validateAuthoritySnapshot(d)
}

func (d RuntimeDecision) AuthorityTransaction() AuthorityTransaction {
	snapshot := d.AuthoritySnapshot
	return AuthorityTransaction{
		AuthoritySnapshotID:       snapshot.ID,
		FocusID:                   snapshot.FocusID,
		Action:                    snapshot.Action,
		RequiredOutputHash:        snapshot.RequiredOutputHash,
		DefinitionRevision:        snapshot.DefinitionRevision,
		EvidenceRevision:          snapshot.EvidenceRevision,
		CapabilityStateHash:       snapshot.CapabilityStateHash,
		PrerequisiteFreshnessHash: snapshot.PrerequisiteFreshnessHash,
		OperationContractRevision: snapshot.OperationContractRevision,
		Attempt:                   snapshot.Attempt,
	}
}

func convergenceStateContract(state ConvergenceState) (RuntimeAction, RequiredOutputKind, bool) {
	switch state {
	case ConvergenceStateNeedPrerequisite:
		return RuntimeActionAcquirePrerequisite, RequiredOutputKindPrerequisite, false
	case ConvergenceStateNeedStateChange:
		return RuntimeActionProduceStateChange, RequiredOutputKindStateChange, false
	case ConvergenceStateNeedValidation:
		return RuntimeActionProduceValidation, RequiredOutputKindValidation, false
	case ConvergenceStateNeedAssessment:
		return RuntimeActionProduceAssessment, RequiredOutputKindAssessment, false
	case ConvergenceStateNeedGovernance:
		return RuntimeActionRequestGovernance, RequiredOutputKindGovernance, false
	case ConvergenceStateComplete:
		return RuntimeActionComplete, RequiredOutputKindNone, true
	case ConvergenceStateFailed:
		return RuntimeActionFail, RequiredOutputKindNone, true
	default:
		return "", "", false
	}
}

func validateAuthoritySnapshot(d RuntimeDecision) error {
	s := d.AuthoritySnapshot
	if s.ID == "" || s.WorkspaceID == "" || s.RunID == "" || s.DefinitionRevision == "" || s.EvidenceRevision == "" {
		return fmt.Errorf("runtime decision requires a sealed authority snapshot")
	}
	if s.RuntimeContractVersion != RuntimeContractVersionV2 {
		return fmt.Errorf("authority snapshot requires runtime contract version %s", RuntimeContractVersionV2)
	}
	if s.Attempt < 0 {
		return fmt.Errorf("authority snapshot attempt cannot be negative")
	}
	if s.State != d.State || s.Action != d.Action || s.FocusID != d.Focus.ID {
		return fmt.Errorf("authority snapshot does not match decision authority")
	}
	if s.RequiredOutputHash != StableRequiredOutputHash(d.RequiredOutput) {
		return fmt.Errorf("authority snapshot required output hash mismatch")
	}
	if s.ID != stableAuthoritySnapshotID(s) {
		return fmt.Errorf("authority snapshot id mismatch")
	}
	return nil
}

func stableAuthoritySnapshotID(snapshot AuthoritySnapshot) string {
	h := sha256.New()
	writeHashString(h, string(snapshot.WorkspaceID))
	writeHashString(h, string(snapshot.RunID))
	writeHashString(h, string(snapshot.State))
	writeHashString(h, string(snapshot.Action))
	writeHashString(h, snapshot.FocusID)
	writeHashString(h, snapshot.RequiredOutputHash)
	writeHashString(h, snapshot.DefinitionRevision)
	writeHashString(h, snapshot.EvidenceRevision)
	writeHashString(h, snapshot.CapabilityStateHash)
	if snapshot.PrerequisiteFreshnessHash != "" {
		writeHashString(h, snapshot.PrerequisiteFreshnessHash)
	}
	writeHashString(h, snapshot.OperationContractRevision)
	writeHashString(h, fmt.Sprintf("%d", snapshot.Attempt))
	writeHashString(h, snapshot.RuntimeContractVersion)
	return "authority-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type hashStringWriter interface {
	Write([]byte) (int, error)
}

func writeHashStrings(w hashStringWriter, values []string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(values)))
	_, _ = w.Write(size[:])
	for _, value := range values {
		writeHashString(w, value)
	}
}

func writeHashString(w hashStringWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write([]byte(value))
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// normalizeOrderedStrings trims and drops empty entries while preserving order
// and duplicates. Use for positional argument lists (e.g. shell args, git pathspecs)
// where sorting would change execution semantics.
func normalizeOrderedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeCoverageWorkItemIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	stringsValue := make([]string, 0, len(values))
	for _, value := range values {
		stringsValue = append(stringsValue, string(value))
	}
	stringsValue = normalizeStrings(stringsValue)
	ret := make([]common.CoverageWorkItemID, 0, len(stringsValue))
	for _, value := range stringsValue {
		ret = append(ret, common.CoverageWorkItemID(value))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeCoverageFocusQueue(values []CoverageFocus) []CoverageFocus {
	ret := make([]CoverageFocus, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeRuntimePrerequisites(values []RuntimePrerequisite) []RuntimePrerequisite {
	ret := make([]RuntimePrerequisite, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Kind = PrerequisiteKind(strings.TrimSpace(string(value.Kind)))
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
		value.RequiredCapabilities = normalizeStrings(value.RequiredCapabilities)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.ID == "" {
			continue
		}
		if _, ok := seen[value.ID]; ok {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, value)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeTargetSnapshots(values []TargetSnapshot) []TargetSnapshot {
	byTarget := make(map[string]TargetSnapshot, len(values))
	for _, value := range values {
		value.TargetRef = strings.TrimSpace(value.TargetRef)
		value.ContentHash = strings.TrimSpace(value.ContentHash)
		value.ObservedAt = strings.TrimSpace(value.ObservedAt)
		if value.TargetRef == "" {
			continue
		}
		byTarget[value.TargetRef] = value
	}
	refs := make([]string, 0, len(byTarget))
	for ref := range byTarget {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	ret := make([]TargetSnapshot, 0, len(refs))
	for _, ref := range refs {
		ret = append(ret, byTarget[ref])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeEvidenceFacts(values []EvidenceFact) []EvidenceFact {
	ret := make([]EvidenceFact, 0, len(values))
	seen := map[common.EvidenceRecordID]struct{}{}
	for _, value := range values {
		value.Kind = strings.TrimSpace(value.Kind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.SourceRef = strings.TrimSpace(value.SourceRef)
		value.AuthoritySnapshotID = strings.TrimSpace(value.AuthoritySnapshotID)
		value.DispatchID = strings.TrimSpace(value.DispatchID)
		value.OperationID = strings.TrimSpace(value.OperationID)
		value.Revision = strings.TrimSpace(value.Revision)
		value.FailureClass = FailureClass(strings.TrimSpace(string(value.FailureClass)))
		value.FailureCode = strings.TrimSpace(value.FailureCode)
		value.FailureSignature = strings.TrimSpace(value.FailureSignature)
		value.FailureMessage = strings.TrimSpace(value.FailureMessage)
		value.Summary = strings.TrimSpace(value.Summary)
		value.ContentHash = strings.TrimSpace(value.ContentHash)
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.TargetRefs = normalizeStrings(value.TargetRefs)
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
		value.Quality = common.EvidenceQuality(strings.TrimSpace(string(value.Quality)))
		value.EvidenceReason = strings.TrimSpace(value.EvidenceReason)
		value.EvidenceTags = normalizeStrings(value.EvidenceTags)
		if value.Validation != nil {
			observation := value.Validation.Clone()
			value.Validation = &observation
		}
		if value.ExitCode != nil {
			exitCode := *value.ExitCode
			value.ExitCode = &exitCode
		}
		if value.ID == "" {
			continue
		}
		if _, ok := seen[value.ID]; ok {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, value)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeRuntimeAssessmentFacts(values []RuntimeAssessmentFact) []RuntimeAssessmentFact {
	ret := make([]RuntimeAssessmentFact, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.FocusID = strings.TrimSpace(value.FocusID)
		value.SliceID = strings.TrimSpace(value.SliceID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.RequirementText = strings.TrimSpace(value.RequirementText)
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
		value.Disposition = value.Disposition.Normalized()
		if len(value.Items) > 0 {
			items := make([]RuntimeAssessmentItemFact, 0, len(value.Items))
			for _, item := range value.Items {
				item = item.Normalized()
				if item.CoverageWorkItemID != "" {
					items = append(items, item)
				}
			}
			value.Items = items
		}
		if value.Gap != nil {
			gap := value.Gap.Normalized()
			value.Gap = &gap
		}
		value.Summary = strings.TrimSpace(value.Summary)
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.ArtifactRefs = normalizeStrings(value.ArtifactRefs)
		if value.FocusID == "" {
			continue
		}
		key := value.FocusID
		if value.ID != "" {
			key = value.ID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, value)
	}
	return nilIfEmpty(ret)
}
