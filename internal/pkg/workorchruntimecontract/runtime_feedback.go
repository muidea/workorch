package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOriginalDiagnosticSchemaCurrent identifies the bounded, audit-safe
// original-diagnostic representation. It is intentionally not a provider
// payload schema and must never contain a credential, endpoint, raw prompt,
// raw provider body, or unredacted file content.
const RuntimeOriginalDiagnosticSchemaCurrent = "workorch.runtime_original_diagnostic"

// RuntimeFeedbackDomain identifies the factual producer domain. It is an
// owner-neutral value vocabulary, not a state transition or retry policy.
type RuntimeFeedbackDomain string

const (
	RuntimeFeedbackDomainFeasibility RuntimeFeedbackDomain = "feasibility"
	RuntimeFeedbackDomainOperation   RuntimeFeedbackDomain = "operation"
	// RuntimeFeedbackDomainDrive identifies an Orchestrator SM-4 observation.
	// It remains factual: consumers, rather than this DTO, decide whether the
	// observation changes their own lifecycle state.
	RuntimeFeedbackDomainDrive RuntimeFeedbackDomain = "drive"
	// RuntimeFeedbackDomainPersistence identifies a Run-owner opaque aggregate
	// persistence outcome. It is a factual contract, not a retry directive.
	RuntimeFeedbackDomainPersistence RuntimeFeedbackDomain = "persistence"
	// RuntimeFeedbackDomainProviderRequest identifies the factual result of one
	// LLM-owner provider submission. It does not describe candidate admission:
	// complete documents are evaluated only by the Orchestrator SM-3 owner.
	RuntimeFeedbackDomainProviderRequest RuntimeFeedbackDomain = "provider_request"
	// RuntimeFeedbackDomainProviderPreflight identifies the LLM owner's
	// read-only request-rendering and context-integrity observation. It never
	// means that a provider call was submitted.
	RuntimeFeedbackDomainProviderPreflight RuntimeFeedbackDomain = "provider_preflight"
)

// RuntimeFeedbackCategory is the producer's stable, factual category. The
// Orchestrator may consume a category through its own owner-local transition
// table; this DTO must not encode that choice.
type RuntimeFeedbackCategory string

const (
	RuntimeFeedbackCategoryAccepted                  RuntimeFeedbackCategory = "accepted"
	RuntimeFeedbackCategoryStateSchemaUnsupported    RuntimeFeedbackCategory = "state_schema_unsupported"
	RuntimeFeedbackCategoryAggregateRevisionMismatch RuntimeFeedbackCategory = "aggregate_revision_mismatch"
	RuntimeFeedbackCategorySemanticRejection         RuntimeFeedbackCategory = "semantic_rejection"
	RuntimeFeedbackCategoryOwnerTransient            RuntimeFeedbackCategory = "owner_transient"
	RuntimeFeedbackCategoryProviderAuthentication    RuntimeFeedbackCategory = "provider_authentication"
	RuntimeFeedbackCategoryProviderConfiguration     RuntimeFeedbackCategory = "provider_configuration"
	RuntimeFeedbackCategoryProviderTransient         RuntimeFeedbackCategory = "provider_transient"
	RuntimeFeedbackCategoryProviderProtocol          RuntimeFeedbackCategory = "provider_protocol"
	RuntimeFeedbackCategoryContextIntegrity          RuntimeFeedbackCategory = "context_integrity"
)

// RuntimeFeedback carries a bounded factual result across owner, aggregate,
// audit, and projection boundaries. It is a pure value contract: it defines
// neither EventHub topics nor Commands/Results, and it performs no state
// transition, retry, I/O, or failure disposition selection.
type RuntimeFeedback struct {
	Domain              RuntimeFeedbackDomain      `json:"domain"`
	StableCategory      RuntimeFeedbackCategory    `json:"stableCategory"`
	WorkspaceID         common.WorkspaceID         `json:"workspaceId"`
	RunID               common.RunID               `json:"runId"`
	AuthoritySnapshotID string                     `json:"authoritySnapshotId,omitempty"`
	DispatchID          string                     `json:"dispatchId,omitempty"`
	OriginalDiagnostic  *RuntimeOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
}

