package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

// WD5 mechanical consolidation: same import contract; source list follows.

// source: closure_trace_graph_fields.go

func applyClosureTraceGraphFields(trace *workorchprojectioncontract.ClosureTrace, input map[string]any, output map[string]any) {
	trace.GraphFollowUp = readModelBoolValue(input["graphFollowUp"])
	trace.AssessmentOnly = readModelBoolValue(input["assessmentOnly"]) || readModelBoolValue(input["providerRetryAssessmentOnly"])
	trace.GraphExpansionLimitFollowUp = readModelBoolValue(input["graphExpansionLimitFollowUp"])
	trace.GraphFollowUpExpansionLimitExceeded = readModelBoolValue(output["graphFollowUpExpansionLimitExceeded"])
	trace.GraphFollowUpExpansionCount = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionCount"]),
		readModelIntValue(input["graphFollowUpExpansionCount"]),
	)
	trace.GraphFollowUpExpansionLimit = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionLimit"]),
		readModelIntValue(input["graphFollowUpExpansionLimit"]),
	)
}

// source: git_changed_files.go

func mapGitChangedFiles(value any) []workorchprojectioncontract.GitChangedFile {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]workorchprojectioncontract.GitChangedFile, 0, len(items))
	for _, item := range items {
		path := readModelStringValue(item["path"])
		if path == "" {
			continue
		}
		ret = append(ret, workorchprojectioncontract.GitChangedFile{
			Path:      path,
			OldPath:   readModelStringValue(item["oldPath"]),
			Status:    readModelStringValue(item["status"]),
			Staged:    readModelBoolValue(item["staged"]),
			Unstaged:  readModelBoolValue(item["unstaged"]),
			Untracked: readModelBoolValue(item["untracked"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: git_operation_trace_item.go

func projectGitOperationTrace(step workorchprojectioncontract.StepView) workorchprojectioncontract.GitOperationTrace {
	commit := readModelMapValue(step.Output["commit"])
	return workorchprojectioncontract.GitOperationTrace{
		StepID:           step.ID,
		Kind:             step.Kind,
		Operation:        readModelStringValue(step.Output["operation"]),
		Status:           readModelStringValue(step.Output["status"]),
		RepoRoot:         readModelStringValue(step.Output["repoRoot"]),
		WorkingDir:       readModelStringValue(step.Output["workingDir"]),
		Branch:           readModelStringValue(step.Output["branch"]),
		HeadCommit:       readModelStringValue(step.Output["headCommit"]),
		HeadCommitBefore: readModelStringValue(step.Output["headCommitBefore"]),
		HeadCommitAfter:  readModelStringValue(step.Output["headCommitAfter"]),
		CommitHash:       readModelStringValue(commit["hash"]),
		CommitSubject:    readModelStringValue(commit["subject"]),
		ChangedFiles:     mapGitChangedFiles(step.Output["changedFiles"]),
		StagedFiles:      mapGitChangedFiles(step.Output["stagedFiles"]),
		Truncated:        readModelBoolValue(step.Output["truncated"]),
		ZeroWork:         readModelBoolValue(step.Output["zeroWork"]),
		ErrorCode:        readModelStringValue(step.Output["errorCode"]),
		Summary:          readModelStringValue(step.Output["summary"]),
		EvidenceRefs:     []string{step.ID},
	}
}

// source: result_quality_issues.go

func appendResultQualityIssue(quality *workorchprojectioncontract.ResultQualityTrace, code string, severity string, message string, suggestion string, refs ...string) {
	if quality == nil {
		return
	}
	issue := workorchprojectioncontract.ResultQualityIssue{
		Code:         code,
		Severity:     severity,
		Message:      message,
		Suggestion:   suggestion,
		EvidenceRefs: compactUniqueStrings(refs),
	}
	quality.Issues = append(quality.Issues, issue)
	if severity == "error" {
		quality.State = "failed"
	} else if quality.State != "failed" {
		quality.State = "warning"
	}
}

// source: web_fetch_trace.go

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

// source: web_search_trace.go

func webSearchTrace(step workorchprojectioncontract.StepView) workorchprojectioncontract.WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	results := readModelMapSliceValue(output["results"])
	sources := make([]workorchprojectioncontract.WebSource, 0, len(results))
	for _, result := range results {
		item := workorchprojectioncontract.WebSource{
			Title:       readModelStringValue(result["title"]),
			URL:         readModelStringValue(result["url"]),
			Snippet:     readModelStringValue(result["snippet"]),
			Source:      readModelStringValue(result["source"]),
			PublishedAt: readModelStringValue(result["publishedAt"]),
			Rank:        readModelIntValue(result["rank"]),
			EvidenceRef: evidenceRef,
		}
		if item.URL != "" {
			sources = append(sources, item)
		}
	}
	return workorchprojectioncontract.WebSourceTrace{
		Query:          readModelStringValue(output["query"]),
		ProviderID:     readModelStringValue(output["provider"]),
		SearchedAt:     readModelStringValue(output["searchedAt"]),
		Sources:        sources,
		EvidenceRefs:   []string{evidenceRef},
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}
