package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func webFetchTrace(step workorchprojectioncontract.StepView) workorchprojectioncontract.WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	fetched := workorchprojectioncontract.FetchedWebSource{
		URL:         readModelStringValue(output["url"]),
		FinalURL:    readModelStringValue(output["finalUrl"]),
		Title:       readModelStringValue(output["title"]),
		Source:      readModelStringValue(output["source"]),
		RetrievedAt: readModelStringValue(output["retrievedAt"]),
		ContentHash: readModelStringValue(output["contentHash"]),
		Truncated:   readModelBoolValue(output["truncated"]),
		EvidenceRef: evidenceRef,
	}
	fetchedSources := []workorchprojectioncontract.FetchedWebSource(nil)
	if fetched.URL != "" {
		fetchedSources = []workorchprojectioncontract.FetchedWebSource{fetched}
	}
	return workorchprojectioncontract.WebSourceTrace{
		ProviderID:     readModelStringValue(output["provider"]),
		FetchedSources: fetchedSources,
		EvidenceRefs:   []string{evidenceRef},
		ApprovalID:     readModelStringValue(output["approvalId"]),
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}
