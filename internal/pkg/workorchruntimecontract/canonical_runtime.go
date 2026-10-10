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

type CoverageFocus struct {
	ID                  string
	SliceID             string
	AssessmentScope     string
	AssessmentID        string
	AssessmentText      string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	// CoverageEvidenceBindings are SM-2-sealed, generation-specific proof
	// bindings for this focus's exact CWI set. They are not a coverage ledger;
	// only SM-2 may convert a sealed-operation receipt match into CWI credit.
	CoverageEvidenceBindings []CoverageEvidenceBinding
	// EvidenceDeclarationRef records the profile/template declaration that
	// supplied semantic proof for this focus.  It is opaque to Runtime and
	// exists solely so activation can prove that validation/behavior proof was
	// declared for this exact CWI rather than copied from a wildcard quality
	// gate.
	EvidenceDeclarationRef string
	RequiredEvidenceKinds  []string
	TargetRefs             []string
	EvidenceRole           string
	TargetRole             string
	TargetDiscoverySources []string
	// TargetSelectorRef is an opaque, profile-declared selector identity. The
	// owning capability validates and applies it; Runtime only seals and
	// compares the binding.
	TargetSelectorRef           string
	TargetSelectorOwnerRef      string
	TargetSelectorSubjectRef    string
	TargetSelectorCandidateRefs []string
	// ProspectiveTargetAllOfConstraintRefs is the immutable Definition-side
	// constraint set. Every ref is mandatory; it never contains or selects an
	// exact prospective target.
	ProspectiveTargetAllOfConstraintRefs []string
	// ProspectiveTargetRefs is the complete bounded target set sealed from one
	// semantic prospective proposal. It is empty for existing-target focuses.
	ProspectiveTargetRefs             []string
	RequireTargetGrounding            bool
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []RuntimeOperationVariantRef
	RejectedStepKinds                 []string
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	RequireCoverageTargets            bool
	RequireAssessment                 bool
	VerificationPolicy                *RuntimeVerificationPolicy `json:",omitempty"`
	// RequiredOwnerFacts are profile-declared postconditions evaluated only
	// from current capability-owner catalogs when SM-2 commits assessment.
	// They do not grant an operation or let another component mutate owner state.
	RequiredOwnerFacts    []RuntimeOwnerFactRequirement
	OwnerFactConvergences []RuntimeOwnerFactConvergence
	ValidationRecipeID    string
	RequiredCheckRefs     []string
	MetricConstraints     []string
	ArtifactRefs          []string
}

type EvidenceLedger struct {
	Accepted []EvidenceFact
	Rejected []EvidenceFact
}

type RuntimeAssessmentFact struct {
	ID string
	// Revision is the one global Runtime evidence-ledger position consumed by
	// this committed assessment dispatch.  Assessments remain their own typed
	// SM-2 ledger; this field only makes their ordering share the same
	// restart-verifiable sequence as operation and governance evidence.
	Revision string
	// AdjudicationHash is the fact side of the committed assessment dispatch's
	// post-seal canonical classification proof.
	AdjudicationHash    string
	FocusID             string
	SliceID             string
	Scope               string
	RequirementID       string
	RequirementText     string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	// ClaimedDisposition is the producer's original assessment observation. It
	// never chooses an SM-2 action, retry, governance state or terminal.
	ClaimedDisposition AssessmentDisposition
	// Disposition is the canonical owner-derived classification after the
	// assessment evidence/coverage gate. RuntimeRepairDirective remains the
	// sole persisted next-action authority.
	Disposition  AssessmentDisposition
	Items        []RuntimeAssessmentItemFact
	Gap          *RuntimeAssessmentGap
	Summary      string
	EvidenceRefs []string
	ArtifactRefs []string
}

type EvidenceFact struct {
	ID   common.EvidenceRecordID
	Kind string
	// FocusID is the reducer-derived immutable SM-2 focus identity of an
	// ordinary operation receipt fact. It is copied from the owning sealed
	// dispatch and never inferred from the mutable current-focus projection.
	FocusID             string
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
	ContentAddress      *RuntimeEvidenceContentAddress
	ExitCode            *int64
	EvidenceRefs        []string
	// TargetRefs are the sealed operation targets that produced this fact. They
	// preserve owner-validated target lineage for SM-2 comparisons; the contract
	// package does not use them to choose a transition.
	TargetRefs []string
	// BindingRefs are copied only from the sealed operation authorization. CWI
	// credit remains a reducer-derived field below, never candidate input.
	BindingRefs         []string
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
	ContentAddress *RuntimeEvidenceContentAddress
	ExitCode       *int64
	EvidenceRefs   []string
	Quality        common.EvidenceQuality
	ZeroWork       bool
	EvidenceReason string
	EvidenceTags   []string
	Validation     *RuntimeValidationObservation
}

type RuntimeEvidenceDigestAlgorithm string

const RuntimeEvidenceDigestSHA256 RuntimeEvidenceDigestAlgorithm = "sha256"

// RuntimeEvidenceContentAddress binds a normalized evidence digest to the
// capability owner's opaque retrieval reference. Runtime validates the typed
// digest/ref relation and never parses an owner-specific URI scheme.
type RuntimeEvidenceContentAddress struct {
	Algorithm RuntimeEvidenceDigestAlgorithm
	Digest    string
	Ref       string
}

func (a RuntimeEvidenceContentAddress) Normalized() RuntimeEvidenceContentAddress {
	a.Algorithm = RuntimeEvidenceDigestAlgorithm(strings.TrimSpace(string(a.Algorithm)))
	a.Digest = strings.ToLower(strings.TrimSpace(a.Digest))
	a.Ref = strings.TrimSpace(a.Ref)
	return a
}

func (a RuntimeEvidenceContentAddress) Validate() error {
	a = a.Normalized()
	if a.Algorithm != RuntimeEvidenceDigestSHA256 || len(a.Digest) != sha256.Size*2 || a.Ref == "" {
		return fmt.Errorf("runtime evidence content address requires sha256 digest and opaque ref")
	}
	if _, err := hex.DecodeString(a.Digest); err != nil {
		return fmt.Errorf("runtime evidence content address digest must be lowercase hex: %w", err)
	}
	return nil
}

func (a RuntimeEvidenceContentAddress) Clone() RuntimeEvidenceContentAddress { return a.Normalized() }

type RuntimeValidationProofClass string

const (
	RuntimeValidationProofStatic     RuntimeValidationProofClass = "static"
	RuntimeValidationProofExecuted   RuntimeValidationProofClass = "executed"
	RuntimeValidationProofBehavioral RuntimeValidationProofClass = "behavioral"
)

// RuntimeValidationObservation is an owner-reported, domain-neutral summary of
// validation work. Capability owners may derive it from tool-specific output,
// while Orchestrator only consumes the typed quality and check identities.
type RuntimeValidationObservation struct {
	// RecipeID records the owner-executed validation recipe identity. It is
	// audit/evidence data only; the Orchestrator continues to decide action and
	// terminal state from canonical receipt facts.
	RecipeID string
	// ProofClass is an optional capability-owner attestation of the native
	// observation's modality. It does not itself grant a grade: SM-2 verifies
	// the exact receipt, sealed recipe, passed checks and execution scope.
	// Empty preserves existing receipt/evidence identities.
	ProofClass RuntimeValidationProofClass `json:",omitempty"`
	// ObservationProtocolRef identifies the owner-native parser/protocol that
	// produced Executed/Passed/Failed/Skipped checks. Raw process output has no
	// protocol identity and cannot attest a Definition check.
	ObservationProtocolRef  string
	ExecutedChecks          []string
	PassedChecks            []string
	FailedChecks            []string
	SkippedChecks           []string
	OutputPreview           string
	CheckFailures           []RuntimeValidationCheckFailure `json:",omitempty"`
	FailureDetailsTruncated bool                            `json:",omitempty"`
	OutputTruncated         bool                            `json:",omitempty"`
	// Optional diagnostic protocol, not a native pass/fail protocol. Empty
	// retains the original receipt and request identity; never backfill it.
	FailureDetailProtocolRef string `json:",omitempty"`
	// IncompleteChecks identifies checks with skipped required subchecks. A
	// top-level pass does not prove that all of its behavior was exercised.
	IncompleteChecks []string `json:",omitempty"`
}

