package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLLMOwnerCommandContractsRemainTypedAndLocal(t *testing.T) {
	want := map[string]struct {
		class eventcontract.CommandClass
		lane  eventcontract.LanePolicy
	}{
		TopicLLMCommandGetActiveConfiguration: {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicAcquireProviderCeiling:           {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicPreflightRuntimeCandidate:        {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicRequestRuntimeCandidate:          {class: eventcontract.CommandClassExecute, lane: eventcontract.LanePolicyExecute},
	}

	contracts := make(map[string]eventcontract.Contract, len(Contracts()))
	for _, contract := range Contracts() {
		contracts[contract.Topic] = contract
	}
	messages := make(map[string]eventcontract.MessageContract, len(MessageContracts()))
	for _, contract := range MessageContracts() {
		messages[contract.Topic] = contract
	}

	for topic, expected := range want {
		contract, ok := contracts[topic]
		if !ok {
			t.Fatalf("LLM owner contract %q is not registered", topic)
		}
		if contract.Owner != "llm" || !slices.Equal(contract.ReceiverOwners, []string{"llm"}) || contract.CommandClass != expected.class || contract.LanePolicy != expected.lane {
			t.Fatalf("LLM owner contract %q = %+v", topic, contract)
		}
		message, ok := messages[topic]
		if !ok {
			t.Fatalf("LLM owner message contract %q is not registered", topic)
		}
		if message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.CommandClass != contract.CommandClass || message.LanePolicy != contract.LanePolicy || message.DefinitionPackage != definitionPackage {
			t.Fatalf("LLM owner message contract %q = %+v, event contract = %+v", topic, message, contract)
		}
	}
}

func TestRequestRuntimeCandidateResultRequiresExclusiveOwnerFact(t *testing.T) {
	valid := validRuntimeCandidateResult()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid complete owner fact rejected: %v", err)
	}
	zeroByteOutputLimit := cloneRuntimeCandidateResult(valid)
	zeroByteOutputLimit.DocumentOutcome = &RuntimeProviderDocumentOutcome{Framing: runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1",
		Code: "max_output_tokens", Signature: "provider-output-limit",
	}}
	if err := zeroByteOutputLimit.Validate(); err != nil {
		t.Fatalf("explicit zero-byte output-limit owner fact rejected: %v", err)
	}
	contradictoryOutputLimitAudit := cloneRuntimeCandidateResult(zeroByteOutputLimit)
	contradictoryOutputLimitAudit.Audit.Status = "failed"
	contradictoryOutputLimitAudit.Audit.FailureClass = string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol)
	if err := contradictoryOutputLimitAudit.Validate(); err == nil {
		t.Fatal("incomplete document fact with a parallel protocol-failure audit was accepted")
	}

	missingContent := cloneRuntimeCandidateResult(valid)
	missingContent.DocumentOutcome.Content = nil
	if err := missingContent.Validate(); err == nil {
		t.Fatal("complete outcome without content was accepted")
	}

	incompleteWithContent := cloneRuntimeCandidateResult(valid)
	incompleteWithContent.DocumentOutcome.Framing.Disposition = runtimecontract.RuntimeProviderDocumentIncomplete
	incompleteWithContent.DocumentOutcome.Framing.Code = "max_output_tokens"
	incompleteWithContent.DocumentOutcome.Framing.Signature = "provider-output-limit"
	if err := incompleteWithContent.Validate(); err == nil {
		t.Fatal("non-complete outcome with content was accepted")
	}
	incompleteWithContent.DocumentOutcome.Content = []byte{}
	if err := incompleteWithContent.Validate(); err == nil {
		t.Fatal("non-complete outcome with an empty content field was accepted")
	}

	multiple := cloneRuntimeCandidateResult(valid)
	feedback := runtimeCandidateFeedback(t, runtimecontract.RuntimeFeedbackCategoryProviderTransient, runtimecontract.RuntimeCandidateFailureClassProviderTransient)
	multiple.Feedback = &feedback
	if err := multiple.Validate(); err == nil {
		t.Fatal("multiple terminal variants were accepted")
	}
	if err := (RequestRuntimeCandidateResult{}).Validate(); err == nil {
		t.Fatal("empty terminal variant was accepted")
	}
}

