package workorchruntimecontract

import (
	"fmt"
	"slices"
	"unicode/utf8"
)

func validateRuntimeExecutionSourceInputs(files []RuntimeValidationInputReviewFile) error {
	if len(files) > RuntimeValidationDiagnosticSourceMaxFiles {
		return fmt.Errorf("failed source input file capacity exceeded")
	}
	previous, size := "", 0
	for _, file := range files {
		size += len(file.Content)
		if !boundedValidationExecutionRef(file.TargetRef, 1024) || file.TargetRef <= previous || !utf8.ValidString(file.Content) || file.ContentHash != RuntimeValidationInputContentHash(file.Content) || size > RuntimeValidationDiagnosticSourceMaxBytes {
			return fmt.Errorf("failed source input identity, content or capacity invalid")
		}
		previous = file.TargetRef
	}
	return nil
}

func (c RuntimeValidationExecutionConjunction) validateSourceInventory() error {
	if len(c.NativeSources) > RuntimeValidationDiagnosticSourceLimit || c.SourceInputFailure != "" && (!boundedValidationExecutionRef(c.SourceInputFailure, 512) || c.Complete || len(c.FailedSourceInputs) != 0) {
		return fmt.Errorf("execution source inventory capacity or incompleteness invalid")
	}
	previous := ""
	native := map[string]bool{}
	for _, source := range c.NativeSources {
		if !boundedValidationExecutionRef(source.SourceRef, 512) || source.SourceRef <= previous || !boundedValidationExecutionRef(source.NativeCaseRef, 1024) || native[source.NativeCaseRef] || !boundedValidationExecutionRef(source.InputTargetRef, 1024) || !validationExecutionDigest(source.InputContentHash) {
			return fmt.Errorf("execution source inventory requires exact unique source and input identities")
		}
		native[source.NativeCaseRef], previous = true, source.SourceRef
	}
	if err := validateRuntimeExecutionSourceInputs(c.FailedSourceInputs); err != nil {
		return err
	}
	for _, file := range c.FailedSourceInputs {
		found := false
		for _, source := range c.NativeSources {
			if source.InputTargetRef != file.TargetRef {
				continue
			}
			if source.InputContentHash != file.ContentHash {
				return fmt.Errorf("failed source body changed the execution input hash")
			}
			for _, stage := range c.Stages {
				found = found || slices.Contains(stage.FailedSourceRefs, source.SourceRef)
			}
		}
		if !found {
			return fmt.Errorf("failed source body is unrelated to the original failure")
		}
	}
	return nil
}

// Receiving owners check provenance against the sealed operation scope and
// reviewed bytes. This does not repeat the input review's semantic judgment.
func validateRuntimeExecutionSourceBinding(c *RuntimeValidationExecutionConjunction, b RuntimeValidationProofRequirementBinding, scope []string) error {
	for _, source := range c.NativeSources {
		if !slices.Contains(scope, source.InputTargetRef) {
			return fmt.Errorf("execution source is outside the sealed operation scope")
		}
		if b.InputReview != nil {
			for _, file := range b.InputReview.Files {
				if source.InputTargetRef == file.TargetRef && source.InputContentHash != file.ContentHash {
					return fmt.Errorf("execution source changed the reviewed input bytes")
				}
			}
		}
	}
	return nil
}

// Exact provenance may come from a reviewed current input or the capability
// owner's actual execution inventory. Neither implies the other's judgment.
func runtimeValidationSourceMatches(c *RuntimeValidationExecutionConjunction, b RuntimeValidationProofRequirementBinding, ref, native string) bool {
	var sources []RuntimeValidationDiagnosticSource
	if c != nil {
		sources = append(sources, c.NativeSources...)
	}
	if b.InputReview != nil {
		sources = append(sources, b.InputReview.DiagnosticSources...)
	}
	found := false
	var identity RuntimeValidationDiagnosticSource
	for _, source := range sources {
		if source.SourceRef != ref {
			continue
		}
		if native != "" && source.NativeCaseRef != native || found && (identity.NativeCaseRef != source.NativeCaseRef || identity.InputTargetRef != source.InputTargetRef) {
			return false
		}
		identity, found = source, true
	}
	return found
}

func validateRuntimeFailedSourceBodies(c *RuntimeValidationExecutionConjunction, b RuntimeValidationProofRequirementBinding) error {
	if c == nil || c.SourceInputFailure != "" {
		return nil // The feedback producer must explicitly preserve incompleteness.
	}
	for _, stage := range c.Stages {
		for _, ref := range stage.FailedSourceRefs {
			for _, source := range c.NativeSources {
				if source.SourceRef != ref {
					continue
				}
				files := slices.Clone(c.FailedSourceInputs)
				if b.InputReview != nil {
					files = append(files, b.InputReview.Files...)
				}
				found := false
				for _, file := range files {
					found = found || file.TargetRef == source.InputTargetRef && file.ContentHash == source.InputContentHash
				}
				if !found {
					return fmt.Errorf("failed source lacks its exact applicable input body")
				}
			}
		}
	}
	return nil
}
