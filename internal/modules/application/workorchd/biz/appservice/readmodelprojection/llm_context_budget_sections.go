package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func llmContextBudgetStrings(budget workorcheventpayload.Payload, key string) []string {
	value, _ := budget.StringSlice(key)
	return value
}

func llmContextBudgetRules(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "rules")
}
func llmContextBudgetSourceRefs(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "sourceRefs")
}
func llmContextBudgetIncludedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "includedSections")
}
func llmContextBudgetNormalizedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "normalizedSections")
}
func llmContextBudgetCompressedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "compressedSections")
}
func llmContextBudgetOmittedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "omittedSections")
}
func llmContextBudgetRedactedPaths(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "redactedPaths")
}
func llmContextBudgetProtectedFields(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "protectedFields")
}
func llmContextBudgetIntegrityWarnings(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "integrityWarnings")
}

func llmContextBudgetSectionHashes(budget workorcheventpayload.Payload) []projectioncontract.LLMContextBudgetSection {
	value, ok := budget.Field("sectionHashes")
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]projectioncontract.LLMContextBudgetSection, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		section := workorcheventpayload.Payload{Fields: item.Object}
		id, _ := section.String("id")
		if id == "" {
			continue
		}
		hash, _ := section.String("hash")
		size, _ := section.Int64("sizeBytes")
		refs, _ := section.StringSlice("sourceRefs")
		ret = append(ret, projectioncontract.LLMContextBudgetSection{ID: id, Hash: hash, SizeBytes: int(size), SourceRefs: refs})
	}
	return ret
}