func TestRequestRuntimeCandidateResultSubmissionMatrix(t *testing.T) {
	tests := []struct {
		name      string
		result    RequestRuntimeCandidateResult
		wantError bool
	}{
		{name: "document outcome submitted", result: validRuntimeCandidateResult()},
		{name: "document outcome unsubmitted", result: mutateRuntimeCandidateResult(validRuntimeCandidateResult(), func(result *RequestRuntimeCandidateResult) {
			setRuntimeCandidateResultSubmitted(result, false)
			result.Usage = nil
			result.Audit.ContextBudget.ActualTokens = 0
		}), wantError: true},
		{name: "stale unsubmitted", result: staleRuntimeCandidateResult()},
		{name: "stale submitted", result: mutateRuntimeCandidateResult(staleRuntimeCandidateResult(), func(result *RequestRuntimeCandidateResult) {
			setRuntimeCandidateResultSubmitted(result, true)
		}), wantError: true},
		{name: "transient submitted", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderTransient, runtimecontract.RuntimeCandidateFailureClassProviderTransient, true)},
		{name: "transient unsubmitted", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderTransient, runtimecontract.RuntimeCandidateFailureClassProviderTransient, false), wantError: true},
		{name: "authentication submitted", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderAuthentication, runtimecontract.RuntimeCandidateFailureClassProviderAuthentication, true)},
		{name: "authentication unsubmitted", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderAuthentication, runtimecontract.RuntimeCandidateFailureClassProviderAuthentication, false), wantError: true},
		{name: "configuration pre-dispatch", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)},
		{name: "configuration provider response", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, true)},
		{name: "protocol pre-dispatch", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderProtocol, runtimecontract.RuntimeCandidateFailureClassInternal, false)},
		{name: "protocol provider response", result: feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderProtocol, runtimecontract.RuntimeCandidateFailureClassProviderProtocol, true)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.result.Validate()
			if testCase.wantError && err == nil {
				t.Fatal("invalid submission/terminal combination was accepted")
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("valid submission/terminal combination rejected: %v", err)
			}
		})
	}
}

func TestRequestRuntimeCandidateResultRejectsAuditTampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RequestRuntimeCandidateResult)
	}{
		{name: "missing audit", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit = nil }},
		{name: "workspace echo", mutate: func(result *RequestRuntimeCandidateResult) { result.WorkspaceID = "workspace-2" }},
		{name: "run audit", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.RunID = "run-2" }},
		{name: "logical request", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.LogicalRequestID = "logical-2" }},
		{name: "physical call", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.PhysicalCallID = "physical-2" }},
		{name: "correlation request", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.RequestID = "correlation-2" }},
		{name: "non-canonical mode", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.PhysicalResponseMode = " streaming " }},
		{name: "non-canonical operation", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.Operation = " responses " }},
		{name: "non-canonical target", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.Target.ModelID = " model-1 " }},
		{name: "non-canonical framing", mutate: func(result *RequestRuntimeCandidateResult) {
			result.DocumentOutcome.Framing.ExpectedDocumentHash = " expected-1 "
		}},
		{name: "empty context audit", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.ContextBudget = llmaudit.ContextBudget{} }},
		{name: "context submission", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.ContextBudget.ProviderSubmitted = false }},
		{name: "context target", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.ContextBudget.ModelID = "model-2" }},
		{name: "context revision", mutate: func(result *RequestRuntimeCandidateResult) {
			result.Audit.ContextBudget.ConfigurationRevision = "config-2"
		}},
		{name: "negative context measurement", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.ContextBudget.MaxBytes = -1 }},
		{name: "submitted without request bytes", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.RequestBytes = 0 }},
		{name: "usage total", mutate: func(result *RequestRuntimeCandidateResult) { result.Usage.TotalTokens++ }},
		{name: "negative usage", mutate: func(result *RequestRuntimeCandidateResult) { result.Usage.InputTokens = -1 }},
		{name: "audit actual tokens", mutate: func(result *RequestRuntimeCandidateResult) { result.Audit.ContextBudget.ActualTokens++ }},
		{name: "feedback class", mutate: func(result *RequestRuntimeCandidateResult) {
			feedback := feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)
			*result = feedback
			result.Audit.FailureClass = string(runtimecontract.RuntimeCandidateFailureClassInternal)
		}},
		{name: "unsubmitted response bytes", mutate: func(result *RequestRuntimeCandidateResult) {
			feedback := feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)
			*result = feedback
			result.Audit.ResponseBytes = 1
		}},
		{name: "unsubmitted streaming", mutate: func(result *RequestRuntimeCandidateResult) {
			feedback := feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)
			*result = feedback
			result.Audit.PhysicalResponseMode = llmaudit.PhysicalResponseModeStreaming
		}},
		{name: "unsubmitted usage", mutate: func(result *RequestRuntimeCandidateResult) {
			feedback := feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)
			*result = feedback
			result.Usage = &llmaudit.ProviderUsage{InputTokens: 1, TotalTokens: 1}
		}},
		{name: "unsubmitted actual tokens", mutate: func(result *RequestRuntimeCandidateResult) {
			feedback := feedbackRuntimeCandidateResult(t, runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, false)
			*result = feedback
			result.Audit.ContextBudget.ActualTokens = 1
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := cloneRuntimeCandidateResult(validRuntimeCandidateResult())
			testCase.mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("tampered result was accepted")
			}
		})
	}
}

