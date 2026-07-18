package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	// RequiredAction is the sole next-action authority for this candidate attempt.
	// When RepairDirective is set it mirrors directive.RequiredAction (INV-05).
	RequiredAction           RuntimeAction
	RequiredEffectScopes     []RuntimeEffectScope
	RequiredEvidenceKinds    []string
	RepairDirective          *RuntimeRepairDirective
	AdmissibleOperationKinds []string
	RejectedOperationShapes  []RuntimeRejectedOperationShape
	Capabilities             RuntimeCapabilities
	Resources                RuntimeResourceFacts
	Activation               RuntimeActivationContext
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
		ctx.RequiredAction = directive.RequiredAction
		ctx.RequiredEffectScopes = append([]RuntimeEffectScope(nil), directive.RequiredEffectScopes...)
		ctx.RequiredEvidenceKinds = append([]string(nil), directive.RequiredEvidenceKinds...)
		if len(directive.AllowedOperationKinds) > 0 {
			ctx.AdmissibleOperationKinds = append([]string(nil), directive.AllowedOperationKinds...)
		}
		if len(directive.RejectedOperationShapes) > 0 {
			ctx.RejectedOperationShapes = append([]RuntimeRejectedOperationShape(nil), directive.RejectedOperationShapes...)
		}
	}
	return ctx.Normalized()
}

func RuntimeCandidateContextForDecision(facts RuntimeFacts, decision RuntimeDecision, completedDispatches []RuntimeDispatchRecord) RuntimeCandidateContext {
	context := RuntimeCandidateContextFromFacts(facts)
	context.AdmissibleOperationKinds = FeasibleOperationKindsWithActivation(decision.RequiredOutput, facts.Capabilities, facts.Activation.MCPCapabilities)
	context.RejectedOperationShapes = ActiveRejectedOperationShapes(facts, completedDispatches)
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
	c.RequiredAction = RuntimeAction(strings.TrimSpace(string(c.RequiredAction)))
	c.RequiredEffectScopes = NormalizeRuntimeEffectScopes(c.RequiredEffectScopes)
	c.RequiredEvidenceKinds = normalizeStrings(c.RequiredEvidenceKinds)
	if c.RepairDirective != nil {
		directive := c.RepairDirective.Normalized()
		c.RepairDirective = &directive
		if c.RequiredAction == "" {
			c.RequiredAction = directive.RequiredAction
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
	RuntimeCandidateFailureClassInternal               RuntimeCandidateFailureClass = "internal"
)

type RuntimeCandidateFailure struct {
	Class  RuntimeCandidateFailureClass
	Reason string
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
		coverage := make([]string, 0, len(operation.CoverageWorkItemIDs))
		for _, id := range operation.CoverageWorkItemIDs {
			coverage = append(coverage, string(id))
		}
		writeHashStrings(h, coverage)
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
	CoverageWorkItemIDs  []common.CoverageWorkItemID
	TargetRefs           []string
	Parameters           []OperationParameter
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
	Kind       string
	SubjectRef string
	Reason     string
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
	switch decision.Action {
	case RuntimeActionProduceStateChange:
		if c.StateChange == nil {
			return fmt.Errorf("action %s requires a state-change candidate", decision.Action)
		}
		return validateRuntimeOperations(stateChangingRuntimeOperations(c.StateChange.Operations), decision.RequiredOutput, activation.MCPCapabilities)
	case RuntimeActionProduceValidation:
		if c.Validation == nil {
			return fmt.Errorf("action %s requires a validation candidate", decision.Action)
		}
		return validateRuntimeOperations(validationRuntimeOperations(c.Validation.Operations), decision.RequiredOutput, activation.MCPCapabilities)
	case RuntimeActionProduceAssessment:
		if c.Assessment == nil {
			return fmt.Errorf("action %s requires an assessment candidate", decision.Action)
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
		for _, item := range assessment.Items {
			if len(requiredCoverage) > 0 {
				if _, ok := requiredCoverage[item.CoverageWorkItemID]; !ok {
					return fmt.Errorf("assessment item %q is outside current slice coverage", item.CoverageWorkItemID)
				}
			}
		}
		disposition, err := ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
		if err != nil {
			return err
		}
		assessment.Disposition = disposition
		if assessment.Disposition == AssessmentDispositionSatisfied {
			if len(assessment.Items) > 0 {
				for id := range requiredCoverage {
					found := false
					for _, item := range assessment.Items {
						if item.CoverageWorkItemID == id && item.Satisfied {
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("satisfied assessment must cover all current slice items")
					}
				}
			}
			if len(assessment.EvidenceRefs) == 0 {
				return fmt.Errorf("satisfied assessment candidate requires current-run evidence refs")
			}
		}
		c.Assessment = &assessment
		return nil
	case RuntimeActionRequestGovernance:
		if c.Governance == nil {
			return fmt.Errorf("action %s requires a governance candidate", decision.Action)
		}
		dependency := c.Governance.ExternalDependency
		if strings.TrimSpace(dependency.Kind) == "" || strings.TrimSpace(dependency.Reason) == "" {
			return fmt.Errorf("governance candidate requires a concrete external dependency")
		}
		return nil
	default:
		return fmt.Errorf("action %s cannot dispatch an LLM candidate", decision.Action)
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

func validateRuntimeOperations(operations []RuntimeOperation, required RequiredOutputContract, capabilities []RuntimeMCPCapabilityContext) error {
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
		if required.Kind == RequiredOutputKindValidation && OperationHasWorkspaceSideEffectsWithCapabilities(operation, capabilities) {
			return fmt.Errorf("runtime candidate operation %q has workspace side effects and cannot satisfy produce_validation; emit a non-mutating validation operation or request produce_state_change first", operation.ID)
		}
		if len(requiredCoverage) > 0 && !coverageIDsIntersect(requiredCoverage, operation.CoverageWorkItemIDs) {
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
	if err := ValidateCandidateEffectCompatibility(operations, required, capabilities); err != nil {
		return err
	}
	return nil
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
	o.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(o.CoverageWorkItemIDs)
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
