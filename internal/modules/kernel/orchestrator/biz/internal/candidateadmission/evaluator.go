package candidateadmission

import (
	"errors"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	CodeInvalid                    = "candidate_contract_invalid"
	CodeActionEvidenceMismatch     = "candidate_action_evidence_mismatch"
	CodeTargetOutsideGrounding     = "candidate_target_outside_grounding"
	CodeRepairClosureMismatch      = "candidate_repair_closure_mismatch"
	CodeCreationScopeMissingTarget = "candidate_creation_scope_missing_file_target"

	SignatureInvalid                = "candidate-contract-invalid"
	SignatureActionEvidenceMismatch = "candidate-action-evidence-mismatch"
	SignatureTargetOutsideGrounding = "candidate-target-outside-grounding"
	SignatureRepairClosureMismatch  = "candidate-repair-closure-mismatch"
	SignatureCreationScopeTarget    = "candidate-creation-scope-missing-file-target"
)

// ValidationGapFacts is the detached SM-2 proof projection needed by the
// candidate evaluator. Runtime owns lineage resolution; this value only says
// which proof remains and whether the latest applicable facts already show a
// state change followed by a state-change failure.
type ValidationGapFacts struct {
	MissingEvidenceKinds       []string
	HasAcceptedStateChange     bool
	LatestFailureStateChanging bool
}

// EvaluationInput is an invocation-local, immutable view of the complete SM-3
// candidate admission facts. Rejected and prior-attempt shapes have already
// been filtered to the current authority contract by the Runtime owner.
type EvaluationInput struct {
	Manifest                  runtimecontract.RuntimeCandidateContractManifest
	Decision                  runtimecontract.RuntimeDecision
	Facts                     runtimecontract.RuntimeFacts
	Candidate                 runtimecontract.RuntimeCandidate
	RejectedOperationShapes   []runtimecontract.RuntimeRejectedOperationShape
	PriorFailedProposalShapes []string
	NoProgressOperationShapes []runtimecontract.RuntimeRejectedOperationShape
	ValidationGap             ValidationGapFacts
}

// Evaluate is the single structural CandidateEvaluator. It returns the first
// complete typed rejection in a stable order, or nil when the candidate is
// structurally accepted. It does not consume an attempt, choose correction or
// terminal state, probe an owner, seal a dispatch, or mutate Runtime state.
func Evaluate(input EvaluationInput) *runtimecontract.RuntimeCandidateFailure {
	ownerFacts := runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
		runtimecontract.RuntimeCandidateContextForDecision(input.Facts, input.Decision, nil),
	)
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(input.Manifest, input.Decision, ownerFacts); err != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, contractFailure(CodeInvalid, InvalidSignature(input.Candidate), fmt.Sprintf("runtime candidate manifest contract: %v", err)))
	}
	if err := runtimecontract.ValidateRuntimeCandidateAgainstManifest(input.Manifest, input.Candidate); err != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, diagnosticFailure(CodeInvalid, InvalidSignature(input.Candidate), err))
	}
	if err := input.Candidate.ValidateWithActivation(input.Decision, input.Facts.Activation); err != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, contractFailure(CodeInvalid, InvalidSignature(input.Candidate), err.Error()))
	}
	// Aggregate-local target/binding/evidence fences are structural and must
	// precede replay so one complete document has one deterministic first error.
	if failure := RepairClosureFailure(input.Facts, input.Decision, input.Candidate); failure != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, failure)
	}
	if failure := MissingValidationFailure(input.Facts, input.Decision, input.Candidate, input.ValidationGap); failure != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, failure)
	}
	if failure := GroundedTargetFailure(input.Facts, input.Decision, input.Candidate); failure != nil {
		return completeCandidateFailure(input.Manifest, input.Candidate, failure)
	}
	if finding, found := RejectedShapeReplay(input.Candidate, input.RejectedOperationShapes); found {
		return completeCandidateFailure(input.Manifest, input.Candidate, failureFromFinding(finding))
	}
	if finding, found := CandidateSemanticReplay(input.Candidate, input.PriorFailedProposalShapes); found {
		return completeCandidateFailure(input.Manifest, input.Candidate, failureFromFinding(finding))
	}
	if finding, found := NoProgressReplay(input.Candidate, input.NoProgressOperationShapes); found {
		return completeCandidateFailure(input.Manifest, input.Candidate, failureFromFinding(finding))
	}
	return nil
}

// InvalidSignature gives malformed provider documents a stable structural
// identity without depending on error wording.
func InvalidSignature(candidate runtimecontract.RuntimeCandidate) string {
	hash := runtimecontract.StableRuntimeCandidateHash(candidate)
	if len(hash) > 20 {
		hash = hash[:20]
	}
	return SignatureInvalid + "-" + hash
}