func (o RuntimeValidationObservation) Normalized() RuntimeValidationObservation {
	o.RecipeID = strings.TrimSpace(o.RecipeID)
	o.ProofClass = RuntimeValidationProofClass(strings.TrimSpace(string(o.ProofClass)))
	o.ObservationProtocolRef = strings.TrimSpace(o.ObservationProtocolRef)
	o.ExecutedChecks = normalizeStrings(o.ExecutedChecks)
	o.PassedChecks = normalizeStrings(o.PassedChecks)
	o.FailedChecks = normalizeStrings(o.FailedChecks)
	o.SkippedChecks = normalizeStrings(o.SkippedChecks)
	o.OutputPreview = strings.TrimSpace(o.OutputPreview)
	o.CheckFailures = normalizeRuntimeValidationCheckFailures(o.CheckFailures)
	o.IncompleteChecks = normalizeStrings(o.IncompleteChecks)
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

// RuntimeValidationCheckPassed reports an unambiguous owner-observed pass.
// A check that is also failed or skipped is contradictory evidence and must
// never be promoted to coverage merely because it appears in PassedChecks.
func RuntimeValidationCheckPassed(observation RuntimeValidationObservation, checkRef string) bool {
	observation = observation.Normalized()
	checkRef = strings.TrimSpace(checkRef)
	if checkRef == "" || !containsString(observation.PassedChecks, checkRef) {
		return false
	}
	return !observation.OutputTruncated && !containsString(observation.FailedChecks, checkRef) && !containsString(observation.SkippedChecks, checkRef) && !containsString(observation.IncompleteChecks, checkRef)
}

// RuntimeValidationConflictingChecks returns checks reported in more than one
// terminal outcome set. The result is stable and owner-neutral so capability
// owners and Runtime gates use the same contradiction rule.
func RuntimeValidationConflictingChecks(observation RuntimeValidationObservation) []string {
	observation = observation.Normalized()
	states := make(map[string]uint8, len(observation.PassedChecks)+len(observation.FailedChecks)+len(observation.SkippedChecks))
	for _, check := range observation.PassedChecks {
		states[check] |= 1
	}
	for _, check := range observation.FailedChecks {
		states[check] |= 2
	}
	for _, check := range observation.SkippedChecks {
		states[check] |= 4
	}
	ret := make([]string, 0)
	for check, state := range states {
		if state != 1 && state != 2 && state != 4 {
			ret = append(ret, check)
		}
	}
	sort.Strings(ret)
	return nilIfEmpty(ret)
}

type FailureClass string

const (
	FailureClassBuildFailure                       FailureClass = "build_failure"
	FailureClassCoverageAssessmentIncomplete       FailureClass = "coverage_assessment_incomplete"
	FailureClassGovernanceDependency               FailureClass = "governance_dependency"
	FailureClassRuntimeContractMisalignment        FailureClass = "runtime_internal_contract_misalignment"
	FailureClassExecutionFailure                   FailureClass = "execution_failure"
	FailureClassNoProgressExhausted                FailureClass = "no_progress_exhausted"
	FailureClassCandidateExhausted                 FailureClass = "candidate_retry_exhausted"
	FailureClassCandidateAuthorityExhausted        FailureClass = "candidate_authority_exhausted"
	FailureClassCandidateDocumentRecoveryExhausted FailureClass = "candidate_document_recovery_exhausted"
	FailureClassProviderProtocol                   FailureClass = "provider_protocol_failure"
	FailureClassProviderSubmissionUnknown          FailureClass = "provider_submission_unknown"
	FailureClassPrerequisiteRefreshExhausted       FailureClass = "prerequisite_refresh_exhausted"
	FailureClassGovernanceRejected                 FailureClass = "governance_rejected"
)

type RuntimeOperationExecutionStatus string

const (
	RuntimeOperationExecutionStatusAccepted RuntimeOperationExecutionStatus = "accepted"
	RuntimeOperationExecutionStatusRejected RuntimeOperationExecutionStatus = "rejected"
)

type FailureFact struct {
	Class     FailureClass
	Code      string
	Signature string
	Message   string
	// OriginalDiagnostic preserves the producing owner's safe diagnostic when
	// the stable failure fields above are reduced by Runtime authority.  It is
	// diagnostic-only: it cannot select a retry, repair action, target, ticket,
	// or terminal.  In particular, Run-owner semantic persistence rejection
	// must retain its complete typed diagnostic instead of being collapsed into
	// an error string while returning through the Orchestrator internal-event
	// path.
	OriginalDiagnostic   *RuntimeOriginalDiagnostic
	TargetRefs           []string
	TargetObservation    *RuntimeFailureTargetObservation
	ValidationDiagnostic *ValidationFailureDiagnostic
	// ValidationFailureObservation is a capability fact only. It remains
	// separate from TargetRefs until the sealed resource owner confirms the repair surface.
	ValidationFailureObservation *RuntimeValidationFailureObservation
	// GovernanceDependency is present only when an owner, prerequisite, or
	// activation fact has already identified a concrete external dependency.
	// Its presence is not a request to open a ticket; the Orchestrator reducer
	// and controller separately decide and bind the governance route.
	GovernanceDependency *ExternalDependency
}

func (f FailureFact) Normalized() FailureFact {
	f.Class = FailureClass(strings.TrimSpace(string(f.Class)))
	f.Code = strings.TrimSpace(f.Code)
	f.Signature = strings.TrimSpace(f.Signature)
	f.Message = strings.TrimSpace(f.Message)
	f.OriginalDiagnostic = f.OriginalDiagnostic.Normalized()
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	if f.TargetObservation != nil {
		observation := f.TargetObservation.Normalized()
		f.TargetObservation = &observation
	}
	if f.ValidationDiagnostic != nil {
		diagnostic := f.ValidationDiagnostic.Normalized()
		f.ValidationDiagnostic = &diagnostic
	}
	if f.ValidationFailureObservation != nil {
		observation := f.ValidationFailureObservation.Normalized()
		f.ValidationFailureObservation = &observation
	}
	if f.GovernanceDependency != nil {
		dependency := f.GovernanceDependency.Normalized()
		f.GovernanceDependency = &dependency
	}
	return f
}

func (f FailureFact) Clone() FailureFact {
	f = f.Normalized()
	f.OriginalDiagnostic = f.OriginalDiagnostic.CloneOriginal()
	f.TargetRefs = append([]string(nil), f.TargetRefs...)
	if f.TargetObservation != nil {
		observation := f.TargetObservation.Clone()
		f.TargetObservation = &observation
	}
	if f.ValidationDiagnostic != nil {
		diagnostic := f.ValidationDiagnostic.Clone()
		f.ValidationDiagnostic = &diagnostic
	}
	if f.ValidationFailureObservation != nil {
		observation := f.ValidationFailureObservation.Clone()
		f.ValidationFailureObservation = &observation
	}
	if f.GovernanceDependency != nil {
		dependency := f.GovernanceDependency.Clone()
		f.GovernanceDependency = &dependency
	}
	return f
}

// ValidateRuntimeFailureGovernanceDependency validates an explicitly supplied
// dependency without selecting a governance action. A missing dependency is
// intentionally valid at this DTO layer: the reducer must turn a governance
// request without a source into a typed terminal instead of inventing a
// default decision ticket.
func ValidateRuntimeFailureGovernanceDependency(failure FailureFact) error {
	if failure.GovernanceDependency == nil {
		return nil
	}
	if err := failure.GovernanceDependency.Validate(); err != nil {
		return fmt.Errorf("failure governance dependency: %w", err)
	}
	return nil
}

const (
	// RuntimeFailureTargetObservationMaxTargetRefs is the shared hard bound for
	// direct owner-authored target observations. Producers and ingresses must
	// reject an overflow; truncation would turn an unvalidated subset into
	// repair authority.
	RuntimeFailureTargetObservationMaxTargetRefs = 16

	// RuntimeFailureTargetDiagnosticReceiptEvidence identifies the generic
	// descriptor-authorized direct target-observation protocol. The observation
	// remains a capability-owner fact and must bind the exact persisted evidence
	// carried by the same receipt.
	RuntimeFailureTargetDiagnosticReceiptEvidence = "receipt_evidence_failure_target"
)

// RuntimeFailureTargetObservation is a bounded, owner-authored observation of
// the repair target behind an execution failure. It is data only: the
// Orchestrator decides whether it is compatible with the current Focus and
// may become persisted repair authority.
type RuntimeFailureTargetObservation struct {
	OwnerModuleID    string   `json:"ownerModuleId"`
	SubjectRef       string   `json:"subjectRef"`
	TargetRole       string   `json:"targetRole"`
	ObservedRevision string   `json:"observedRevision"`
	TargetRefs       []string `json:"targetRefs"`
	// CatalogDependency is required by the descriptor-authorized
	// receipt-evidence protocol and binds the observation to one exact owner
	// catalog dependency. Owner-confirmed observations use their separate typed
	// confirmation lineage and leave this nil.
	CatalogDependency *RuntimeFreshnessDependency `json:"catalogDependency,omitempty"`
	// Structured observations bind a target claim to the tool output from
	// which its owner derived it. Capability-specific admission decides whether
	// these fields are mandatory for a particular owner protocol.
	DiagnosticClass string   `json:"diagnosticClass,omitempty"`
	EvidenceRefs    []string `json:"evidenceRefs,omitempty"`
	ContentHash     string   `json:"contentHash,omitempty"`
}

func (o RuntimeFailureTargetObservation) Normalized() RuntimeFailureTargetObservation {
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.SubjectRef = strings.TrimSpace(o.SubjectRef)
	o.TargetRole = strings.TrimSpace(o.TargetRole)
	o.ObservedRevision = strings.TrimSpace(o.ObservedRevision)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	if o.CatalogDependency != nil {
		dependency := o.CatalogDependency.Normalized()
		o.CatalogDependency = &dependency
	}
	o.DiagnosticClass = strings.TrimSpace(o.DiagnosticClass)
	o.EvidenceRefs = normalizeStrings(o.EvidenceRefs)
	o.ContentHash = strings.TrimSpace(o.ContentHash)
	return o
}

func (o RuntimeFailureTargetObservation) Validate() error {
	o = o.Normalized()
	if o.OwnerModuleID == "" || o.SubjectRef == "" || o.TargetRole == "" || o.ObservedRevision == "" || len(o.TargetRefs) == 0 || len(o.TargetRefs) > RuntimeFailureTargetObservationMaxTargetRefs {
		return fmt.Errorf("runtime failure target observation requires owner, subject, role, revision and 1..%d bounded targets", RuntimeFailureTargetObservationMaxTargetRefs)
	}
	structured := o.DiagnosticClass != "" || len(o.EvidenceRefs) != 0 || o.ContentHash != ""
	if structured && (o.DiagnosticClass == "" || len(o.EvidenceRefs) == 0 || len(o.ContentHash) != sha256.Size*2) {
		return fmt.Errorf("structured runtime failure target observation requires diagnostic class, evidence refs and sha256 content hash")
	}
	if o.ContentHash != "" {
		if _, err := hex.DecodeString(o.ContentHash); err != nil {
			return fmt.Errorf("runtime failure target observation content hash must be hex: %w", err)
		}
	}
	if o.CatalogDependency != nil {
		if err := o.CatalogDependency.Validate(); err != nil {
			return fmt.Errorf("runtime failure target catalog dependency: %w", err)
		}
	}
	return nil
}

func (o RuntimeFailureTargetObservation) Clone() RuntimeFailureTargetObservation {
	o = o.Normalized()
	o.TargetRefs = append([]string(nil), o.TargetRefs...)
	o.EvidenceRefs = append([]string(nil), o.EvidenceRefs...)
	if o.CatalogDependency != nil {
		dependency := o.CatalogDependency.Normalized()
		o.CatalogDependency = &dependency
	}
	return o
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
	// Retired keeps completed Focus-scoped prerequisite identities outside
	// reducer, freshness and authority inputs. When a bounded CWI reopen returns
	// to its canonical semantic-pending identity, Orchestrator may atomically
	// remove the same identity before reacquisition; immutable audit for that
	// generation remains in the CWI reopen and semantic-generation histories.
	Retired []RuntimePrerequisite
}

type PrerequisiteKind string

const (
	PrerequisiteKindManifest          PrerequisiteKind = "manifest"
	PrerequisiteKindTargetSnapshot    PrerequisiteKind = "target_snapshot"
	PrerequisiteKindTargetSelection   PrerequisiteKind = "target_selection"
	PrerequisiteKindCapability        PrerequisiteKind = "capability"
	PrerequisiteKindCapabilityState   PrerequisiteKind = "capability_state"
	PrerequisiteKindValidationScratch PrerequisiteKind = "validation_scratch"
	PrerequisiteKindProviderCeiling   PrerequisiteKind = "provider_ceiling"
	// PrerequisiteKindOperationAdmissibility acquires a bounded owner-authored
	// parameter inventory before candidate generation. It is an SM-2 fact, not
	// a candidate or an execution receipt.
	PrerequisiteKindOperationAdmissibility PrerequisiteKind = "operation_admissibility"
)

// RuntimePrerequisiteKindSupported is the closed production prerequisite
// inventory. Unknown and retired kinds are rejected instead of being decoded
// into a dormant resolver path.
func RuntimePrerequisiteKindSupported(kind PrerequisiteKind) bool {
	switch PrerequisiteKind(strings.TrimSpace(string(kind))) {
	case PrerequisiteKindManifest, PrerequisiteKindTargetSnapshot, PrerequisiteKindTargetSelection,
		PrerequisiteKindCapability, PrerequisiteKindCapabilityState, PrerequisiteKindValidationScratch,
		PrerequisiteKindProviderCeiling, PrerequisiteKindOperationAdmissibility:
		return true
	default:
		return false
	}
}

type RuntimePrerequisite struct {
	ID                                   string
	Kind                                 PrerequisiteKind
	Scope                                RuntimePrerequisiteScope
	FocusID                              string
	OwnerModuleID                        string
	SubjectRef                           string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	ProspectiveTargetRef                 string
	CapabilityClass                      RuntimeCapabilityClass
	RequiredCapabilities                 []string
	CapabilityStateKind                  string
	MinimumBytes                         int
	Reason                               string
}

// RuntimePrerequisiteScope identifies whether a prerequisite may be reused
// across Focuses or is authority for one sealed Focus only.
type RuntimePrerequisiteScope string

const (
	RuntimePrerequisiteScopeGlobal RuntimePrerequisiteScope = "global"
	RuntimePrerequisiteScopeFocus  RuntimePrerequisiteScope = "focus"
)

// RuntimeOperationPrerequisiteRequest is an owner-authored, read-only
// feasibility outcome. It names the missing owner fact needed to make a
// corrected operation candidate possible; Orchestrator alone decides the
// RuntimeFacts transition and prerequisite acquisition.
//
// It intentionally has no prerequisite ID. IDs and persistence scope remain
// Orchestrator authority, while an owner may only name its concrete resource.
type RuntimeOperationPrerequisiteRequest struct {
	OperationID   string
	Kind          PrerequisiteKind
	OwnerModuleID string
	SubjectRef    string
	MinimumBytes  int
	Reason        string
}

func (r RuntimeOperationPrerequisiteRequest) Normalized() RuntimeOperationPrerequisiteRequest {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.Kind = PrerequisiteKind(strings.TrimSpace(string(r.Kind)))
	r.OwnerModuleID = strings.TrimSpace(r.OwnerModuleID)
	r.SubjectRef = strings.TrimSpace(r.SubjectRef)
	if r.MinimumBytes < 0 {
		r.MinimumBytes = 0
	}
	r.Reason = strings.TrimSpace(r.Reason)
	return r
}

func (r RuntimeOperationPrerequisiteRequest) Validate() error {
	r = r.Normalized()
	if r.OperationID == "" || r.OwnerModuleID == "" || r.SubjectRef == "" || r.Reason == "" {
		return fmt.Errorf("runtime operation prerequisite request requires operation, owner, subject and reason")
	}
	if r.Kind != PrerequisiteKindTargetSnapshot {
		return fmt.Errorf("runtime operation prerequisite request has unsupported kind %q", r.Kind)
	}
	return nil
}

func (r RuntimeOperationPrerequisiteRequest) Same(other RuntimeOperationPrerequisiteRequest) bool {
	r = r.Normalized()
	other = other.Normalized()
	return r.OperationID == other.OperationID && r.Kind == other.Kind && r.OwnerModuleID == other.OwnerModuleID &&
		r.SubjectRef == other.SubjectRef && r.MinimumBytes == other.MinimumBytes && r.Reason == other.Reason
}

type RuntimeRequirement struct {
	Kind                                 PrerequisiteKind
	Scope                                RuntimePrerequisiteScope
	FocusID                              string
	OwnerModuleID                        string
	SubjectRef                           string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	ProspectiveTargetRef                 string
	CapabilityClass                      RuntimeCapabilityClass
	RequiredCapabilities                 []string
	CapabilityStateKind                  string
	MinimumBytes                         int
	Reason                               string
}

type RuntimeFacts struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DefinitionRevision string
	// CWILedgerSchemaVersion gates the canonical CWI row fields in Coverage.
	// Fresh activation writes it; persisted records without it are rejected by
	// the current RuntimeStateRecord schema rather than being inferred.
	CWILedgerSchemaVersion string
	// DefinitionSeed is the persisted, non-authoritative semantic planning
	// input. Only the Orchestrator semantic planning ledger may turn it into
	// proof bindings.
	DefinitionSeed RuntimeDefinitionSeed
	// DefinitionFocuses are the immutable activation declarations for each CWI.
	// They remain separate from Focus/RemainingFocuses/CompletedFocuses because
	// those queues are replaced with generation-specific semantic bindings by
	// SM-2. Only fresh activation may populate this declaration queue.
	DefinitionFocuses []RuntimeDefinitionFocus
	EvidenceRevision  string
	Focus             CoverageFocus
	RemainingFocuses  []CoverageFocus
	// CompletedFocuses retains the already-advanced SM-2 queue entries.  It is
	// required to deterministically requeue a profile-declared dependent focus
	// when later accepted source evidence invalidates its prior closure.
	CompletedFocuses []CoverageFocus
	Evidence         EvidenceLedger
	Assessments      []RuntimeAssessmentFact
	Failure          *FailureFact
	Prerequisites    PrerequisiteState
	Capabilities     RuntimeCapabilities
	Resources        RuntimeResourceFacts
	Activation       RuntimeActivationContext
	// ReconciliationContract is the SM-2-sealed generation-specific CWI proof contract.
	// It is persisted as part of the Orchestrator aggregate facts so a resume
	// never reconstructs reconciliation authority from mutable profile text.
	ReconciliationContract RuntimeCWIReconciliationContract
	StateChange            RuntimeStateChangeLedger
	ConvergencePolicy      RuntimeConvergencePolicy
	RecoveryPolicy         RuntimeRecoveryPolicy
	// ExecutionBudgetPolicy is the fresh-activation-sealed policy. Consumption
	// remains in the Orchestrator aggregate sub-ledger, never in owner facts.
	ExecutionBudgetPolicy RuntimeExecutionBudgetPolicy
	Recovery              RuntimeRecoveryLedger
	Repair                RuntimeRepairLedger
	// RepairDirective is the single persisted next-action authority for the current
	// focus. When set it overrides raw MissingRequiredEvidenceKinds for candidate
	// action selection (INV-05).
	RepairDirective *RuntimeRepairDirective
	Coverage        []RuntimeCoverageItemFact
	Feasibility     *RuntimeFeasibilityFact
	Slices          []RuntimeAcceptanceSlice
}

