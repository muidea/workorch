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
	ID                    string
	AssessmentScope       string
	AssessmentID          string
	AssessmentText        string
	CoverageWorkItemIDs   []common.CoverageWorkItemID
	RequiredEvidenceKinds []string
	TargetRefs            []string
	EvidenceRole          string
	AcceptedStepKinds     []string
	AcceptedOperations    []string
	RejectedStepKinds     []string
	RequireAssessment     bool
}

type EvidenceLedger struct {
	Accepted []EvidenceFact
	Rejected []EvidenceFact
}

type RuntimeAssessmentFact struct {
	FocusID             string
	Scope               string
	RequirementID       string
	RequirementText     string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Satisfied           bool
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
	CoverageWorkItemIDs []common.CoverageWorkItemID
}

type RuntimeOperationEvidence struct {
	Summary      string
	ContentHash  string
	ExitCode     *int64
	EvidenceRefs []string
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
	FailureClassGovernanceRejected           FailureClass = "governance_rejected"
)

type RuntimeOperationExecutionStatus string

const (
	RuntimeOperationExecutionStatusAccepted RuntimeOperationExecutionStatus = "accepted"
	RuntimeOperationExecutionStatusRejected RuntimeOperationExecutionStatus = "rejected"
)

type FailureFact struct {
	Class      FailureClass
	Code       string
	Signature  string
	Message    string
	TargetRefs []string
}

type PrerequisiteState struct {
	Pending   []RuntimePrerequisite
	Satisfied []RuntimePrerequisite
}

type PrerequisiteKind string

const (
	PrerequisiteKindManifest          PrerequisiteKind = "manifest"
	PrerequisiteKindTargetSnapshot    PrerequisiteKind = "target_snapshot"
	PrerequisiteKindFailureTarget     PrerequisiteKind = "failure_target"
	PrerequisiteKindCapability        PrerequisiteKind = "capability"
	PrerequisiteKindValidationScratch PrerequisiteKind = "validation_scratch"
	PrerequisiteKindProviderCeiling   PrerequisiteKind = "provider_ceiling"
)

type RuntimePrerequisite struct {
	ID                   string
	Kind                 PrerequisiteKind
	OwnerModuleID        string
	SubjectRef           string
	CapabilityClass      RuntimeCapabilityClass
	RequiredCapabilities []string
	MinimumBytes         int
	Reason               string
}

