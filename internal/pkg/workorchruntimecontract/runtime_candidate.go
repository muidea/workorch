package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeCandidateContext struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DefinitionRevision string
	EvidenceRevision   string
	Focus              CoverageFocus
	RemainingFocuses   []CoverageFocus
	Evidence           EvidenceLedger
	Assessments        []RuntimeAssessmentFact
	Failure            *FailureFact
	// RequiredState is the canonical decision authority for this candidate attempt.
	// When RepairDirective is set it mirrors directive.RequiredState (INV-05).
	RequiredState             ConvergenceState
	RequiredEffectScopes      []RuntimeEffectScope
	RequiredEvidenceKinds     []string
	RepairDirective           *RuntimeRepairDirective
	AdmissibleOperationKinds  []string
	RejectedOperationShapes   []RuntimeRejectedOperationShape
	NoProgressOperationShapes []RuntimeRejectedOperationShape
	Capabilities              RuntimeCapabilities
	Resources                 RuntimeResourceFacts
	Activation                RuntimeActivationContext
}

func RuntimeCandidateContextFromFacts(facts RuntimeFacts) RuntimeCandidateContext {
	facts = facts.Normalized()
	ctx := RuntimeCandidateContext{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		DefinitionRevision: facts.DefinitionRevision, EvidenceRevision: facts.EvidenceRevision,
		Focus: facts.Focus, RemainingFocuses: facts.RemainingFocuses, Evidence: facts.Evidence,
		Assessments: facts.Assessments, Failure: facts.Failure,
		Capabilities: facts.Capabilities, Resources: facts.Resources, Activation: facts.Activation,
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID {
		directive := facts.RepairDirective.Clone()
		ctx.RepairDirective = &directive
		ctx.RequiredState = directive.RequiredState
		ctx.RequiredEffectScopes = append([]RuntimeEffectScope(nil), directive.RequiredEffectScopes...)
		ctx.RequiredEvidenceKinds = append([]string(nil), directive.RequiredEvidenceKinds...)
		if len(directive.AllowedOperationKinds) > 0 {
			ctx.AdmissibleOperationKinds = append([]string(nil), directive.AllowedOperationKinds...)
		}
		if len(directive.RejectedOperationShapes) > 0 {
			ctx.RejectedOperationShapes = append([]RuntimeRejectedOperationShape(nil), directive.RejectedOperationShapes...)
		}
		if len(directive.NoProgressOperationShapes) > 0 {
			ctx.NoProgressOperationShapes = append([]RuntimeRejectedOperationShape(nil), directive.NoProgressOperationShapes...)
		}
	}
	return ctx.Normalized()
}

// RuntimeCandidateContextForDecision builds a transport DTO from already-derived
// owner facts. It must not inspect dispatch or attempt ledgers to rebuild
// authority; those facts belong to the Orchestrator candidate-admission owner.
func RuntimeCandidateContextForDecision(facts RuntimeFacts, decision RuntimeDecision, rejectedShapes []RuntimeRejectedOperationShape) RuntimeCandidateContext {
	context := RuntimeCandidateContextFromFacts(facts)
	// The decision is the sealed SM-2/SM-3 authority for this request. Copy its
	// execution requirements even when no RepairDirective exists: leaving the
	// C3 requiredState blank made the provider-facing instruction weaker than
	// the persisted decision and invited candidates for a different state.
	required := decision.RequiredOutput.Normalized()
	context.RequiredState = decision.State
	context.RequiredEffectScopes = append([]RuntimeEffectScope(nil), required.RequiredEffectScopes...)
	context.RequiredEvidenceKinds = append([]string(nil), required.RequiredEvidenceKinds...)
	context.AdmissibleOperationKinds = FeasibleOperationKindsWithOwnerValidationRecipes(decision.State, decision.RequiredOutput, facts.Capabilities, facts.Activation.MCPCapabilities, RuntimeValidationRecipesFromOperationAdmissibilities(facts.Resources.OperationAdmissibilities))
	context.RejectedOperationShapes = NormalizeRuntimeRejectedOperationShapes(rejectedShapes)
	return context.Normalized()
}

func (c RuntimeCandidateContext) Normalized() RuntimeCandidateContext {
	facts := RuntimeFacts{
		WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.DefinitionRevision,
		EvidenceRevision: c.EvidenceRevision, Focus: c.Focus, RemainingFocuses: c.RemainingFocuses,
		Evidence: c.Evidence, Assessments: c.Assessments,
		Failure: c.Failure, Capabilities: c.Capabilities, Resources: c.Resources,
		Activation: c.Activation,
	}.Normalized()
	c.DefinitionRevision = facts.DefinitionRevision
	c.EvidenceRevision = facts.EvidenceRevision
	c.Focus = facts.Focus
	c.RemainingFocuses = facts.RemainingFocuses
	c.Evidence = facts.Evidence
	c.Assessments = facts.Assessments
	c.Failure = facts.Failure
	c.RequiredState = ConvergenceState(strings.TrimSpace(string(c.RequiredState)))
	c.RequiredEffectScopes = NormalizeRuntimeEffectScopes(c.RequiredEffectScopes)
	c.RequiredEvidenceKinds = normalizeStrings(c.RequiredEvidenceKinds)
	if c.RepairDirective != nil {
		directive := c.RepairDirective.Normalized()
		c.RepairDirective = &directive
		if c.RequiredState == "" {
			c.RequiredState = directive.RequiredState
		}
		if len(c.RequiredEffectScopes) == 0 {
			c.RequiredEffectScopes = append([]RuntimeEffectScope(nil), directive.RequiredEffectScopes...)
		}
		if len(c.RequiredEvidenceKinds) == 0 {
			c.RequiredEvidenceKinds = append([]string(nil), directive.RequiredEvidenceKinds...)
		}
	}
	c.AdmissibleOperationKinds = normalizeStrings(c.AdmissibleOperationKinds)
	c.RejectedOperationShapes = NormalizeRuntimeRejectedOperationShapes(c.RejectedOperationShapes)
	c.NoProgressOperationShapes = NormalizeRuntimeRejectedOperationShapes(c.NoProgressOperationShapes)
	c.Capabilities = facts.Capabilities
	c.Resources = facts.Resources
	c.Activation = facts.Activation
	return c
}

func (c RuntimeCandidateContext) Validate(decision RuntimeDecision) error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.DefinitionRevision == "" || c.EvidenceRevision == "" || c.Focus.ID == "" {
		return fmt.Errorf("runtime candidate context requires scope, revisions and focus")
	}
	if err := c.Activation.Validate(); err != nil {
		return err
	}
	snapshot := decision.AuthoritySnapshot
	if c.WorkspaceID != snapshot.WorkspaceID || c.RunID != snapshot.RunID || c.DefinitionRevision != snapshot.DefinitionRevision ||
		c.EvidenceRevision != snapshot.EvidenceRevision || c.Focus.ID != snapshot.FocusID {
		return fmt.Errorf("runtime candidate context does not match sealed authority")
	}
	required := decision.RequiredOutput.Normalized()
	if c.RequiredState != decision.State {
		return fmt.Errorf("runtime candidate context required state %q does not match sealed decision state %q", c.RequiredState, decision.State)
	}
	if !slices.Equal(c.RequiredEffectScopes, required.RequiredEffectScopes) || !slices.Equal(c.RequiredEvidenceKinds, required.RequiredEvidenceKinds) {
		return fmt.Errorf("runtime candidate context requirements do not match sealed decision output")
	}
	wantKinds := FeasibleOperationKindsWithOwnerValidationRecipes(decision.State, decision.RequiredOutput, c.Capabilities, c.Activation.MCPCapabilities, RuntimeValidationRecipesFromOperationAdmissibilities(c.Resources.OperationAdmissibilities))
	if !slices.Equal(c.AdmissibleOperationKinds, wantKinds) {
		return fmt.Errorf("runtime candidate context admissible operation kinds do not match sealed decision")
	}
	return nil
}