func validRuntimeCandidateResult() RequestRuntimeCandidateResult {
	content := []byte(`{"operations":[]}`)
	usage := &llmaudit.ProviderUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	return RequestRuntimeCandidateResult{
		WorkspaceID: "workspace-1", RunID: "run-1",
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: "logical-1", Sequence: 1, PhysicalCallID: "physical-1",
		},
		ExpectedDocumentHash: "expected-1",
		DocumentOutcome: &RuntimeProviderDocumentOutcome{
			Framing: runtimecontract.RuntimeProviderDocumentFraming{
				Disposition: runtimecontract.RuntimeProviderDocumentComplete, ExpectedDocumentHash: "expected-1",
				ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(content), ResponseBytes: int64(len(content)),
			},
			Content: content,
		},
		Audit: &runtimecontract.ProviderRequestAuditView{
			RequestID: "physical-1", LogicalRequestID: "logical-1", PhysicalCallID: "physical-1",
			WorkspaceID: "workspace-1", RunID: "run-1",
			Target:    runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"},
			Operation: "responses", StablePrefixHash: "stable-prefix-1", Kind: "runtime_candidate",
			PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
			ContextBudget: llmaudit.ContextBudget{
				ProviderID: "ai-proxy", ModelID: "model-1", ActualTokens: 5, ProviderSubmitted: true,
			},
			ProviderSubmitted: true, Status: "completed", RequestBytes: 128, ResponseBytes: 64,
		},
		Usage: usage,
	}
}

func feedbackRuntimeCandidateResult(t *testing.T, category runtimecontract.RuntimeFeedbackCategory, class runtimecontract.RuntimeCandidateFailureClass, submitted bool) RequestRuntimeCandidateResult {
	t.Helper()
	result := validRuntimeCandidateResult()
	feedback := runtimeCandidateFeedback(t, category, class)
	result.DocumentOutcome = nil
	result.Feedback = &feedback
	result.Usage = nil
	result.Audit.Status = "failed"
	result.Audit.FailureClass = string(class)
	result.Audit.ContextBudget.ActualTokens = 0
	setRuntimeCandidateResultSubmitted(&result, submitted)
	return result
}

