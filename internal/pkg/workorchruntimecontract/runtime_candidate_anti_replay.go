package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeRejectedOperationShape is the exact, typed operation structure that
// remains rejected on the current focus. It is volatile candidate context, not
// stable-prefix configuration or a new authority source.
type RuntimeRejectedOperationShape struct {
	Hash             string
	Operation        RuntimeOperation
	EvidenceID       common.EvidenceRecordID
	FailureClass     FailureClass
	FailureCode      string
	FailureSignature string
	FailureMessage   string
}

func NormalizeRuntimeRejectedOperationShapes(values []RuntimeRejectedOperationShape) []RuntimeRejectedOperationShape {
	byHash := map[string]RuntimeRejectedOperationShape{}
	for _, value := range values {
		value.Operation = value.Operation.Normalized()
		value.Hash = stableRuntimeOperationShape(value.Operation)
		value.EvidenceID = common.EvidenceRecordID(strings.TrimSpace(string(value.EvidenceID)))
		value.FailureClass = FailureClass(strings.TrimSpace(string(value.FailureClass)))
		value.FailureCode = strings.TrimSpace(value.FailureCode)
		value.FailureSignature = strings.TrimSpace(value.FailureSignature)
		value.FailureMessage = strings.TrimSpace(value.FailureMessage)
		if value.Hash == "" || value.Operation.Kind == "" {
			continue
		}
		byHash[value.Hash] = value
	}
	hashes := make([]string, 0, len(byHash))
	for hash := range byHash {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	ret := make([]RuntimeRejectedOperationShape, 0, len(hashes))
	for _, hash := range hashes {
		ret = append(ret, byHash[hash])
	}
	return nilIfEmpty(ret)
}

// ValidateCandidateAgainstRejectedShapes rejects candidates that only replay
// still-active rejected operation shapes on the current focus.
//
// A rejected shape is active only when no later accepted evidence exists on the
// same focus. Successful repair therefore clears the block and allows the same
// validation/state-change shape to run again.
//
// Shape hashing stays domain-neutral but includes structural parameters that
// distinguish corrected retries (args, oldText/newText/baseHash, dynamic input.*).
func ValidateCandidateAgainstRejectedShapes(facts RuntimeFacts, completedDispatches []RuntimeDispatchRecord, candidate RuntimeCandidate) error {
	facts = facts.Normalized()
	rejectedShapes := activeRejectedOperationShapeSet(facts, completedDispatches)
	if len(rejectedShapes) == 0 {
		return nil
	}
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 {
		return nil
	}
	replayCount := 0
	for _, operation := range operations {
		shape := stableRuntimeOperationShape(operation)
		if _, ok := rejectedShapes[shape]; ok {
			replayCount++
		}
	}
	if replayCount == 0 {
		return nil
	}
	if replayCount == len(operations) {
		return fmt.Errorf("runtime candidate replays active rejected operation shape(s); change kind, targets, or structural parameters before retry")
	}
	return fmt.Errorf("runtime candidate includes %d operation(s) that replay active rejected shape(s); remove or rewrite those operations", replayCount)
}

func ActiveRejectedOperationShapes(facts RuntimeFacts, completedDispatches []RuntimeDispatchRecord) []RuntimeRejectedOperationShape {
	dispatchByID := make(map[string]RuntimeDispatchRecord, len(completedDispatches))
	for _, dispatch := range completedDispatches {
		dispatchByID[strings.TrimSpace(dispatch.ID)] = dispatch
	}
	latestAcceptedRevision := latestAcceptedEvidenceRevision(facts)
	ret := make([]RuntimeRejectedOperationShape, 0, len(facts.Evidence.Rejected))
	for _, evidence := range facts.Evidence.Rejected {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		// Successful later progress supersedes earlier rejected shapes.
		if latestAcceptedRevision != "" && !evidenceRevisionAfter(evidence.Revision, latestAcceptedRevision) {
			continue
		}
		dispatch, ok := dispatchByID[strings.TrimSpace(evidence.DispatchID)]
		if !ok {
			continue
		}
		for _, operation := range candidateRuntimeOperations(dispatch.Candidate) {
			if operation.Normalized().ID != strings.TrimSpace(evidence.OperationID) {
				continue
			}
			ret = append(ret, RuntimeRejectedOperationShape{
				Hash: stableRuntimeOperationShape(operation), Operation: operation.Normalized(), EvidenceID: evidence.ID,
				FailureClass: evidence.FailureClass, FailureCode: evidence.FailureCode,
				FailureSignature: evidence.FailureSignature, FailureMessage: evidence.FailureMessage,
			})
		}
	}
	return NormalizeRuntimeRejectedOperationShapes(ret)
}

func activeRejectedOperationShapeSet(facts RuntimeFacts, completedDispatches []RuntimeDispatchRecord) map[string]struct{} {
	ret := map[string]struct{}{}
	for _, shape := range ActiveRejectedOperationShapes(facts, completedDispatches) {
		ret[shape.Hash] = struct{}{}
	}
	return ret
}

func latestAcceptedEvidenceRevision(facts RuntimeFacts) string {
	latest := ""
	for _, evidence := range facts.Evidence.Accepted {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		if latest == "" || evidenceRevisionAfter(evidence.Revision, latest) {
			latest = evidence.Revision
		}
	}
	return latest
}

func candidateRuntimeOperations(candidate RuntimeCandidate) []RuntimeOperation {
	switch {
	case candidate.StateChange != nil:
		return stateChangingRuntimeOperations(candidate.StateChange.Operations)
	case candidate.Validation != nil:
		return validationRuntimeOperations(candidate.Validation.Operations)
	default:
		return nil
	}
}

func stableRuntimeOperationShape(operation RuntimeOperation) string {
	operation = operation.Normalized()
	h := sha256.New()
	writeHashString(h, operation.Kind)
	writeHashStrings(h, operation.DependsOn)
	writeHashStrings(h, operationShapeTargets(operation))
	// Hash only parameters that affect execution semantics. Display-only metadata
	// such as summary/boundedReason must not create a new shape.
	for _, name := range operationShapeSemanticParameterNames(operation.Kind) {
		parameter, ok := operationParameterByName(operation.Parameters, name)
		if !ok {
			continue
		}
		writeHashString(h, name)
		switch {
		case parameter.String != nil:
			writeHashString(h, *parameter.String)
		case parameter.Integer != nil:
			writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
		case parameter.Boolean != nil:
			writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
		case len(parameter.StringList) > 0:
			writeHashStrings(h, parameter.StringList)
		}
	}
	// Dynamic MCP/prompt fields (input.*/arguments.*) are execution-semantic.
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if !strings.HasPrefix(name, "input.") && !strings.HasPrefix(name, "arguments.") {
			continue
		}
		writeHashString(h, name)
		switch {
		case parameter.String != nil:
			writeHashString(h, *parameter.String)
		case parameter.Integer != nil:
			writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
		case parameter.Boolean != nil:
			writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
		case len(parameter.StringList) > 0:
			writeHashStrings(h, parameter.StringList)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// operationShapeNonSemanticParameterNames are contract parameters that do not change
// execution shape identity: display metadata, runtime-injected scratch, or target aliases
// already covered by operationShapeTargets.
func operationShapeNonSemanticParameterNames() map[string]struct{} {
	return map[string]struct{}{
		"summary":                      {},
		"boundedReason":                {},
		"validationScratchDir":         {},
		"validationScratchAutoCleanup": {},
		"targetRef":                    {},
		"targetRefs":                   {},
		"relatedFailureTargets":        {},
		"coverageTargets":              {},
		"verificationScope":            {},
		// path is folded into operationShapeTargets with targetRef.
		"path": {},
	}
}

func operationShapeSemanticParameterNames(kind string) []string {
	contract, known := RuntimeOperationContractForKind(kind)
	if !known {
		// Conservative fallback for unknown kinds: structural keys only.
		return []string{"command", "args", "operation", "tool", "serverId", "patchType", "oldText", "newText", "baseHash", "content", "diff", "uri", "query", "url", "stateChanging", "recursive"}
	}
	exclude := operationShapeNonSemanticParameterNames()
	ret := make([]string, 0, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		if _, skip := exclude[name]; skip {
			continue
		}
		ret = append(ret, name)
	}
	sort.Strings(ret)
	return ret
}

func operationParameterByName(parameters []OperationParameter, name string) (OperationParameter, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name {
			return parameter, true
		}
	}
	return OperationParameter{}, false
}

func operationShapeTargets(operation RuntimeOperation) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(operation.TargetRefs)+2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	for _, value := range operation.TargetRefs {
		add(value)
	}
	// Treat path and targetRef as the same structural target identity.
	add(operationStringParameter(operation.Parameters, "targetRef"))
	add(operationStringParameter(operation.Parameters, "path"))
	sort.Strings(ret)
	return ret
}

// ValidateCandidateAgainstMissingValidationGap rejects state-changing retries when
// the only remaining required evidence kind is validation and accepted state_change
// evidence already exists. This is domain-neutral evidence routing.
func ValidateCandidateAgainstMissingValidationGap(facts RuntimeFacts, decision RuntimeDecision, candidate RuntimeCandidate) error {
	facts = facts.Normalized()
	if decision.Action != RuntimeActionProduceStateChange {
		return nil
	}
	// INV-05: anti-replay must not switch action when a repair directive requires state-change.
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID &&
		facts.RepairDirective.RequiredAction == RuntimeActionProduceStateChange {
		return nil
	}
	missing := MissingRequiredEvidenceKinds(facts)
	if !(len(missing) == 1 && missing[0] == EvidenceKindValidation) {
		return nil
	}
	hasAcceptedStateChange := false
	for _, evidence := range facts.Evidence.Accepted {
		if evidenceAppliesToFocus(facts.Focus, evidence) && evidence.Kind == EvidenceKindStateChange {
			hasAcceptedStateChange = true
			break
		}
	}
	if !hasAcceptedStateChange {
		return nil
	}
	// Only block when latest focus-local failure is itself a state_change rejection.
	// Validation failures should not permanently ban state_change if a later repair
	// decision intentionally requests it; the missing-only-validation case with a
	// recent state_change failure is the empty-retry pattern we stop.
	if !latestFocusFailureIsStateChange(facts) {
		return nil
	}
	if len(candidateRuntimeOperations(candidate)) == 0 {
		return nil
	}
	return fmt.Errorf("runtime candidate continues state-changing retries while only validation evidence is missing; produce_validation instead")
}

func latestFocusFailureIsStateChange(facts RuntimeFacts) bool {
	latestRevision := ""
	isStateChange := false
	for _, evidence := range facts.Evidence.Rejected {
		if !evidenceAppliesToFocus(facts.Focus, evidence) {
			continue
		}
		if evidence.Kind != EvidenceKindStateChange {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			latestRevision = evidence.Revision
			isStateChange = true
		}
	}
	// If a later rejected validation exists, state-change is not the latest failure class.
	for _, evidence := range facts.Evidence.Rejected {
		if !evidenceAppliesToFocus(facts.Focus, evidence) || evidence.Kind != EvidenceKindValidation {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			return false
		}
	}
	return isStateChange
}