type RuntimeCandidateFailureClass string

const (
	RuntimeCandidateFailureClassSchema                 RuntimeCandidateFailureClass = "schema"
	RuntimeCandidateFailureClassContract               RuntimeCandidateFailureClass = "contract"
	RuntimeCandidateFailureClassProvider               RuntimeCandidateFailureClass = "provider"
	RuntimeCandidateFailureClassProviderTransient      RuntimeCandidateFailureClass = "provider_transient"
	RuntimeCandidateFailureClassProviderAuthentication RuntimeCandidateFailureClass = "provider_authentication"
	RuntimeCandidateFailureClassProviderConfiguration  RuntimeCandidateFailureClass = "provider_configuration"
	RuntimeCandidateFailureClassProviderProtocol       RuntimeCandidateFailureClass = "provider_protocol"
	RuntimeCandidateFailureClassInternal               RuntimeCandidateFailureClass = "internal"
)

type RuntimeCandidateFailure struct {
	Class      RuntimeCandidateFailureClass
	Code       string
	Signature  string
	Reason     string
	TargetRefs []string
	// Diagnostic is machine-actionable correction metadata for a schema or
	// sealed-manifest rejection. Reason remains persisted/audit context and
	// must never be rendered back into a provider correction request.
	Diagnostic           *RuntimeCandidateFailureDiagnostic
	ValidationDiagnostic *ValidationFailureDiagnostic
}