// RuntimeFeedbackScope carries the identity required by every workspace-scoped
// feedback value. It is deliberately a value object rather than an EventHub
// envelope or aggregate handle.
type RuntimeFeedbackScope struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	DispatchID          string
}

func (s RuntimeFeedbackScope) normalized() RuntimeFeedbackScope {
	s.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(s.WorkspaceID)))
	s.RunID = common.RunID(strings.TrimSpace(string(s.RunID)))
	s.AuthoritySnapshotID = strings.TrimSpace(s.AuthoritySnapshotID)
	s.DispatchID = strings.TrimSpace(s.DispatchID)
	return s
}

func (s RuntimeFeedbackScope) validate() error {
	s = s.normalized()
	if s.WorkspaceID == "" || s.RunID == "" {
		return fmt.Errorf("runtime feedback scope requires workspace id and run id")
	}
	return nil
}

// RuntimeOriginalDiagnostic preserves the producer-authored, audit-safe
// diagnostic exactly as supplied. Do not call Normalized and then persist its
// result in place of this object: Normalized is only for canonical comparison.
//
// FieldViolations and TargetRefs have sequence semantics. Their order is kept
// in both CloneOriginal and Normalized; neither operation sorts or de-duplicates
// them.
type RuntimeOriginalDiagnostic struct {
	SchemaVersion      string                             `json:"schemaVersion"`
	OwnerModuleID      string                             `json:"ownerModuleId"`
	OriginalClass      string                             `json:"originalClass"`
	OriginalCode       string                             `json:"originalCode"`
	OriginalMessage    string                             `json:"originalMessage"`
	Signature          string                             `json:"signature"`
	OperationID        string                             `json:"operationId,omitempty"`
	TargetRefs         []string                           `json:"targetRefs,omitempty"`
	ExpectedRevision   string                             `json:"expectedRevision,omitempty"`
	ObservedRevision   string                             `json:"observedRevision,omitempty"`
	FieldViolations    []RuntimeFieldViolation            `json:"fieldViolations,omitempty"`
	SemanticViolations []RuntimeSemanticContractViolation `json:"semanticViolations,omitempty"`
	DetailsRef         string                             `json:"detailsRef,omitempty"`
	DetailsHash        string                             `json:"detailsHash,omitempty"`
	DetailsBytes       int64                              `json:"detailsBytes,omitempty"`
	ContentType        string                             `json:"contentType,omitempty"`
	OmittedReason      string                             `json:"omittedReason,omitempty"`
	Truncated          bool                               `json:"truncated,omitempty"`
	Redacted           bool                               `json:"redacted,omitempty"`
	// The following typed observations preserve safe producer facts required by
	// the Orchestrator reducer. They are diagnostic detail, never an outcome,
	// retry, target authorization, or transition instruction.
	TargetObservation            *RuntimeFailureTargetObservation     `json:"targetObservation,omitempty"`
	ValidationDiagnostic         *ValidationFailureDiagnostic         `json:"validationDiagnostic,omitempty"`
	ValidationFailureObservation *RuntimeValidationFailureObservation `json:"validationFailureObservation,omitempty"`
	GovernanceDependency         *ExternalDependency                  `json:"governanceDependency,omitempty"`
}

// RuntimeFieldViolation preserves an individual wire or contract violation.
// Allowed, Required, and Forbidden retain producer order because that order can
// explain the original rejection and must not be rewritten by summary logic.
type RuntimeFieldViolation struct {
	Path      string   `json:"path"`
	Actual    string   `json:"actual,omitempty"`
	Allowed   []string `json:"allowed,omitempty"`
	Required  []string `json:"required,omitempty"`
	Forbidden []string `json:"forbidden,omitempty"`
}

