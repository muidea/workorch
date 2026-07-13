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
	Evidence           EvidenceLedger
	Assessments        []RuntimeAssessmentFact
	Failure            *FailureFact
	Capabilities       RuntimeCapabilities
	Resources          RuntimeResourceFacts
	Activation         RuntimeActivationContext
}

func RuntimeCandidateContextFromFacts(facts RuntimeFacts) RuntimeCandidateContext {
	facts = facts.Normalized()
	return RuntimeCandidateContext{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		DefinitionRevision: facts.DefinitionRevision, EvidenceRevision: facts.EvidenceRevision,
		Focus: facts.Focus, Evidence: facts.Evidence, Assessments: facts.Assessments, Failure: facts.Failure,
		Capabilities: facts.Capabilities, Resources: facts.Resources, Activation: facts.Activation,
	}.Normalized()
}

func (c RuntimeCandidateContext) Normalized() RuntimeCandidateContext {
	facts := RuntimeFacts{
		WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.DefinitionRevision,
		EvidenceRevision: c.EvidenceRevision, Focus: c.Focus, Evidence: c.Evidence, Assessments: c.Assessments,
		Failure: c.Failure, Capabilities: c.Capabilities, Resources: c.Resources,
		Activation: c.Activation,
	}.Normalized()
	c.DefinitionRevision = facts.DefinitionRevision
	c.EvidenceRevision = facts.EvidenceRevision
	c.Focus = facts.Focus
	c.Evidence = facts.Evidence
	c.Assessments = facts.Assessments
	c.Failure = facts.Failure
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
	RuntimeCandidateFailureClassSchema   RuntimeCandidateFailureClass = "schema"
	RuntimeCandidateFailureClassContract RuntimeCandidateFailureClass = "contract"
	RuntimeCandidateFailureClassProvider RuntimeCandidateFailureClass = "provider"
	RuntimeCandidateFailureClassInternal RuntimeCandidateFailureClass = "internal"
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
		writeHashString(h, fmt.Sprintf("%t", value.Satisfied))
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
	ID                  string
	Kind                string
	DependsOn           []string
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
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Satisfied           bool
	Summary             string
	EvidenceRefs        []string
	ArtifactRefs        []string
}

func (c AssessmentCandidate) Normalized() AssessmentCandidate {
	c.FocusID = strings.TrimSpace(c.FocusID)
	c.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.CoverageWorkItemIDs)
	c.Summary = strings.TrimSpace(c.Summary)
	c.EvidenceRefs = normalizeStrings(c.EvidenceRefs)
	c.ArtifactRefs = normalizeStrings(c.ArtifactRefs)
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
	if err := decision.Validate(); err != nil {
		return err
	}
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
		return validateRuntimeOperations(stateChangingRuntimeOperations(c.StateChange.Operations), decision.RequiredOutput)
	case RuntimeActionProduceValidation:
		if c.Validation == nil {
			return fmt.Errorf("action %s requires a validation candidate", decision.Action)
		}
		return validateRuntimeOperations(validationRuntimeOperations(c.Validation.Operations), decision.RequiredOutput)
	case RuntimeActionProduceAssessment:
		if c.Assessment == nil {
			return fmt.Errorf("action %s requires an assessment candidate", decision.Action)
		}
		assessment := c.Assessment.Normalized()
		if assessment.FocusID != decision.Focus.ID || assessment.Summary == "" {
			return fmt.Errorf("assessment candidate requires current focus and summary")
		}
		requiredCoverage := coverageIDSet(decision.RequiredOutput.CoverageWorkItemIDs)
		if len(requiredCoverage) > 0 && !coverageIDsIntersect(requiredCoverage, assessment.CoverageWorkItemIDs) {
			return fmt.Errorf("assessment candidate is not bound to required coverage")
		}
		if assessment.Satisfied && len(assessment.EvidenceRefs) == 0 {
			return fmt.Errorf("satisfied assessment candidate requires current-run evidence refs")
		}
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

func validateRuntimeOperations(operations []RuntimeOperation, required RequiredOutputContract) error {
	if len(operations) == 0 {
		return fmt.Errorf("runtime candidate requires at least one operation")
	}
	accepted := stringSet(required.AcceptedKinds)
	acceptedOperations := stringSet(required.AcceptedOperations)
	acceptedOperationObserved := len(acceptedOperations) == 0
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
		if _, ok := accepted[operation.Kind]; !ok {
			return fmt.Errorf("runtime candidate operation kind %q is not accepted", operation.Kind)
		}
		if len(requiredCoverage) > 0 && !coverageIDsIntersect(requiredCoverage, operation.CoverageWorkItemIDs) {
			return fmt.Errorf("runtime candidate operation %q is not bound to required coverage", operation.ID)
		}
		if err := validateOperationParameters(operation.Parameters); err != nil {
			return fmt.Errorf("runtime candidate operation %q: %w", operation.ID, err)
		}
		if operationName := operationStringParameter(operation.Parameters, "operation"); operationName != "" {
			if _, ok := acceptedOperations[operationName]; ok {
				acceptedOperationObserved = true
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
	if !acceptedOperationObserved {
		return fmt.Errorf("runtime candidate requires one accepted operation: %s", strings.Join(required.AcceptedOperations, ","))
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
	o.DependsOn = normalizeStrings(o.DependsOn)
	o.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(o.CoverageWorkItemIDs)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	parameters := make([]OperationParameter, 0, len(o.Parameters))
	for _, parameter := range o.Parameters {
		parameter.Name = strings.TrimSpace(parameter.Name)
		parameter.StringList = normalizeStrings(parameter.StringList)
		if parameter.Name != "" {
			parameters = append(parameters, parameter)
		}
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	o.Parameters = parameters
	return o
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