// RuntimeCandidateFailureDiagnostic is the bounded, provider-safe projection
// of a schema or semantic candidate rejection. It is a pure value DTO: no
// action selection, aggregate mutation, EventHub, or I/O belongs here.
type RuntimeCandidateFailureDiagnostic struct {
	OperationID       string                                `json:"operationId,omitempty"`
	Path              string                                `json:"path,omitempty"`
	Actual            string                                `json:"actual,omitempty"`
	Allowed           []string                              `json:"allowed,omitempty"`
	Required          []string                              `json:"required,omitempty"`
	Forbidden         []string                              `json:"forbidden,omitempty"`
	ManifestHash      string                                `json:"manifestHash,omitempty"`
	RejectedShapeHash string                                `json:"rejectedShapeHash,omitempty"`
	Objectives        []RuntimeCandidateCorrectionObjective `json:"objectives,omitempty"`
}

// RuntimeCandidateCorrectionObjective is a bounded, machine-readable delta
// for the one permitted same-contract correction. It is a DTO vocabulary, not
// an action selector or retry policy.
type RuntimeCandidateCorrectionObjective string

const (
	RuntimeCandidateCorrectionRemoveForbiddenField RuntimeCandidateCorrectionObjective = "remove_forbidden_field"
	RuntimeCandidateCorrectionSupplyRequiredField  RuntimeCandidateCorrectionObjective = "supply_required_field"
	RuntimeCandidateCorrectionReplaceTarget        RuntimeCandidateCorrectionObjective = "replace_target_with_allowed_target"
	RuntimeCandidateCorrectionSelectOperationKind  RuntimeCandidateCorrectionObjective = "select_allowed_operation_kind"
	RuntimeCandidateCorrectionSelectDurableEffect  RuntimeCandidateCorrectionObjective = "select_durable_effect"
	RuntimeCandidateCorrectionChangeReplayedShape  RuntimeCandidateCorrectionObjective = "change_replayed_shape"
	RuntimeCandidateCorrectionSatisfyCoverage      RuntimeCandidateCorrectionObjective = "satisfy_required_coverage"
	RuntimeCandidateCorrectionSatisfyDependency    RuntimeCandidateCorrectionObjective = "satisfy_dependency_binding"
)

func (d *RuntimeCandidateFailureDiagnostic) Clone() *RuntimeCandidateFailureDiagnostic {
	if d == nil {
		return nil
	}
	clone := d.Normalized()
	return &clone
}

func (d RuntimeCandidateFailureDiagnostic) Normalized() RuntimeCandidateFailureDiagnostic {
	d.OperationID = strings.TrimSpace(d.OperationID)
	d.Path = strings.TrimSpace(d.Path)
	d.Actual = strings.TrimSpace(d.Actual)
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.RejectedShapeHash = strings.TrimSpace(d.RejectedShapeHash)
	d.Allowed = normalizeStrings(d.Allowed)
	d.Required = normalizeStrings(d.Required)
	d.Forbidden = normalizeStrings(d.Forbidden)
	seen := map[RuntimeCandidateCorrectionObjective]struct{}{}
	objectives := make([]RuntimeCandidateCorrectionObjective, 0, len(d.Objectives))
	for _, objective := range d.Objectives {
		objective = RuntimeCandidateCorrectionObjective(strings.TrimSpace(string(objective)))
		if objective == "" {
			continue
		}
		if _, found := seen[objective]; !found {
			seen[objective] = struct{}{}
			objectives = append(objectives, objective)
		}
	}
	sort.Slice(objectives, func(left, right int) bool { return objectives[left] < objectives[right] })
	d.Objectives = objectives
	return d
}