func runtimeCandidateFeedback(t *testing.T, category runtimecontract.RuntimeFeedbackCategory, class runtimecontract.RuntimeCandidateFailureClass) runtimecontract.RuntimeFeedback {
	t.Helper()
	feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(
		runtimecontract.RuntimeFeedbackScope{WorkspaceID: "workspace-1", RunID: "run-1"},
		runtimecontract.RuntimeFeedbackDomainProviderRequest,
		category,
		runtimecontract.RuntimeModuleLLM,
		runtimecontract.RuntimeCandidateFailure{Class: class, Code: "provider-failure", Signature: "provider-failure-1", Reason: "safe provider diagnostic"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return feedback
}

func staleRuntimeCandidateResult() RequestRuntimeCandidateResult {
	result := validRuntimeCandidateResult()
	result.DocumentOutcome = nil
	result.Usage = nil
	result.Stale = &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: "provider-ceiling:llm", ObservationID: "provider-observation-1",
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: "ai-proxy/model-1", ExpectedRevision: "config-1", ObservedRevision: "config-2",
		Reason: "provider configuration changed",
	}
	result.Audit.Status = "failed"
	result.Audit.FailureClass = string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration)
	result.Audit.ContextBudget.ActualTokens = 0
	setRuntimeCandidateResultSubmitted(&result, false)
	return result
}

func setRuntimeCandidateResultSubmitted(result *RequestRuntimeCandidateResult, submitted bool) {
	result.Audit.ProviderSubmitted = submitted
	result.Audit.ContextBudget.ProviderSubmitted = submitted
	if submitted {
		result.Audit.RequestBytes = 128
		return
	}
	result.Audit.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming
	result.Audit.ResponseBytes = 0
}

func mutateRuntimeCandidateResult(result RequestRuntimeCandidateResult, mutate func(*RequestRuntimeCandidateResult)) RequestRuntimeCandidateResult {
	result = cloneRuntimeCandidateResult(result)
	mutate(&result)
	return result
}

func cloneRuntimeCandidateResult(result RequestRuntimeCandidateResult) RequestRuntimeCandidateResult {
	if result.Audit != nil {
		audit := *result.Audit
		result.Audit = &audit
	}
	if result.Usage != nil {
		usage := *result.Usage
		result.Usage = &usage
	}
	if result.DocumentOutcome != nil {
		outcome := result.DocumentOutcome.Normalized()
		result.DocumentOutcome = &outcome
	}
	if result.Feedback != nil {
		feedback := result.Feedback.CloneOriginal()
		result.Feedback = &feedback
	}
	if result.Stale != nil {
		stale := result.Stale.Normalized()
		result.Stale = &stale
	}
	return result
}

func TestLLMUsageRecordedDataRequiresBoundedPhysicalResponseMode(t *testing.T) {
	unsubmitted := LLMUsageRecordedData{RequestID: "setup-request-1", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming}
	if err := unsubmitted.Validate(); err != nil {
		t.Fatalf("valid unsubmitted usage notification rejected: %v", err)
	}
	submitted := validRuntimeLLMUsageRecordedData()
	if err := submitted.Validate(); err != nil {
		t.Fatalf("valid submitted usage notification rejected: %v", err)
	}

	tests := []struct {
		name string
		data LLMUsageRecordedData
	}{
		{name: "unbounded mode", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.PhysicalResponseMode = "stream" })},
		{name: "non-canonical mode", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.PhysicalResponseMode = " streaming " })},
		{name: "non-canonical status", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.Status = " completed " })},
		{name: "non-canonical kind", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.Kind = " runtime_candidate " })},
		{name: "missing logical identity", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.LogicalRequestID = "" })},
		{name: "physical correlation mismatch", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.RequestID = "physical-2" })},
		{name: "negative measurement", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.ResponseBytes = -1 })},
		{name: "negative cache tokens", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.CacheUsage.CacheReadTokens = -1 })},
		{name: "inconsistent total", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.TotalTokens = 1 })},
		{name: "context submission mismatch", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.ContextBudget.ProviderSubmitted = false })},
		{name: "context actual token mismatch", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.ContextBudget.ActualTokens++ })},
		{name: "submitted without request bytes", data: mutateLLMUsageRecordedData(submitted, func(data *LLMUsageRecordedData) { data.RequestBytes = 0 })},
		{name: "unsubmitted streaming", data: mutateLLMUsageRecordedData(unsubmitted, func(data *LLMUsageRecordedData) { data.PhysicalResponseMode = llmaudit.PhysicalResponseModeStreaming })},
		{name: "unsubmitted response bytes", data: mutateLLMUsageRecordedData(unsubmitted, func(data *LLMUsageRecordedData) { data.ResponseBytes = 1 })},
		{name: "unsubmitted tokens", data: mutateLLMUsageRecordedData(unsubmitted, func(data *LLMUsageRecordedData) { data.TotalTokens = 1 })},
		{name: "unsubmitted context actual tokens", data: mutateLLMUsageRecordedData(unsubmitted, func(data *LLMUsageRecordedData) {
			data.ContextBudget = llmaudit.ContextBudget{ProviderID: "ai-proxy", ModelID: "model-1", ActualTokens: 1}
		})},
		{name: "unsubmitted cache tokens", data: mutateLLMUsageRecordedData(unsubmitted, func(data *LLMUsageRecordedData) { data.CacheUsage.CacheReadTokens = 1 })},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.data.Validate(); err == nil {
				t.Fatal("tampered LLM usage notification was accepted")
			}
		})
	}
}

