package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	RuntimeValidationGenerationContextVersion  = "runtime/validation-generation-context/v1"
	RuntimeValidationGenerationContextMaxFiles = 64
	RuntimeValidationGenerationContextMaxBytes = 256 * 1024
)

// RuntimeValidationGenerationContext is an owner-selected, bounded observation
// for preparing validation input. It grants neither a mutation target nor proof
// credit. ContractRef defines task-specific selection; Core never interprets
// languages, paths, assertions or project types. Unavailable/over-capacity facts
// carry no partial files pretending to be a complete selection.
type RuntimeValidationGenerationContextOutcome string

const (
	RuntimeValidationGenerationContextComplete     RuntimeValidationGenerationContextOutcome = "complete"
	RuntimeValidationGenerationContextUnavailable  RuntimeValidationGenerationContextOutcome = "unavailable"
	RuntimeValidationGenerationContextOverCapacity RuntimeValidationGenerationContextOutcome = "over_capacity"
)

type RuntimeValidationGenerationContext struct {
	Version          string
	ContractRef      string
	ObservedRevision string
	Outcome          RuntimeValidationGenerationContextOutcome
	Files            []RuntimeValidationInputReviewFile
}

func (c *RuntimeValidationGenerationContext) Clone() *RuntimeValidationGenerationContext {
	if c == nil {
		return nil
	}
	value := *c
	value.Files = slices.Clone(c.Files)
	return &value
}

func (c RuntimeValidationGenerationContext) Validate(revision string) error {
	if c.Version != RuntimeValidationGenerationContextVersion || strings.TrimSpace(c.ContractRef) != c.ContractRef || c.ContractRef == "" || len(c.ContractRef) > 512 || strings.IndexFunc(c.ContractRef, func(r rune) bool { return r < 32 || r == 127 }) >= 0 || !utf8.ValidString(c.ContractRef) || c.ObservedRevision == "" || c.ObservedRevision != revision {
		return fmt.Errorf("validation generation context requires exact contract and owner revision")
	}
	switch c.Outcome {
	case RuntimeValidationGenerationContextUnavailable, RuntimeValidationGenerationContextOverCapacity:
		if len(c.Files) != 0 {
			return fmt.Errorf("incomplete generation context must not publish partial files")
		}
		return nil
	case RuntimeValidationGenerationContextComplete:
	default:
		return fmt.Errorf("unknown validation generation context outcome")
	}
	if len(c.Files) > RuntimeValidationGenerationContextMaxFiles {
		return fmt.Errorf("validation generation context file capacity exceeded")
	}
	bytes, previous := 0, ""
	for _, file := range c.Files {
		if !runtimeProspectiveTargetRefValid(file.TargetRef) || file.TargetRef <= previous || !utf8.ValidString(file.Content) || file.ContentHash != RuntimeValidationInputContentHash(file.Content) {
			return fmt.Errorf("validation generation context requires unique ordered exact source bytes")
		}
		previous = file.TargetRef
		bytes += len(file.Content)
		if bytes > RuntimeValidationGenerationContextMaxBytes {
			return fmt.Errorf("validation generation context byte capacity exceeded")
		}
	}
	return nil
}