func StableRuntimeCandidateHash(candidate RuntimeCandidate) string {
	h := sha256.New()
	switch {
	case candidate.StateChange != nil:
		writeHashString(h, "state_change")
		writeCandidateOperations(h, stateChangingRuntimeOperations(candidate.StateChange.Operations))
	case candidate.Validation != nil:
		writeHashString(h, "validation")
		writeCandidateOperations(h, validationRuntimeOperations(candidate.Validation.Operations))
	case candidate.Assessment != nil:
		writeHashString(h, "assessment")
		value := candidate.Assessment.Normalized()
		writeHashString(h, value.FocusID)
		coverage := make([]string, 0, len(value.CoverageWorkItemIDs))
		for _, id := range value.CoverageWorkItemIDs {
			coverage = append(coverage, string(id))
		}
		writeHashStrings(h, coverage)
		writeHashString(h, string(value.Disposition))
		writeHashString(h, value.Summary)
		writeHashStrings(h, value.EvidenceRefs)
		writeHashStrings(h, value.ArtifactRefs)
	case candidate.Governance != nil:
		writeHashString(h, "governance")
		writeHashString(h, strings.TrimSpace(candidate.Governance.ExternalDependency.Kind))
		writeHashString(h, strings.TrimSpace(candidate.Governance.ExternalDependency.SubjectRef))
		writeHashString(h, strings.TrimSpace(candidate.Governance.ExternalDependency.Reason))
	default:
		writeHashString(h, "empty")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeCandidateOperations(h hashStringWriter, operations []RuntimeOperation) {
	writeHashString(h, fmt.Sprintf("%d", len(operations)))
	for _, value := range operations {
		operation := value.Normalized()
		writeHashString(h, operation.ID)
		writeHashString(h, operation.Kind)
		writeHashString(h, operation.RequiredOperationRef)
		writeHashStrings(h, operation.DependsOn)
		writeHashStrings(h, operation.BindingRefs)
		writeHashStrings(h, operation.TargetRefs)
		writeHashString(h, fmt.Sprintf("%d", len(operation.Parameters)))
		for _, parameter := range operation.Parameters {
			writeHashString(h, parameter.Name)
			switch {
			case parameter.String != nil:
				writeHashString(h, "string")
				writeHashString(h, *parameter.String)
			case parameter.Integer != nil:
				writeHashString(h, "integer")
				writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
			case parameter.Boolean != nil:
				writeHashString(h, "boolean")
				writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
			case len(parameter.StringList) > 0:
				writeHashString(h, "string_list")
				writeHashStrings(h, parameter.StringList)
			default:
				writeHashString(h, "empty")
			}
		}
	}
}

type OperationParameter struct {
	Name       string
	String     *string
	Integer    *int64
	Boolean    *bool
	StringList []string
}

type RuntimeOperation struct {
	ID                   string
	Kind                 string
	RequiredOperationRef string
	DependsOn            []string
	// BindingRefs select only immutable proof declarations sealed in the
	// current manifest. CoverageWorkItemIDs remains temporarily representable
	// for in-process diagnostic fixtures, but the manifest evaluator rejects it
	// for every executable candidate and it never reaches a sealed dispatch.
	BindingRefs         []string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	TargetRefs          []string
	Parameters          []OperationParameter
}

type StateChangingOperation struct {
	Operation RuntimeOperation
}

type ValidationOperation struct {
	Operation RuntimeOperation
}

type StateChangeCandidate struct {
	Operations []StateChangingOperation
}

type ValidationCandidate struct {
	Operations []ValidationOperation
}

type AssessmentCandidate struct {
	FocusID             string
	SliceID             string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Disposition         AssessmentDisposition
	Items               []RuntimeAssessmentItemFact
	Gap                 *RuntimeAssessmentGap
	Summary             string
	EvidenceRefs        []string
	ArtifactRefs        []string
}

func (c AssessmentCandidate) Normalized() AssessmentCandidate {
	c.FocusID = strings.TrimSpace(c.FocusID)
	c.SliceID = strings.TrimSpace(c.SliceID)
	c.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.CoverageWorkItemIDs)
	c.Disposition = c.Disposition.Normalized()
	c.Summary = strings.TrimSpace(c.Summary)
	c.EvidenceRefs = normalizeStrings(c.EvidenceRefs)
	c.ArtifactRefs = normalizeStrings(c.ArtifactRefs)
	if len(c.Items) > 0 {
		items := make([]RuntimeAssessmentItemFact, 0, len(c.Items))
		for _, item := range c.Items {
			item = item.Normalized()
			if item.CoverageWorkItemID != "" {
				items = append(items, item)
			}
		}
		c.Items = items
	}
	if c.Gap != nil {
		gap := c.Gap.Normalized()
		c.Gap = &gap
	}
	return c
}

type ExternalDependency struct {
	Kind       string `json:"kind"`
	SubjectRef string `json:"subjectRef"`
	Reason     string `json:"reason"`
}

// Normalized returns the canonical, value-only representation of an external
// dependency. It deliberately carries no owner handle, ticket, or state
// transition: the dependency is an input contract, while the Orchestrator
// remains the governance state owner.
func (d ExternalDependency) Normalized() ExternalDependency {
	d.Kind = strings.TrimSpace(d.Kind)
	d.SubjectRef = strings.TrimSpace(d.SubjectRef)
	d.Reason = strings.TrimSpace(d.Reason)
	return d
}

func (d ExternalDependency) Validate() error {
	d = d.Normalized()
	if d.Kind == "" || d.SubjectRef == "" || d.Reason == "" {
		return fmt.Errorf("external dependency requires kind, subject ref and reason")
	}
	return nil
}

func (d ExternalDependency) Clone() ExternalDependency {
	return d.Normalized()
}

type GovernanceCandidate struct {
	ExternalDependency ExternalDependency
}

type RuntimeCandidate struct {
	StateChange *StateChangeCandidate
	Validation  *ValidationCandidate
	Assessment  *AssessmentCandidate
	Governance  *GovernanceCandidate
}

func (c RuntimeCandidate) Validate(decision RuntimeDecision) error {
	return c.ValidateWithActivation(decision, RuntimeActivationContext{})
}

// ValidateWithActivation validates a candidate against a sealed decision, using
// owner-sealed activation capability metadata for side-effect gating. Unknown MCP
// tools default to mutating and cannot enter produce_validation.
func (c RuntimeCandidate) ValidateWithActivation(decision RuntimeDecision, activation RuntimeActivationContext) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	activation = activation.Normalized()
	count := 0
	if c.StateChange != nil {
		count++
	}
	if c.Validation != nil {
		count++
	}
	if c.Assessment != nil {
		count++
	}
	if c.Governance != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("runtime candidate requires exactly one candidate kind")
	}
	switch decision.State {
	case ConvergenceStateNeedStateChange:
		if c.StateChange == nil {
			return fmt.Errorf("convergence state %s requires a state-change candidate", decision.State)
		}
		return validateRuntimeOperations(decision, stateChangingRuntimeOperations(c.StateChange.Operations), activation.MCPCapabilities)
	case ConvergenceStateNeedValidation:
		if c.Validation == nil {
			return fmt.Errorf("convergence state %s requires a validation candidate", decision.State)
		}
		operations := validationRuntimeOperations(c.Validation.Operations)
		if err := validateRuntimeOperations(decision, operations, activation.MCPCapabilities); err != nil {
			return err
		}
		return ValidateRuntimeValidationOperationRecipes(operations, activation.EvidencePolicies)
	case ConvergenceStateNeedAssessment:
		if c.Assessment == nil {
			return fmt.Errorf("convergence state %s requires an assessment candidate", decision.State)
		}
		assessment := c.Assessment.Normalized()
		if assessment.FocusID != decision.Focus.ID || assessment.Summary == "" {
			return fmt.Errorf("assessment candidate requires current focus and summary")
		}
		if decision.Focus.SliceID != "" && assessment.SliceID != "" && assessment.SliceID != decision.Focus.SliceID {
			return fmt.Errorf("assessment candidate slice %q does not match current focus slice %q", assessment.SliceID, decision.Focus.SliceID)
		}
		requiredCoverage := coverageIDSet(decision.RequiredOutput.CoverageWorkItemIDs)
		if len(requiredCoverage) == 0 {
			requiredCoverage = coverageIDSet(decision.Focus.CoverageWorkItemIDs)
		}
		if len(requiredCoverage) > 0 && !coverageIDsIntersect(requiredCoverage, assessment.CoverageWorkItemIDs) {
			return fmt.Errorf("assessment candidate is not bound to required coverage")
		}
		disposition, err := ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
		if err != nil {
			return err
		}
		assessment.Disposition = disposition
		// Assessment disposition, items and references are an original producer
		// observation. Scope binding is enforced by FocusID and the declared
		// coverage intersection above; completeness is established only by the
		// Orchestrator from accepted owner evidence at commit time.
		c.Assessment = &assessment
		return nil
	case ConvergenceStateNeedGovernance:
		if c.Governance == nil {
			return fmt.Errorf("convergence state %s requires a governance candidate", decision.State)
		}
		if err := c.Governance.ExternalDependency.Validate(); err != nil {
			return fmt.Errorf("governance candidate: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("convergence state %s cannot dispatch an LLM candidate", decision.State)
	}
}

func ValidateAuthorityTransaction(decision RuntimeDecision, transaction AuthorityTransaction) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	want := decision.AuthorityTransaction()
	if transaction != want {
		return fmt.Errorf("authority transaction does not match sealed runtime decision")
	}
	return nil
}