// RuntimeFeasibilityFact is the persisted result of the pre-candidate
// capability gate. It is a reducer input, not a drive error or a candidate
// retry signal.
type RuntimeFeasibilityFact struct {
	FocusID string
	State   ConvergenceState
	// Category is an owner-confirmed, stable feasibility classification. It is
	// not an action or terminal instruction; the Orchestrator reducer remains
	// the only authority that maps it to a canonical decision.
	Category               RuntimeOwnerFeasibilityCategory
	RequiredOperationKinds []string
	FeasibleOperationKinds []string
	MissingCapabilityKinds []string
	Reason                 string
}

type RuntimeCapabilities struct {
	StateChangeKinds []string
	ValidationKinds  []string
	GovernanceKinds  []string
	// PrerequisiteFacts are resource dependencies published by the capability
	// owner for a concrete operation kind. Runtime only compiles the typed
	// requirements; it must not infer owner-specific prerequisites from kinds.
	PrerequisiteFacts []RuntimeCapabilityPrerequisiteFact
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
	// RetiredPrerequisiteObservations are lineage only and must never be used to
	// build current authority. Orchestrator discards the old observation when a
	// bounded CWI reopen atomically reactivates its canonical pending
	// prerequisite identity; the observation itself is never reused.
	RetiredPrerequisiteObservations []RuntimePrerequisiteObservation
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
	// OperationVariants is the owner-authored, variant-scoped target policy
	// surface.  OperationKinds remains the bounded kind inventory; this slice
	// is the only authority that can say which discriminated operation variant
	// may use a prospective target.  It is intentionally a typed slice rather
	// than a process-wide index or map so the fact is persisted with the owner
	// catalog and can be validated/replayed as one immutable snapshot.
	OperationVariants       []RuntimeOperationAdmissibilityVariant
	TargetParameterBindings []RuntimeTargetParameterBinding `json:",omitempty"`
	TargetRefs              []string
	SelectorRefs            []string
	ConstraintRefs          []string
	// ValidationRecipeRefs is the ready-only owner-neutral identity set of
	// validation surfaces the owner can execute with an already sealed input
	// binding. Shell may additionally retain the full command recipe below;
	// other owners such as MCP expose only opaque recipe identities and keep
	// transport parameters inside their own boundary.
	ValidationRecipeRefs []string
	// ValidationSurfaces preserves both ready and non-ready owner facts. Only
	// surfaces whose Readiness is ready and whose ProofApplicability is direct
	// may appear in ValidationRecipeRefs and therefore in a provider-visible
	// semantic manifest.
	ValidationSurfaces []RuntimeValidationSurface
	// OperationPolicies are effect/replay facts published by this capability
	// owner. Runtime matches their typed parameter selectors generically.
	OperationPolicies []RuntimeOperationPolicyFact
	// MCPCapabilities is the exact owner-sealed capability metadata used for
	// provider rendering and protocol-specific feasibility. Effect/replay
	// authority lives only in OperationPolicies above.
	MCPCapabilities []RuntimeMCPCapabilityContext
	// ValidationRecipes contains only the Shell-owner-confirmed executable
	// recipes for validation/managed_session. It is never provider supplied and
	// is included in this owner snapshot identity.
	ValidationRecipes []RuntimeValidationRecipe
	ObservedRevision  string
}

// RuntimeOperationAdmissibilityVariant is one owner-confirmed target-policy
// fact for an operation kind and (when the executable contract has variants)
// its concrete variant.  The owner publishes the allowed target policies and
// the selector/constraint vocabulary; Runtime core only compiles and checks
// those values and never infers them from owner identity or effect scope.
type RuntimeOperationAdmissibilityVariant struct {
	Kind                  string
	VariantID             string
	AllowedTargetPolicies []RuntimeOperationTargetPolicy
	SelectorRefs          []string
	ConstraintRefs        []string
}

func (v RuntimeOperationAdmissibilityVariant) Normalized() RuntimeOperationAdmissibilityVariant {
	v.Kind = CanonicalRuntimeBlockKind(v.Kind)
	v.VariantID = strings.TrimSpace(v.VariantID)
	v.AllowedTargetPolicies = normalizeRuntimeSemanticTargetPolicies(v.AllowedTargetPolicies)
	v.SelectorRefs = normalizeStrings(v.SelectorRefs)
	v.ConstraintRefs = normalizeStrings(v.ConstraintRefs)
	return v
}

func (v RuntimeOperationAdmissibilityVariant) Validate(owner string, operationKinds []string, selectors, constraints []string) error {
	rawPolicies := append([]RuntimeOperationTargetPolicy(nil), v.AllowedTargetPolicies...)
	v = v.Normalized()
	owner = strings.TrimSpace(owner)
	if v.Kind == "" || !runtimeOperationKindsContain(operationKinds, v.Kind) {
		return fmt.Errorf("runtime operation admissibility variant %q is absent from owner kind inventory", v.Kind)
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(v.Kind)
	if !ok || descriptor.OwnerModuleID != owner {
		return fmt.Errorf("runtime operation admissibility variant %q does not belong to owner %q", v.Kind, owner)
	}
	contract, found := RuntimeOperationContractForKind(v.Kind)
	if found && len(contract.Variants) != 0 && v.VariantID == "" {
		return fmt.Errorf("runtime operation admissibility kind %q requires a concrete variant fact", v.Kind)
	}
	if v.VariantID != "" && (!found || !runtimeSemanticVariantExists(contract, v.VariantID)) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q is outside owner operation contract", v.Kind, v.VariantID)
	}
	for _, raw := range rawPolicies {
		switch RuntimeOperationTargetPolicy(strings.TrimSpace(string(raw))) {
		case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		default:
			return fmt.Errorf("runtime operation admissibility variant %q/%q declares unknown target policy %q", v.Kind, v.VariantID, raw)
		}
	}
	if len(v.AllowedTargetPolicies) == 0 {
		return fmt.Errorf("runtime operation admissibility variant %q/%q requires target policies", v.Kind, v.VariantID)
	}
	if !allStringsContained(v.SelectorRefs, selectors) || !allStringsContained(v.ConstraintRefs, constraints) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q references undeclared selector or constraint", v.Kind, v.VariantID)
	}
	if runtimeOperationAdmissibilityVariantAllowsTargetPolicy(v, RuntimeOperationTargetPolicyProspectiveOwnerTarget) &&
		(len(v.SelectorRefs) == 0 || len(v.ConstraintRefs) == 0) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q prospective target policy requires owner selector and constraint", v.Kind, v.VariantID)
	}
	return nil
}

// RuntimeOperationAdmissibilityVariantForOperation returns the exact owner
// fact selected by a concrete operation.  For non-discriminated kinds the
// empty VariantID is used.  A missing variant fact is distinguishable from an
// invalid operation so callers can fail closed without guessing.
func RuntimeOperationAdmissibilityVariantForOperation(inventory RuntimeOperationAdmissibility, operation RuntimeOperation) (RuntimeOperationAdmissibilityVariant, bool, error) {
	inventory = inventory.Normalized()
	operation = operation.Normalized()
	if !runtimeOperationKindsContain(inventory.OperationKinds, operation.Kind) {
		return RuntimeOperationAdmissibilityVariant{}, false, nil
	}
	variantID := ""
	contract, found := RuntimeOperationContractForKind(operation.Kind)
	if found && len(contract.Variants) != 0 {
		variant, err := MatchRuntimeOperationVariant(operation, contract)
		if err != nil {
			return RuntimeOperationAdmissibilityVariant{}, false, err
		}
		variantID = strings.TrimSpace(variant.ID)
	}
	return runtimeOperationAdmissibilityVariantForRef(inventory, operation.Kind, variantID)
}

// RuntimeOperationAdmissibilityVariantForProposal is the semantic-plan
// equivalent of RuntimeOperationAdmissibilityVariantForOperation.
func RuntimeOperationAdmissibilityVariantForProposal(inventory RuntimeOperationAdmissibility, kind, variantID string) (RuntimeOperationAdmissibilityVariant, bool, error) {
	inventory = inventory.Normalized()
	kind = CanonicalRuntimeBlockKind(kind)
	variantID = strings.TrimSpace(variantID)
	if !runtimeOperationKindsContain(inventory.OperationKinds, kind) {
		return RuntimeOperationAdmissibilityVariant{}, false, nil
	}
	if variantID == "" {
		if contract, found := RuntimeOperationContractForKind(kind); found && len(contract.Variants) != 0 {
			return RuntimeOperationAdmissibilityVariant{}, false, fmt.Errorf("runtime operation admissibility kind %q requires a concrete variant", kind)
		}
	}
	return runtimeOperationAdmissibilityVariantForRef(inventory, kind, variantID)
}

func runtimeOperationAdmissibilityVariantForRef(inventory RuntimeOperationAdmissibility, kind, variantID string) (RuntimeOperationAdmissibilityVariant, bool, error) {
	var matches []RuntimeOperationAdmissibilityVariant
	for _, raw := range inventory.OperationVariants {
		value := raw.Normalized()
		if value.Kind == CanonicalRuntimeBlockKind(kind) && value.VariantID == strings.TrimSpace(variantID) {
			matches = append(matches, value)
		}
	}
	if len(matches) > 1 {
		return RuntimeOperationAdmissibilityVariant{}, false, fmt.Errorf("runtime operation admissibility has duplicate variant fact %q/%q", kind, variantID)
	}
	if len(matches) == 1 {
		return matches[0], true, nil
	}
	return RuntimeOperationAdmissibilityVariant{}, false, nil
}

// RuntimeOperationAdmissibilityAllowsTargetPolicy is the single policy lookup
// shared by semantic planning, candidate compilation, reducer gates and owner
// preflight.  Variant-scoped facts always win.  Variant-less operations may
// use the owner catalog's explicit selector/constraint declaration, but no
// same-owner/durable-effect inference is permitted.
func RuntimeOperationAdmissibilityAllowsTargetPolicy(inventory RuntimeOperationAdmissibility, kind, variantID string, policy RuntimeOperationTargetPolicy) (allowed bool, selectorRefs, constraintRefs []string, factFound bool, err error) {
	inventory = inventory.Normalized()
	kind = CanonicalRuntimeBlockKind(kind)
	policy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(policy)))
	fact, found, lookupErr := RuntimeOperationAdmissibilityVariantForProposal(inventory, kind, variantID)
	if lookupErr != nil {
		return false, nil, nil, false, lookupErr
	}
	if found {
		return runtimeOperationAdmissibilityVariantAllowsTargetPolicy(fact, policy), append([]string(nil), fact.SelectorRefs...), append([]string(nil), fact.ConstraintRefs...), true, nil
	}
	// A static-variant operation always requires its exact owner fact. Variant
	// facts for another operation kind in the same owner catalog are unrelated:
	// they must not suppress the explicit target/selector inventory of a
	// variant-less kind such as managed validation.
	if contract, contractFound := RuntimeOperationContractForKind(kind); contractFound && len(contract.Variants) != 0 {
		return false, nil, nil, false, nil
	}
	// Existing/exact scope may still use a non-discriminated owner catalog.
	// Prospective authority cannot: a broad selector/constraint union is only
	// vocabulary, not a create grant.  The owner must publish the exact
	// kind/variant fact above.
	if policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return false, append([]string(nil), inventory.SelectorRefs...), append([]string(nil), inventory.ConstraintRefs...), false, nil
	}
	// A catalog without discriminated facts is still explicit for a
	// non-variant kind's existing/exact scope.
	if variantID != "" {
		return false, nil, nil, false, nil
	}
	if policy == RuntimeOperationTargetPolicyExactTarget && len(inventory.TargetRefs) == 0 {
		return false, nil, nil, false, nil
	}
	if policy == RuntimeOperationTargetPolicyExistingOwnerScope && len(inventory.TargetRefs) == 0 && len(inventory.SelectorRefs) == 0 {
		return false, nil, nil, false, nil
	}
	return policy == RuntimeOperationTargetPolicyExactTarget || policy == RuntimeOperationTargetPolicyExistingOwnerScope,
		append([]string(nil), inventory.SelectorRefs...), append([]string(nil), inventory.ConstraintRefs...), false, nil
}

// RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants applies one
// owner-published target policy to every sealed variant of an operation kind.
// It is deliberately a pure value helper: callers retain their own state
// ownership, while a mixed or missing variant fact fails closed instead of
// being widened through a kind-level selector union.
func RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(inventory RuntimeOperationAdmissibility, kind string, variantIDs []string, policy RuntimeOperationTargetPolicy) (allowed bool, selectorRefs, constraintRefs []string, factFound bool, err error) {
	inventory = inventory.Normalized()
	kind = CanonicalRuntimeBlockKind(kind)
	variantIDs = normalizeStrings(variantIDs)
	if len(variantIDs) == 0 {
		if contract, found := RuntimeOperationContractForKind(kind); found && len(contract.Variants) != 0 {
			for _, variant := range contract.Variants {
				variantIDs = append(variantIDs, variant.ID)
			}
			variantIDs = normalizeStrings(variantIDs)
		} else {
			return RuntimeOperationAdmissibilityAllowsTargetPolicy(inventory, kind, "", policy)
		}
	}
	var expectedSelectors []string
	var expectedConstraints []string
	for index, variantID := range variantIDs {
		variantAllowed, selectors, constraints, found, lookupErr := RuntimeOperationAdmissibilityAllowsTargetPolicy(inventory, kind, variantID, policy)
		if lookupErr != nil {
			return false, nil, nil, false, lookupErr
		}
		// A selected static variant without an exact owner fact has no target
		// authority. Do not fall back to a broad catalog declaration.
		if !found || !variantAllowed {
			return false, nil, nil, found, nil
		}
		if index == 0 {
			expectedSelectors = append([]string(nil), selectors...)
			expectedConstraints = append([]string(nil), constraints...)
			continue
		}
		// One candidate authorization cannot safely express different selector
		// vocabularies for multiple variants. The semantic planner may instead
		// seal one concrete variant per CWI.
		if !sameStrings(expectedSelectors, selectors) || !sameStrings(expectedConstraints, constraints) {
			return false, nil, nil, true, fmt.Errorf("runtime operation admissibility variants for %q have incompatible target policy facts", kind)
		}
	}
	return true, expectedSelectors, expectedConstraints, true, nil
}

func runtimeOperationAdmissibilityVariantAllowsTargetPolicy(variant RuntimeOperationAdmissibilityVariant, policy RuntimeOperationTargetPolicy) bool {
	policy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(policy)))
	for _, value := range variant.Normalized().AllowedTargetPolicies {
		if value == policy {
			return true
		}
	}
	return false
}

func (a RuntimeOperationAdmissibility) Clone() RuntimeOperationAdmissibility {
	return cloneRuntimeOperationAdmissibilityRaw(a)
}

func (a RuntimeOperationAdmissibility) Normalized() RuntimeOperationAdmissibility {
	a.PrerequisiteID = strings.TrimSpace(a.PrerequisiteID)
	a.SnapshotID = strings.TrimSpace(a.SnapshotID)
	a.AcquisitionBinding = strings.TrimSpace(a.AcquisitionBinding)
	a.OwnerModuleID = strings.TrimSpace(a.OwnerModuleID)
	a.OperationKinds = canonicalizeOperationKinds(a.OperationKinds)
	if len(a.OperationVariants) > 0 {
		variants := make([]RuntimeOperationAdmissibilityVariant, 0, len(a.OperationVariants))
		for _, raw := range a.OperationVariants {
			value := raw.Normalized()
			if value.Kind == "" {
				continue
			}
			variants = append(variants, value)
		}
		sort.Slice(variants, func(i, j int) bool {
			if variants[i].Kind != variants[j].Kind {
				return variants[i].Kind < variants[j].Kind
			}
			return variants[i].VariantID < variants[j].VariantID
		})
		a.OperationVariants = nilIfEmpty(variants)
	}
	a.TargetRefs = normalizeStrings(a.TargetRefs)
	a.TargetParameterBindings = normalizeRuntimeTargetParameterBindings(a.TargetParameterBindings)
	a.SelectorRefs = normalizeStrings(a.SelectorRefs)
	a.ConstraintRefs = normalizeStrings(a.ConstraintRefs)
	a.ValidationRecipeRefs = normalizeStrings(a.ValidationRecipeRefs)
	a.ValidationSurfaces = NormalizeRuntimeValidationSurfaces(a.ValidationSurfaces)
	a.OperationPolicies = NormalizeRuntimeOperationPolicyFacts(a.OperationPolicies)
	if len(a.MCPCapabilities) > 0 {
		caps := make([]RuntimeMCPCapabilityContext, 0, len(a.MCPCapabilities))
		for _, capability := range a.MCPCapabilities {
			caps = append(caps, SealMCPCapabilityEffect(capability))
		}
		sort.Slice(caps, func(i, j int) bool {
			if caps[i].ServerID != caps[j].ServerID {
				return caps[i].ServerID < caps[j].ServerID
			}
			return caps[i].Name < caps[j].Name
		})
		a.MCPCapabilities = caps
	}
	a.ValidationRecipes = NormalizeRuntimeValidationRecipes(a.ValidationRecipes)
	a.ObservedRevision = strings.TrimSpace(a.ObservedRevision)
	return a
}

func (a RuntimeOperationAdmissibility) Validate() error {
	if err := ValidateRuntimeValidationRecipeProjection(a.ValidationRecipes); err != nil {
		return fmt.Errorf("runtime operation admissibility validation recipes: %w", err)
	}
	// Preserve duplicate variant facts until validation. Silently canonicalizing
	// them would make two conflicting owner declarations replay as one fact.
	seenVariants := make(map[string]struct{}, len(a.OperationVariants))
	for _, raw := range a.OperationVariants {
		value := raw.Normalized()
		if value.Kind == "" {
			return fmt.Errorf("runtime operation admissibility has a variant fact without operation kind")
		}
		key := value.Kind + "|" + value.VariantID
		if _, duplicate := seenVariants[key]; duplicate {
			return fmt.Errorf("runtime operation admissibility has duplicate variant fact %q/%q", value.Kind, value.VariantID)
		}
		seenVariants[key] = struct{}{}
	}
	seenSurfaces := make(map[string]struct{}, len(a.ValidationSurfaces))
	reviewInputVersions := make(map[string]string)
	for _, raw := range a.ValidationSurfaces {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if value.ID == "" {
			return fmt.Errorf("runtime operation admissibility has a validation surface without identity")
		}
		if _, duplicate := seenSurfaces[value.ID]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates validation surface %q", value.ID)
		}
		seenSurfaces[value.ID] = struct{}{}
		recipe, _ := RuntimeValidationRecipeForID(a.ValidationRecipes, value.ID)
		for _, binding := range raw.ProofRequirementBindings {
			if recipe.InputReviewProtocolRef != "" && (binding.InputReview == nil || binding.InputReview.ProtocolRef != recipe.InputReviewProtocolRef) {
				return fmt.Errorf("runtime owner catalog downgraded the sealed input review protocol")
			}
			if binding.InputReview != nil && binding.InputReview.ProtocolRef == RuntimeValidationInputReviewProtocolV2 && recipe.InputReviewProtocolRef != RuntimeValidationInputReviewProtocolV2 {
				return fmt.Errorf("runtime owner catalog introduced an undeclared v2 input review protocol")
			}
			if binding.InputReview == nil {
				continue
			}
			for _, file := range binding.InputReview.Files {
				if prior, found := reviewInputVersions[file.TargetRef]; found && prior != file.ContentHash {
					return fmt.Errorf("runtime owner catalog has conflicting review input versions for %q", file.TargetRef)
				}
				reviewInputVersions[file.TargetRef] = file.ContentHash
			}
		}
	}
	if err := ValidateRuntimeOperationPolicyFacts(a.OperationPolicies); err != nil {
		return err
	}
	a = a.Normalized()
	if err := validateRuntimeTargetParameterBindings(a.TargetParameterBindings, a.OwnerModuleID, a.OperationKinds, a.TargetRefs); err != nil {
		return err
	}
	if a.PrerequisiteID == "" || a.SnapshotID == "" || a.OwnerModuleID == "" || len(a.OperationKinds) == 0 || a.ObservedRevision == "" {
		return fmt.Errorf("runtime operation admissibility requires identity, owner, kinds and revision")
	}
	for _, kind := range a.OperationKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != a.OwnerModuleID {
			return fmt.Errorf("runtime operation admissibility kind %q does not belong to owner %q", kind, a.OwnerModuleID)
		}
	}
	for _, variant := range a.OperationVariants {
		if err := variant.Validate(a.OwnerModuleID, a.OperationKinds, a.SelectorRefs, a.ConstraintRefs); err != nil {
			return err
		}
	}
	if len(a.ValidationRecipes) > 0 {
		if a.OwnerModuleID != RuntimeModuleShell || !runtimeOperationKindsContain(a.OperationKinds, RuntimeBlockKindManagedValidationSession) {
			return fmt.Errorf("runtime operation admissibility validation recipes require Shell managed-validation inventory")
		}
	}
	if len(a.ValidationSurfaces) > 0 {
		for _, surface := range a.ValidationSurfaces {
			if surface.OwnerModuleID != a.OwnerModuleID || !runtimeOperationKindsContain(a.OperationKinds, surface.OperationKind) {
				return fmt.Errorf("runtime validation surface %q is outside owner operation inventory", surface.ID)
			}
			if surface.ProspectiveTargetSatisfactionConstraint != nil {
				constraint := surface.ProspectiveTargetSatisfactionConstraint.Normalized()
				if constraint.CatalogObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q prospective target constraint catalog revision mismatch", surface.ID)
				}
			}
			for _, preparation := range surface.ProofRequirementPreparations {
				constraint := preparation.ProspectiveTargetSatisfactionConstraint.Normalized()
				if constraint.CatalogObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q proof preparation catalog revision mismatch", surface.ID)
				}
			}
			for _, unavailable := range surface.ProofRequirementUnavailabilities {
				if unavailable.OriginalDiagnostic == nil || unavailable.OriginalDiagnostic.ObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q proof unavailability catalog revision mismatch", surface.ID)
				}
			}
		}
		if selectable := runtimeSelectableValidationRecipeRefsCanonical(a.ValidationSurfaces); !sameStrings(selectable, a.ValidationRecipeRefs) {
			return fmt.Errorf("runtime operation admissibility validation recipe refs do not match selectable validation surfaces")
		}
	}
	for _, policy := range a.OperationPolicies {
		if policy.OwnerModuleID != a.OwnerModuleID || !runtimeOperationKindsContain(a.OperationKinds, policy.OperationKind) {
			return fmt.Errorf("runtime operation policy fact %q is outside owner operation inventory", policy.ID)
		}
	}
	if expected := stableRuntimeOperationAdmissibilitySnapshotIDCanonical(a); a.SnapshotID != expected {
		return fmt.Errorf("runtime operation admissibility snapshot identity mismatch")
	}
	return nil
}

func runtimeOperationKindsContain(values []string, wanted string) bool {
	wanted = CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

func StableRuntimeOperationAdmissibilitySnapshotID(value RuntimeOperationAdmissibility) string {
	value = value.Normalized()
	return stableRuntimeOperationAdmissibilitySnapshotIDCanonical(value)
}

// stableRuntimeOperationAdmissibilitySnapshotIDCanonical hashes a value that
// has already passed the catalog's canonicalization boundary. Keeping this
// helper private prevents callers from bypassing raw fail-closed validation.
func stableRuntimeOperationAdmissibilitySnapshotIDCanonical(value RuntimeOperationAdmissibility) string {
	h := sha256.New()
	writeHashString(h, value.PrerequisiteID)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.AcquisitionBinding)
	writeHashStrings(h, value.OperationKinds)
	for _, variant := range value.OperationVariants {
		writeHashString(h, variant.Kind)
		writeHashString(h, variant.VariantID)
		for _, policy := range variant.AllowedTargetPolicies {
			writeHashString(h, string(policy))
		}
		writeHashStrings(h, variant.SelectorRefs)
		writeHashStrings(h, variant.ConstraintRefs)
	}
	writeHashStrings(h, value.TargetRefs)
	writeRuntimeTargetParameterBindingsHash(h, value.TargetParameterBindings)
	writeHashStrings(h, value.SelectorRefs)
	writeHashStrings(h, value.ConstraintRefs)
	writeHashStrings(h, value.ValidationRecipeRefs)
	for _, surface := range value.ValidationSurfaces {
		writeRuntimeValidationInputTargetsHash(h, surface)
		writeHashString(h, surface.ID)
		writeHashString(h, surface.OwnerModuleID)
		writeHashString(h, surface.OperationKind)
		writeHashString(h, string(surface.Readiness))
		writeHashString(h, surface.ReadinessReason)
		writeHashString(h, surface.InputSchemaHash)
		writeHashString(h, surface.InputBindingRef)
		writeHashString(h, surface.PrerequisiteRef)
		writeHashString(h, surface.ProspectiveSatisfactionContractRef)
		if surface.ProspectiveTargetSatisfactionConstraint == nil {
			writeHashString(h, "no-prospective-target-satisfaction-constraint")
		} else {
			writeHashString(h, stableRuntimeProspectiveTargetSatisfactionConstraintHashCanonical(*surface.ProspectiveTargetSatisfactionConstraint))
		}
		writeHashStrings(h, surface.CheckRefs)
		writeHashString(h, string(surface.ProofApplicability))
		for _, binding := range surface.ProofRequirementBindings {
			if binding.InputReview != nil {
				writeHashString(h, RuntimeValidationInputReviewProtocol)
				writeHashString(h, binding.InputReview.Hash)
			}
			writeHashString(h, binding.RequirementHash)
			writeHashString(h, binding.DefinitionRevision)
			writeHashString(h, string(binding.CoverageWorkItemID))
			writeHashString(h, binding.DeclarationRef)
			writeHashString(h, binding.ValidationRecipeID)
			writeHashStrings(h, binding.RequiredCheckRefs)
			for _, attestation := range binding.CheckAttestations {
				writeHashString(h, attestation.RequiredCheckRef)
				writeHashString(h, attestation.ObservationProtocolRef)
				writeHashString(h, attestation.NativeCheckRef)
				writeRuntimeSupplementalNativeCheckHash(h, attestation)
			}
		}
		for _, preparation := range surface.ProofRequirementPreparations {
			writeHashString(h, preparation.RequirementHash)
			writeHashString(h, preparation.DefinitionRevision)
			writeHashString(h, string(preparation.CoverageWorkItemID))
			writeHashString(h, preparation.DeclarationRef)
			writeHashString(h, preparation.ValidationRecipeID)
			writeHashString(h, preparation.TargetRef)
			for _, attestation := range preparation.CheckAttestations {
				writeHashString(h, attestation.RequiredCheckRef)
				writeHashString(h, attestation.ObservationProtocolRef)
				writeHashString(h, attestation.NativeCheckRef)
				writeRuntimeSupplementalNativeCheckHash(h, attestation)
			}
			writeHashString(h, stableRuntimeProspectiveTargetSatisfactionConstraintHashCanonical(preparation.ProspectiveTargetSatisfactionConstraint))
			if preparation.OriginalDiagnostic != nil {
				writeHashString(h, "proof-preparation-original-diagnostic/v1")
				writeHashString(h, StableRuntimeFeedbackHash(RuntimeFeedback{OriginalDiagnostic: preparation.OriginalDiagnostic}))
			}
			if preparation.ContentContractRef != "" || preparation.ContentRequirements != "" {
				writeHashString(h, "proof-preparation-content-contract/v1")
				writeHashStrings(h, []string{preparation.ContentContractRef, preparation.ContentRequirements})
			}
		}
		writeRuntimeValidationProofUnavailabilitiesHash(h, surface.ProofRequirementUnavailabilities)
		writeHashString(h, string(surface.EffectScope))
		writeHashString(h, string(surface.IdempotencyClass))
	}
	for _, policy := range value.OperationPolicies {
		writeHashString(h, policy.ID)
		writeHashString(h, policy.OwnerModuleID)
		writeHashString(h, policy.OperationKind)
		writeHashString(h, policy.SubjectRef)
		for _, match := range policy.ParameterMatches {
			writeHashString(h, match.Name)
			if match.String != nil {
				writeHashString(h, "string:"+*match.String)
			}
			if match.Boolean != nil {
				writeHashString(h, fmt.Sprintf("boolean:%t", *match.Boolean))
			}
		}
		writeHashString(h, string(policy.EffectScope))
		writeHashString(h, string(policy.IdempotencyClass))
	}
	for _, capability := range value.MCPCapabilities {
		writeHashString(h, string(capability.ID))
		writeHashString(h, string(capability.ServerID))
		writeHashString(h, capability.Kind)
		writeHashString(h, capability.OperationKind)
		writeHashString(h, capability.Name)
		writeHashString(h, capability.EffectScope)
		writeHashString(h, capability.IdempotencyClass)
		writeHashString(h, fmt.Sprintf("%t", capability.Enabled))
		writeHashString(h, capability.Status)
	}
	for _, recipe := range value.ValidationRecipes {
		writeHashString(h, recipe.ID)
		writeHashString(h, recipe.AdapterRef)
		writeRuntimeValidationRecipeInputReviewHash(h, recipe)
		writeHashString(h, recipe.Command)
		writeHashStrings(h, recipe.Args)
		writeHashString(h, fmt.Sprintf("%d", recipe.TimeoutMillis))
	}
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
	RequirementID string
	Kind          string
	VariantID     string
	// TargetRef binds this required operation to one exact target. It remains
	// empty for targetless owner sequences. A bounded prospective all-of
	// acquisition uses one requirement per target so a partial candidate cannot
	// consume the single acquisition transaction.
	TargetRef               string
	DependsOnRequirementIDs []string
	Reason                  string
}