// RepairClosureFailure checks that proof-bearing operations stay within the
// active repair closure's sealed coverage and target set.
func RepairClosureFailure(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateFailure {
	directive := facts.RepairDirective
	if directive == nil || directive.Closure == nil || directive.FocusID != facts.Focus.ID {
		return nil
	}
	closure := directive.Closure.Normalized()
	if decision.RequiredOutput.RepairClosureID != closure.ID {
		return contractFailure(
			CodeRepairClosureMismatch,
			SignatureRepairClosureMismatch,
			"sealed repair decision is not bound to the active repair closure",
		)
	}
	for _, operation := range Operations(candidate) {
		if !operationCanCloseCoverageForDecision(facts.Focus, decision, operation) {
			continue
		}
		if !operationBindingRefsIntersectCoverage(facts.Focus, operation, closure.CoverageWorkItemIDs) {
			return contractFailure(
				CodeRepairClosureMismatch,
				SignatureRepairClosureMismatch,
				"runtime repair candidate operation is outside the sealed repair coverage",
			)
		}
		if len(closure.GroundedTargetRefs) > 0 && !grounding.TargetRefsIntersect(closure.GroundedTargetRefs, operation.TargetRefs) {
			return allowedTargetsFailure(
				CodeRepairClosureMismatch,
				SignatureRepairClosureMismatch,
				"runtime repair candidate operation does not target the sealed repair closure",
				operation.TargetRefs,
				closure.GroundedTargetRefs,
				candidate,
			)
		}
	}
	return nil
}

// MissingValidationFailure rejects another state-changing proposal when the
// only unresolved proof is validation and current lineage already contains an
// accepted state change followed by a state-change failure.
func MissingValidationFailure(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, gap ValidationGapFacts) *runtimecontract.RuntimeCandidateFailure {
	facts = facts.Normalized()
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		return nil
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID &&
		facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange {
		return nil
	}
	if len(gap.MissingEvidenceKinds) != 1 || gap.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		!gap.HasAcceptedStateChange || !gap.LatestFailureStateChanging || len(Operations(candidate)) == 0 {
		return nil
	}
	return contractFailure(
		CodeActionEvidenceMismatch,
		SignatureActionEvidenceMismatch,
		"runtime candidate continues state-changing retries while only validation evidence is missing; produce_validation instead",
	)
}

// GroundedTargetFailure verifies aggregate-local target scope that cannot be
// reconstructed from provider text. File still decides actual target
// feasibility in its separate typed owner probe.
func GroundedTargetFailure(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateFailure {
	facts = facts.Normalized()
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || !facts.Focus.RequireTargetGrounding {
		return nil
	}
	if grounding.HasOwnerGroundedCreationScope(facts) {
		selection := grounding.CurrentFocusTargetSelection(facts).Normalized()
		durableFileEffect := false
		for _, operation := range Operations(candidate) {
			descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || !runtimecontract.RuntimeEffectIsDurable(runtimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
				continue
			}
			if descriptor.OwnerModuleID != runtimecontract.RuntimeModuleFile || len(operation.TargetRefs) != 1 || operation.TargetRefs[0] != selection.ProspectiveTargetRef {
				return allowedTargetsFailure(
					CodeTargetOutsideGrounding,
					SignatureTargetOutsideGrounding,
					"empty workspace creation scope permits durable effects only on the sealed prospective File target",
					operation.TargetRefs,
					[]string{selection.ProspectiveTargetRef},
					candidate,
				)
			}
			durableFileEffect = true
		}
		if durableFileEffect {
			return nil
		}
		return allowedTargetsFailure(
			CodeCreationScopeMissingTarget,
			SignatureCreationScopeTarget,
			"empty workspace creation scope requires a durable File operation targeting the sealed prospective workspace-relative path",
			nil,
			[]string{selection.ProspectiveTargetRef},
			candidate,
		)
	}
	requiredTargets := grounding.RepairTargets(facts)
	if grounding.OwnerGroundedTargetScopeMaySelectNewFileTarget(facts) {
		selection := grounding.CurrentFocusTargetSelection(facts).Normalized()
		allowedTargets := grounding.WorkspaceScopeTargets(&selection)
		durableEffect := false
		for _, operation := range Operations(candidate) {
			descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || !runtimecontract.RuntimeEffectIsDurable(runtimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
				continue
			}
			durableEffect = true
			if descriptor.OwnerModuleID == runtimecontract.RuntimeModuleFile && len(operation.TargetRefs) != 0 {
				continue
			}
			if descriptor.AllowsGroundedWorkspaceTarget && grounding.TargetRefsAllWithin(operation.TargetRefs, allowedTargets) {
				continue
			}
			return allowedTargetsFailure(
				CodeTargetOutsideGrounding,
				SignatureTargetOutsideGrounding,
				"owner-grounded workspace scope permits only a durable File operation or an explicitly target-bound operation within the sealed File inventory",
				operation.TargetRefs,
				allowedTargets,
				candidate,
			)
		}
		if durableEffect {
			return nil
		}
		return allowedTargetsFailure(
			CodeTargetOutsideGrounding,
			SignatureTargetOutsideGrounding,
			"owner-grounded workspace scope requires a durable File operation or an explicitly target-bound operation within the sealed File inventory",
			nil,
			allowedTargets,
			candidate,
		)
	}
	if len(requiredTargets) == 0 {
		return nil
	}
	for _, operation := range Operations(candidate) {
		if !runtimecontract.RuntimeEffectIsDurable(runtimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
			continue
		}
		if grounding.TargetRefsIntersect(requiredTargets, operation.TargetRefs) {
			return nil
		}
	}
	return allowedTargetsFailure(
		CodeTargetOutsideGrounding,
		SignatureTargetOutsideGrounding,
		"runtime state-changing candidate does not target the sealed focus, failure, or repair target set",
		actualTargets(candidate),
		requiredTargets,
		candidate,
	)
}

func contractFailure(code, signature, reason string) *runtimecontract.RuntimeCandidateFailure {
	return &runtimecontract.RuntimeCandidateFailure{
		Class:     runtimecontract.RuntimeCandidateFailureClassContract,
		Code:      strings.TrimSpace(code),
		Signature: strings.TrimSpace(signature),
		Reason:    strings.TrimSpace(reason),
	}
}

func diagnosticFailure(code, signature string, err error) *runtimecontract.RuntimeCandidateFailure {
	failure := contractFailure(code, signature, err.Error())
	var diagnostic *runtimecontract.RuntimeCandidateContractDiagnostic
	if errors.As(err, &diagnostic) {
		failure.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticFromContractDiagnostic(diagnostic)
	}
	return failure
}

func allowedTargetsFailure(code, signature, reason string, actual, allowed []string, candidate runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateFailure {
	failure := contractFailure(code, signature, reason)
	failure.Diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{
		Path:              "$.operations.targetRefs",
		Actual:            strings.Join(actual, ","),
		Allowed:           append([]string(nil), allowed...),
		RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
	}
	return failure
}

func completeCandidateFailure(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, failure *runtimecontract.RuntimeCandidateFailure) *runtimecontract.RuntimeCandidateFailure {
	if failure == nil {
		return nil
	}
	if failure.Diagnostic == nil {
		failure.Diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{}
	} else {
		failure.Diagnostic = failure.Diagnostic.Clone()
	}
	if failure.Diagnostic.ManifestHash == "" {
		failure.Diagnostic.ManifestHash = strings.TrimSpace(manifest.Hash)
	}
	if failure.Diagnostic.RejectedShapeHash == "" {
		failure.Diagnostic.RejectedShapeHash = runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	}
	return failure
}

func failureFromFinding(finding Finding) *runtimecontract.RuntimeCandidateFailure {
	if !finding.Valid() {
		return nil
	}
	return contractFailure(finding.Code, finding.Signature, finding.Reason)
}

func operationCanCloseCoverageForDecision(focus runtimecontract.CoverageFocus, decision runtimecontract.RuntimeDecision, operation runtimecontract.RuntimeOperation) bool {
	evidenceKind := runtimecontract.EvidenceKindStateChange
	if decision.State == runtimecontract.ConvergenceStateNeedValidation {
		evidenceKind = runtimecontract.EvidenceKindValidation
	}
	return len(runtimecontract.CoverageEvidenceBindingRefsForOperationEvidenceKind(
		focus.Normalized().CoverageEvidenceBindings,
		operation.Normalized().Kind,
		evidenceKind,
	)) != 0
}

func operationBindingRefsIntersectCoverage(focus runtimecontract.CoverageFocus, operation runtimecontract.RuntimeOperation, coverage []common.CoverageWorkItemID) bool {
	focus = focus.Normalized()
	if len(focus.CoverageEvidenceBindings) == 0 {
		return coverageIDsIntersect(coverage, operation.CoverageWorkItemIDs)
	}
	for _, ref := range operation.Normalized().BindingRefs {
		binding, found := runtimecontract.CoverageEvidenceBindingForRef(focus.CoverageEvidenceBindings, ref)
		if found && coverageWorkItemIDIncluded(coverage, binding.CoverageWorkItemID) {
			return true
		}
	}
	return false
}

func coverageIDsIntersect(left, right []common.CoverageWorkItemID) bool {
	seen := make(map[common.CoverageWorkItemID]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, found := seen[value]; found {
			return true
		}
	}
	return false
}

func coverageWorkItemIDIncluded(values []common.CoverageWorkItemID, expected common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func actualTargets(candidate runtimecontract.RuntimeCandidate) []string {
	actual := make([]string, 0)
	for _, operation := range Operations(candidate) {
		actual = append(actual, operation.TargetRefs...)
	}
	return actual
}