func validateRuntimeOperations(decision RuntimeDecision, operations []RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) error {
	state := decision.State
	required := decision.RequiredOutput
	if len(operations) == 0 {
		return fmt.Errorf("runtime candidate requires at least one operation")
	}
	accepted := stringSet(canonicalizeOperationKinds(required.AcceptedKinds))
	acceptedVariants := make(map[string]struct{}, len(required.AcceptedOperationVariants))
	for _, variant := range required.AcceptedOperationVariants {
		acceptedVariants[variant.Kind+"|"+variant.VariantID] = struct{}{}
	}
	acceptedVariantObserved := len(acceptedVariants) == 0
	requiredCoverage := coverageIDSet(required.CoverageWorkItemIDs)
	ids := make(map[string]struct{}, len(operations))
	for idx := range operations {
		operation := operations[idx].Normalized()
		if operation.ID == "" || operation.Kind == "" {
			return fmt.Errorf("runtime candidate operation requires id and kind")
		}
		if _, exists := ids[operation.ID]; exists {
			return fmt.Errorf("runtime candidate operation id %q is duplicated", operation.ID)
		}
		ids[operation.ID] = struct{}{}
		canonicalKind := CanonicalRuntimeBlockKind(operation.Kind)
		if _, ok := accepted[canonicalKind]; !ok {
			return fmt.Errorf("runtime candidate operation kind %q is not accepted", operation.Kind)
		}
		operation.Kind = canonicalKind
		if state == ConvergenceStateNeedValidation && OperationHasWorkspaceSideEffectsWithCapabilities(operation, capabilities) {
			return fmt.Errorf("runtime candidate operation %q has workspace side effects and cannot satisfy produce_validation; emit a non-mutating validation operation or request produce_state_change first", operation.ID)
		}
		if len(requiredCoverage) > 0 && !runtimeOperationBoundToRequiredCoverage(decision, operation, requiredCoverage) {
			return fmt.Errorf("runtime candidate operation %q is not bound to required coverage", operation.ID)
		}
		if err := validateOperationParameters(operation.Parameters); err != nil {
			return fmt.Errorf("runtime candidate operation %q: %w", operation.ID, err)
		}
		if err := ValidateRuntimeOperationContract(operation); err != nil {
			return fmt.Errorf("runtime candidate operation %q: %w", operation.ID, err)
		}
		if len(acceptedVariants) > 0 {
			contract, known := RuntimeOperationContractForKind(operation.Kind)
			if !known {
				return fmt.Errorf("runtime candidate operation %q has no variant contract", operation.ID)
			}
			variant, err := MatchRuntimeOperationVariant(operation, contract)
			if err != nil {
				return fmt.Errorf("runtime candidate operation %q: %w", operation.ID, err)
			}
			if _, ok := acceptedVariants[operation.Kind+"|"+variant.ID]; ok {
				acceptedVariantObserved = true
			}
		}
		operations[idx] = operation
	}
	for _, operation := range operations {
		for _, dependency := range operation.DependsOn {
			if _, ok := ids[dependency]; !ok {
				return fmt.Errorf("runtime candidate operation %q depends on unknown operation %q", operation.ID, dependency)
			}
		}
	}
	if err := ValidateRequiredOperationBindings(operations, required.RequiredOperationSequence); err != nil {
		return err
	}
	if !acceptedVariantObserved {
		return fmt.Errorf("runtime candidate requires one accepted operation variant")
	}
	// Forward/reverse effect gate before seal (INV-02). Failures here must not
	// consume execution/repair budget — they are candidate contract failures.
	if err := ValidateCandidateEffectCompatibility(state, operations, required, capabilities); err != nil {
		return err
	}
	return nil
}

