package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// An opt-in input-review extension, not execution authority. Native identity is
// opaque to Core; only the declaring capability owner may resolve it.
const RuntimeValidationDiagnosticSourceReviewV1 = "validation/diagnostic-source-review/v1"
const RuntimeValidationDiagnosticSourceLimit = 64
const RuntimeValidationDiagnosticSourceMaxFiles = 64
const RuntimeValidationDiagnosticSourceMaxBytes = 256 * 1024
const RuntimeValidationDiagnosticSourceMaxReportBytes = 64 * 1024

// Diagnostic-source reviews and ordinary input reviews have separate finite
// envelopes determined by their declared semantic scope.
func RuntimeValidationInputReviewBounds(s RuntimeValidationInputReviewSubject) (files, bytes, report int) {
	if s.DiagnosticSourceContractRef == RuntimeValidationDiagnosticSourceReviewV1 {
		return RuntimeValidationDiagnosticSourceMaxFiles, RuntimeValidationDiagnosticSourceMaxBytes, RuntimeValidationDiagnosticSourceMaxReportBytes
	}
	return RuntimeValidationInputReviewMaxFiles, RuntimeValidationInputReviewMaxBytes, RuntimeValidationInputReviewMaxReportBytes
}

type RuntimeValidationDiagnosticSource struct {
	SourceRef      string `json:"sourceRef"`
	NativeCaseRef  string `json:"nativeCaseRef"`
	InputTargetRef string `json:"inputTargetRef"`
}

type RuntimeValidationDiagnosticSourceReview struct {
	SourceRef       string   `json:"sourceRef"`
	Conclusion      string   `json:"conclusion"`
	Reason          string   `json:"reason"`
	InputTargetRefs []string `json:"inputTargetRefs"`
	UncertaintyKind string   `json:"uncertaintyKind,omitempty"`
}

