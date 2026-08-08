package workorchruntimecontract

import (
	"fmt"
	"strings"
	"unicode"
)

// ProviderInferenceSelection is the immutable provider/model and reasoning
// selection sealed for one Runtime activation. Empty ReasoningEffort means the
// provider default; "none" remains an explicit, distinct selection.
type ProviderInferenceSelection struct {
	Target          ProviderModelTarget `json:"target"`
	ReasoningEffort string              `json:"reasoningEffort,omitempty"`
}

func (s ProviderInferenceSelection) Normalized() ProviderInferenceSelection {
	s.Target = s.Target.Normalized()
	s.ReasoningEffort = strings.TrimSpace(s.ReasoningEffort)
	return s
}

func (s ProviderInferenceSelection) Validate() error {
	s = s.Normalized()
	if err := s.Target.Validate(); err != nil {
		return fmt.Errorf("provider inference selection target: %w", err)
	}
	if len(s.ReasoningEffort) > 64 {
		return fmt.Errorf("provider inference selection reasoning effort exceeds 64 bytes")
	}
	for _, value := range s.ReasoningEffort {
		if unicode.IsSpace(value) || unicode.IsControl(value) {
			return fmt.Errorf("provider inference selection reasoning effort contains whitespace or control characters")
		}
	}
	return nil
}

func (s ProviderInferenceSelection) Same(other ProviderInferenceSelection) bool {
	left := s.Normalized()
	right := other.Normalized()
	return left.Target.Same(right.Target) && left.ReasoningEffort == right.ReasoningEffort
}

func (s ProviderInferenceSelection) Clone() ProviderInferenceSelection {
	return s.Normalized()
}

func CloneProviderInferenceSelection(selection *ProviderInferenceSelection) *ProviderInferenceSelection {
	if selection == nil {
		return nil
	}
	cloned := selection.Clone()
	return &cloned
}

func SameProviderInferenceSelection(left ProviderInferenceSelection, right ProviderInferenceSelection) bool {
	return left.Same(right)
}