func runtimeOperationBoundToRequiredCoverage(decision RuntimeDecision, operation RuntimeOperation, required map[common.CoverageWorkItemID]struct{}) bool {
	bindings := decision.Focus.Normalized().CoverageEvidenceBindings
	if len(bindings) == 0 {
		// This decision-only helper has no manifest identity and therefore cannot
		// authorize or credit coverage. Current Runtime reaches execution
		// through ValidateRuntimeCandidateAgainstManifest, which rejects raw CWI
		// attribution and requires sealed binding refs. Keep this generic shape
		// validation neutral so it does not become a second authorization owner.
		return true
	}
	// A repair-time validation/read operation can be executable without being a
	// closable proof for the current CWI (for example, confirming a durable
	// no-op before a provenance terminal). This decision-only helper cannot
	// authorize coverage: the manifest evaluator later requires bindingRefs for
	// every operation that actually has a matching proof. Treat an operation
	// with no proof of this decision's evidence kind as non-crediting here,
	// rather than forcing it to invent a binding.
	evidenceKind := EvidenceKindStateChange
	if decision.State == ConvergenceStateNeedValidation {
		evidenceKind = EvidenceKindValidation
	}
	if len(CoverageEvidenceBindingRefsForOperationEvidenceKind(bindings, operation.Kind, evidenceKind)) == 0 {
		return true
	}
	// A selected operation without BindingRefs is explicitly non-crediting. It
	// may still be a necessary prerequisite or auxiliary operation in the same
	// sealed dispatch. Only a supplied sealed ref asks SM-2 to evaluate CWI
	// credit from the subsequent owner receipt.
	if len(operation.Normalized().BindingRefs) == 0 {
		return true
	}
	for _, ref := range operation.Normalized().BindingRefs {
		binding, found := CoverageEvidenceBindingForRef(bindings, ref)
		if !found {
			continue
		}
		if _, ok := required[binding.CoverageWorkItemID]; ok {
			return true
		}
	}
	return false
}

