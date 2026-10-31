package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
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
			return fmt.Errorf("review without declared diagnostic sources cannot add source findings")
		}
		return nil
	}
	if len(r.DiagnosticSources) != len(s.DiagnosticSources) {
		return fmt.Errorf("review must independently cover every declared diagnostic source")
	}
	seen := map[string]bool{}
	for _, review := range r.DiagnosticSources {
		if seen[review.SourceRef] || review.Reason == "" || strings.TrimSpace(review.Reason) != review.Reason || !utf8.ValidString(review.Reason) || len(review.Reason) > RuntimeValidationInputReviewMaxReasonBytes {
			return fmt.Errorf("invalid or duplicate diagnostic source review")
		}
		var source *RuntimeValidationDiagnosticSource
		for i := range s.DiagnosticSources {
			if s.DiagnosticSources[i].SourceRef == review.SourceRef {
				source = &s.DiagnosticSources[i]
			}
		}
		if source == nil || !slices.Contains(review.InputTargetRefs, source.InputTargetRef) || !reflect.DeepEqual(review.InputTargetRefs, normalizeStrings(review.InputTargetRefs)) {
			return fmt.Errorf("diagnostic source review omits its exact input")
		}
		for _, ref := range review.InputTargetRefs {
			found := false
			for _, file := range s.Files {
				if file.TargetRef == ref {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("diagnostic review references unobserved input")
			}
		}
		switch review.Conclusion {
		case RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive:
		default:
			return fmt.Errorf("unsupported diagnostic review conclusion")
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
func ValidateRuntimeValidationDiagnosticFailureObservation(observation *RuntimeValidationObservation, binding RuntimeValidationProofRequirementBinding, scope []string, failure *RuntimeValidationFailureObservation) error {
	if observation != nil {
		if err := ValidateRuntimeValidationDiagnosticExecution(*observation, binding, scope); err != nil {
			return err
		}
	}
	if failure == nil || binding.InputReview == nil || binding.InputReview.DiagnosticSourceContractRef != RuntimeValidationDiagnosticSourceReviewV1 {
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