// CloneRequiredOperationVariants detaches the nested dependency slices while
// preserving the canonical Runtime value used by read projections.
func CloneRequiredOperationVariants(values []RuntimeRequiredOperationVariant) []RuntimeRequiredOperationVariant {
	ret := make([]RuntimeRequiredOperationVariant, len(values))
	for idx := range values {
		ret[idx] = values[idx]
		ret[idx].DependsOnRequirementIDs = append([]string(nil), values[idx].DependsOnRequirementIDs...)
	}
	return ret
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
	// owner-observed empty workspace. It carries an SM-2 binding-sealed
	// prospective path; File validates that path against its
	// workspace boundary and operation admissibility inventory.
	RuntimeTargetSelectionCreate RuntimeTargetSelectionDisposition = "create"
	// RuntimeTargetSelectionScope binds either a bounded complete File
	// feasibility inventory or a profile-declared empty-workspace creation
	// scope. It is not a concrete repair target or a filename heuristic: a
	// candidate still proposes concrete operations and their owner validates
	// every target against the sealed admissibility inventory.
	RuntimeTargetSelectionScope     RuntimeTargetSelectionDisposition = "scope"
	RuntimeTargetSelectionAbsent    RuntimeTargetSelectionDisposition = "absent"
	RuntimeTargetSelectionAmbiguous RuntimeTargetSelectionDisposition = "ambiguous"
	// RuntimeTargetSelectionUnsupported records that the queried owner does not
	// implement the declared selector. It is a typed owner observation, not a
	// transport or contract failure; SM-2 decides the resulting unresolved
	// target outcome without requesting a provider candidate.
	RuntimeTargetSelectionUnsupported RuntimeTargetSelectionDisposition = "unsupported"
)