// ValidateRuntimeValidationOperationRecipes is a pure policy-shape check for
// validation candidates. It never chooses a Runtime action, interprets
// Goal/DoD text, observes a process, or resolves recipe IDs. The exact recipe
// selection is validated only by RuntimeCandidateContractManifest, which is
// built from the Shell-owner-confirmed admissibility snapshot.
func ValidateRuntimeValidationOperationRecipes(operations []RuntimeOperation, policies []RuntimeEvidencePolicy) error {
	recipeDeclarations := RuntimeValidationRecipeDeclarationsFromEvidencePolicies(policies)
	for _, operation := range operations {
		operation = operation.Normalized()
		switch CanonicalRuntimeBlockKind(operation.Kind) {
		case RuntimeBlockKindManagedValidationSession:
			recipeID := runtimeValidationRecipeID(operation)
			if recipeID == "" {
				return fmt.Errorf("runtime validation operation %q requires recipeId", operation.ID)
			}
			// The manifest owns ID membership. Do not fall back to activation
			// declarations here: Shell may have excluded a declaration during
			// owner-local preflight.
			continue
		case StepKindShellCommand:
			if len(recipeDeclarations) != 0 {
				return fmt.Errorf("runtime validation operation %q must select a managed validation recipe instead of command and args", operation.ID)
			}
		default:
			continue
		}
		command, args := runtimeShellOperationCommandAndArgs(operation)
		for _, policy := range NormalizeRuntimeEvidencePolicies(policies) {
			if len(policy.RequiredArgs) == 0 || !runtimeEvidencePolicyMatchesShellCommand(policy, command) {
				continue
			}
			for _, required := range policy.RequiredArgs {
				if !containsStringValue(args, required) {
					return fmt.Errorf("runtime validation operation %q command %q requires observable recipe argument %q from evidence policy %q", operation.ID, command, required, policy.ID)
				}
			}
		}
	}
	return nil
}

func runtimeValidationRecipeID(operation RuntimeOperation) string {
	for _, parameter := range operation.Parameters {
		if strings.TrimSpace(parameter.Name) == "recipeId" && parameter.String != nil {
			return strings.TrimSpace(*parameter.String)
		}
	}
	return ""
}

func runtimeShellOperationCommandAndArgs(operation RuntimeOperation) (string, []string) {
	for _, parameter := range operation.Parameters {
		switch strings.TrimSpace(parameter.Name) {
		case "command":
			if parameter.String != nil {
				return strings.TrimSpace(*parameter.String), runtimeShellOperationArgs(operation.Parameters)
			}
		}
	}
	return "", runtimeShellOperationArgs(operation.Parameters)
}

func runtimeShellOperationArgs(parameters []OperationParameter) []string {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == "args" {
			return normalizeOrderedStrings(parameter.StringList)
		}
	}
	return nil
}

func runtimeEvidencePolicyMatchesShellCommand(policy RuntimeEvidencePolicy, command string) bool {
	policy = policy.Normalized()
	if policy.AppliesTo != StepKindShellCommand || policy.CommandName == "" {
		return false
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(command))
	expected := strings.ToLower(policy.CommandName)
	return base == expected || base == expected+".exe"
}