func validateRuntimeDiagnosticSourceSubject(s RuntimeValidationInputReviewSubject) error {
	if s.DiagnosticSourceContractRef == "" {
		if s.DiagnosticSources != nil {
			return fmt.Errorf("diagnostic sources require an explicit diagnostic-source review contract")
		}
		return nil
	}
	if s.ProtocolRef != RuntimeValidationInputReviewProtocol || s.DiagnosticSourceContractRef != RuntimeValidationDiagnosticSourceReviewV1 || len(s.DiagnosticSources) == 0 || len(s.DiagnosticSources) > RuntimeValidationDiagnosticSourceLimit {
		return fmt.Errorf("unsupported or incomplete diagnostic source review contract")
	}
	previous := ""
	natives := map[string]bool{}
	for _, source := range s.DiagnosticSources {
		if !boundedValidationExecutionRef(source.SourceRef, 512) || source.SourceRef <= previous || !boundedValidationExecutionRef(source.NativeCaseRef, 1024) || natives[source.NativeCaseRef] {
			return fmt.Errorf("diagnostic sources require canonical unique complete identities")
		}
		found := false
		for _, file := range s.Files {
			if file.TargetRef == source.InputTargetRef {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("diagnostic source has no exact observed input bytes")
		}
		natives[source.NativeCaseRef], previous = true, source.SourceRef
	}
	return nil
}

func validateRuntimeDiagnosticSourceReview(r RuntimeValidationInputReview, s RuntimeValidationInputReviewSubject) error {
	if s.DiagnosticSourceContractRef == "" {
		if r.DiagnosticSources != nil {
			return runtimeInputViolation(".diagnosticSources", "undeclared_diagnostic_sources", "omit diagnosticSources without an owner declaration", "review without declared diagnostic sources cannot add source findings")
		}
		return nil
	}
	if len(r.DiagnosticSources) != len(s.DiagnosticSources) {
		return runtimeInputViolation(".diagnosticSources", "incomplete_diagnostic_source_set", "exactly one review per declared diagnostic source", "review must independently cover every declared diagnostic source")
	}
	seen := map[string]bool{}
	for index, review := range r.DiagnosticSources {
		path := fmt.Sprintf(".diagnosticSources[%d]", index)
		if seen[review.SourceRef] || review.Reason == "" || strings.TrimSpace(review.Reason) != review.Reason || !utf8.ValidString(review.Reason) || len(review.Reason) > RuntimeValidationInputReviewMaxReasonBytes {
			return runtimeInputViolation(path, "invalid_diagnostic_source_review", "unique declared sourceRef and bounded nonempty UTF-8 reason without surrounding whitespace", "invalid or duplicate diagnostic source review")
		}
		var source *RuntimeValidationDiagnosticSource
		for i := range s.DiagnosticSources {
			if s.DiagnosticSources[i].SourceRef == review.SourceRef {
				source = &s.DiagnosticSources[i]
			}
		}
		if source == nil || !slices.Contains(review.InputTargetRefs, source.InputTargetRef) || !reflect.DeepEqual(review.InputTargetRefs, normalizeStrings(review.InputTargetRefs)) {
			return runtimeInputViolation(path+".inputTargetRefs", "missing_diagnostic_input", "unique sorted observed refs including this declared source's exact inputTargetRef", "diagnostic source review omits its exact input")
		}
		for _, ref := range review.InputTargetRefs {
			found := false
			for _, file := range s.Files {
				if file.TargetRef == ref {
					found = true
				}
			}
			if !found {
				return runtimeInputViolation(path+".inputTargetRefs", "unobserved_diagnostic_input", "refs from selected owner input-review subject.files only", "diagnostic review references unobserved input")
			}
		}
		switch review.Conclusion {
		case RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive:
		default:
			return runtimeInputViolation(path+".conclusion", "unsupported_diagnostic_conclusion", "consistent | conflict | inconclusive", "unsupported diagnostic review conclusion")
		}
		if err := validateRuntimeInputReviewUncertainty(path, review.Conclusion, review.UncertaintyKind, s); err != nil {
			return err
		}
		seen[review.SourceRef] = true
	}
	return nil
}

func writeRuntimeDiagnosticSourceSubjectHash(h interface{ Write([]byte) (int, error) }, s RuntimeValidationInputReviewSubject) {
	if s.DiagnosticSourceContractRef == "" && s.DiagnosticSources == nil {
		return
	}
	writeHashString(h, s.DiagnosticSourceContractRef)
	for _, source := range s.DiagnosticSources {
		writeHashString(h, source.SourceRef)
		writeHashString(h, source.NativeCaseRef)
		writeHashString(h, source.InputTargetRef)
	}
}

func writeRuntimeDiagnosticSourceReviewHash(h interface{ Write([]byte) (int, error) }, sources []RuntimeValidationDiagnosticSourceReview) {
	if len(sources) == 0 {
		return
	}
	writeHashString(h, RuntimeValidationDiagnosticSourceReviewV1)
	for _, source := range sources {
		writeHashString(h, source.SourceRef)
		writeHashString(h, source.Conclusion)
		writeHashString(h, source.Reason)
		writeHashStrings(h, source.InputTargetRefs)
		writeRuntimeInputReviewUncertaintyHash(h, source.UncertaintyKind)
	}
}

// Reverse proof against the immutable owner binding, not a current inventory.
// Core compares opaque identities only; native parsing and actual attribution
// remain the capability owner's responsibility.
func ValidateRuntimeValidationDiagnosticExecution(observation RuntimeValidationObservation, binding RuntimeValidationProofRequirementBinding, scope []string) error {
	c := observation.ExecutionConjunction
	if c == nil || c.Complete {
		return nil
	}
	hasSources := false
	for _, stage := range c.Stages {
		hasSources = hasSources || stage.DiagnosticSourceRef != "" || stage.BehaviorTargetRefs != nil
	}
	extended := binding.InputReview != nil && binding.InputReview.DiagnosticSourceContractRef == RuntimeValidationDiagnosticSourceReviewV1
	// A noncomplete ordinary execution can retain behavior observed before a
	// later deadline/storage failure. It is historical output, not source
	// attribution or repair authority. The failure consumer below separately
	// requires attribution whenever a structured repair observation is present.
	if !hasSources {
		return nil
	}
	if !extended || binding.Validate() != nil || c.Validate() != nil || observation.RecipeID != binding.ValidationRecipeID || observation.ObservationProtocolRef != c.ProtocolRef {
		return fmt.Errorf("diagnostic execution has no exact sealed source binding")
	}
	for _, check := range binding.CheckAttestations {
		if check.ObservationProtocolRef != c.ProtocolRef || check.ExecutionStagesHash != c.RequiredStagesHash {
			return fmt.Errorf("diagnostic execution changed sealed protocol or stages")
		}
	}
	var targets []string
	for _, stage := range c.Stages {
		if stage.DiagnosticSourceRef == "" {
			continue
		}
		found := false
		for _, source := range binding.InputReview.DiagnosticSources {
			found = found || source.SourceRef == stage.DiagnosticSourceRef
		}
		if !found {
			return fmt.Errorf("diagnostic execution source is absent from sealed inventory")
		}
		for _, ref := range stage.BehaviorTargetRefs {
			if !slices.Contains(scope, ref) {
				return fmt.Errorf("diagnostic behavior target is outside original owner scope")
			}
			targets = append(targets, ref)
		}
	}
	if !reflect.DeepEqual(normalizeStrings(targets), c.BehaviorTargetRefs) {
		return fmt.Errorf("diagnostic behavior is not the exact independently attributed source union")
	}
	return nil
}

// The immutable binding, not optional receipt fields, selects the source
// contract. A repair-authorizing failure may not erase attribution or borrow
// targets outside the exact source-stage union. Unobserved failures remain
// persistable without granting repair authority; bindings without source declarations cannot gain them.
func ValidateRuntimeValidationDiagnosticFailureObservation(observation *RuntimeValidationObservation, binding RuntimeValidationProofRequirementBinding, scope []string, failure *RuntimeValidationFailureObservation, diagnostic *ValidationFailureDiagnostic) error {
	if observation != nil {
		if err := ValidateRuntimeValidationDiagnosticExecution(*observation, binding, scope); err != nil {
			return err
		}
	}
	if binding.InputReview == nil || binding.InputReview.DiagnosticSourceContractRef != RuntimeValidationDiagnosticSourceReviewV1 {
		return nil
	}
	// Build/dependency rejection is a tool fact about a prerequisite stage,
	// not a failed assertion needing independent behavior attribution. Consume
	// the owner's typed classification; never parse native diagnostics here.
	if diagnostic != nil && (diagnostic.Class == ValidationDiagnosticBuildFailed || diagnostic.Class == ValidationDiagnosticDependencyMismatch) {
		return validateRuntimeValidationPrerequisiteFailure(observation, binding, scope, failure, *diagnostic)
	}
	if failure == nil {
		return nil
	}
	if observation == nil || observation.ExecutionConjunction == nil || observation.ExecutionConjunction.Complete || len(observation.ExecutionConjunction.BehaviorTargetRefs) == 0 {
		return fmt.Errorf("diagnostic failure requires original independently attributed source execution")
	}
	hasSources := false
	for _, stage := range observation.ExecutionConjunction.Stages {
		hasSources = hasSources || stage.DiagnosticSourceRef != ""
		if stage.Stdout.Storage == nil || stage.Stderr.Storage == nil || stage.Stdout.Storage.Reference == "" || stage.Stderr.Storage.Reference == "" {
			return fmt.Errorf("diagnostic failure requires all original output storage, not historical attribution alone")
		}
	}
	if !hasSources {
		return fmt.Errorf("diagnostic failure requires original independently attributed source execution")
	}
	if !reflect.DeepEqual(failure.CandidateTargetRefs, observation.ExecutionConjunction.BehaviorTargetRefs) {
		return fmt.Errorf("diagnostic failure candidates differ from original independently attributed source union")
	}
	return nil
}

func validateRuntimeValidationPrerequisiteFailure(observation *RuntimeValidationObservation, binding RuntimeValidationProofRequirementBinding, scope []string, failure *RuntimeValidationFailureObservation, diagnostic ValidationFailureDiagnostic) error {
	if observation == nil || observation.ExecutionConjunction == nil || observation.ExecutionConjunction.Complete || observation.ProofClass != "" ||
		observation.RecipeID != binding.ValidationRecipeID || observation.ObservationProtocolRef != observation.ExecutionConjunction.ProtocolRef {
		return fmt.Errorf("prerequisite failure requires its original incomplete execution")
	}
	c := observation.ExecutionConjunction
	if err := c.Validate(); err != nil {
		return err
	}
	if len(c.Stages) == 0 || len(c.BehaviorTargetRefs) != 0 {
		return fmt.Errorf("prerequisite failure cannot borrow behavior attribution")
	}
	for _, check := range binding.CheckAttestations {
		if check.ObservationProtocolRef != c.ProtocolRef || check.ExecutionStagesHash != c.RequiredStagesHash {
			return fmt.Errorf("prerequisite failure changed sealed protocol or stages")
		}
	}
	var previousEnd time.Time
	for index, stage := range c.Stages {
		start, startErr := time.Parse(time.RFC3339Nano, stage.StartedAt)
		end, endErr := time.Parse(time.RFC3339Nano, stage.FinishedAt)
		if startErr != nil || endErr != nil || end.Before(start) || index > 0 && start.Before(previousEnd) {
			return fmt.Errorf("prerequisite failure has invalid stage execution order")
		}
		previousEnd = end
		if !stage.Started || stage.TimedOut || stage.CleanupFailed || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter ||
			index > 0 && c.Stages[index-1].SourceAfter != stage.SourceBefore || stage.DiagnosticSourceRef != "" || len(stage.BehaviorTargetRefs) != 0 {
			return fmt.Errorf("prerequisite failure lacks unchanged original stage source")
		}
		for _, output := range []RuntimeValidationOutputIdentity{stage.Stdout, stage.Stderr} {
			if !output.Complete || output.Storage == nil || output.Storage.Reference == "" || output.Storage.OmittedReason != "" {
				return fmt.Errorf("prerequisite failure requires complete original output storage")
			}
		}
		if index < len(c.Stages)-1 && (stage.ExitCode != 0 || stage.KnownFailure || !stage.TerminalComplete) {
			return fmt.Errorf("prerequisite failure followed an unsettled stage")
		}
	}
	stage := c.Stages[len(c.Stages)-1]
	if stage.Identity.RequireBehavior || !stage.KnownFailure || stage.ExitCode == 0 || stage.FailureCode == "" ||
		diagnostic.Stage != stage.Identity.ID || int64(diagnostic.ExitCode) != stage.ExitCode || diagnostic.TimedOut || diagnostic.CommandPolicyViolation {
		return fmt.Errorf("prerequisite diagnostic does not match original failed stage")
	}
	if failure != nil && (failure.DiagnosticClass != diagnostic.Class || !sameStrings(failure.CandidateTargetRefs, diagnostic.TargetRefs) ||
		failure.DiagnosticHash != StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, scope)) {
		return fmt.Errorf("prerequisite failure changed typed diagnostic candidates or scope")
	}
	return nil
}
