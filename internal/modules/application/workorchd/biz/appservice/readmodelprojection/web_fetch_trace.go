package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func webFetchTrace(step StepView) readmodelcore.WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	fetched := readmodelcore.FetchedWebSource{
		URL:         readModelStringValue(output["url"]),
		FinalURL:    readModelStringValue(output["finalUrl"]),
		Title:       readModelStringValue(output["title"]),
		Source:      readModelStringValue(output["source"]),
		RetrievedAt: readModelStringValue(output["retrievedAt"]),
		ContentHash: readModelStringValue(output["contentHash"]),
		Truncated:   readModelBoolValue(output["truncated"]),
		EvidenceRef: evidenceRef,
	}
	fetchedSources := []readmodelcore.FetchedWebSource(nil)
	if fetched.URL != "" {
		fetchedSources = []readmodelcore.FetchedWebSource{fetched}
	}
	return readmodelcore.WebSourceTrace{
		ProviderID:     readModelStringValue(output["provider"]),
		FetchedSources: fetchedSources,
		EvidenceRefs:   []string{evidenceRef},
		ApprovalID:     readModelStringValue(output["approvalId"]),
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}