type RuntimeTargetSelection struct {
	PrerequisiteID        string
	SnapshotID            string
	OwnerModuleID         string
	TargetRole            string
	SelectorRef           string
	SelectorOwnerRef      string
	SelectorSubjectRef    string
	SelectorCandidateRefs []string
	// ProspectiveTargetAllOfConstraintRefs echoes the exact mandatory pre-seal
	// declaration that File accepted for this selector observation.
	ProspectiveTargetAllOfConstraintRefs []string
	Disposition                          RuntimeTargetSelectionDisposition
	TargetRefs                           []string
	CandidateTargetRefs                  []string
	// ProspectiveTargetRef is the single concrete candidate used by the ordinary
	// File-owner target-selection prerequisite. Bounded semantic acquisition is
	// sealed separately as CoverageEvidenceBinding.ProspectiveTargetRefs and
	// does not pass through this single-target discovery record.
	ProspectiveTargetRef string
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
	s.SelectorOwnerRef = strings.TrimSpace(s.SelectorOwnerRef)
	s.SelectorSubjectRef = strings.TrimSpace(s.SelectorSubjectRef)
	s.SelectorCandidateRefs = normalizeStrings(s.SelectorCandidateRefs)
	s.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(s.ProspectiveTargetAllOfConstraintRefs)
	s.Disposition = RuntimeTargetSelectionDisposition(strings.TrimSpace(string(s.Disposition)))
	s.TargetRefs = normalizeStrings(s.TargetRefs)
	s.CandidateTargetRefs = normalizeStrings(s.CandidateTargetRefs)
	s.ProspectiveTargetRef = strings.TrimSpace(s.ProspectiveTargetRef)
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
	if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(s.ProspectiveTargetAllOfConstraintRefs); err != nil {
		return err
	}
	if len(s.ProspectiveTargetAllOfConstraintRefs) != 0 && (s.SelectorOwnerRef == "" || s.SelectorSubjectRef == "") {
		return fmt.Errorf("runtime target selection prospective all-of declaration requires selector owner and subject")
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
		if s.ProspectiveTargetRef == "" {
			return fmt.Errorf("create runtime target selection requires an SM-2 binding-sealed prospective target")
		}
	case RuntimeTargetSelectionScope:
		if s.CandidateCount == 0 {
			if len(s.TargetRefs) != 0 || len(s.CandidateTargetRefs) != 0 || s.ProspectiveTargetRef != "" || s.SelectorOwnerRef == "" || s.SelectorSubjectRef == "" {
				return fmt.Errorf("empty workspace scope requires a profile owner and subject without selected or prospective targets")
			}
		} else if s.CandidateCount < 2 || len(s.TargetRefs) != s.CandidateCount || len(s.CandidateTargetRefs) != s.CandidateCount || !sameStrings(s.TargetRefs, s.CandidateTargetRefs) {
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
	case RuntimeTargetSelectionUnsupported:
		if s.CandidateCount != 0 || len(s.TargetRefs) != 0 || len(s.CandidateTargetRefs) != 0 {
			return fmt.Errorf("unsupported runtime target selection cannot contain owner inventory targets")
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
	writeHashString(h, value.SelectorOwnerRef)
	writeHashString(h, value.SelectorSubjectRef)
	writeHashStrings(h, value.SelectorCandidateRefs)
	writeHashStrings(h, value.ProspectiveTargetAllOfConstraintRefs)
	writeHashString(h, string(value.Disposition))
	writeHashStrings(h, value.TargetRefs)
	writeHashStrings(h, value.CandidateTargetRefs)
	writeHashString(h, value.ProspectiveTargetRef)
	writeHashString(h, fmt.Sprintf("%d", value.CandidateCount))
	writeHashString(h, value.ObservedRevision)
	return "target-selection-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// SameRuntimeTargetSelectorCandidateRefs compares the canonical bounded
// candidate declaration carried across activation, prerequisite and owner
// observation contracts. It is deliberately a pure value helper: ownership
// and semantic interpretation remain outside the runtime contract package.
func SameRuntimeTargetSelectorCandidateRefs(left, right []string) bool {
	return sameStrings(normalizeStrings(left), normalizeStrings(right))
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

// ProviderCeiling seals the provider/model capability budget for one Runtime Run.
// Runtime core only checks identity, positive ceilings, and revision presence.
// Budget derivation remains owned by blocks/llm.
type ProviderCeiling struct {
	Selection                ProviderInferenceSelection
	ContextWindowTokens      int
	ProviderMaxOutputTokens  int
	EffectiveMaxOutputTokens int
	MaxInputTokens           int
	MaxBytes                 int
	CapabilityRevision       string
	ConfigurationRevision    string
}

func (c ProviderCeiling) Normalized() ProviderCeiling {
	c.Selection = c.Selection.Normalized()
	c.CapabilityRevision = strings.TrimSpace(c.CapabilityRevision)
	c.ConfigurationRevision = strings.TrimSpace(c.ConfigurationRevision)
	return c
}

func (c ProviderCeiling) Validate() error {
	c = c.Normalized()
	if err := c.Selection.Validate(); err != nil {
		return fmt.Errorf("provider ceiling selection: %w", err)
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
	return l.Selection.Same(r.Selection) &&
		l.ContextWindowTokens == r.ContextWindowTokens &&
		l.ProviderMaxOutputTokens == r.ProviderMaxOutputTokens &&
		l.EffectiveMaxOutputTokens == r.EffectiveMaxOutputTokens &&
		l.MaxInputTokens == r.MaxInputTokens &&
		l.MaxBytes == r.MaxBytes &&
		l.CapabilityRevision == r.CapabilityRevision &&
		l.ConfigurationRevision == r.ConfigurationRevision
}

type RequiredOutputContract struct {
	AcceptedKinds []string
	// ImplementationWorkBindingHash is the private working commitment, never
	// a terminal coverage binding or validation-repair permission.
	ImplementationWorkBindingHash string
	AcceptedOperationVariants     []RuntimeOperationVariantRef
	// AcceptedEffectScopes are the effect scopes that may appear in a sealed candidate.
	// RequiredEffectScopes must be satisfied by at least one operation for state-change.
	AcceptedEffectScopes  []RuntimeEffectScope
	RequiredEffectScopes  []RuntimeEffectScope
	RequiredEvidenceKinds []string
	CoverageWorkItemIDs   []common.CoverageWorkItemID
	TargetRefs            []string
	// RepairClosureID binds this action-level output to the persisted SM-2
	// repair objective. It is empty for ordinary, non-repair decisions.
	RepairClosureID string
	// RepairPurpose is the typed SM-2 purpose copied into a need_state_change
	// pre-seal candidate manifest. It makes an admission-time repair commitment
	// mandatory without consulting a later mutable directive. The subsequent
	// fresh-validation phase retains RepairClosureID and clears RepairPurpose.
	RepairPurpose             RuntimeDispatchRepairPurpose
	PrerequisiteIDs           []string
	RequiredOperationSequence []RuntimeRequiredOperationVariant
}

type AuthoritySnapshot struct {
	ID                        string
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	State                     ConvergenceState
	FocusID                   string
	RequiredOutputHash        string
	DefinitionRevision        string
	EvidenceRevision          string
	CapabilityStateHash       string
	PrerequisiteFreshnessHash string
	OperationContractRevision string
	AuthorityInputID          string
	AuthorityInput            RuntimeEquivalentAuthorityInputIdentity
	Attempt                   int
}

// RuntimeTerminalDecision is the terminal detail sealed by SM-2.  It
// deliberately carries no outcome kind: the terminal kind is an SM-4 delivery
// fact derived from the canonical convergence state (or an SM-1
// administrative termination record), not a second persisted decision.
type RuntimeTerminalDecision struct {
	Code    string
	Summary string
	// Audit-only cause sealed by SM-2 from exact-current owner facts. It cannot
	// choose a transition or authorize another acquisition.
	OriginalDiagnostic *RuntimeOriginalDiagnostic `json:",omitempty"`
}

// TerminalOutcomeKind is the one terminal identity that crosses the SM-4
// Result -> Workspace -> Run delivery saga.
type TerminalOutcomeKind string

const (
	TerminalOutcomeKindCompleted  TerminalOutcomeKind = "completed"
	TerminalOutcomeKindFailed     TerminalOutcomeKind = "failed"
	TerminalOutcomeKindCancelled  TerminalOutcomeKind = "cancelled"
	TerminalOutcomeKindSuperseded TerminalOutcomeKind = "superseded"
)

// TerminalOutcome is sealed only for terminal delivery. It must never be
// embedded in RuntimeDecision: SM-2 persists its canonical ConvergenceState
// and terminal detail, while SM-4 owns this delivery identity.
type TerminalOutcome struct {
	Kind    TerminalOutcomeKind
	Code    string
	Summary string
}

type RuntimeDecision struct {
	State             ConvergenceState
	Focus             CoverageFocus
	RequiredOutput    RequiredOutputContract
	Prerequisites     []RuntimePrerequisite
	AuthoritySnapshot AuthoritySnapshot
	Terminal          *RuntimeTerminalDecision
}

type AuthorityTransaction struct {
	AuthoritySnapshotID       string
	FocusID                   string
	State                     ConvergenceState
	RequiredOutputHash        string
	DefinitionRevision        string
	EvidenceRevision          string
	CapabilityStateHash       string
	PrerequisiteFreshnessHash string
	OperationContractRevision string
	AuthorityInputID          string
	Attempt                   int
}

func (f RuntimeFacts) Normalized() RuntimeFacts {
	f.DefinitionRevision = strings.TrimSpace(f.DefinitionRevision)
	f.CWILedgerSchemaVersion = strings.TrimSpace(f.CWILedgerSchemaVersion)
	f.DefinitionSeed = f.DefinitionSeed.Clone()
	f.DefinitionFocuses = NormalizeRuntimeDefinitionFocuses(f.DefinitionFocuses)
	f.EvidenceRevision = strings.TrimSpace(f.EvidenceRevision)
	f.Focus = f.Focus.Normalized()
	f.RemainingFocuses = normalizeCoverageFocusQueue(f.RemainingFocuses)
	f.CompletedFocuses = normalizeCoverageFocusQueue(f.CompletedFocuses)
	f.Evidence.Accepted = normalizeEvidenceFacts(f.Evidence.Accepted)
	f.Evidence.Rejected = normalizeEvidenceFacts(f.Evidence.Rejected)
	f.Assessments = normalizeRuntimeAssessmentFacts(f.Assessments)
	f.Prerequisites.Pending = normalizeRuntimePrerequisites(f.Prerequisites.Pending)
	f.Prerequisites.Satisfied = normalizeRuntimePrerequisites(f.Prerequisites.Satisfied)
	f.Prerequisites.Retired = normalizeRuntimePrerequisites(f.Prerequisites.Retired)
	f.Capabilities.StateChangeKinds = normalizeStrings(f.Capabilities.StateChangeKinds)
	f.Capabilities.ValidationKinds = normalizeStrings(f.Capabilities.ValidationKinds)
	f.Capabilities.GovernanceKinds = normalizeStrings(f.Capabilities.GovernanceKinds)
	f.Capabilities.PrerequisiteFacts = NormalizeRuntimeCapabilityPrerequisiteFacts(f.Capabilities.PrerequisiteFacts)
	f.Resources = f.Resources.Normalized()
	f.Activation = f.Activation.Normalized()
	f.ReconciliationContract = f.ReconciliationContract.Normalized()
	f.StateChange = f.StateChange.Normalized()
	f.ConvergencePolicy = f.ConvergencePolicy.Normalized()
	f.RecoveryPolicy = f.RecoveryPolicy.Normalized()
	f.ExecutionBudgetPolicy = f.ExecutionBudgetPolicy.Normalized()
	f.Recovery = f.Recovery.Normalized()
	f.Repair = f.Repair.Normalized()
	f.Coverage = NormalizeRuntimeCoverageFacts(f.Coverage)
	f.Slices = NormalizeRuntimeAcceptanceSlices(f.Slices)
	if f.Feasibility != nil {
		value := *f.Feasibility
		value.FocusID = strings.TrimSpace(value.FocusID)
		value.State = ConvergenceState(strings.TrimSpace(string(value.State)))
		value.Category = RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(value.Category)))
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
	raw := f
	// Facts normalization must not repair a coordinated persisted rewrite of
	// activation evidence authority. Check this nested raw collection before
	// f.Normalized() can canonicalize nested authority fields.
	if err := ValidateRuntimeFactsRaw(raw); err != nil {
		return err
	}
	rawFocuses := make([]CoverageFocus, 0, 1+len(raw.RemainingFocuses)+len(raw.CompletedFocuses))
	rawFocuses = append(rawFocuses, raw.Focus)
	rawFocuses = append(rawFocuses, raw.RemainingFocuses...)
	rawFocuses = append(rawFocuses, raw.CompletedFocuses...)
	for _, focus := range rawFocuses {
		if len(focus.CoverageEvidenceBindings) == 0 {
			continue
		}
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, raw.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime facts focus %s: %w", strings.TrimSpace(focus.ID), err)
		}
	}
	for _, snapshot := range raw.Resources.CapabilityStates {
		if err := validateRequiredOperationVariants(snapshot.RequiredOperationSequence); err != nil {
			return fmt.Errorf("runtime capability-state snapshot %q operation sequence: %w", strings.TrimSpace(snapshot.SnapshotID), err)
		}
	}
	if err := ValidateRuntimeOperationAdmissibilities(raw.Resources.OperationAdmissibilities); err != nil {
		return fmt.Errorf("runtime operation admissibility: %w", err)
	}
	for _, fact := range append(append([]EvidenceFact(nil), raw.Evidence.Accepted...), raw.Evidence.Rejected...) {
		if fact.Validation != nil {
			if err := fact.Validation.ValidateFailureDetails(); err != nil {
				return fmt.Errorf("runtime evidence validation observation %q: %w", fact.ID, err)
			}
		}
	}
	f = f.Normalized()
	if f.WorkspaceID == "" || f.RunID == "" {
		return fmt.Errorf("runtime facts require workspace and run scope")
	}
	if f.DefinitionRevision == "" || f.EvidenceRevision == "" {
		return fmt.Errorf("runtime facts require definition and evidence revisions")
	}
	if raw.CWILedgerSchemaVersion != "" {
		if err := ValidateRuntimeCWILedger(raw); err != nil {
			return err
		}
		if err := ValidateRuntimeCWIFocusBindings(f); err != nil {
			return err
		}
	}
	if f.DefinitionSeed.SchemaVersion != "" {
		if err := f.DefinitionSeed.Validate(); err != nil {
			return fmt.Errorf("runtime facts definition seed: %w", err)
		}
		if err := ValidateRuntimeSourceObligationApplicability(f.DefinitionSeed.SourceObligations, f.Activation.ExecutionContract.ProfileContracts); err != nil {
			return fmt.Errorf("runtime facts source applicability: %w", err)
		}
		if f.DefinitionSeed.DefinitionRevision != f.DefinitionRevision {
			return fmt.Errorf("runtime facts definition seed revision mismatch")
		}
		if len(f.DefinitionFocuses) != 0 {
			if err := ValidateRuntimeDefinitionFocuses(f.DefinitionSeed, f.DefinitionFocuses); err != nil {
				return fmt.Errorf("runtime facts definition focuses: %w", err)
			}
			if err := ValidateRuntimeDefinitionImplementationWorkPolicies(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return fmt.Errorf("runtime facts definition work policies: %w", err)
			}
			if err := ValidateRuntimeDefinitionLineageAssertions(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return err
			}
			if err := ValidateRuntimeDefinitionVerificationPolicies(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return fmt.Errorf("runtime facts definition verification policies: %w", err)
			}
		}
	}
	if f.Focus.ID == "" {
		return fmt.Errorf("runtime facts require a coverage focus")
	}
	if err := ValidateRuntimeCapabilityPrerequisiteFacts(raw.Capabilities.PrerequisiteFacts); err != nil {
		return fmt.Errorf("runtime capability prerequisite facts: %w", err)
	}
	for _, focus := range append(append([]CoverageFocus{f.Focus}, f.RemainingFocuses...), f.CompletedFocuses...) {
		if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(focus.ProspectiveTargetAllOfConstraintRefs); err != nil {
			return fmt.Errorf("runtime focus %q prospective target all-of declaration: %w", focus.ID, err)
		}
		if len(focus.ProspectiveTargetAllOfConstraintRefs) != 0 &&
			(!focus.RequireTargetGrounding || focus.TargetSelectorRef == "" || focus.TargetSelectorOwnerRef == "" || focus.TargetSelectorSubjectRef == "") {
			return fmt.Errorf("runtime focus %q prospective target all-of declaration requires grounding and a complete selector", focus.ID)
		}
		if err := ValidateRuntimeOwnerFactRequirements(focus.RequiredOwnerFacts); err != nil {
			return fmt.Errorf("runtime focus %q owner facts: %w", focus.ID, err)
		}
		if err := ValidateRuntimeOwnerFactConvergences(focus.RequiredOwnerFacts, focus.OwnerFactConvergences); err != nil {
			return fmt.Errorf("runtime focus %q owner-fact convergence: %w", focus.ID, err)
		}
	}
	if err := f.Activation.Validate(); err != nil {
		return err
	}
	if err := f.ConvergencePolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts convergence policy: %w", err)
	}
	if err := f.RecoveryPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts recovery policy: %w", err)
	}
	if err := f.ExecutionBudgetPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts execution budget policy: %w", err)
	}
	if err := ValidateRuntimeExecutionBudgetPolicySources(f.ExecutionBudgetPolicy, f.ConvergencePolicy, f.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime facts execution budget policy: %w", err)
	}
	if err := ValidateRuntimeExecutionLimitsBinding(f.ExecutionBudgetPolicy, f.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return fmt.Errorf("runtime facts execution budget limits: %w", err)
	}
	if err := f.Recovery.Validate(f.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime facts recovery ledger: %w", err)
	}
	if f.Resources.TargetSelection != nil {
		if err := f.Resources.TargetSelection.Validate(); err != nil {
			return fmt.Errorf("runtime facts target selection: %w", err)
		}
		selection := f.Resources.TargetSelection.Normalized()
		bound := false
		for _, prerequisite := range f.Prerequisites.Satisfied {
			if prerequisite.ID == selection.PrerequisiteID &&
				prerequisite.Kind == PrerequisiteKindTargetSelection &&
				prerequisite.Scope == RuntimePrerequisiteScopeFocus &&
				prerequisite.FocusID == f.Focus.ID &&
				prerequisite.OwnerModuleID == selection.OwnerModuleID {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("runtime facts target selection %q is not bound to an active current-focus prerequisite", selection.PrerequisiteID)
		}
	}
	if f.Failure != nil {
		if f.Failure.OriginalDiagnostic != nil {
			if err := f.Failure.OriginalDiagnostic.Validate(); err != nil {
				return fmt.Errorf("runtime facts failure original diagnostic: %w", err)
			}
		}
		if err := ValidateRuntimeFailureTargetObservationBinding(*f.Failure); err != nil {
			return fmt.Errorf("runtime facts failure target observation: %w", err)
		}
		if err := ValidateRuntimeFailureGovernanceDependency(*f.Failure); err != nil {
			return fmt.Errorf("runtime facts governance dependency: %w", err)
		}
		if f.Failure.ValidationFailureObservation != nil {
			if err := f.Failure.ValidationFailureObservation.Validate(); err != nil {
				return fmt.Errorf("runtime facts validation failure observation: %w", err)
			}
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
	activePrerequisites := append(append([]RuntimePrerequisite(nil), f.Prerequisites.Pending...), f.Prerequisites.Satisfied...)
	activeIDs := make(map[string]struct{}, len(activePrerequisites))
	for _, prerequisite := range activePrerequisites {
		if !RuntimePrerequisiteKindSupported(prerequisite.Kind) {
			return fmt.Errorf("runtime prerequisite %q has unsupported kind %q", prerequisite.ID, prerequisite.Kind)
		}
		if err := validateRuntimePrerequisiteProspectiveTargetAllOfDeclaration(prerequisite); err != nil {
			return err
		}
		if prerequisite.Scope == RuntimePrerequisiteScopeFocus && prerequisite.FocusID != f.Focus.ID {
			return fmt.Errorf("runtime active focus-scoped prerequisite %q is bound to focus %q instead of current focus %q", prerequisite.ID, prerequisite.FocusID, f.Focus.ID)
		}
		activeIDs[prerequisite.ID] = struct{}{}
	}
	retiredIDs := make(map[string]RuntimePrerequisite, len(f.Prerequisites.Retired))
	for _, prerequisite := range f.Prerequisites.Retired {
		if !RuntimePrerequisiteKindSupported(prerequisite.Kind) {
			return fmt.Errorf("runtime retired prerequisite %q has unsupported kind %q", prerequisite.ID, prerequisite.Kind)
		}
		if err := validateRuntimePrerequisiteProspectiveTargetAllOfDeclaration(prerequisite); err != nil {
			return err
		}
		if _, active := activeIDs[prerequisite.ID]; active {
			return fmt.Errorf("runtime prerequisite %q cannot be both active and retired", prerequisite.ID)
		}
		if prerequisite.Scope != RuntimePrerequisiteScopeFocus || prerequisite.FocusID == "" {
			return fmt.Errorf("runtime retired prerequisite %q must be focus-scoped", prerequisite.ID)
		}
		retiredIDs[prerequisite.ID] = prerequisite
	}
	for _, observation := range f.Resources.RetiredPrerequisiteObservations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("runtime retired prerequisite observation: %w", err)
		}
		prerequisite, found := retiredIDs[observation.PrerequisiteID]
		if !found || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			return fmt.Errorf("runtime retired prerequisite observation %q has no matching retired prerequisite", observation.PrerequisiteID)
		}
	}
	seenFocus := map[string]struct{}{f.Focus.ID: {}}
	for _, focus := range append(append([]CoverageFocus(nil), f.RemainingFocuses...), f.CompletedFocuses...) {
		if focus.ID == "" {
			return fmt.Errorf("runtime facts queued focus requires identity")
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
	for _, focus := range append(append([]CoverageFocus{f.Focus}, f.RemainingFocuses...), f.CompletedFocuses...) {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := coverage[id]; !ok {
				return fmt.Errorf("runtime facts focus coverage item %s has no coverage ledger entry", id)
			}
		}
		if len(focus.CoverageEvidenceBindings) != 0 {
			if err := ValidateCoverageEvidenceBindingsForFocus(focus, f.DefinitionRevision); err != nil {
				return fmt.Errorf("runtime facts focus %s: %w", focus.ID, err)
			}
		}
	}
	if f.ReconciliationContract.SchemaVersion != "" {
		if err := f.ReconciliationContract.Validate(); err != nil {
			return fmt.Errorf("runtime facts reconciliation contract: %w", err)
		}
	}
	coverageItems := make([]RuntimeCoverageItem, 0, len(f.Coverage))
	for _, item := range f.Coverage {
		coverageItems = append(coverageItems, RuntimeCoverageItem{
			ID: item.ID, SliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID,
			TargetText: item.TargetText, PrerequisiteCoverageWorkItemIDs: item.PrerequisiteCoverageWorkItemIDs,
			ReopenDependencies: item.ReopenDependencies,
		})
	}
	if err := ValidateRuntimeCoverageItemDependencies(coverageItems); err != nil {
		return fmt.Errorf("runtime facts coverage dependencies: %w", err)
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
		if assessment.ID == "" || assessment.Revision == "" || assessment.FocusID == "" {
			return fmt.Errorf("runtime assessment fact requires identity, revision and focus")
		}
		if err := assessment.Disposition.Validate(); err != nil {
			return fmt.Errorf("runtime assessment fact %s canonical disposition: %w", assessment.ID, err)
		}
		if assessment.ClaimedDisposition != "" {
			if err := assessment.ClaimedDisposition.Validate(); err != nil {
				return fmt.Errorf("runtime assessment fact %s claimed disposition: %w", assessment.ID, err)
			}
		}
	}
	return nil
}

// ValidateRuntimeFactsRaw checks authority-bearing raw shape without requiring
// a fully initialized Runtime ledger. Callers that must initialize missing
// owner state use it before their first normalization, then run Validate on the
// complete facts.
func ValidateRuntimeFactsRaw(f RuntimeFacts) error {
	if err := ValidateRuntimeEvidencePolicies(f.Activation.EvidencePolicies); err != nil {
		return fmt.Errorf("runtime facts activation evidence policies: %w", err)
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
		credited := false
		for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
			if EvidenceAppliesToCoverageWorkItem(facts.Focus, evidence, coverageID) {
				credited = true
				break
			}
		}
		if len(facts.Focus.RequiredEvidenceKinds) > 0 && !credited {
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
	f.CoverageEvidenceBindings = NormalizeCoverageEvidenceBindings(f.CoverageEvidenceBindings)
	if f.VerificationPolicy != nil {
		value := f.VerificationPolicy.Normalized()
		f.VerificationPolicy = &value
	}
	f.EvidenceDeclarationRef = strings.TrimSpace(f.EvidenceDeclarationRef)
	f.RequiredEvidenceKinds = normalizeStrings(f.RequiredEvidenceKinds)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.EvidenceRole = strings.TrimSpace(f.EvidenceRole)
	f.TargetRole = strings.TrimSpace(f.TargetRole)
	f.TargetDiscoverySources = normalizeStrings(f.TargetDiscoverySources)
	f.TargetSelectorRef = strings.TrimSpace(f.TargetSelectorRef)
	f.TargetSelectorOwnerRef = strings.TrimSpace(f.TargetSelectorOwnerRef)
	f.TargetSelectorSubjectRef = strings.TrimSpace(f.TargetSelectorSubjectRef)
	f.TargetSelectorCandidateRefs = normalizeStrings(f.TargetSelectorCandidateRefs)
	f.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(f.ProspectiveTargetAllOfConstraintRefs)
	f.ProspectiveTargetRefs = normalizeStrings(f.ProspectiveTargetRefs)
	f.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(f.RequiredOwnerFacts)
	f.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(f.OwnerFactConvergences)
	f.ValidationRecipeID = strings.TrimSpace(f.ValidationRecipeID)
	f.RequiredCheckRefs = normalizeRuntimeValidationProofCheckRefs(f.RequiredCheckRefs)
	f.MetricConstraints = normalizeStrings(f.MetricConstraints)
	f.ArtifactRefs = normalizeStrings(f.ArtifactRefs)
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
			observation := target.TargetObservation.Clone()
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
	r.RetiredPrerequisiteObservations = NormalizeRuntimePrerequisiteObservations(r.RetiredPrerequisiteObservations)
	return r
}

func NormalizeRuntimeOperationAdmissibilities(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
	ret := make([]RuntimeOperationAdmissibility, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.PrerequisiteID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].PrerequisiteID != ret[j].PrerequisiteID {
			return ret[i].PrerequisiteID < ret[j].PrerequisiteID
		}
		return ret[i].OwnerModuleID < ret[j].OwnerModuleID
	})
	return nilIfEmpty(ret)
}

// ValidateRuntimeOperationAdmissibilities rejects duplicate authority before
// canonical ordering. A normalizer must never choose which owner catalog wins.
func ValidateRuntimeOperationAdmissibilities(values []RuntimeOperationAdmissibility) error {
	seenPrerequisites := make(map[string]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, duplicate := seenPrerequisites[value.PrerequisiteID]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates prerequisite %q", value.PrerequisiteID)
		}
		seenPrerequisites[value.PrerequisiteID] = struct{}{}
	}
	return nil
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
			writeHashString(h, required.TargetRef)
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
	ret := make([]RuntimeRequiredOperationVariant, 0, len(values))
	for _, value := range values {
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		value.TargetRef = strings.TrimSpace(value.TargetRef)
		dependencies := append([]string(nil), value.DependsOnRequirementIDs...)
		for index := range dependencies {
			dependencies[index] = strings.TrimSpace(dependencies[index])
		}
		sort.Strings(dependencies)
		value.DependsOnRequirementIDs = nilIfEmpty(dependencies)
		value.Reason = strings.TrimSpace(value.Reason)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].RequirementID < ret[j].RequirementID })
	return nilIfEmpty(ret)
}