// CloneRuntimeFieldViolations preserves producer order and multiplicity while
// detaching the nested slices used by projection and transport consumers.
func CloneRuntimeFieldViolations(values []RuntimeFieldViolation) []RuntimeFieldViolation {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeFieldViolation, 0, len(values))
	for _, value := range values {
		value.Allowed = append([]string(nil), value.Allowed...)
		value.Required = append([]string(nil), value.Required...)
		value.Forbidden = append([]string(nil), value.Forbidden...)
		ret = append(ret, value)
	}
	return ret
}

// CloneOriginal returns an independent exact copy. It deliberately does not
// trim, sort, summarize, or otherwise normalize original producer fields.
func (f RuntimeFeedback) CloneOriginal() RuntimeFeedback {
	f.OriginalDiagnostic = f.OriginalDiagnostic.CloneOriginal()
	return f
}

// Normalized returns a comparison-only copy. It never mutates the original
// feedback and preserves every ordered slice's order and multiplicity.
func (f RuntimeFeedback) Normalized() RuntimeFeedback {
	f.Domain = RuntimeFeedbackDomain(strings.TrimSpace(string(f.Domain)))
	f.StableCategory = RuntimeFeedbackCategory(strings.TrimSpace(string(f.StableCategory)))
	f.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(f.WorkspaceID)))
	f.RunID = common.RunID(strings.TrimSpace(string(f.RunID)))
	f.AuthoritySnapshotID = strings.TrimSpace(f.AuthoritySnapshotID)
	f.DispatchID = strings.TrimSpace(f.DispatchID)
	f.OriginalDiagnostic = f.OriginalDiagnostic.Normalized()
	return f
}

func (f RuntimeFeedback) Validate() error {
	canonical := f.Normalized()
	if canonical.Domain == "" || canonical.StableCategory == "" || canonical.WorkspaceID == "" || canonical.RunID == "" {
		return fmt.Errorf("runtime feedback requires domain, stable category, workspace id, and run id")
	}
	if canonical.OriginalDiagnostic == nil {
		if canonical.StableCategory != RuntimeFeedbackCategoryAccepted {
			return fmt.Errorf("runtime feedback without original diagnostic must be accepted")
		}
		return nil
	}
	if err := canonical.OriginalDiagnostic.Validate(); err != nil {
		return fmt.Errorf("runtime feedback original diagnostic: %w", err)
	}
	return nil
}

// RuntimeFeedbackFromCandidateFailure preserves a candidate rejection without
// assigning a retry, correction, terminal, or other state-machine outcome.
// The producer supplies its stable factual category; this helper deliberately
// does not map failure strings to categories.
func RuntimeFeedbackFromCandidateFailure(scope RuntimeFeedbackScope, domain RuntimeFeedbackDomain, category RuntimeFeedbackCategory, ownerModuleID string, failure RuntimeCandidateFailure) (RuntimeFeedback, error) {
	scope = scope.normalized()
	diagnostic := &RuntimeOriginalDiagnostic{
		SchemaVersion:   RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   strings.TrimSpace(ownerModuleID),
		OriginalClass:   string(failure.Class),
		OriginalCode:    failure.Code,
		OriginalMessage: failure.Reason,
		Signature:       failure.Signature,
		TargetRefs:      append([]string(nil), failure.TargetRefs...),
	}
	if failure.Diagnostic != nil {
		diagnostic.OperationID = failure.Diagnostic.OperationID
		if failure.Diagnostic.Path != "" || failure.Diagnostic.Actual != "" || len(failure.Diagnostic.Allowed) != 0 || len(failure.Diagnostic.Required) != 0 || len(failure.Diagnostic.Forbidden) != 0 {
			diagnostic.FieldViolations = []RuntimeFieldViolation{{
				Path: failure.Diagnostic.Path, Actual: failure.Diagnostic.Actual,
				Allowed: append([]string(nil), failure.Diagnostic.Allowed...), Required: append([]string(nil), failure.Diagnostic.Required...), Forbidden: append([]string(nil), failure.Diagnostic.Forbidden...),
			}}
		}
	}
	feedback := RuntimeFeedback{Domain: domain, StableCategory: category, WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: scope.AuthoritySnapshotID, DispatchID: scope.DispatchID, OriginalDiagnostic: diagnostic}
	if err := feedback.Validate(); err != nil {
		return RuntimeFeedback{}, err
	}
	return feedback, nil
}