func ValidateRequiredOperationBindings(operations []RuntimeOperation, requirements []RuntimeRequiredOperationVariant) error {
	if len(requirements) == 0 {
		return nil
	}
	bound := map[string]RuntimeOperation{}
	for _, operation := range operations {
		ref := strings.TrimSpace(operation.RequiredOperationRef)
		if ref == "" {
			continue
		}
		if _, exists := bound[ref]; exists {
			return fmt.Errorf("required operation ref %q is bound more than once", ref)
		}
		bound[ref] = operation
	}
	for _, requirement := range requirements {
		operation, ok := bound[requirement.RequirementID]
		if !ok {
			return fmt.Errorf("runtime candidate requires operation ref %q", requirement.RequirementID)
		}
		contract, known := RuntimeOperationContractForKind(operation.Kind)
		if !known {
			return fmt.Errorf("operation ref %q has unknown kind %q", requirement.RequirementID, operation.Kind)
		}
		variant, err := MatchRuntimeOperationVariant(operation, contract)
		if err != nil || variant.ID != requirement.VariantID || CanonicalRuntimeBlockKind(operation.Kind) != requirement.Kind {
			return fmt.Errorf("operation ref %q does not match required variant %s:%s", requirement.RequirementID, requirement.Kind, requirement.VariantID)
		}
		for _, dependency := range requirement.DependsOnRequirementIDs {
			dep, ok := bound[dependency]
			if !ok || !containsString(operation.DependsOn, dep.ID) {
				return fmt.Errorf("operation ref %q must depend on %q", requirement.RequirementID, dependency)
			}
		}
	}
	return nil
}

func operationStringParameter(parameters []OperationParameter, name string) string {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name && parameter.String != nil {
			return strings.TrimSpace(*parameter.String)
		}
	}
	return ""
}

func (o RuntimeOperation) Normalized() RuntimeOperation {
	o.ID = strings.TrimSpace(o.ID)
	o.Kind = strings.TrimSpace(o.Kind)
	o.RequiredOperationRef = strings.TrimSpace(o.RequiredOperationRef)
	o.DependsOn = normalizeStrings(o.DependsOn)
	o.BindingRefs = normalizeStrings(o.BindingRefs)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	parameters := make([]OperationParameter, 0, len(o.Parameters))
	for _, parameter := range o.Parameters {
		parameter.Name = strings.TrimSpace(parameter.Name)
		parameter.StringList = normalizeOperationParameterStringList(parameter.Name, parameter.StringList)
		if parameter.Name != "" {
			parameters = append(parameters, parameter)
		}
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	o.Parameters = parameters
	return o
}

// orderedStringListParameterNames are positional lists: sorting/deduping changes
// execution semantics (e.g. go test -run becomes go -run when "test" is reordered).
func orderedStringListParameterNames() map[string]struct{} {
	return map[string]struct{}{
		"args":      {},
		"pathspecs": {},
	}
}

func normalizeOperationParameterStringList(name string, values []string) []string {
	if _, ordered := orderedStringListParameterNames()[strings.TrimSpace(name)]; ordered {
		return normalizeOrderedStrings(values)
	}
	return normalizeStrings(values)
}

func validateOperationParameters(parameters []OperationParameter) error {
	seen := map[string]struct{}{}
	for _, parameter := range parameters {
		if _, ok := seen[parameter.Name]; ok {
			return fmt.Errorf("parameter %q is duplicated", parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
		valueCount := 0
		if parameter.String != nil {
			valueCount++
		}
		if parameter.Integer != nil {
			valueCount++
		}
		if parameter.Boolean != nil {
			valueCount++
		}
		if len(parameter.StringList) > 0 {
			valueCount++
		}
		if valueCount != 1 {
			return fmt.Errorf("parameter %q requires exactly one concrete value", parameter.Name)
		}
	}
	return nil
}

func stateChangingRuntimeOperations(values []StateChangingOperation) []RuntimeOperation {
	ret := make([]RuntimeOperation, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Operation)
	}
	return ret
}

func validationRuntimeOperations(values []ValidationOperation) []RuntimeOperation {
	ret := make([]RuntimeOperation, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Operation)
	}
	return ret
}

func stringSet(values []string) map[string]struct{} {
	ret := make(map[string]struct{}, len(values))
	for _, value := range values {
		ret[value] = struct{}{}
	}
	return ret
}

func coverageIDSet(values []common.CoverageWorkItemID) map[common.CoverageWorkItemID]struct{} {
	ret := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, value := range values {
		ret[value] = struct{}{}
	}
	return ret
}

func coverageIDsIntersect(required map[common.CoverageWorkItemID]struct{}, values []common.CoverageWorkItemID) bool {
	for _, value := range values {
		if _, ok := required[value]; ok {
			return true
		}
	}
	return false
}