type RuntimeRequirement struct {
	Kind                 PrerequisiteKind
	OwnerModuleID        string
	SubjectRef           string
	CapabilityClass      RuntimeCapabilityClass
	RequiredCapabilities []string
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
	Manifest          *WorkspaceManifest
	TargetSnapshots   []TargetSnapshot
	FailureTarget     *FailureTarget
	ValidationScratch *ValidationScratch
	ProviderCeiling   *ProviderCeiling
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

func (s TargetSnapshot) Complete() bool {
	return s.ContentState == SnapshotContentStateComplete && s.ContentHash != ""
}

type FailureTarget struct {
	Signature  string
	TargetRefs []string
}

type ValidationScratch struct {
	Reference string
	Revision  string
}

type ProviderCeiling struct {
	ProviderID string
	MaxBytes   int
	Revision   string
}

type RequiredOutputContract struct {
	Kind                  RequiredOutputKind
	AcceptedKinds         []string
	AcceptedOperations    []string
	RequiredEvidenceKinds []string
	CoverageWorkItemIDs   []common.CoverageWorkItemID
	TargetRefs            []string
	PrerequisiteIDs       []string
}

type AuthoritySnapshot struct {
	ID                     string
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	State                  ConvergenceState
	Action                 RuntimeAction
	FocusID                string
	RequiredOutputHash     string
	DefinitionRevision     string
	EvidenceRevision       string
	Attempt                int
	RuntimeContractVersion string
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
	AuthoritySnapshotID string
	FocusID             string
	Action              RuntimeAction
	RequiredOutputHash  string
	DefinitionRevision  string
	EvidenceRevision    string
	Attempt             int
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
	if f.Failure != nil {
		failure := *f.Failure
		failure.Class = FailureClass(strings.TrimSpace(string(failure.Class)))
		failure.Code = strings.TrimSpace(failure.Code)
		failure.Signature = strings.TrimSpace(failure.Signature)
		failure.Message = strings.TrimSpace(failure.Message)
		failure.TargetRefs = normalizeStrings(failure.TargetRefs)
		f.Failure = &failure
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
	return nil
}

func (f CoverageFocus) Normalized() CoverageFocus {
	f.ID = strings.TrimSpace(f.ID)
	f.AssessmentScope = strings.TrimSpace(f.AssessmentScope)
	f.AssessmentID = strings.TrimSpace(f.AssessmentID)
	f.AssessmentText = strings.TrimSpace(f.AssessmentText)
	f.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(f.CoverageWorkItemIDs)
	f.RequiredEvidenceKinds = normalizeStrings(f.RequiredEvidenceKinds)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.EvidenceRole = strings.TrimSpace(f.EvidenceRole)
	f.AcceptedStepKinds = normalizeStrings(f.AcceptedStepKinds)
	f.AcceptedOperations = normalizeStrings(f.AcceptedOperations)
	f.RejectedStepKinds = normalizeStrings(f.RejectedStepKinds)
	return f
}

func (r RuntimeResourceFacts) Normalized() RuntimeResourceFacts {
	if r.Manifest != nil {
		manifest := *r.Manifest
		manifest.Revision = strings.TrimSpace(manifest.Revision)
		manifest.TargetRefs = normalizeStrings(manifest.TargetRefs)
		r.Manifest = &manifest
	}
	r.TargetSnapshots = normalizeTargetSnapshots(r.TargetSnapshots)
	if r.FailureTarget != nil {
		target := *r.FailureTarget
		target.Signature = strings.TrimSpace(target.Signature)
		target.TargetRefs = normalizeStrings(target.TargetRefs)
		r.FailureTarget = &target
	}
	if r.ValidationScratch != nil {
		scratch := *r.ValidationScratch
		scratch.Reference = strings.TrimSpace(scratch.Reference)
		scratch.Revision = strings.TrimSpace(scratch.Revision)
		r.ValidationScratch = &scratch
	}
	if r.ProviderCeiling != nil {
		ceiling := *r.ProviderCeiling
		ceiling.ProviderID = strings.TrimSpace(ceiling.ProviderID)
		ceiling.Revision = strings.TrimSpace(ceiling.Revision)
		r.ProviderCeiling = &ceiling
	}
	return r
}

func (c RequiredOutputContract) Normalized() RequiredOutputContract {
	c.AcceptedKinds = normalizeStrings(c.AcceptedKinds)
	c.AcceptedOperations = normalizeStrings(c.AcceptedOperations)
	c.RequiredEvidenceKinds = normalizeStrings(c.RequiredEvidenceKinds)
	c.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.CoverageWorkItemIDs)
	c.TargetRefs = normalizeStrings(c.TargetRefs)
	c.PrerequisiteIDs = normalizeStrings(c.PrerequisiteIDs)
	return c
}

func (c RequiredOutputContract) Validate() error {
	c = c.Normalized()
	switch c.Kind {
	case RequiredOutputKindNone:
		if len(c.AcceptedKinds)+len(c.AcceptedOperations)+len(c.RequiredEvidenceKinds)+len(c.CoverageWorkItemIDs)+len(c.TargetRefs)+len(c.PrerequisiteIDs) != 0 {
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
	writeHashStrings(h, contract.AcceptedOperations)
	writeHashStrings(h, contract.RequiredEvidenceKinds)
	coverageIDs := make([]string, 0, len(contract.CoverageWorkItemIDs))
	for _, id := range contract.CoverageWorkItemIDs {
		coverageIDs = append(coverageIDs, string(id))
	}
	writeHashStrings(h, coverageIDs)
	writeHashStrings(h, contract.TargetRefs)
	writeHashStrings(h, contract.PrerequisiteIDs)
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
		WorkspaceID:            facts.WorkspaceID,
		RunID:                  facts.RunID,
		State:                  decision.State,
		Action:                 decision.Action,
		FocusID:                decision.Focus.ID,
		RequiredOutputHash:     requiredOutputHash,
		DefinitionRevision:     facts.DefinitionRevision,
		EvidenceRevision:       facts.EvidenceRevision,
		Attempt:                attempt,
		RuntimeContractVersion: RuntimeContractVersionV2,
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
		AuthoritySnapshotID: snapshot.ID,
		FocusID:             snapshot.FocusID,
		Action:              snapshot.Action,
		RequiredOutputHash:  snapshot.RequiredOutputHash,
		DefinitionRevision:  snapshot.DefinitionRevision,
		EvidenceRevision:    snapshot.EvidenceRevision,
		Attempt:             snapshot.Attempt,
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
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
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
		value.FocusID = strings.TrimSpace(value.FocusID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.RequirementText = strings.TrimSpace(value.RequirementText)
		value.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(value.CoverageWorkItemIDs)
		value.Summary = strings.TrimSpace(value.Summary)
		value.EvidenceRefs = normalizeStrings(value.EvidenceRefs)
		value.ArtifactRefs = normalizeStrings(value.ArtifactRefs)
		if value.FocusID == "" {
			continue
		}
		if _, ok := seen[value.FocusID]; ok {
			continue
		}
		seen[value.FocusID] = struct{}{}
		ret = append(ret, value)
	}
	return nilIfEmpty(ret)
}