func validRuntimeLLMUsageRecordedData() LLMUsageRecordedData {
	return LLMUsageRecordedData{
		RequestID: "physical-1", LogicalRequestID: "logical-1", PhysicalCallID: "physical-1",
		WorkspaceID: "workspace-1", RunID: "run-1", Kind: "runtime_candidate",
		Target:    runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"},
		Operation: "responses", StablePrefixHash: "stable-prefix-1",
		PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming, ProviderSubmitted: true,
		Status: "completed", RequestBytes: 128, ResponseBytes: 64,
		InputTokens: 3, OutputTokens: 2, TotalTokens: 5,
		ContextBudget: llmaudit.ContextBudget{
			ProviderID: "ai-proxy", ModelID: "model-1", ProviderSubmitted: true, ActualTokens: 5,
		},
	}
}

func mutateLLMUsageRecordedData(data LLMUsageRecordedData, mutate func(*LLMUsageRecordedData)) LLMUsageRecordedData {
	mutate(&data)
	return data
}

func TestValidateRuntimeProviderRequestFeedbackOwnsClosedCategoryClassMapping(t *testing.T) {
	tests := []struct {
		name     string
		category runtimecontract.RuntimeFeedbackCategory
		class    runtimecontract.RuntimeCandidateFailureClass
	}{
		{name: "authentication", category: runtimecontract.RuntimeFeedbackCategoryProviderAuthentication, class: runtimecontract.RuntimeCandidateFailureClassProviderAuthentication},
		{name: "configuration", category: runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, class: runtimecontract.RuntimeCandidateFailureClassProviderConfiguration},
		{name: "transient", category: runtimecontract.RuntimeFeedbackCategoryProviderTransient, class: runtimecontract.RuntimeCandidateFailureClassProviderTransient},
		{name: "explicit protocol", category: runtimecontract.RuntimeFeedbackCategoryProviderProtocol, class: runtimecontract.RuntimeCandidateFailureClassProviderProtocol},
		{name: "generic provider protocol", category: runtimecontract.RuntimeFeedbackCategoryProviderProtocol, class: runtimecontract.RuntimeCandidateFailureClassProvider},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(
				runtimecontract.RuntimeFeedbackScope{WorkspaceID: "workspace-1", RunID: "run-1"},
				runtimecontract.RuntimeFeedbackDomainProviderRequest,
				tc.category,
				runtimecontract.RuntimeModuleLLM,
				runtimecontract.RuntimeCandidateFailure{Class: tc.class, Code: "provider-failure", Signature: "provider-failure-1", Reason: "safe provider diagnostic"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateRuntimeProviderRequestFeedback(feedback); err != nil {
				t.Fatalf("valid owner feedback rejected: %v", err)
			}
			mismatch := feedback.CloneOriginal()
			mismatch.StableCategory = runtimecontract.RuntimeFeedbackCategoryProviderAuthentication
			if mismatch.StableCategory == tc.category {
				mismatch.StableCategory = runtimecontract.RuntimeFeedbackCategoryProviderTransient
			}
			if err := ValidateRuntimeProviderRequestFeedback(mismatch); err == nil {
				t.Fatal("category/class mismatch was accepted")
			}
		})
	}
}