func (c RequiredOutputContract) Normalized() RequiredOutputContract {
	c.AcceptedKinds = normalizeStrings(c.AcceptedKinds)
	c.ImplementationWorkBindingHash = strings.TrimSpace(c.ImplementationWorkBindingHash)
	c.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(c.AcceptedOperationVariants)
	c.AcceptedEffectScopes = NormalizeRuntimeEffectScopes(c.AcceptedEffectScopes)
	c.RequiredEffectScopes = NormalizeRuntimeEffectScopes(c.RequiredEffectScopes)
	c.RequiredEvidenceKinds = normalizeStrings(c.RequiredEvidenceKinds)
	c.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.CoverageWorkItemIDs)
	c.TargetRefs = normalizeStrings(c.TargetRefs)
	c.RepairClosureID = strings.TrimSpace(c.RepairClosureID)
	c.RepairPurpose = RuntimeDispatchRepairPurpose(strings.TrimSpace(string(c.RepairPurpose)))
	c.PrerequisiteIDs = normalizeStrings(c.PrerequisiteIDs)
	c.RequiredOperationSequence = normalizeRequiredOperationVariants(c.RequiredOperationSequence)
	return c
}
func (c RequiredOutputContract) Validate() error {
	if c.ImplementationWorkBindingHash != "" {
		if !implementationWorkExactRef(c.ImplementationWorkBindingHash) || c.RepairPurpose != "" || c.RepairClosureID != "" || len(c.CoverageWorkItemIDs) != 0 || !sameStrings(c.RequiredEvidenceKinds, []string{EvidenceKindStateChange}) || !sameStrings(RuntimeEffectScopesAsStrings(c.RequiredEffectScopes), []string{string(RuntimeEffectWorkspaceDurable)}) {
			return fmt.Errorf("required output work commitment cannot confer coverage or bypass exact repair")
		}
	}
	if c.RepairPurpose != "" {
		if c.RepairPurpose != RuntimeDispatchRepairPurposeConfirmedValidation || strings.TrimSpace(c.RepairClosureID) == "" {
			return fmt.Errorf("required output has invalid repair purpose %q", c.RepairPurpose)
		}
	}
	return validateRequiredOperationVariants(c.RequiredOperationSequence)
}

// validateRequiredOperationVariants rejects malformed or ambiguous SM-3
// sequence authority before normalization can hide it. Dependencies, rather
// than slice order, define the transaction order.
func validateRequiredOperationVariants(values []RuntimeRequiredOperationVariant) error {
	if len(values) == 0 {
		return nil
	}
	byID := make(map[string]RuntimeRequiredOperationVariant, len(values))
	targetOwners := make(map[string]string, len(values))
	for index, raw := range values {
		value := raw
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		value.TargetRef = strings.TrimSpace(value.TargetRef)
		if value.RequirementID == "" || value.Kind == "" {
			return fmt.Errorf("required operation sequence entry %d requires an id and kind", index)
		}
		if _, duplicate := byID[value.RequirementID]; duplicate {
			return fmt.Errorf("required operation sequence duplicates requirement id %q", value.RequirementID)
		}
		if value.TargetRef != "" {
			if strings.TrimSpace(raw.TargetRef) != raw.TargetRef {
				return fmt.Errorf("required operation %q target ref is not canonical", value.RequirementID)
			}
			if owner, duplicate := targetOwners[value.TargetRef]; duplicate {
				return fmt.Errorf("required operation %q duplicates exact target %q owned by %q", value.RequirementID, value.TargetRef, owner)
			}
			targetOwners[value.TargetRef] = value.RequirementID
		}
		byID[value.RequirementID] = value
	}
	for _, raw := range values {
		id := strings.TrimSpace(raw.RequirementID)
		seenDependencies := make(map[string]struct{}, len(raw.DependsOnRequirementIDs))
		for _, dependency := range raw.DependsOnRequirementIDs {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" {
				return fmt.Errorf("required operation %q has an empty dependency", id)
			}
			if dependency == id {
				return fmt.Errorf("required operation %q cannot depend on itself", id)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("required operation %q duplicates dependency %q", id, dependency)
			}
			seenDependencies[dependency] = struct{}{}
			if _, found := byID[dependency]; !found {
				return fmt.Errorf("required operation %q depends on unknown requirement %q", id, dependency)
			}
		}
	}
	visiting := make(map[string]bool, len(byID))
	visited := make(map[string]bool, len(byID))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("required operation sequence contains a dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOnRequirementIDs {
			if err := visit(strings.TrimSpace(dependency)); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRequiredOutputForState checks the output constraints against the
// single canonical SM-2 decision. RequiredOutputContract intentionally carries
// constraints only: it does not duplicate a state or action discriminator.
func ValidateRequiredOutputForState(state ConvergenceState, contract RequiredOutputContract) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	contract = contract.Normalized()
	if contract.ImplementationWorkBindingHash != "" && state != ConvergenceStateNeedStateChange {
		return fmt.Errorf("implementation working output requires need_state_change")
	}
	if contract.RepairPurpose != "" && state != ConvergenceStateNeedStateChange {
		return fmt.Errorf("required output repair purpose %q requires need_state_change", contract.RepairPurpose)
	}
	empty := len(contract.AcceptedKinds)+len(contract.AcceptedOperationVariants)+len(contract.RequiredEvidenceKinds)+len(contract.CoverageWorkItemIDs)+len(contract.TargetRefs)+len(contract.PrerequisiteIDs)+len(contract.RequiredOperationSequence)+len(contract.AcceptedEffectScopes)+len(contract.RequiredEffectScopes) == 0 && contract.RepairClosureID == "" && contract.RepairPurpose == ""
	switch state {
	case ConvergenceStateComplete, ConvergenceStateFailed:
		if !empty {
			return fmt.Errorf("terminal convergence state %q cannot carry required-output constraints", state)
		}
	case ConvergenceStateNeedPrerequisite:
		if len(contract.PrerequisiteIDs) == 0 {
			return fmt.Errorf("prerequisite convergence state requires prerequisite ids")
		}
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		if len(contract.AcceptedKinds) == 0 {
			return fmt.Errorf("%s convergence state requires accepted operation kinds", state)
		}
		if len(contract.CoverageWorkItemIDs) == 0 && contract.ImplementationWorkBindingHash == "" {
			return fmt.Errorf("%s convergence state requires coverage work items", state)
		}
		if len(contract.RequiredEffectScopes) == 0 || len(contract.AcceptedEffectScopes) == 0 {
			return fmt.Errorf("%s convergence state requires explicit required and accepted effect scopes", state)
		}
		if err := ValidateEffectScopesCompatibleWithState(state, contract.RequiredEffectScopes); err != nil {
			return fmt.Errorf("%s required effect scopes: %w", state, err)
		}
		if err := ValidateEffectScopesCompatibleWithState(state, contract.AcceptedEffectScopes); err != nil {
			return fmt.Errorf("%s accepted effect scopes: %w", state, err)
		}
	case ConvergenceStateNeedGovernance:
		if len(contract.AcceptedKinds) == 0 {
			return fmt.Errorf("governance convergence state requires accepted operation kinds")
		}
	case ConvergenceStateNeedAssessment:
		if len(contract.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("assessment convergence state requires coverage work items")
		}
	default:
		return fmt.Errorf("unknown convergence state %q", state)
	}
	return nil
}

