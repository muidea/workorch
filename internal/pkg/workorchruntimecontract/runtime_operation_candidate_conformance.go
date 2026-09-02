package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOperationCandidateConstraintKind identifies a side-effect-free
// owner check that cannot be expressed by the generic JSON/wire schema. The
// constraint is sealed in the executable operation catalog and candidate
// manifest before a provider candidate is requested.
type RuntimeOperationCandidateConstraintKind string

const (
	RuntimeOperationCandidateConstraintParameterShape RuntimeOperationCandidateConstraintKind = "parameter_shape"
)

// RuntimeOperationCandidateRequiredShape is a closed, provider-safe shape
// vocabulary. It describes only the required representation; it carries no
// candidate payload, owner diagnostic text, retry choice, or Runtime state.
type RuntimeOperationCandidateRequiredShape string

const (
	RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile RuntimeOperationCandidateRequiredShape = "unified_diff_single_file"
)

// RuntimeOperationCandidateActualShape is the bounded factual classification
// returned by a capability owner. It never contains the rejected parameter
// value or a free-form parser error.
type RuntimeOperationCandidateActualShape string

const (
	RuntimeOperationCandidateActualShapeInvalidSyntax  RuntimeOperationCandidateActualShape = "invalid_syntax"
	RuntimeOperationCandidateActualShapeTargetMismatch RuntimeOperationCandidateActualShape = "target_mismatch"
)

// RuntimeOperationCandidateConstraint is immutable executable-catalog data.
// Ref is the stable identity shared by the manifest, owner observation and
// detached C4 correction witness.
type RuntimeOperationCandidateConstraint struct {
	Ref           string
	Kind          RuntimeOperationCandidateConstraintKind
	ParameterName string
	RequiredShape RuntimeOperationCandidateRequiredShape
}

func (c RuntimeOperationCandidateConstraint) Normalized() RuntimeOperationCandidateConstraint {
	c.Ref = strings.TrimSpace(c.Ref)
	c.Kind = RuntimeOperationCandidateConstraintKind(strings.TrimSpace(string(c.Kind)))
	c.ParameterName = strings.TrimSpace(c.ParameterName)
	c.RequiredShape = RuntimeOperationCandidateRequiredShape(strings.TrimSpace(string(c.RequiredShape)))
	return c
}

func (c RuntimeOperationCandidateConstraint) Validate() error {
	c = c.Normalized()
	if c.Ref == "" || c.ParameterName == "" {
		return fmt.Errorf("runtime operation candidate constraint requires ref and parameter")
	}
	if c.Kind != RuntimeOperationCandidateConstraintParameterShape {
		return fmt.Errorf("runtime operation candidate constraint kind %q is not declared", c.Kind)
	}
	switch c.RequiredShape {
	case RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile:
		return nil
	default:
		return fmt.Errorf("runtime operation candidate required shape %q is not declared", c.RequiredShape)
	}
}