// RuntimeFeedbackFromFailureFact preserves the established owner failure
// identity without interpreting it. The caller must provide the owning module
// and already-classified factual category.
func RuntimeFeedbackFromFailureFact(scope RuntimeFeedbackScope, domain RuntimeFeedbackDomain, category RuntimeFeedbackCategory, ownerModuleID string, failure FailureFact) (RuntimeFeedback, error) {
	scope = scope.normalized()
	diagnostic := RuntimeOriginalDiagnosticFromFailureFact(ownerModuleID, failure)
	feedback := RuntimeFeedback{Domain: domain, StableCategory: category, WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: scope.AuthoritySnapshotID, DispatchID: scope.DispatchID, OriginalDiagnostic: diagnostic}
	if err := feedback.Validate(); err != nil {
		return RuntimeFeedback{}, err
	}
	return feedback, nil
}

// RuntimeOriginalDiagnosticFromFailureFact creates the single audit-safe
// producer diagnostic. Raw validation stdout/stderr are deliberately omitted;
// their presence is recorded without persisting execution content.
func RuntimeOriginalDiagnosticFromFailureFact(ownerModuleID string, failure FailureFact) *RuntimeOriginalDiagnostic {
	failure = failure.Clone()
	diagnostic := &RuntimeOriginalDiagnostic{
		SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: strings.TrimSpace(ownerModuleID), OriginalClass: string(failure.Class), OriginalCode: failure.Code,
		OriginalMessage: failure.Message, Signature: failure.Signature, TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if failure.TargetObservation != nil {
		value := failure.TargetObservation.Clone()
		diagnostic.TargetObservation = &value
		diagnostic.ObservedRevision = value.ObservedRevision
	}
	if failure.ValidationDiagnostic != nil {
		value := failure.ValidationDiagnostic.Clone()
		if value.StdoutPreview != "" || value.StderrPreview != "" {
			value.StdoutPreview = ""
			value.StderrPreview = ""
			diagnostic.OmittedReason = "raw_validation_output"
			diagnostic.Redacted = true
		}
		diagnostic.ValidationDiagnostic = &value
	}
	if failure.ValidationFailureObservation != nil {
		value := failure.ValidationFailureObservation.Clone()
		diagnostic.ValidationFailureObservation = &value
	}
	if failure.GovernanceDependency != nil {
		value := failure.GovernanceDependency.Clone()
		diagnostic.GovernanceDependency = &value
	}
	return diagnostic
}

// RuntimeFeedbackFromValidationDiagnostic performs the bounded conversion for
// a validation failure. Stdout/stderr previews are intentionally excluded:
// they are raw execution content, not audit-safe feedback fields.
func RuntimeFeedbackFromValidationDiagnostic(scope RuntimeFeedbackScope, domain RuntimeFeedbackDomain, category RuntimeFeedbackCategory, ownerModuleID, signature string, diagnostic ValidationFailureDiagnostic) (RuntimeFeedback, error) {
	scope = scope.normalized()
	original := &RuntimeOriginalDiagnostic{
		SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: strings.TrimSpace(ownerModuleID), OriginalClass: diagnostic.Class, OriginalCode: diagnostic.Class,
		OriginalMessage: diagnostic.Reason, Signature: strings.TrimSpace(signature), OperationID: diagnostic.Operation,
		TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if diagnostic.CommandKind != "" {
		original.FieldViolations = []RuntimeFieldViolation{{Path: "validation.commandKind", Actual: diagnostic.CommandKind}}
	}
	feedback := RuntimeFeedback{Domain: domain, StableCategory: category, WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: scope.AuthoritySnapshotID, DispatchID: scope.DispatchID, OriginalDiagnostic: original}
	if err := feedback.Validate(); err != nil {
		return RuntimeFeedback{}, err
	}
	return feedback, nil
}

// StableRuntimeFeedbackHash is an exact original-value hash. Unlike
// comparison normalization, it intentionally includes whitespace, ordering,
// and duplicates so an audit record cannot silently overwrite original detail.
func StableRuntimeFeedbackHash(feedback RuntimeFeedback) string {
	feedback = feedback.CloneOriginal()
	h := sha256.New()
	writeHashString(h, string(feedback.Domain))
	writeHashString(h, string(feedback.StableCategory))
	writeHashString(h, string(feedback.WorkspaceID))
	writeHashString(h, string(feedback.RunID))
	writeHashString(h, feedback.AuthoritySnapshotID)
	writeHashString(h, feedback.DispatchID)
	if feedback.OriginalDiagnostic == nil {
		writeHashString(h, "original-diagnostic:nil")
	} else {
		diagnostic := feedback.OriginalDiagnostic
		writeHashString(h, diagnostic.SchemaVersion)
		writeHashString(h, diagnostic.OwnerModuleID)
		writeHashString(h, diagnostic.OriginalClass)
		writeHashString(h, diagnostic.OriginalCode)
		writeHashString(h, diagnostic.OriginalMessage)
		writeHashString(h, diagnostic.Signature)
		writeHashString(h, diagnostic.OperationID)
		writeHashStrings(h, diagnostic.TargetRefs)
		writeHashString(h, diagnostic.ExpectedRevision)
		writeHashString(h, diagnostic.ObservedRevision)
		writeRuntimeFieldViolationsHash(h, diagnostic.FieldViolations)
		writeRuntimeSemanticContractViolationsHash(h, diagnostic.SemanticViolations)
		writeHashString(h, diagnostic.DetailsRef)
		writeHashString(h, diagnostic.DetailsHash)
		writeHashString(h, fmt.Sprintf("%d", diagnostic.DetailsBytes))
		writeHashString(h, diagnostic.ContentType)
		writeHashString(h, diagnostic.OmittedReason)
		writeHashString(h, fmt.Sprintf("%t", diagnostic.Truncated))
		writeHashString(h, fmt.Sprintf("%t", diagnostic.Redacted))
		writeRuntimeOriginalDiagnosticTypedFactsHash(h, diagnostic)
	}
	return "runtime-feedback-" + hex.EncodeToString(h.Sum(nil))
}

func writeRuntimeOriginalDiagnosticTypedFactsHash(h interface{ Write([]byte) (int, error) }, diagnostic *RuntimeOriginalDiagnostic) {
	if diagnostic.TargetObservation == nil {
		writeHashString(h, "target-observation:nil")
	} else {
		value := diagnostic.TargetObservation
		writeHashString(h, value.OwnerModuleID)
		writeHashString(h, value.SubjectRef)
		writeHashString(h, value.TargetRole)
		writeHashString(h, value.ObservedRevision)
		writeHashStrings(h, value.TargetRefs)
		writeHashString(h, value.DiagnosticClass)
		writeHashStrings(h, value.EvidenceRefs)
		writeHashString(h, value.ContentHash)
	}
	if diagnostic.ValidationDiagnostic == nil {
		writeHashString(h, "validation-diagnostic:nil")
	} else {
		value := diagnostic.ValidationDiagnostic
		writeHashString(h, value.SchemaVersion)
		writeHashString(h, value.Class)
		writeHashString(h, value.Reason)
		writeHashString(h, value.RecommendedEvidenceTargetType)
		writeHashString(h, value.Stage)
		writeHashString(h, value.Operation)
		writeHashString(h, value.CommandKind)
		writeHashString(h, fmt.Sprintf("%d", value.ExitCode))
		writeHashString(h, fmt.Sprintf("%t", value.TimedOut))
		writeHashString(h, fmt.Sprintf("%t", value.CommandPolicyViolation))
		writeHashString(h, value.UnavailableCommand)
		writeHashStrings(h, value.TargetRefs)
		writeHashString(h, value.ObservationType)
		writeHashString(h, value.EvidenceMode)
		writeHashString(h, value.StdoutPreview)
		writeHashString(h, value.StderrPreview)
	}
	if diagnostic.ValidationFailureObservation == nil {
		writeHashString(h, "validation-failure-observation:nil")
	} else {
		value := diagnostic.ValidationFailureObservation
		writeHashString(h, string(value.WorkspaceID))
		writeHashString(h, string(value.RunID))
		writeHashString(h, value.AuthoritySnapshotID)
		writeHashString(h, value.DispatchID)
		writeHashString(h, value.OperationID)
		writeHashString(h, value.FailureSignature)
		writeHashString(h, value.TargetRole)
		writeHashString(h, value.DiagnosticClass)
		writeHashStrings(h, value.CandidateTargetRefs)
		writeHashStrings(h, value.ValidationScopeTargetRefs)
		writeHashString(h, value.DiagnosticHash)
	}
	if diagnostic.GovernanceDependency == nil {
		writeHashString(h, "governance-dependency:nil")
	} else {
		writeHashString(h, diagnostic.GovernanceDependency.Kind)
		writeHashString(h, diagnostic.GovernanceDependency.SubjectRef)
		writeHashString(h, diagnostic.GovernanceDependency.Reason)
	}
}

func (d *RuntimeOriginalDiagnostic) CloneOriginal() *RuntimeOriginalDiagnostic {
	if d == nil {
		return nil
	}
	clone := *d
	clone.TargetRefs = append([]string(nil), d.TargetRefs...)
	clone.FieldViolations = make([]RuntimeFieldViolation, len(d.FieldViolations))
	for index := range d.FieldViolations {
		clone.FieldViolations[index] = d.FieldViolations[index].CloneOriginal()
	}
	clone.SemanticViolations = cloneRuntimeSemanticContractViolations(d.SemanticViolations)
	if d.TargetObservation != nil {
		value := d.TargetObservation.Clone()
		clone.TargetObservation = &value
	}
	if d.ValidationDiagnostic != nil {
		value := d.ValidationDiagnostic.Clone()
		clone.ValidationDiagnostic = &value
	}
	if d.ValidationFailureObservation != nil {
		value := d.ValidationFailureObservation.Clone()
		clone.ValidationFailureObservation = &value
	}
	if d.GovernanceDependency != nil {
		value := d.GovernanceDependency.Clone()
		clone.GovernanceDependency = &value
	}
	return &clone
}

// Normalized returns a canonical comparison copy only. It does not sort or
// collapse TargetRefs or FieldViolations because both preserve original order.
func (d *RuntimeOriginalDiagnostic) Normalized() *RuntimeOriginalDiagnostic {
	if d == nil {
		return nil
	}
	canonical := d.CloneOriginal()
	canonical.SchemaVersion = strings.TrimSpace(canonical.SchemaVersion)
	canonical.OwnerModuleID = strings.TrimSpace(canonical.OwnerModuleID)
	canonical.OriginalClass = strings.TrimSpace(canonical.OriginalClass)
	canonical.OriginalCode = strings.TrimSpace(canonical.OriginalCode)
	canonical.OriginalMessage = strings.TrimSpace(canonical.OriginalMessage)
	canonical.Signature = strings.TrimSpace(canonical.Signature)
	canonical.OperationID = strings.TrimSpace(canonical.OperationID)
	canonical.TargetRefs = normalizeOrderedStrings(canonical.TargetRefs)
	canonical.ExpectedRevision = strings.TrimSpace(canonical.ExpectedRevision)
	canonical.ObservedRevision = strings.TrimSpace(canonical.ObservedRevision)
	for index := range canonical.FieldViolations {
		canonical.FieldViolations[index] = canonical.FieldViolations[index].Normalized()
	}
	canonical.SemanticViolations = normalizeRuntimeSemanticContractViolations(canonical.SemanticViolations)
	canonical.DetailsRef = strings.TrimSpace(canonical.DetailsRef)
	canonical.DetailsHash = strings.TrimSpace(canonical.DetailsHash)
	canonical.ContentType = strings.TrimSpace(canonical.ContentType)
	canonical.OmittedReason = strings.TrimSpace(canonical.OmittedReason)
	if canonical.TargetObservation != nil {
		value := canonical.TargetObservation.Normalized()
		canonical.TargetObservation = &value
	}
	if canonical.ValidationDiagnostic != nil {
		value := canonical.ValidationDiagnostic.Normalized()
		canonical.ValidationDiagnostic = &value
	}
	if canonical.ValidationFailureObservation != nil {
		value := canonical.ValidationFailureObservation.Normalized()
		canonical.ValidationFailureObservation = &value
	}
	if canonical.GovernanceDependency != nil {
		value := canonical.GovernanceDependency.Normalized()
		canonical.GovernanceDependency = &value
	}
	return canonical
}

func (d *RuntimeOriginalDiagnostic) Validate() error {
	if d == nil {
		return fmt.Errorf("original diagnostic is required")
	}
	canonical := d.Normalized()
	if canonical.SchemaVersion != RuntimeOriginalDiagnosticSchemaCurrent || canonical.OwnerModuleID == "" || canonical.OriginalClass == "" || canonical.OriginalCode == "" || canonical.OriginalMessage == "" || canonical.Signature == "" {
		return fmt.Errorf("original diagnostic requires schema, owner, class, code, message, and signature")
	}
	if err := validateRuntimeFeedbackSafeText("original message", canonical.OriginalMessage); err != nil {
		return err
	}
	for _, target := range canonical.TargetRefs {
		if err := validateRuntimeFeedbackSafeText("target ref", target); err != nil {
			return err
		}
	}
	for _, violation := range canonical.FieldViolations {
		if err := violation.Validate(); err != nil {
			return err
		}
	}
	for index, violation := range canonical.SemanticViolations {
		if err := violation.Validate(); err != nil {
			return fmt.Errorf("original diagnostic semantic violation %d: %w", index, err)
		}
	}
	if canonical.DetailsBytes < 0 {
		return fmt.Errorf("original diagnostic details bytes cannot be negative")
	}
	if canonical.DetailsRef != "" && canonical.DetailsHash == "" {
		return fmt.Errorf("original diagnostic details ref requires details hash")
	}
	if canonical.DetailsHash != "" && canonical.DetailsRef == "" {
		return fmt.Errorf("original diagnostic details hash requires details ref")
	}
	if canonical.DetailsRef != "" && !runtimeFeedbackSafeDetailsRef(canonical.DetailsRef) {
		return fmt.Errorf("original diagnostic details ref is not an approved opaque reference")
	}
	if canonical.Truncated && canonical.DetailsHash == "" && canonical.DetailsBytes == 0 && canonical.OmittedReason == "" {
		return fmt.Errorf("truncated original diagnostic requires details bytes, hash/ref, or omitted reason")
	}
	if canonical.Redacted && canonical.OmittedReason == "" && canonical.DetailsRef == "" {
		return fmt.Errorf("redacted original diagnostic requires omitted reason or safe details ref")
	}
	if canonical.TargetObservation != nil {
		if err := canonical.TargetObservation.Validate(); err != nil {
			return fmt.Errorf("original diagnostic target observation: %w", err)
		}
	}
	if canonical.ValidationDiagnostic != nil && (canonical.ValidationDiagnostic.StdoutPreview != "" || canonical.ValidationDiagnostic.StderrPreview != "") {
		return fmt.Errorf("original diagnostic cannot persist raw validation output")
	}
	if canonical.ValidationFailureObservation != nil {
		if err := canonical.ValidationFailureObservation.Validate(); err != nil {
			return fmt.Errorf("original diagnostic validation failure observation: %w", err)
		}
		if canonical.ValidationFailureObservation.FailureSignature != canonical.Signature {
			return fmt.Errorf("original diagnostic validation failure observation signature mismatch")
		}
	}
	if canonical.GovernanceDependency != nil {
		if err := canonical.GovernanceDependency.Validate(); err != nil {
			return fmt.Errorf("original diagnostic governance dependency: %w", err)
		}
	}
	return nil
}

func (v RuntimeFieldViolation) CloneOriginal() RuntimeFieldViolation {
	v.Allowed = append([]string(nil), v.Allowed...)
	v.Required = append([]string(nil), v.Required...)
	v.Forbidden = append([]string(nil), v.Forbidden...)
	return v
}

func (v RuntimeFieldViolation) Normalized() RuntimeFieldViolation {
	v = v.CloneOriginal()
	v.Path = strings.TrimSpace(v.Path)
	v.Actual = strings.TrimSpace(v.Actual)
	v.Allowed = normalizeOrderedStrings(v.Allowed)
	v.Required = normalizeOrderedStrings(v.Required)
	v.Forbidden = normalizeOrderedStrings(v.Forbidden)
	return v
}

func (v RuntimeFieldViolation) Validate() error {
	canonical := v.Normalized()
	if canonical.Path == "" {
		return fmt.Errorf("runtime field violation requires path")
	}
	if err := validateRuntimeFeedbackSafeText("path", canonical.Path); err != nil {
		return err
	}
	if err := validateRuntimeFeedbackSafeText("actual", canonical.Actual); err != nil {
		return err
	}
	for _, values := range [][]string{canonical.Allowed, canonical.Required, canonical.Forbidden} {
		for _, value := range values {
			if err := validateRuntimeFeedbackSafeText("field violation value", value); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeRuntimeFieldViolationsHash(writer hashStringWriter, values []RuntimeFieldViolation) {
	writeHashString(writer, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		writeHashString(writer, value.Path)
		writeHashString(writer, value.Actual)
		writeHashStrings(writer, value.Allowed)
		writeHashStrings(writer, value.Required)
		writeHashStrings(writer, value.Forbidden)
	}
}

func writeRuntimeSemanticContractViolationsHash(writer hashStringWriter, values []RuntimeSemanticContractViolation) {
	writeHashString(writer, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		value = value.Normalized()
		writeHashString(writer, string(value.Kind))
		writeHashString(writer, value.Path)
		writeHashString(writer, string(value.RequiredBeforeID))
		writeHashString(writer, string(value.RequiredAfterID))
		writeHashString(writer, fmt.Sprintf("%d", value.ActualBeforePosition))
		writeHashString(writer, fmt.Sprintf("%d", value.ActualAfterPosition))
		for _, id := range value.RequiredCoverageWorkItemIDs {
			writeHashString(writer, string(id))
		}
		writeHashString(writer, string(value.CoverageWorkItemID))
		writeHashString(writer, fmt.Sprintf("%d", value.ProposalIndex))
		writeHashString(writer, string(value.Rule))
		writeHashString(writer, value.Actual)
		writeHashStrings(writer, value.Required)
		writeHashStrings(writer, value.Allowed)
	}
}

func runtimeFeedbackSafeDetailsRef(value string) bool {
	return strings.HasPrefix(value, "workorch://") || strings.HasPrefix(value, "artifact://")
}

func validateRuntimeFeedbackSafeText(field string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"authorization:", "bearer ", "api_key", "api-key", "access_token", "refresh_token",
		"password=", "password:", "private_key", "private-key", "privatekey", "credential=", "credential:",
		"-----begin ", "http://", "https://", "raw prompt:",
	} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("runtime feedback %s contains prohibited sensitive or raw transport content", field)
		}
	}
	return nil
}
