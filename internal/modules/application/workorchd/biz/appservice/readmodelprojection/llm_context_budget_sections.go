package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmContextBudgetStrings(budget workorcheventpayload.Payload, key string) []string {
	value, ok := budget.StringSlice(key)
	if !ok {
		if processing, exists := budget.Object("processing"); exists {
			value, _ = processing.StringSlice(key)
		}
	}
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

func llmContextBudgetSectionHashes(budget workorcheventpayload.Payload) []llmaudit.ContextSection {
	value, ok := budget.Field("sectionHashes")
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]llmaudit.ContextSection, 0, len(value.Array))
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
		body, _ := section.Int64("bodyBytes")
		expanded, _ := section.Int64("expandedBytes")
		localRefs, _ := section.Int64("localReferences")
		applicability, _ := section.String("applicability")
		paths, sizes := llmContextComponentArrays(section)
		var components []llmaudit.ContextComponent
		for i, path := range paths {
			components = append(components, llmaudit.ContextComponent{Path: path, Bytes: int(sizes[i].Int)})
		}
		ret = append(ret, llmaudit.ContextSection{ID: id, Hash: hash, SizeBytes: int(size), SourceRefs: refs, BodyBytes: int(body), ExpandedBytes: int(expanded), LocalReferences: int(localRefs), Applicability: applicability, Components: components})
	}
	return ret
}

// Parallel scalar arrays keep field accounting within the existing audit depth.
// Partial or malformed pairs never become a claimed complete component index.
func llmContextComponentArrays(section workorcheventpayload.Payload) ([]string, []workorcheventpayload.Value) {
	paths, ok := section.StringSlice("componentPaths")
	sizes, found := section.Field("componentBytes")
	if !ok || !found || sizes.Kind != workorcheventpayload.ValueArray || len(paths) != len(sizes.Array) {
		return nil, nil
	}
	for i, path := range paths {
		if path == "" || sizes.Array[i].Kind != workorcheventpayload.ValueInt || sizes.Array[i].Int < 0 {
			return nil, nil
		}
	}
	return paths, sizes.Array
}