func NormalizeRuntimeOperationCandidateConstraints(values []RuntimeOperationCandidateConstraint) []RuntimeOperationCandidateConstraint {
	byRef := make(map[string]RuntimeOperationCandidateConstraint, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Ref != "" {
			byRef[value.Ref] = value
		}
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	ret := make([]RuntimeOperationCandidateConstraint, 0, len(refs))
	for _, ref := range refs {
		ret = append(ret, byRef[ref])
	}
	return ret
}

// RuntimeOperationCandidateConstraintsForOperation returns the exact sealed
// constraints selected by the operation variant. It performs no owner call and
// never reads mutable resources.
func RuntimeOperationCandidateConstraintsForOperation(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest) (string, []RuntimeOperationCandidateConstraint, error) {
	operation = operation.Normalized()
	authorization = authorization.Normalized()
	if operation.Kind == "" || authorization.Kind != operation.Kind || authorization.OwnerModuleID == "" {
		return "", nil, fmt.Errorf("runtime operation candidate constraints require matching operation authorization")
	}
	variant, err := MatchRuntimeOperationVariant(operation, RuntimeOperationContract{Kind: authorization.Kind, Variants: authorization.Variants})
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(variant.ID) == "" {
		return "", nil, nil
	}
	constraints := NormalizeRuntimeOperationCandidateConstraints(variant.CandidateConstraints)
	parameterTypes := make(map[string]RuntimeOperationParameterType, len(authorization.Parameters))
	for _, parameter := range authorization.Parameters {
		parameterTypes[strings.TrimSpace(parameter.Name)] = parameter.Type
	}
	for _, constraint := range constraints {
		if err := constraint.Validate(); err != nil {
			return "", nil, err
		}
		if parameterTypes[constraint.ParameterName] != RuntimeOperationParameterTypeString {
			return "", nil, fmt.Errorf("runtime operation candidate constraint %q requires a declared string parameter", constraint.Ref)
		}
	}
	return strings.TrimSpace(variant.ID), constraints, nil
}

// RuntimeCandidateConstraintForOperationRef resolves one unique sealed
// constraint within an exact operation kind. Constraint refs describe an owner
// rule and may therefore be reused by multiple operation kinds; operation kind
// is part of the lookup identity so a manifest containing both operations does
// not become ambiguous. The helper is shared by the Orchestrator correction
// fence and the LLM receiver so neither side invents owner syntax from
// diagnostics.
func RuntimeCandidateConstraintForOperationRef(manifest RuntimeCandidateContractManifest, operationKind, ref string) (RuntimeOperationCandidateConstraint, bool) {
	manifest = manifest.Normalized()
	operationKind = CanonicalRuntimeBlockKind(operationKind)
	ref = strings.TrimSpace(ref)
	if operationKind == "" || ref == "" {
		return RuntimeOperationCandidateConstraint{}, false
	}
	var found RuntimeOperationCandidateConstraint
	for _, operation := range manifest.Operations {
		if operation.Kind != operationKind {
			continue
		}
		for _, variant := range operation.Variants {
			for _, constraint := range NormalizeRuntimeOperationCandidateConstraints(variant.CandidateConstraints) {
				if constraint.Ref != ref {
					continue
				}
				if found.Ref != "" {
					if found == constraint {
						continue
					}
					return RuntimeOperationCandidateConstraint{}, false
				}
				found = constraint
			}
		}
	}
	if found.Ref == "" || found.Validate() != nil {
		return RuntimeOperationCandidateConstraint{}, false
	}
	return found, true
}

// RuntimeOperationCandidateConformanceOutcome is a capability-owner fact. It
// is deliberately distinct from owner feasibility and from the Orchestrator's
// candidate correction disposition.
type RuntimeOperationCandidateConformanceOutcome string

const (
	RuntimeOperationCandidateConformant     RuntimeOperationCandidateConformanceOutcome = "conformant"
	RuntimeOperationCandidateNonconformant  RuntimeOperationCandidateConformanceOutcome = "nonconformant"
	RuntimeOperationCandidateContractDefect RuntimeOperationCandidateConformanceOutcome = "owner_contract_defect"
)

// RuntimeOperationCandidateConformanceViolation is the only bounded rejection
// fact that may leave the capability owner. It contains no raw parameter,
// parser message, correction disposition, or next-action choice.
type RuntimeOperationCandidateConformanceViolation struct {
	ConstraintRef string
	ParameterName string
	RequiredShape RuntimeOperationCandidateRequiredShape
	ActualShape   RuntimeOperationCandidateActualShape
}

func (v RuntimeOperationCandidateConformanceViolation) Normalized() RuntimeOperationCandidateConformanceViolation {
	v.ConstraintRef = strings.TrimSpace(v.ConstraintRef)
	v.ParameterName = strings.TrimSpace(v.ParameterName)
	v.RequiredShape = RuntimeOperationCandidateRequiredShape(strings.TrimSpace(string(v.RequiredShape)))
	v.ActualShape = RuntimeOperationCandidateActualShape(strings.TrimSpace(string(v.ActualShape)))
	return v
}

func (v RuntimeOperationCandidateConformanceViolation) Validate() error {
	v = v.Normalized()
	if v.ConstraintRef == "" || v.ParameterName == "" {
		return fmt.Errorf("runtime operation candidate conformance violation requires constraint and parameter")
	}
	switch v.RequiredShape {
	case RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile:
	default:
		return fmt.Errorf("runtime operation candidate conformance violation required shape %q is not declared", v.RequiredShape)
	}
	switch v.ActualShape {
	case RuntimeOperationCandidateActualShapeInvalidSyntax, RuntimeOperationCandidateActualShapeTargetMismatch:
		return nil
	default:
		return fmt.Errorf("runtime operation candidate actual shape %q is not declared", v.ActualShape)
	}
}

type RuntimeOperationCandidateConformanceRequest struct {
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	Transaction            AuthorityTransaction
	CandidateManifestHash  string
	Operation              RuntimeOperation
	OperationAuthorization RuntimeCandidateOperationManifest
}

func (r RuntimeOperationCandidateConformanceRequest) Normalized() RuntimeOperationCandidateConformanceRequest {
	r.CandidateManifestHash = strings.TrimSpace(r.CandidateManifestHash)
	r.Operation = r.Operation.Normalized()
	r.OperationAuthorization = r.OperationAuthorization.Normalized()
	return r
}

func ValidateRuntimeOperationCandidateConformanceRequest(r RuntimeOperationCandidateConformanceRequest, ownerModuleID string) error {
	r = r.Normalized()
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	if r.WorkspaceID == "" || r.RunID == "" || strings.TrimSpace(r.Transaction.AuthoritySnapshotID) == "" || r.CandidateManifestHash == "" || r.Operation.ID == "" || ownerModuleID == "" {
		return common.ErrInvalidArgument
	}
	descriptor, found := RuntimeOperationDescriptorForKind(r.Operation.Kind)
	if !found || descriptor.OwnerModuleID != ownerModuleID || r.OperationAuthorization.OwnerModuleID != ownerModuleID || r.OperationAuthorization.Kind != r.Operation.Kind {
		return fmt.Errorf("runtime operation candidate conformance owner authorization mismatch")
	}
	if r.OperationAuthorization.OperationContractRevision != RuntimeOperationContractRevision() {
		return fmt.Errorf("runtime operation candidate conformance operation contract revision mismatch")
	}
	if err := ValidateRuntimeOperationContract(r.Operation); err != nil {
		return fmt.Errorf("runtime operation candidate conformance operation contract: %w", err)
	}
	_, constraints, err := RuntimeOperationCandidateConstraintsForOperation(r.Operation, r.OperationAuthorization)
	if err != nil {
		return err
	}
	if len(constraints) == 0 {
		return fmt.Errorf("runtime operation candidate conformance request has no sealed owner constraint")
	}
	return nil
}

type RuntimeOperationCandidateConformanceObservation struct {
	OperationID           string
	OperationKind         string
	VariantID             string
	OwnerModuleID         string
	CandidateManifestHash string
	Outcome               RuntimeOperationCandidateConformanceOutcome
	Violation             *RuntimeOperationCandidateConformanceViolation
}

func (o RuntimeOperationCandidateConformanceObservation) Normalized() RuntimeOperationCandidateConformanceObservation {
	o.OperationID = strings.TrimSpace(o.OperationID)
	o.OperationKind = CanonicalRuntimeBlockKind(o.OperationKind)
	o.VariantID = strings.TrimSpace(o.VariantID)
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.CandidateManifestHash = strings.TrimSpace(o.CandidateManifestHash)
	o.Outcome = RuntimeOperationCandidateConformanceOutcome(strings.TrimSpace(string(o.Outcome)))
	if o.Violation != nil {
		value := o.Violation.Normalized()
		o.Violation = &value
	}
	return o
}

func (o RuntimeOperationCandidateConformanceObservation) Clone() RuntimeOperationCandidateConformanceObservation {
	o = o.Normalized()
	if o.Violation != nil {
		value := *o.Violation
		o.Violation = &value
	}
	return o
}

func (o RuntimeOperationCandidateConformanceObservation) Validate() error {
	o = o.Normalized()
	if o.OperationID == "" || o.OperationKind == "" || o.VariantID == "" || o.OwnerModuleID == "" || o.CandidateManifestHash == "" {
		return fmt.Errorf("runtime operation candidate conformance observation requires operation, owner, variant and manifest identity")
	}
	switch o.Outcome {
	case RuntimeOperationCandidateConformant:
		if o.Violation != nil {
			return fmt.Errorf("conformant runtime operation candidate cannot carry a violation")
		}
	case RuntimeOperationCandidateNonconformant:
		if o.Violation == nil {
			return fmt.Errorf("nonconformant runtime operation candidate requires a bounded violation")
		}
		if err := o.Violation.Validate(); err != nil {
			return err
		}
	case RuntimeOperationCandidateContractDefect:
		if o.Violation != nil {
			return fmt.Errorf("owner contract defect must not expose an untrusted candidate violation")
		}
	default:
		return fmt.Errorf("runtime operation candidate conformance outcome %q is not declared", o.Outcome)
	}
	return nil
}

func (o RuntimeOperationCandidateConformanceObservation) ValidateForRequest(r RuntimeOperationCandidateConformanceRequest, ownerModuleID string) error {
	r = r.Normalized()
	o = o.Normalized()
	if err := ValidateRuntimeOperationCandidateConformanceRequest(r, ownerModuleID); err != nil {
		return err
	}
	if err := o.Validate(); err != nil {
		return err
	}
	variantID, constraints, err := RuntimeOperationCandidateConstraintsForOperation(r.Operation, r.OperationAuthorization)
	if err != nil {
		return err
	}
	if o.OperationID != r.Operation.ID || o.OperationKind != r.Operation.Kind || o.VariantID != variantID || o.OwnerModuleID != strings.TrimSpace(ownerModuleID) || o.CandidateManifestHash != r.CandidateManifestHash {
		return fmt.Errorf("runtime operation candidate conformance observation identity mismatch")
	}
	if o.Violation == nil {
		return nil
	}
	for _, constraint := range constraints {
		if constraint.Ref == o.Violation.ConstraintRef && constraint.ParameterName == o.Violation.ParameterName && constraint.RequiredShape == o.Violation.RequiredShape {
			return nil
		}
	}
	return fmt.Errorf("runtime operation candidate conformance violation is not sealed in the operation authorization")
}

// RuntimeCandidateConformanceRecord is the SM-3 restart fence for one owner
// observation. It is neither an execution receipt nor evidence.
type RuntimeCandidateConformanceRecord struct {
	OperationID           string
	LogicalRequestID      string
	CandidateManifestHash string
	Observation           RuntimeOperationCandidateConformanceObservation
}

func (r RuntimeCandidateConformanceRecord) Clone() RuntimeCandidateConformanceRecord {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.LogicalRequestID = strings.TrimSpace(r.LogicalRequestID)
	r.CandidateManifestHash = strings.TrimSpace(r.CandidateManifestHash)
	r.Observation = r.Observation.Clone()
	return r
}

func ValidateRuntimeCandidateConformanceRecords(values []RuntimeCandidateConformanceRecord) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.OperationID = strings.TrimSpace(value.OperationID)
		value.LogicalRequestID = strings.TrimSpace(value.LogicalRequestID)
		value.CandidateManifestHash = strings.TrimSpace(value.CandidateManifestHash)
		value.Observation = value.Observation.Normalized()
		if value.OperationID == "" || value.LogicalRequestID == "" || value.CandidateManifestHash == "" || value.OperationID != value.Observation.OperationID || value.CandidateManifestHash != value.Observation.CandidateManifestHash {
			return fmt.Errorf("runtime candidate conformance record identity mismatch")
		}
		if _, duplicate := seen[value.OperationID]; duplicate {
			return fmt.Errorf("runtime candidate conformance operation %q is duplicated", value.OperationID)
		}
		seen[value.OperationID] = struct{}{}
		if err := value.Observation.Validate(); err != nil {
			return err
		}
	}
	return nil
}