func StableRequiredOutputHash(contract RequiredOutputContract) string {
	contract = contract.Normalized()
	h := sha256.New()
	writeHashStrings(h, contract.AcceptedKinds)
	if contract.ImplementationWorkBindingHash != "" {
		writeHashString(h, contract.ImplementationWorkBindingHash)
	}
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
	writeHashString(h, contract.RepairClosureID)
	writeHashString(h, string(contract.RepairPurpose))
	writeHashStrings(h, contract.PrerequisiteIDs)
	for _, requirement := range contract.RequiredOperationSequence {
		writeHashString(h, requirement.RequirementID)
		writeHashString(h, requirement.Kind)
		writeHashString(h, requirement.VariantID)
		writeHashString(h, requirement.TargetRef)
		writeHashStrings(h, requirement.DependsOnRequirementIDs)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func SealRuntimeDecision(facts RuntimeFacts, decision RuntimeDecision, attempt int) (RuntimeDecision, error) {
	facts = facts.Normalized()
	if err := facts.Validate(); err != nil {
		return RuntimeDecision{}, err
	}
	if err := decision.RequiredOutput.Validate(); err != nil {
		return RuntimeDecision{}, err
	}
	decision.Focus = decision.Focus.Normalized()
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	decision.Prerequisites = normalizeRuntimePrerequisites(decision.Prerequisites)
	if decision.Terminal != nil {
		terminal := *decision.Terminal
		terminal.Code = strings.TrimSpace(terminal.Code)
		terminal.Summary = strings.TrimSpace(terminal.Summary)
		terminal.OriginalDiagnostic = terminal.OriginalDiagnostic.CloneOriginal()
		decision.Terminal = &terminal
	}
	requiredOutputHash := StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot = AuthoritySnapshot{
		WorkspaceID:               facts.WorkspaceID,
		RunID:                     facts.RunID,
		State:                     decision.State,
		FocusID:                   decision.Focus.ID,
		RequiredOutputHash:        requiredOutputHash,
		DefinitionRevision:        facts.DefinitionRevision,
		EvidenceRevision:          facts.EvidenceRevision,
		CapabilityStateHash:       StableRuntimeCapabilityStateHash(facts.Resources.CapabilityStates),
		PrerequisiteFreshnessHash: StableRuntimePrerequisiteFreshnessHash(facts.Resources.PrerequisiteObservations),
		OperationContractRevision: facts.Activation.ExecutionContract.OperationContractRevision,
		Attempt:                   attempt,
	}
	decision.AuthoritySnapshot.AuthorityInput = runtimeEquivalentAuthorityInput(facts)
	decision.AuthoritySnapshot.AuthorityInputID = StableRuntimeEquivalentAuthorityInputID(decision.AuthoritySnapshot.AuthorityInput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)
	if err := decision.Validate(); err != nil {
		return RuntimeDecision{}, err
	}
	return decision, nil
}

func (d RuntimeDecision) Validate() error {
	if err := d.RequiredOutput.Validate(); err != nil {
		return err
	}
	d.Focus = d.Focus.Normalized()
	d.RequiredOutput = d.RequiredOutput.Normalized()
	if d.Focus.ID == "" {
		return fmt.Errorf("runtime decision requires a coverage focus")
	}
	terminal, known := convergenceStateContract(d.State)
	if !known {
		return fmt.Errorf("unknown convergence state %q", d.State)
	}
	if err := ValidateRequiredOutputForState(d.State, d.RequiredOutput); err != nil {
		return err
	}
	if terminal != (d.Terminal != nil) {
		return fmt.Errorf("convergence state %s terminal outcome mismatch", d.State)
	}
	if d.Terminal != nil && d.Terminal.Code == "" {
		return fmt.Errorf("terminal outcome requires a code")
	}
	if d.Terminal != nil && d.Terminal.OriginalDiagnostic != nil {
		if err := d.Terminal.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("terminal owner diagnostic: %w", err)
		}
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
		State:                     snapshot.State,
		RequiredOutputHash:        snapshot.RequiredOutputHash,
		DefinitionRevision:        snapshot.DefinitionRevision,
		EvidenceRevision:          snapshot.EvidenceRevision,
		CapabilityStateHash:       snapshot.CapabilityStateHash,
		PrerequisiteFreshnessHash: snapshot.PrerequisiteFreshnessHash,
		OperationContractRevision: snapshot.OperationContractRevision,
		AuthorityInputID:          snapshot.AuthorityInputID,
		Attempt:                   snapshot.Attempt,
	}
}

func convergenceStateContract(state ConvergenceState) (terminal bool, known bool) {
	switch state {
	case ConvergenceStateNeedPrerequisite, ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation,
		ConvergenceStateNeedAssessment, ConvergenceStateNeedGovernance:
		return false, true
	case ConvergenceStateComplete:
		return true, true
	case ConvergenceStateFailed:
		return true, true
	default:
		return false, false
	}
}

func validateAuthoritySnapshot(d RuntimeDecision) error {
	s := d.AuthoritySnapshot
	if s.ID == "" || s.WorkspaceID == "" || s.RunID == "" || s.DefinitionRevision == "" || s.EvidenceRevision == "" || s.AuthorityInputID == "" {
		return fmt.Errorf("runtime decision requires a sealed authority snapshot")
	}
	if s.Attempt < 0 {
		return fmt.Errorf("authority snapshot attempt cannot be negative")
	}
	if s.State != d.State || s.FocusID != d.Focus.ID {
		return fmt.Errorf("authority snapshot does not match decision authority")
	}
	if s.RequiredOutputHash != StableRequiredOutputHash(d.RequiredOutput) {
		return fmt.Errorf("authority snapshot required output hash mismatch")
	}
	if err := s.AuthorityInput.Validate(); err != nil || s.AuthorityInputID != StableRuntimeEquivalentAuthorityInputID(s.AuthorityInput) {
		return fmt.Errorf("authority snapshot input identity mismatch")
	}
	if s.ID != stableAuthoritySnapshotID(s) {
		return fmt.Errorf("authority snapshot id mismatch")
	}
	return nil
}

func stableAuthoritySnapshotID(snapshot AuthoritySnapshot) string {
	return StableAuthoritySnapshotIDForTransaction(snapshot.WorkspaceID, snapshot.RunID, AuthorityTransaction{
		FocusID: snapshot.FocusID, State: snapshot.State, RequiredOutputHash: snapshot.RequiredOutputHash,
		DefinitionRevision: snapshot.DefinitionRevision, EvidenceRevision: snapshot.EvidenceRevision,
		CapabilityStateHash: snapshot.CapabilityStateHash, PrerequisiteFreshnessHash: snapshot.PrerequisiteFreshnessHash,
		OperationContractRevision: snapshot.OperationContractRevision, AuthorityInputID: snapshot.AuthorityInputID, Attempt: snapshot.Attempt,
	})
}

func StableAuthoritySnapshotIDForTransaction(workspaceID common.WorkspaceID, runID common.RunID, transaction AuthorityTransaction) string {
	h := sha256.New()
	writeHashString(h, string(workspaceID))
	writeHashString(h, string(runID))
	writeHashString(h, string(transaction.State))
	writeHashString(h, transaction.FocusID)
	writeHashString(h, transaction.RequiredOutputHash)
	writeHashString(h, transaction.DefinitionRevision)
	writeHashString(h, transaction.EvidenceRevision)
	writeHashString(h, transaction.CapabilityStateHash)
	if transaction.PrerequisiteFreshnessHash != "" {
		writeHashString(h, transaction.PrerequisiteFreshnessHash)
	}
	writeHashString(h, transaction.OperationContractRevision)
	writeHashString(h, transaction.AuthorityInputID)
	writeHashString(h, fmt.Sprintf("%d", transaction.Attempt))
	return "authority-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func runtimeEquivalentAuthorityInput(facts RuntimeFacts) RuntimeEquivalentAuthorityInputIdentity {
	facts = facts.Normalized()
	observationHash := sha256.New()
	for _, observation := range facts.Resources.PrerequisiteObservations {
		writeHashString(observationHash, observation.ObservationID)
		writeHashString(observationHash, observation.ContentHash)
	}
	return RuntimeEquivalentAuthorityInputIdentity{
		DefinitionRevision:        facts.DefinitionRevision,
		FocusHash:                 StableRuntimeAuthorityFocusHash(facts.Focus),
		BindingSetHash:            facts.ReconciliationContract.BindingSetHash,
		EvidenceRevision:          facts.EvidenceRevision,
		CapabilityStateHash:       StableRuntimeCapabilityStateHash(facts.Resources.CapabilityStates),
		CatalogRevisionHash:       StableRuntimeOperationAdmissibilityCatalogRevisionHash(facts.Resources.OperationAdmissibilities),
		PrerequisiteFreshnessHash: StableRuntimePrerequisiteFreshnessHash(facts.Resources.PrerequisiteObservations),
		OwnerObservationHash:      "owner-observation-" + hex.EncodeToString(observationHash.Sum(nil))[:20],
	}.Normalized()
}

// StableRuntimeAuthorityFocusHash is the exact focus identity used by a
// sealed authority input. It intentionally covers only the historical fields
// that have always participated in RuntimeEquivalentAuthorityInputIdentity.
func StableRuntimeAuthorityFocusHash(focus CoverageFocus) string {
	focus = focus.Normalized()
	h := sha256.New()
	writeHashString(h, focus.ID)
	writeHashString(h, focus.SliceID)
	for _, id := range focus.CoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	for _, binding := range focus.CoverageEvidenceBindings {
		writeHashString(h, binding.ID)
		writeHashString(h, binding.Hash)
	}
	if focus.VerificationPolicy != nil {
		writeHashString(h, "verification-policy")
		writeHashString(h, StableRuntimeVerificationPolicyHash(*focus.VerificationPolicy))
	}
	return "focus-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeOperationAdmissibilityCatalogRevisionHash is the catalog
// identity embedded in the authority input. Keeping the preimage algorithm
// public lets historical assessment adjudication prove owner-fact state
// without consulting mutable current resources.
func StableRuntimeOperationAdmissibilityCatalogRevisionHash(values []RuntimeOperationAdmissibility) string {
	values = NormalizeRuntimeOperationAdmissibilities(values)
	h := sha256.New()
	for _, inventory := range values {
		writeHashString(h, inventory.OwnerModuleID)
		writeHashString(h, inventory.SnapshotID)
		writeHashString(h, inventory.ObservedRevision)
	}
	return "catalog-" + hex.EncodeToString(h.Sum(nil))[:20]
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
	// 保持原长度前缀和正文 bytes，仅合并同一次编码的缓冲与 Write。
	encoded := make([]byte, 8+len(value))
	binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
	copy(encoded[8:], value)
	_, _ = w.Write(encoded)
}

func normalizeStrings(values []string) []string {
	canonical := true
	for index, value := range values {
		if value == "" || strings.TrimSpace(value) != value || (index > 0 && values[index-1] >= value) {
			canonical = false
			break
		}
	}
	if canonical {
		return append([]string(nil), values...)
	}
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
		value.Scope = RuntimePrerequisiteScope(strings.TrimSpace(string(value.Scope)))
		if value.Scope == "" {
			value.Scope = RuntimePrerequisiteScopeGlobal
		}
		value.FocusID = strings.TrimSpace(value.FocusID)
		if value.Scope != RuntimePrerequisiteScopeFocus {
			value.Scope = RuntimePrerequisiteScopeGlobal
			value.FocusID = ""
		}
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeStrings(value.TargetSelectorCandidateRefs)
		value.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(value.ProspectiveTargetAllOfConstraintRefs)
		value.ProspectiveTargetRef = strings.TrimSpace(value.ProspectiveTargetRef)
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

func validateRuntimePrerequisiteProspectiveTargetAllOfDeclaration(prerequisite RuntimePrerequisite) error {
	prerequisite = normalizeRuntimePrerequisites([]RuntimePrerequisite{prerequisite})[0]
	if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(prerequisite.ProspectiveTargetAllOfConstraintRefs); err != nil {
		return fmt.Errorf("runtime prerequisite %q prospective target all-of declaration: %w", prerequisite.ID, err)
	}
	if len(prerequisite.ProspectiveTargetAllOfConstraintRefs) != 0 &&
		(prerequisite.Kind != PrerequisiteKindTargetSelection || prerequisite.OwnerModuleID != RuntimeModuleFile ||
			prerequisite.TargetSelectorRef == "" || prerequisite.TargetSelectorOwnerRef == "" || prerequisite.TargetSelectorSubjectRef == "") {
		return fmt.Errorf("runtime prerequisite %q prospective target all-of declaration requires one File target-selection prerequisite with a complete selector", prerequisite.ID)
	}
	return nil
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
		value.FocusID = strings.TrimSpace(value.FocusID)
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
		if value.ContentAddress != nil {
			address := value.ContentAddress.Clone()
			value.ContentAddress = &address
		}
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.TargetRefs = normalizeStrings(value.TargetRefs)
		value.BindingRefs = normalizeStrings(value.BindingRefs)
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
		value.Revision = strings.TrimSpace(value.Revision)
		value.AdjudicationHash = strings.TrimSpace(value.AdjudicationHash)
		value.FocusID = strings.TrimSpace(value.FocusID)
		value.SliceID = strings.TrimSpace(value.SliceID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.RequirementText = strings.TrimSpace(value.RequirementText)
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
		value.ClaimedDisposition = value.ClaimedDisposition.Normalized()
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
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Revision != ret[j].Revision {
			return ret[i].Revision < ret[j].Revision
		}
		return ret[i].ID < ret[j].ID
	})
	return nilIfEmpty(ret)
}
