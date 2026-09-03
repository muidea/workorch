package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func runtimeCandidateCorrectionInputForProviderTest(manifestHash, path, alternative string) *llmmodels.RuntimeCandidateCorrectionInput {
	if alternative == "" {
		alternative = "field_present"
	}
	witness := runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension:        runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
		Path:             path,
		RejectedValue:    "field_absent",
		AlternativeValue: alternative,
	}
	if strings.Contains(path, "targetRefs") {
		witness.Dimension = runtimecontract.RuntimeCandidateCorrectionWitnessTarget
		witness.OperationKind = runtimecontract.RuntimeBlockKindPatchApply
		witness.RejectedValue = "other.go"
		witness.RejectedShapeHash = "candidate-shape-rejected"
		witness.AlternativeShapeHash = "candidate-shape-alternative"
	}
	input := llmmodels.RuntimeCandidateCorrectionInput{
		ManifestHash: manifestHash,
		Witness:      witness,
	}.Normalized()
	return &input
}

func runtimeCandidateCorrectionObservationForProviderTest(failure runtimecontract.RuntimeCandidateFailure, path string) *runtimecontract.RuntimeCandidateCorrectionObservation {
	diagnostic := failure.Diagnostic.Clone()
	if diagnostic == nil {
		diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: path}
	}
	value := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: failure.Class, Code: failure.Code, Signature: failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...), Diagnostic: diagnostic,
	}.Clone()
	return &value
}

func runtimeCandidateWireRuleIdentityForProviderTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, operationIndex int, requiredOperationRef, operationKind string) *runtimecontract.RuntimeCandidateWireRuleIdentity {
	t.Helper()
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var (
		rule  runtimecontract.RuntimeCandidateWireOperationRule
		found bool
	)
	if strings.TrimSpace(requiredOperationRef) != "" {
		rule, found = runtimecontract.RuntimeCandidateWireRuleForRequiredOperation(wire, requiredOperationRef)
	} else {
		rule, found = runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, operationKind)
	}
	if !found {
		t.Fatalf("candidate wire rule ref=%q kind=%q not found in manifest", requiredOperationRef, operationKind)
	}
	identity, err := runtimecontract.RuntimeCandidateWireRuleIdentityForOperation(wire, rule, operationIndex)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func runtimeCandidateCorrectionPairForProviderTest(failure runtimecontract.RuntimeCandidateFailure, manifestHash, path, alternative string) (*runtimecontract.RuntimeCandidateCorrectionObservation, *llmmodels.RuntimeCandidateCorrectionInput) {
	observation := runtimeCandidateCorrectionObservationForProviderTest(failure, path)
	input := runtimeCandidateCorrectionInputForProviderTest(manifestHash, path, alternative)
	switch input.Witness.Dimension {
	case runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField:
		if input.Witness.AlternativeValue == "field_absent" {
			input.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField}
		} else {
			input.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField}
		}
	case runtimecontract.RuntimeCandidateCorrectionWitnessTarget:
		switch failure.Code {
		case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
			input.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape}
		default:
			input.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReplaceTarget}
		}
	}
	if observation.Diagnostic == nil {
		observation.Diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: path}
	}
	observation.Diagnostic.ManifestHash = manifestHash
	observation.Diagnostic.Objectives = append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...)
	field := runtimeCandidateCorrectionPathFieldForProviderTest(path)
	if input.Witness.AlternativeValue == "field_present" && observation.Diagnostic.Actual == "" {
		observation.Diagnostic.Actual = "missing_" + field
	} else if observation.Diagnostic.Actual == "" {
		observation.Diagnostic.Actual = input.Witness.RejectedValue
	}
	if input.Witness.AlternativeValue == "field_absent" && observation.Diagnostic.Actual != "" {
		input.Witness.RejectedValue = observation.Diagnostic.Actual
	}
	if input.Witness.RejectedShapeHash != "" && observation.Diagnostic.RejectedShapeHash == "" {
		observation.Diagnostic.RejectedShapeHash = input.Witness.RejectedShapeHash
	}
	if observation.Diagnostic.RejectedShapeHash != "" && input.Witness.RejectedShapeHash != "" {
		input.Witness.RejectedShapeHash = observation.Diagnostic.RejectedShapeHash
		if input.Witness.AlternativeShapeHash == input.Witness.RejectedShapeHash {
			input.Witness.AlternativeShapeHash += "-alternative"
		}
	}
	if observation.Diagnostic.RejectedShapeHash != "" && input.Witness.RejectedShapeHash == "" {
		input.Witness.RejectedShapeHash = observation.Diagnostic.RejectedShapeHash
		input.Witness.AlternativeShapeHash = observation.Diagnostic.RejectedShapeHash + "-alternative"
	}
	if input.Witness.AlternativeValue == "field_present" && len(observation.Diagnostic.Required) == 0 {
		observation.Diagnostic.Required = []string{field}
	}
	if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField && input.Witness.AlternativeValue == runtimecontract.RuntimeCandidateCorrectionFieldPresent {
		if rejected, found := runtimecontract.RuntimeCandidateRequiredFieldCorrectionRejectedValue(*observation.Diagnostic); found {
			input.Witness.RejectedValue = rejected
		}
	}
	if input.Witness.AlternativeValue == "field_absent" && len(observation.Diagnostic.Forbidden) == 0 {
		observation.Diagnostic.Forbidden = []string{field}
	}
	observation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness)
	value := observation.Clone()
	return &value, input
}

func runtimeCandidateCorrectionPathFieldForProviderTest(path string) string {
	path = strings.TrimSpace(path)
	if dot := strings.LastIndex(path, "."); dot >= 0 && dot+1 < len(path) {
		return path[dot+1:]
	}
	return path
}

func runtimeSemanticCorrectionObservationForProviderTest(diagnostic *runtimecontract.RuntimeOriginalDiagnostic) *runtimecontract.RuntimeSemanticCorrectionObservation {
	if diagnostic == nil {
		return nil
	}
	value := runtimecontract.RuntimeSemanticCorrectionObservation{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: diagnostic.OwnerModuleID, Class: diagnostic.OriginalClass, Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, OperationID: diagnostic.OperationID,
		TargetRefs: append([]string(nil), diagnostic.TargetRefs...), ExpectedRevision: diagnostic.ExpectedRevision,
		ObservedRevision: diagnostic.ObservedRevision, FieldViolations: runtimecontract.CloneRuntimeFieldViolations(diagnostic.FieldViolations),
		SemanticViolations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), diagnostic.SemanticViolations...),
	}.Clone()
	if diagnostic.TargetObservation != nil {
		target := diagnostic.TargetObservation.Clone()
		value.TargetObservation = &target
	}
	return &value
}

func TestRuntimeCandidatePreflightAuditIsExplicitlyNonStreaming(t *testing.T) {
	now := time.Now().UTC()
	audit := sealedAuditFromRequest(llmmodels.RuntimeCandidateRequest{}, "preflight-1", nil, now, now, false, llmmodels.RuntimeCandidateStatusCompleted, "", 0)
	if audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || audit.ProviderSubmitted {
		t.Fatalf("preflight audit = %+v", audit)
	}
}

func TestRuntimeCandidatePreDispatchFailureRetainsCanonicalNonStreamingAudit(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	generation, err := (&Biz{}).GenerateCandidate(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "configuration is not initialized") {
		t.Fatalf("pre-dispatch error = %v", err)
	}
	if generation.Audit == nil {
		t.Fatal("pre-dispatch failure lost sealed request audit")
	}
	if generation.Audit.ProviderSubmitted || generation.Audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("pre-dispatch audit = %+v", generation.Audit)
	}
	if generation.Audit.ContextBudget == nil || generation.Audit.ContextBudget.ProviderSubmitted {
		t.Fatalf("pre-dispatch budget audit = %+v", generation.Audit.ContextBudget)
	}
	if generation.Audit.RequestID != request.RequestIdentity.PhysicalCallID || generation.Audit.PhysicalCallID != request.RequestIdentity.PhysicalCallID ||
		generation.Audit.LogicalRequestID != request.RequestIdentity.LogicalRequestID || generation.Audit.WorkspaceID != string(request.SemanticContext.WorkspaceID) ||
		generation.Audit.RunID != string(request.SemanticContext.RunID) || generation.Audit.FailureClass != string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration) ||
		generation.Usage != nil || generation.Audit.ResponseBytes != 0 {
		t.Fatalf("pre-dispatch exact result audit = %+v usage=%+v", generation.Audit, generation.Usage)
	}
}

func TestRuntimeCandidatePhysicalModeUsesDispatchOnlyAfterSubmission(t *testing.T) {
	streaming := providerExecutionMetadata{PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming}
	if got := canonicalProviderPhysicalResponseMode(streaming); got != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("unsubmitted streaming selection mode = %q, want explicit non-streaming preflight", got)
	}
	streaming.Submitted = true
	if got := canonicalProviderPhysicalResponseMode(streaming); got != llmaudit.PhysicalResponseModeStreaming {
		t.Fatalf("submitted streaming dispatch mode = %q", got)
	}
}

func TestRuntimeProviderCompletedWithoutFinalOutputRequiresExactSubmittedEmptyTypedObservation(t *testing.T) {
	validCompletion := llmmodels.ProviderCompletionResult{
		ProviderSubmitted:    true,
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
		Usage:                &llmaudit.ProviderUsage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
	}
	validExecution := providerExecutionMetadata{
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
		Submitted:            true,
	}
	tests := []struct {
		name       string
		err        error
		completion llmmodels.ProviderCompletionResult
		execution  providerExecutionMetadata
		want       bool
	}{
		{
			name:       "submitted non-stream completed empty",
			err:        &ProviderProtocolError{Code: providerFinalOutputMissingCode, ResponseBytes: 73},
			completion: validCompletion,
			execution:  validExecution,
			want:       true,
		},
		{
			name:       "nil error",
			completion: validCompletion,
			execution:  validExecution,
		},
		{
			name:       "untyped matching text",
			err:        errors.New("llm provider protocol error: " + providerFinalOutputMissingCode),
			completion: validCompletion,
			execution:  validExecution,
		},
		{
			name:       "different typed code",
			err:        &ProviderProtocolError{Code: "invalid_response_json", ResponseBytes: 73},
			completion: validCompletion,
			execution:  validExecution,
		},
		{
			name:       "non-canonical typed code",
			err:        &ProviderProtocolError{Code: " " + providerFinalOutputMissingCode + " ", ResponseBytes: 73},
			completion: validCompletion,
			execution:  validExecution,
		},
		{
			name:       "unsubmitted",
			err:        &ProviderProtocolError{Code: providerFinalOutputMissingCode, ResponseBytes: 73},
			completion: validCompletion,
			execution: providerExecutionMetadata{
				PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
			},
		},
		{
			name: "non-empty payload",
			err:  &ProviderProtocolError{Code: providerFinalOutputMissingCode, ResponseBytes: 73},
			completion: llmmodels.ProviderCompletionResult{
				Payload:              []byte("provider-private-fragment"),
				ProviderSubmitted:    true,
				PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
				Usage:                validCompletion.Usage,
			},
			execution: validExecution,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeProviderCompletedWithoutFinalOutput(test.err, test.completion, test.execution); got != test.want {
				t.Fatalf("completed-without-final-output = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRuntimeCandidateProviderClassifiesAuthenticationConfigurationAndTransientFailures(t *testing.T) {
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 401, Body: "authentication_failed"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderAuthentication {
		t.Fatalf("401 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 404, Body: "model not found"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderConfiguration {
		t.Fatalf("404 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 503, Body: "unavailable"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
		t.Fatalf("503 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(context.DeadlineExceeded, true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
		t.Fatalf("deadline class = %s, want ordinary provider transport failure", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderProtocolError{Code: "invalid_response_json", ResponseBytes: 17}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderProtocol {
		t.Fatalf("protocol class = %s, want provider protocol", got)
	}
	if got := runtimeCandidateProviderFailureClass(llmmodels.ProviderModelFailure{Code: llmmodels.ProviderModelFailureModelNotFound, Message: "typed catalog fact"}, false); got != runtimecontract.RuntimeCandidateFailureClassProviderConfiguration {
		t.Fatalf("typed model failure class = %s, want provider configuration", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderTransportError{Code: "network_timeout"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
		t.Fatalf("typed transport failure class = %s, want provider transient", got)
	}
}

func TestRuntimeCandidateProviderFailureClassIgnoresAdversarialGenericMessages(t *testing.T) {
	for _, message := range []string{
		"authentication_failed: invalid api key",
		"unauthorized forbidden",
		"provider client is not configured",
		"model not found configuration",
	} {
		t.Run(message+" submitted", func(t *testing.T) {
			if got := runtimeCandidateProviderFailureClass(errors.New(message), true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
				t.Fatalf("generic submitted message %q class = %s, want provider transient", message, got)
			}
		})
		t.Run(message+" unsubmitted", func(t *testing.T) {
			if got := runtimeCandidateProviderFailureClass(errors.New(message), false); got != runtimecontract.RuntimeCandidateFailureClassProviderConfiguration {
				t.Fatalf("generic unsubmitted message %q class = %s, want provider configuration", message, got)
			}
		})
	}
}

func TestProviderErrorsExposeOnlyAuditSafeRuntimeIdentity(t *testing.T) {
	tests := []struct {
		name          string
		errorIdentity interface {
			RuntimeProviderFailureIdentity() (string, string)
		}
		wantCode      string
		wantSignature string
	}{
		{
			name: "protocol guard", errorIdentity: &ProviderProtocolError{Code: "response_byte_limit_exceeded", ResponseBytes: 761537},
			wantCode: "response_byte_limit_exceeded", wantSignature: "response-byte-limit-exceeded",
		},
		{
			name: "transport category", errorIdentity: &ProviderTransportError{Code: "network_timeout", cause: fmt.Errorf("https://secret.invalid")},
			wantCode: "network_timeout", wantSignature: "network-timeout",
		},
		{
			name: "http status", errorIdentity: &ProviderHTTPStatusError{StatusCode: 401, Body: "private provider body"},
			wantCode: "provider_http_status_401", wantSignature: "provider-http-status-401",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, signature := tc.errorIdentity.RuntimeProviderFailureIdentity()
			if code != tc.wantCode || signature != tc.wantSignature {
				t.Fatalf("runtime provider identity = %q/%q, want %q/%q", code, signature, tc.wantCode, tc.wantSignature)
			}
			if strings.Contains(code, "secret") || strings.Contains(signature, "secret") || strings.Contains(code, "private") || strings.Contains(signature, "private") {
				t.Fatalf("runtime provider identity leaked adapter-private content: %q/%q", code, signature)
			}
		})
	}
}

func TestRuntimeProviderFeedbackPreservesExplicitProtocolClass(t *testing.T) {
	feedback, err := runtimeProviderRequestFeedback(llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "workspace-1", RunID: "run-1", DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "request-1", Sequence: 1, PhysicalCallID: "physical-1"},
	}, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassProviderProtocol, Code: "invalid_response_json",
		Signature: "provider-protocol-invalid-json", Reason: "provider response violated the protocol contract",
	})
	if err != nil {
		t.Fatal(err)
	}
	if feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryProviderProtocol || feedback.OriginalDiagnostic == nil || feedback.OriginalDiagnostic.OriginalClass != string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol) {
		t.Fatalf("protocol feedback = %+v", feedback)
	}
}

func TestRuntimeCandidateProviderUsesOrchestratorPhysicalCallID(t *testing.T) {
	request := llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-1", Sequence: 3, PhysicalCallID: "physical-call-3"}}
	if got := newProviderRequestID(request); got != request.RequestIdentity.PhysicalCallID {
		t.Fatalf("provider request ID = %q, want persisted physical call ID %q", got, request.RequestIdentity.PhysicalCallID)
	}
}

func TestProviderAuditRequestBytesPreferAdapterWireMeasurement(t *testing.T) {
	budget := &llmmodels.ProviderContextBudget{ActualBytes: 41}
	if got := providerAuditRequestBytes(73, budget); got != 73 {
		t.Fatalf("submitted request bytes = %d, want adapter wire measurement 73", got)
	}
	if got := providerAuditRequestBytes(0, budget); got != 41 {
		t.Fatalf("pre-dispatch request bytes = %d, want rendered budget measurement 41", got)
	}
}

func TestRuntimeSemanticPlanProviderUsesSingleCachePipelineAndSafeCorrection(t *testing.T) {
	base := runtimeSemanticProviderTestRequest(t)
	base.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	base.SemanticContext.BatchID = ""
	base.SemanticContext.BatchCoverageWorkItemIDs = nil
	base.SemanticManifest.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-000"}
	prerequisite := runtimecontract.RuntimeCoverageItem{ID: "cwi-000", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", TargetText: "prerequisite"}
	base.SemanticManifest.CoverageItems = append(base.SemanticManifest.CoverageItems, prerequisite)
	base.SemanticManifest.ProofObligations = append(base.SemanticManifest.ProofObligations, runtimecontract.RuntimeSemanticProofObligation{
		CoverageWorkItemID: "cwi-000", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	})
	base.SemanticContext.DefinitionSeed.CoverageItems = append(base.SemanticContext.DefinitionSeed.CoverageItems, prerequisite)
	base.SemanticContext.DefinitionSeed.Slices[0].CoverageWorkItemIDs = append(base.SemanticContext.DefinitionSeed.Slices[0].CoverageWorkItemIDs, "cwi-000")
	base.SemanticContext.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(base.SemanticContext.DefinitionSeed)
	base.SemanticManifest.DefinitionSeedHash = base.SemanticContext.DefinitionSeed.Hash
	base.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(base.SemanticManifest)
	initial, err := runtimeProviderRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Kind != "runtime_semantic_plan" || initial.RawTextOutput || !strings.Contains(initial.Schema, `"semantic_plan"`) {
		t.Fatalf("semantic provider request = %+v", initial)
	}
	if initial.ContextBudget == nil || initial.ContextBudget.CachePlan == nil || initial.ContextBudget.CachePlan.StablePrefixHash != base.StablePrefixHash {
		t.Fatalf("semantic cache plan = %+v", initial.ContextBudget)
	}
	if got := runtimePromptSectionIDs(initial.PromptContext); !reflect.DeepEqual(got, []string{
		"C0.runtime-semantic-contract", "C1.goal-dod", "C2.capability-contract", "C3.semantic-authority", "C4.semantic-tail", "C5.acceptance-slices",
	}) {
		t.Fatalf("semantic section order = %v", got)
	}
	if authority := initial.PromptContext.Sections[3].Content; !strings.Contains(authority, "planning-soul-guidance") {
		t.Fatalf("semantic C3 lost planning Soul guidance: %s", authority)
	}
	if authority := initial.PromptContext.Sections[3].Content; !strings.Contains(authority, "proposals=[]") || !strings.Contains(authority, "must be present and empty") {
		t.Fatalf("coverage-order instruction contradicts its required empty proposals field: %s", authority)
	}

	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "schema", OriginalCode: "missing_target",
		OriginalMessage: "secret raw provider message must not be rendered", Signature: "semantic/missing-target",
		TargetRefs: []string{"main.go"}, FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.coverageOrder", Actual: "cwi-001 before cwi-000", Required: []string{"cwi-000 before cwi-001"}}},
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{{Kind: runtimecontract.RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder", RequiredBeforeID: "cwi-000", RequiredAfterID: "cwi-001", ActualBeforePosition: 1, ActualAfterPosition: 0}},
	}
	observation := runtimeSemanticCorrectionObservationForProviderTest(rejection)
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder, ManifestHash: base.SemanticManifest.Hash,
		DefinitionSeedHash: base.SemanticManifest.DefinitionSeedHash, StablePrefixHash: base.StablePrefixHash,
		SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
		Violations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), rejection.SemanticViolations...),
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	retryRequest := base
	retryRequest.CandidateAttempt = 1
	retryRequest.SemanticCorrectionObservation = observation
	retryRequest.SemanticCorrectionDirective = &directive
	retry, err := runtimeProviderRequest(retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 3; idx++ {
		if initial.PromptContext.Sections[idx].Content != retry.PromptContext.Sections[idx].Content {
			t.Fatalf("semantic stable section %d drifted between attempts", idx)
		}
	}
	if retry.ContextBudget.CachePlan.StablePrefixHash != initial.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("semantic correction changed stable prefix")
	}
	for _, name := range []string{"WorkspaceDefinitionHash", "CapabilityContractHash"} {
		initialPlan := initial.ContextBudget.CachePlan
		retryPlan := retry.ContextBudget.CachePlan
		var initialHash, retryHash string
		switch name {
		case "WorkspaceDefinitionHash":
			initialHash, retryHash = initialPlan.WorkspaceDefinitionHash, retryPlan.WorkspaceDefinitionHash
		case "CapabilityContractHash":
			initialHash, retryHash = initialPlan.CapabilityContractHash, retryPlan.CapabilityContractHash
		}
		if initialHash == "" || initialHash != retryHash {
			t.Fatalf("semantic correction changed stable %s: initial=%q retry=%q", name, initialHash, retryHash)
		}
	}
	tail := retry.PromptContext.Sections[4].Content
	if strings.Contains(tail, rejection.OriginalMessage) || !strings.Contains(tail, `"missing_target"`) || !strings.Contains(tail, `"$.coverageOrder"`) || !strings.Contains(tail, `"requiredBeforeId":"cwi-000"`) {
		t.Fatalf("semantic correction tail is not provider-safe: %s", tail)
	}
}

func TestRuntimeSemanticCoverageOrderCorrectionProjectsExactManifestCoverage(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	request.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	request.SemanticContext.BatchID = ""
	request.SemanticContext.BatchSequence = 0
	request.SemanticContext.BatchCoverageWorkItemIDs = nil
	violation := runtimecontract.RuntimeSemanticContractViolation{
		Kind: runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.proposals", Actual: "missing_required",
		RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "schema", OriginalCode: "missing_required",
		OriginalMessage: "private provider response must not be rendered", Signature: "semantic/missing-required",
		FieldViolations:    []runtimecontract.RuntimeFieldViolation{{Path: "$.proposals", Actual: "missing_required"}},
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{violation},
	}
	observation := runtimeSemanticCorrectionObservationForProviderTest(rejection)
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder, ManifestHash: request.SemanticManifest.Hash,
		DefinitionSeedHash: request.SemanticManifest.DefinitionSeedHash, StablePrefixHash: request.StablePrefixHash,
		SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
		Violations: []runtimecontract.RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	request.CandidateAttempt = 1
	request.SemanticCorrectionObservation = observation
	request.SemanticCorrectionDirective = &directive
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	tail := providerRequest.PromptContext.Sections[4].Content
	for _, required := range []string{`"kind":"coverage_order_document"`, `"path":"$.proposals"`, `"requiredCoverageWorkItemIds":["cwi-001"]`} {
		if !strings.Contains(tail, required) {
			t.Fatalf("coverage-order correction tail lost %s: %s", required, tail)
		}
	}
	if strings.Contains(tail, rejection.OriginalMessage) {
		t.Fatalf("coverage-order correction leaked provider message: %s", tail)
	}
}

func TestRuntimeSemanticProviderKeepsOwnerFactDeclarationInC2AndCurrentFactsInC3(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	request.SemanticContext.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "software-dev", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "validation_preparation", Scope: "deliverable",
			AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, RequiredOwnerFacts: []runtimecontract.RuntimeOwnerFactRequirement{requirement},
		}},
	}}
	request.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(
		request.SemanticContext.Definition, request.SemanticContext.Activation.ExecutionContract,
	)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	c2 := providerRequest.PromptContext.Sections[2].Content
	c3 := providerRequest.PromptContext.Sections[3].Content
	for _, required := range []string{"RequiredOwnerFacts", runtimecontract.RuntimeModuleShell, string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), runtimecontract.RuntimeBlockKindManagedValidationSession} {
		if !strings.Contains(c2, required) {
			t.Fatalf("semantic C2 lost owner-fact declaration %q: %s", required, c2)
		}
	}
	if strings.Contains(c3, "RequiredOwnerFacts") {
		t.Fatalf("immutable owner-fact declaration leaked into volatile current catalog: %s", c3)
	}
}

func TestRuntimeSemanticCorrectionBoundaryClonesNestedValues(t *testing.T) {
	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "semantic", OriginalCode: "order",
		OriginalMessage: "order rejected", Signature: "semantic/order",
		FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.coverageOrder", Required: []string{"cwi-000 before cwi-001"}}},
	}
	directive := &runtimecontract.RuntimeSemanticCorrectionDirective{Violations: []runtimecontract.RuntimeSemanticContractViolation{{RequiredBeforeID: "cwi-000"}}}
	clonedRejection := cloneRuntimeOriginalDiagnostic(rejection)
	clonedDirective := cloneRuntimeSemanticCorrectionDirective(directive)
	clonedRejection.FieldViolations[0].Required[0] = "changed"
	clonedDirective.Violations[0].RequiredBeforeID = "changed"
	if rejection.FieldViolations[0].Required[0] != "cwi-000 before cwi-001" || directive.Violations[0].RequiredBeforeID != "cwi-000" {
		t.Fatal("LLM owner boundary retained caller-owned nested storage")
	}
}

func TestRuntimeSemanticExistingTargetCorrectionCarriesOwnerRevisionAndReplacementFact(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	catalog := request.SemanticManifest.OwnerCatalogs[0].Normalized()
	catalog.OperationKinds = append(catalog.OperationKinds, runtimecontract.RuntimeBlockKindPatchApply)
	catalog.OperationVariants = append(catalog.OperationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
		Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace",
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		ConstraintRefs:        []string{"existing-target"},
	})
	catalog.TargetRefs = []string{"existing.go"}
	catalog.ConstraintRefs = append(catalog.ConstraintRefs, "existing-target")
	catalog.ObservedRevision = "tree-v2"
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	request.SemanticManifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	request.SemanticManifest.AllowedOperationKinds = append(request.SemanticManifest.AllowedOperationKinds, runtimecontract.RuntimeBlockKindPatchApply)
	request.SemanticManifest.AllowedTargetPolicies = append(request.SemanticManifest.AllowedTargetPolicies, runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "semantic_feasibility", OriginalCode: "operation_target_exists", OriginalMessage: "private owner failure must not be rendered",
		Signature: "file/operation-target-exists", TargetRefs: []string{"existing.go"}, ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2",
	}
	violation := runtimecontract.RuntimeSemanticContractViolation{
		Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].target.policy", CoverageWorkItemID: "cwi-001", ProposalIndex: 0,
		Rule: runtimecontract.RuntimeSemanticConstraintAllowedValue, Actual: string(runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget), Allowed: []string{string(runtimecontract.RuntimeOperationTargetPolicyExactTarget)},
	}
	diagnostic.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{violation}
	observation := runtimeSemanticCorrectionObservationForProviderTest(diagnostic)
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent, PlanStep: request.SemanticContext.PlanStep,
		ManifestHash: request.SemanticManifest.Hash, DefinitionSeedHash: request.SemanticManifest.DefinitionSeedHash,
		StablePrefixHash: request.StablePrefixHash, SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
		Violations: []runtimecontract.RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	request.CandidateAttempt = 1
	request.SemanticCorrectionObservation = observation
	request.SemanticCorrectionDirective = &directive
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	tail := providerRequest.PromptContext.Sections[4].Content
	authority := providerRequest.PromptContext.Sections[3].Content
	for _, want := range []string{"\"existingTarget\"", "\"observedRevision\":\"tree-v2\"", "\"catalogSnapshotId\":\"" + catalog.SnapshotID + "\"", "patch/apply/text_replace", "\"existingTargetObserved\":true"} {
		if !strings.Contains(tail, want) {
			t.Fatalf("semantic correction tail lost %q: %s", want, tail)
		}
	}
	if strings.Contains(tail, diagnostic.OriginalMessage) || strings.Contains(tail, "file/write is create-only") {
		t.Fatalf("semantic correction tail leaked owner message: %s", tail)
	}
	for _, want := range []string{catalog.SnapshotID, "tree-v2", "existing-target"} {
		if !strings.Contains(authority, want) {
			t.Fatalf("sealed C3 authority lost latest catalog fact %q: %s", want, authority)
		}
	}
	if providerRequest.ContextBudget.CachePlan.StablePrefixHash != request.StablePrefixHash || providerRequest.ContextBudget.CachePlan.WorkspaceDefinitionHash == "" || providerRequest.ContextBudget.CachePlan.CapabilityContractHash == "" {
		t.Fatalf("semantic correction changed stable cache groups: %+v", providerRequest.ContextBudget.CachePlan)
	}
}

func TestRuntimeSemanticProofBatchProviderProjectionOmitsUnrelatedCoverage(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	second := runtimecontract.RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-2", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "unrelated second requirement", Order: 2}
	items := append(append([]runtimecontract.RuntimeCoverageItem(nil), request.SemanticContext.DefinitionSeed.CoverageItems...), second)
	slices := append(append([]runtimecontract.RuntimeAcceptanceSlice(nil), request.SemanticContext.DefinitionSeed.Slices...), runtimecontract.RuntimeAcceptanceSlice{ID: "slice-2", Title: "Second", Priority: 2, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"}})
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(request.SemanticContext.Definition, request.SemanticContext.Activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	request.SemanticContext.DefinitionSeed = seed
	request.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	request.SemanticContext.BatchID = "batch-1"
	request.SemanticContext.BatchSequence = 0
	request.SemanticContext.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001"}
	request.SemanticManifest.CoverageItems = items
	request.SemanticManifest.ProofObligations = append(request.SemanticManifest.ProofObligations, runtimecontract.RuntimeSemanticProofObligation{
		CoverageWorkItemID: "cwi-002", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	})
	request.SemanticManifest.DefinitionSeedHash = seed.Hash
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := providerRequest.PromptContext.Sections[3].Content
	sliceProjection := providerRequest.PromptContext.Sections[5].Content
	if strings.Contains(authority, second.TargetText) || strings.Contains(sliceProjection, "cwi-002") || strings.Contains(authority, `"CoverageItems"`) || strings.Contains(authority, `"Slices"`) {
		t.Fatalf("proof batch leaked unrelated/full definition context: authority=%s slices=%s", authority, sliceProjection)
	}
	if !strings.Contains(authority, `"batchId":"batch-1"`) || !strings.Contains(authority, `"batchSequence":0`) || !strings.Contains(authority, `"coverageItems":[{"ID":"cwi-001"`) || !strings.Contains(providerRequest.Schema, `"planStep":{"const":"proof_batch"}`) {
		t.Fatalf("proof batch lost current authority: schema=%s authority=%s", providerRequest.Schema, authority)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(providerRequest.Schema), &schema); err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("proof batch schema has no top-level properties: %s", providerRequest.Schema)
	}
	if _, exposesCoverageOrder := properties["coverageOrder"]; exposesCoverageOrder || !strings.Contains(authority, "Do not return coverageOrder") {
		t.Fatalf("proof batch repeated SM-3 coverage order: schema=%s authority=%s", providerRequest.Schema, authority)
	}
}

func TestRuntimeSemanticProviderProjectionOmitsNonReadyValidationAndOwnerPrivateFacts(t *testing.T) {
	readyID := runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-lsp-bridge", "direct_probe")
	auxiliaryID := runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-lsp-bridge", "lsp_status")
	requiresInputID := runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-lsp-bridge", "lsp_definition")
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"mcp-lsp-bridge"},
		ValidationRecipeRefs: []string{readyID}, ObservedRevision: "mcp-revision-v1",
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{
			{
				ID: readyID, OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
				Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "status-schema", InputBindingRef: "private-empty-input-binding",
				CheckRefs: []string{readyID}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			},
			{
				ID: auxiliaryID, OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
				Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "status-schema", InputBindingRef: "private-status-input-binding",
				CheckRefs: []string{auxiliaryID}, ProofApplicability: runtimecontract.RuntimeValidationProofAuxiliary, EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			},
			{
				ID: requiresInputID, OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
				Readiness: runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "input_binding_required",
				InputSchemaHash: "definition-schema", PrerequisiteRef: "private-definition-input-prerequisite",
				CheckRefs: []string{requiresInputID}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			},
		},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: "mcp-lsp-bridge:tool:lsp_definition", ServerID: "mcp-lsp-bridge", Kind: "tool", OperationKind: runtimecontract.StepKindMCPToolCall,
			Name: "lsp_definition", EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay), Enabled: true,
		}},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	projection := runtimeSemanticManifestProjection{OwnerCatalogs: runtimeSemanticOwnerCatalogsForProvider([]runtimecontract.RuntimeOperationAdmissibility{catalog})}
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	providerContext := string(raw)
	if !strings.Contains(providerContext, readyID) {
		t.Fatalf("provider catalog lost ready recipe identity: %s", providerContext)
	}
	for _, forbidden := range []string{auxiliaryID, requiresInputID, "private-empty-input-binding", "private-status-input-binding", "private-definition-input-prerequisite", "MCPCapabilities", "ValidationSurfaces", "EffectScope", "IdempotencyClass"} {
		if strings.Contains(providerContext, forbidden) {
			t.Fatalf("provider catalog leaked owner-private/non-ready fact %q: %s", forbidden, providerContext)
		}
	}
}

func TestRuntimeSemanticProviderProjectsDefinitionLineageAndExactAcquisitionSelector(t *testing.T) {
	const prospectiveTarget = "validation-readiness.spec"
	request := runtimeSemanticProviderTestRequest(t)
	baseline, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	catalog := request.SemanticManifest.OwnerCatalogs[0].Normalized()
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind:                  runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		SelectorRefs:          []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	prospectiveConstraint := runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
		ValidationSurfaceID: "validation-surface-dependency", ValidationRecipeID: "validation-surface-dependency",
		InputSchemaHash: "shell-schema-dependency", CatalogObservedRevision: "shell-policy-dependency-v1",
		AllOf: []runtimecontract.RuntimeProspectiveTargetClause{{ID: "required-input", AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{
			Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathSuffix, Value: ".spec",
		}}}},
	}
	convergence := runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef:       "workspace-relative-new-file-target",
		ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
		TargetSelectorRef:                  "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	request.SemanticManifest.OwnerCatalogs[0] = catalog
	request.SemanticManifest.ProofObligations = []runtimecontract.RuntimeSemanticProofObligation{{
		CoverageWorkItemID: "cwi-001", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		DeclarationTargetSelectorRef: "file/workspace_deliverable", DeclarationTargetSelectorOwnerRef: "software-dev", DeclarationTargetSelectorSubjectRef: "validation_preparation",
		DeclarationTargetSelectorCandidateRefs:          []string{prospectiveTarget},
		DeclarationProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path"},
		RequireTargetGrounding:                          true,
		RequiredOwnerFacts:                              []runtimecontract.RuntimeOwnerFactRequirement{requirement}, OwnerFactConvergences: []runtimecontract.RuntimeOwnerFactConvergence{convergence},
		OwnerFactAcquisition: &runtimecontract.RuntimeSemanticOwnerFactAcquisition{
			Requirement:                  requirement,
			DeclarationTargetSelectorRef: "file/workspace_deliverable", DeclarationTargetSelectorOwnerRef: "software-dev", DeclarationTargetSelectorSubjectRef: "validation_preparation",
			AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
			AcquisitionTargetPolicy:      runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			AcquisitionTargetSelectorRef: "workspace-relative-new-file-target", AcquisitionConstraintRefs: []string{"workspace-relative-path"},
			ProspectiveSatisfactionContractRef:      "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
			ProspectiveTargetSatisfactionConstraint: &prospectiveConstraint,
			OwnerCatalogSnapshotID:                  catalog.SnapshotID, OwnerCatalogObservedRevision: catalog.ObservedRevision,
			ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
			MaxAcquisitionAttempts: 1,
		},
	}}
	dependencyCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-prospective-dependency", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "validation-surface-dependency", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:     runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "workspace input required",
			InputSchemaHash: "shell-schema-dependency", PrerequisiteRef: "workspace/validation-input",
			ProspectiveSatisfactionContractRef:      "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
			ProspectiveTargetSatisfactionConstraint: &prospectiveConstraint,
			CheckRefs:                               []string{"check/validation-surface-dependency"},
			ProofApplicability:                      runtimecontract.RuntimeValidationProofDirect,
			EffectScope:                             runtimecontract.RuntimeEffectEphemeralSession,
			IdempotencyClass:                        runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
		ObservedRevision: "shell-policy-dependency-v1",
	}
	dependencyCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(dependencyCatalog)
	request.SemanticManifest.OwnerFactDependencyCatalogs = []runtimecontract.RuntimeOperationAdmissibility{dependencyCatalog}
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := providerRequest.PromptContext.Sections[3].Content
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(authority), &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Manifest.ProofObligations) != 1 || projection.Manifest.ProofObligations[0].OwnerFactAcquisition == nil {
		t.Fatalf("provider projection lost current owner-fact acquisition: %+v", projection.Manifest.ProofObligations)
	}
	obligation := projection.Manifest.ProofObligations[0]
	acquisition := obligation.OwnerFactAcquisition
	if obligation.EvidencePurpose != runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition ||
		!slices.Equal(obligation.TerminalRequiredEvidenceKinds, []string{runtimecontract.EvidenceKindValidation}) ||
		!slices.Equal(obligation.CurrentRequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange}) ||
		!acquisition.DefinitionPrerequisiteDeclared || acquisition.TerminalEvidenceCredit ||
		acquisition.AcquisitionOwnerModuleID != runtimecontract.RuntimeModuleFile ||
		acquisition.AcquisitionOperationKind != runtimecontract.StepKindFileWrite {
		t.Fatalf("provider projection conflated current acquisition with terminal validation proof: obligation=%+v acquisition=%+v", obligation, acquisition)
	}
	constraintProjectionJSON, err := json.Marshal(acquisition.ProspectiveTargetConstraint)
	if err != nil {
		t.Fatal(err)
	}
	if acquisition.ProspectiveTargetConstraint == nil ||
		strings.Contains(string(constraintProjectionJSON), `"validationRecipeId"`) ||
		strings.Contains(string(constraintProjectionJSON), `"validationSurfaceId"`) ||
		strings.Contains(string(constraintProjectionJSON), prospectiveConstraint.ValidationRecipeID) ||
		strings.Contains(authority, prospectiveConstraint.ValidationRecipeID) {
		t.Fatalf("provider-safe prospective target constraint leaked its dependency recipe identity: %s", constraintProjectionJSON)
	}
	if !strings.Contains(authority, `"declarationTargetSelectorRef":"file/workspace_deliverable"`) ||
		!strings.Contains(authority, `"acquisitionTargetSelectorRef":"workspace-relative-new-file-target"`) ||
		!strings.Contains(authority, `"prospectiveSatisfactionContractRef":"shell/selectable-validation-surface/prospective-workspace-manifest/v1"`) ||
		!strings.Contains(authority, `"declarationTargetSelectorCandidateRefs":["validation-readiness.spec"]`) ||
		!strings.Contains(authority, `"declarationProspectiveTargetAllOfConstraintRefs":["workspace-relative-path"]`) ||
		!strings.Contains(authority, `"prospectiveTargetConstraint"`) ||
		!strings.Contains(authority, `"kind":"target_ref_path_suffix","value":".spec"`) ||
		strings.Contains(authority, `"declarationProspectiveTargetRef"`) || strings.Contains(authority, `"prospectiveTargetRefs":["validation-readiness.spec"]`) {
		t.Fatalf("provider projection conflated Definition lineage and operation selector: %s", authority)
	}
	if strings.Contains(authority, dependencyCatalog.PrerequisiteID) || strings.Contains(providerRequest.Schema, dependencyCatalog.PrerequisiteID) {
		t.Fatalf("provider projection leaked non-selectable dependency catalog: authority=%s schema=%s", authority, providerRequest.Schema)
	}
	if strings.Contains(providerRequest.Schema, prospectiveConstraint.ValidationRecipeID) {
		t.Fatalf("provider schema promoted a dependency recipe into selectable authority: %s", providerRequest.Schema)
	}
	for _, required := range []string{"exact current acquisition proof", "complete mandatory set", "ownerModuleId=acquisitionOwnerModuleId", "bounded target.prospectiveTargetRefs", "every allOf target clause", "not Definition authority", "only an SM-2 CoverageEvidenceBinding", `requiredEvidenceKinds=["state_change"]`, "do not return gapKind=undeclared_prerequisite solely", "does not grant terminal validation evidence or coverage credit"} {
		if !strings.Contains(projection.OutputInstruction, required) {
			t.Fatalf("provider acquisition instruction lost %q: %s", required, projection.OutputInstruction)
		}
	}
	prospectiveTargetSchema := runtimeSemanticProviderProspectiveTargetSchemaForTest(t, providerRequest.Schema)
	prospectiveTargetSchemaJSON, err := json.Marshal(prospectiveTargetSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(providerRequest.Schema, `"const":"workspace-relative-new-file-target"`) ||
		prospectiveTargetSchema["items"].(map[string]any)["const"] != prospectiveTarget ||
		!strings.Contains(string(prospectiveTargetSchemaJSON), `"pattern"`) || !strings.Contains(string(prospectiveTargetSchemaJSON), ".spec") ||
		strings.Contains(providerRequest.Schema, `"const":"file/workspace_deliverable"`) {
		t.Fatalf("provider schema did not constrain the exact operation selector: %s", providerRequest.Schema)
	}
	for index := 0; index < 3; index++ {
		if baseline.PromptContext.Sections[index].Content != providerRequest.PromptContext.Sections[index].Content {
			t.Fatalf("owner-fact acquisition projection changed stable section %d", index)
		}
	}
	baselinePlan := baseline.ContextBudget.CachePlan
	acquisitionPlan := providerRequest.ContextBudget.CachePlan
	if baselinePlan.StablePrefixHash != acquisitionPlan.StablePrefixHash ||
		baselinePlan.RenderedStablePrefixHash != acquisitionPlan.RenderedStablePrefixHash ||
		baselinePlan.WorkspaceDefinitionHash != acquisitionPlan.WorkspaceDefinitionHash ||
		baselinePlan.CapabilityContractHash != acquisitionPlan.CapabilityContractHash {
		t.Fatalf("owner-fact acquisition changed C0-C2 cache identity: baseline=%+v acquisition=%+v", baselinePlan, acquisitionPlan)
	}

	openSelectorRequest := request
	openSelectorRequest.SemanticManifest = request.SemanticManifest.Clone()
	openSelectorRequest.SemanticManifest.ProofObligations[0].DeclarationTargetSelectorCandidateRefs = nil
	openSelectorRequest.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(openSelectorRequest.SemanticManifest)
	openSelectorProviderRequest, err := runtimeProviderRequest(openSelectorRequest)
	if err != nil {
		t.Fatal(err)
	}
	invalidAuthorityRequest := openSelectorRequest
	invalidAuthorityRequest.SemanticManifest = openSelectorRequest.SemanticManifest.Clone()
	invalidConstraint := invalidAuthorityRequest.SemanticManifest.ProofObligations[0].OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint.Clone()
	invalidConstraint.AllOf = nil
	invalidAuthorityRequest.SemanticManifest.ProofObligations[0].OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint = &invalidConstraint
	invalidAuthorityRequest.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(invalidAuthorityRequest.SemanticManifest)
	if _, err := runtimeProviderRequest(invalidAuthorityRequest); err == nil {
		t.Fatal("semantic provider accepted an acquisition with an empty owner-authored all-of constraint")
	}
	openTargetSchema := runtimeSemanticProviderProspectiveTargetSchemaForTest(t, openSelectorProviderRequest.Schema)
	openTargetSchemaJSON, err := json.Marshal(openTargetSchema)
	if err != nil {
		t.Fatal(err)
	}
	openTargetItems := openTargetSchema["items"].(map[string]any)
	if _, hasConst := openTargetItems["const"]; hasConst || openTargetItems["enum"] != nil ||
		!strings.Contains(string(openTargetSchemaJSON), `"pattern"`) || !strings.Contains(string(openTargetSchemaJSON), ".spec") {
		t.Fatalf("open selector schema did not retain only the owner-authored all-of predicate: %s", openTargetSchemaJSON)
	}
	validPayload := runtimeSemanticProviderAcquisitionPayloadForTest(t, openSelectorRequest.SemanticManifest, prospectiveTarget)
	if _, err := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(openSelectorRequest.SemanticManifest, validPayload); err != nil {
		t.Fatalf("owner-compatible prospective target was rejected before feasibility: %v", err)
	}
	invalidPayload := runtimeSemanticProviderAcquisitionPayloadForTest(t, openSelectorRequest.SemanticManifest, "workspace-manifest.data")
	_, err = runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(openSelectorRequest.SemanticManifest, invalidPayload)
	var diagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].target.prospectiveTargetRefs" {
		t.Fatalf("owner-incompatible prospective target did not fail as a typed pre-probe field violation: diagnostic=%+v err=%v", diagnostic, err)
	}
	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "semantic", OriginalCode: "prospective_target_constraint",
		OriginalMessage: "private evaluator detail must remain outside the correction prompt", Signature: "semantic/prospective-target-constraint",
		FieldViolations: []runtimecontract.RuntimeFieldViolation{{
			Path: diagnostic.Path, Actual: diagnostic.Actual,
			Allowed: append([]string(nil), diagnostic.Allowed...), Required: append([]string(nil), diagnostic.Required...),
		}},
		SemanticViolations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), diagnostic.SemanticViolations...),
	}
	observation := runtimeSemanticCorrectionObservationForProviderTest(rejection)
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepProofBatch, ManifestHash: openSelectorRequest.SemanticManifest.Hash,
		DefinitionSeedHash: openSelectorRequest.SemanticManifest.DefinitionSeedHash, StablePrefixHash: openSelectorRequest.StablePrefixHash,
		SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
		Violations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), rejection.SemanticViolations...),
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	retryRequest := openSelectorRequest
	retryRequest.CandidateAttempt = 1
	retryRequest.SemanticCorrectionObservation = observation
	retryRequest.SemanticCorrectionDirective = &directive
	retryProviderRequest, err := runtimeProviderRequest(retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= 3; index++ {
		if openSelectorProviderRequest.PromptContext.Sections[index].Content != retryProviderRequest.PromptContext.Sections[index].Content {
			t.Fatalf("typed prospective-target correction drifted C%d authority", index)
		}
	}
	retryTail := retryProviderRequest.PromptContext.Sections[4].Content
	if !strings.Contains(retryTail, diagnostic.Path) || strings.Contains(retryTail, `"prospectiveTargetConstraint"`) || strings.Contains(retryTail, rejection.OriginalMessage) {
		t.Fatalf("C4 did not remain a typed reference to the single C3 prospective-target authority: %s", retryTail)
	}
}

func TestRuntimeSemanticProviderProjectsTypedValidationFailureRepairOnlyInC3(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	definition := request.SemanticContext.Definition
	definition.CapabilityContract = []string{
		runtimecontract.StepKindFileWrite,
		runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	items := append([]runtimecontract.RuntimeCoverageItem(nil), request.SemanticManifest.CoverageItems...)
	acceptanceSlices := []runtimecontract.RuntimeAcceptanceSlice{{
		ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, request.SemanticContext.Activation, items, acceptanceSlices)
	if err != nil {
		t.Fatal(err)
	}

	convergence := runtimecontract.RuntimeValidationFailureRepairConvergence{
		ID:                                           "dod-validation-repair",
		TriggerOwnerModuleID:                         runtimecontract.RuntimeModuleShell,
		TriggerOperationKind:                         runtimecontract.RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClasses:                     []string{runtimecontract.ValidationDiagnosticBuildFailed},
		RepairEvidenceKind:                           runtimecontract.EvidenceKindStateChange,
		RepairOwnerModuleID:                          runtimecontract.RuntimeModuleFile,
		RepairOperationKinds:                         []string{runtimecontract.StepKindFileWrite},
		RepairTargetPolicy:                           runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		RepairTargetRole:                             "deliverable_implementation",
		RepairDeclarationTargetSelectorRef:           "file/workspace_deliverable",
		RepairDeclarationTargetSelectorOwnerRef:      "software-dev",
		RepairDeclarationTargetSelectorSubjectRef:    "dodItem_repair",
		RepairDeclarationTargetSelectorCandidateRefs: []string{"docs/lsp_tools.md", "lsp_tools.go"},
		RepairOperationTargetSelectorRef:             "workspace-relative-new-file-target",
		RequireFreshValidation:                       true,
		MaxAttempts:                                  2,
	}
	focuses := runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{{
		ID: "focus-repair", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-001",
		AssessmentText:        "the artifact works",
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		TargetRole:            "deliverable_implementation", TargetSelectorRef: "file/workspace_deliverable",
		TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "dodItem",
		TargetSelectorCandidateRefs: []string{"docs/lsp_tools.md", "lsp_tools.go"}, RequireTargetGrounding: true,
		AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession},
		RequireAssessment: true,
	}})
	focuses[0].ValidationFailureRepairConvergence = &convergence
	focuses = runtimecontract.NormalizeRuntimeDefinitionFocuses(focuses)

	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "file-repair-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		SelectorRefs:   []string{"workspace-relative-new-file-target"},
		ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
			SelectorRefs:          []string{"workspace-relative-new-file-target"},
			ConstraintRefs:        []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		}},
		ObservedRevision: "file-repair-revision",
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	shellCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-validation-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{
			ID: "recipe/go-test", Command: "go", Args: []string{"test", "./..."},
		}},
		ObservedRevision: "shell-validation-revision",
	}
	shellCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	catalogs := []runtimecontract.RuntimeOperationAdmissibility{fileCatalog, shellCatalog}
	baselineManifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		"ws-1", "run-1", definition, seed, focuses, request.SemanticContext.Activation.ExecutionContract,
		catalogs, []common.CoverageWorkItemID{"cwi-001"},
	)
	if err != nil {
		t.Fatal(err)
	}
	repairManifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair(
		"ws-1", "run-1", definition, seed, focuses, request.SemanticContext.Activation.ExecutionContract,
		catalogs, []common.CoverageWorkItemID{"cwi-001"}, runtimecontract.RuntimeValidationFailureRepairGenerationInput{
			CoverageWorkItemID: "cwi-001", FailedFocusID: "focus-repair",
			FailureSignature:       "validation-failure/package-conflict/1",
			TriggerOwnerModuleID:   runtimecontract.RuntimeModuleShell,
			TriggerOperationKind:   runtimecontract.RuntimeBlockKindManagedValidationSession,
			TriggerDiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	request.SemanticContext.Definition = definition
	request.SemanticContext.DefinitionSeed = seed
	request.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(
		definition, request.SemanticContext.Activation.ExecutionContract,
	)
	baselineRequest := request
	baselineRequest.SemanticManifest = baselineManifest
	baseline, err := runtimeProviderRequest(baselineRequest)
	if err != nil {
		t.Fatal(err)
	}
	repairRequest := request
	repairRequest.SemanticManifest = repairManifest
	providerRequest, err := runtimeProviderRequest(repairRequest)
	if err != nil {
		t.Fatal(err)
	}

	authority := providerRequest.PromptContext.Sections[3].Content
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(authority), &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Manifest.ProofObligations) != 1 || projection.Manifest.ProofObligations[0].ValidationFailureRepair == nil {
		t.Fatalf("provider projection lost validation-failure repair: %+v", projection.Manifest.ProofObligations)
	}
	obligation := projection.Manifest.ProofObligations[0]
	repair := obligation.ValidationFailureRepair
	if obligation.EvidencePurpose != runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair ||
		obligation.OwnerFactAcquisition != nil ||
		!slices.Equal(obligation.TerminalRequiredEvidenceKinds, []string{runtimecontract.EvidenceKindValidation}) ||
		!slices.Equal(obligation.CurrentRequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange}) ||
		repair.FailedFocusID != "focus-repair" || repair.ConvergenceID != convergence.ID ||
		repair.FailureSignature != "validation-failure/package-conflict/1" ||
		repair.TriggerDiagnosticClass != runtimecontract.ValidationDiagnosticBuildFailed ||
		repair.RepairOwnerModuleID != runtimecontract.RuntimeModuleFile ||
		!slices.Equal(repair.RepairOperationKinds, []string{runtimecontract.StepKindFileWrite}) ||
		repair.RepairTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
		repair.RepairOperationTargetSelectorRef != "workspace-relative-new-file-target" ||
		!slices.Equal(repair.RepairConstraintRefs, []string{"new-target-requires-owner-selector", "workspace-relative-path"}) ||
		!repair.RequireFreshValidation || repair.MaxAttempts != 2 {
		t.Fatalf("provider projection weakened typed repair authority: obligation=%+v repair=%+v", obligation, repair)
	}
	for _, required := range []string{
		"validation-failure repair", "every proposed target must contribute", "unrelated artifact is invalid",
		"only an SM-2 private event may seal it", "grants no terminal validation evidence or coverage credit",
	} {
		if !strings.Contains(projection.OutputInstruction, required) {
			t.Fatalf("provider repair instruction lost %q: %s", required, projection.OutputInstruction)
		}
	}
	if strings.Contains(baseline.PromptContext.Sections[3].Content, "Specialized validation-failure repair rule") {
		t.Fatalf("ordinary semantic generation received irrelevant repair instructions: %s", baseline.PromptContext.Sections[3].Content)
	}
	for _, forbidden := range []string{"originalMessage", "rawDiagnostic", "receipt", "runtimeAction", "terminalState", "retryDecision"} {
		if strings.Contains(authority, forbidden) {
			t.Fatalf("provider repair projection leaked non-proposal authority %q: %s", forbidden, authority)
		}
	}
	for index := 0; index < 3; index++ {
		if baseline.PromptContext.Sections[index].Content != providerRequest.PromptContext.Sections[index].Content {
			t.Fatalf("validation-failure repair changed stable section C%d", index)
		}
	}
	baselinePlan := baseline.ContextBudget.CachePlan
	repairPlan := providerRequest.ContextBudget.CachePlan
	if baselinePlan.StablePrefixHash != repairPlan.StablePrefixHash ||
		baselinePlan.RenderedStablePrefixHash != repairPlan.RenderedStablePrefixHash ||
		baselinePlan.WorkspaceDefinitionHash != repairPlan.WorkspaceDefinitionHash ||
		baselinePlan.CapabilityContractHash != repairPlan.CapabilityContractHash {
		t.Fatalf("validation-failure repair changed C0-C2 cache identity: baseline=%+v repair=%+v", baselinePlan, repairPlan)
	}
}

func runtimeSemanticProviderProspectiveTargetSchemaForTest(t *testing.T, schema string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(schema), &decoded); err != nil {
		t.Fatal(err)
	}
	properties, _ := decoded["properties"].(map[string]any)
	proposals, _ := properties["proposals"].(map[string]any)
	items, _ := proposals["items"].(map[string]any)
	allOf, _ := items["allOf"].([]any)
	for _, rawBranch := range allOf {
		branch, _ := rawBranch.(map[string]any)
		then, _ := branch["then"].(map[string]any)
		thenProperties, _ := then["properties"].(map[string]any)
		target, _ := thenProperties["target"].(map[string]any)
		targetProperties, _ := target["properties"].(map[string]any)
		if prospective, ok := targetProperties["prospectiveTargetRefs"].(map[string]any); ok {
			return prospective
		}
	}
	t.Fatalf("semantic provider schema has no owner-fact prospective target constraint: %s", schema)
	return nil
}

func runtimeSemanticProviderAcquisitionPayloadForTest(t *testing.T, manifest runtimecontract.RuntimeSemanticPlanManifest, targetRef string) []byte {
	t.Helper()
	manifest = manifest.Normalized()
	if len(manifest.ProofObligations) != 1 || manifest.ProofObligations[0].OwnerFactAcquisition == nil {
		t.Fatalf("semantic provider acquisition payload requires one owner-fact obligation: %+v", manifest.ProofObligations)
	}
	acquisition := manifest.ProofObligations[0].OwnerFactAcquisition.Normalized()
	payload, err := json.Marshal(map[string]any{
		"documentKind":             runtimecontract.RuntimeProviderDocumentSemanticPlan,
		"schemaVersion":            runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		"manifestHash":             manifest.Hash,
		"definitionSeedHash":       manifest.DefinitionSeedHash,
		"planStep":                 runtimecontract.RuntimeSemanticPlanStepProofBatch,
		"batchId":                  "batch-1",
		"batchSequence":            0,
		"batchCoverageWorkItemIds": []common.CoverageWorkItemID{"cwi-001"},
		"proposals": []any{map[string]any{
			"coverageWorkItemId": "cwi-001",
			"ownerModuleId":      acquisition.AcquisitionOwnerModuleID,
			"operationKind":      acquisition.AcquisitionOperationKind,
			"target": map[string]any{
				"policy":                    acquisition.AcquisitionTargetPolicy,
				"ownerSelectorRefs":         []string{acquisition.AcquisitionTargetSelectorRef},
				"prospectiveConstraintRefs": acquisition.AcquisitionConstraintRefs,
				"prospectiveTargetRefs":     []string{targetRef},
			},
			"requiredEvidenceKinds": []string{runtimecontract.EvidenceKindStateChange},
			"declarationRef":        "semantic-plan/dod-001",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestRuntimeSemanticCoverageOrderProjectionOmitsUnrelatedDefinitionSlices(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	request.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	request.SemanticContext.BatchID = ""
	request.SemanticContext.BatchCoverageWorkItemIDs = nil
	for index := 2; index <= 24; index++ {
		id := common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", index))
		request.SemanticContext.DefinitionSeed.CoverageItems = append(request.SemanticContext.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
			ID: id, SliceID: fmt.Sprintf("slice-%d", index), SourceScope: "dodItem", SourceID: fmt.Sprintf("dod-%03d", index), TargetText: strings.Repeat("unrelated ", 64), Order: index,
		})
		request.SemanticContext.DefinitionSeed.Slices = append(request.SemanticContext.DefinitionSeed.Slices, runtimecontract.RuntimeAcceptanceSlice{
			ID: fmt.Sprintf("slice-%d", index), Title: strings.Repeat("unrelated ", 16), CoverageWorkItemIDs: []common.CoverageWorkItemID{id},
		})
	}
	request.SemanticContext.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(request.SemanticContext.DefinitionSeed)
	request.SemanticManifest.DefinitionSeedHash = request.SemanticContext.DefinitionSeed.Hash
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := providerRequest.PromptContext.Sections[3].Content
	slices := providerRequest.PromptContext.Sections[5].Content
	if strings.Contains(authority, "cwi-024") || strings.Contains(slices, "cwi-024") || strings.Contains(slices, "unrelated") {
		t.Fatalf("coverage-order dynamic context leaked unrelated Definition rows: authority=%s slices=%s", authority, slices)
	}
	if !strings.Contains(authority, `"coverageItems":[{"ID":"cwi-001"`) || !strings.Contains(slices, `"coverageWorkItemIds":["cwi-001"]`) {
		t.Fatalf("coverage-order projection lost eligible CWI: authority=%s slices=%s", authority, slices)
	}
	if providerRequest.ContextBudget == nil || providerRequest.ContextBudget.CachePlan == nil || providerRequest.ContextBudget.CachePlan.StablePrefixHash != request.StablePrefixHash {
		t.Fatalf("coverage-order projection changed stable prefix: %+v", providerRequest.ContextBudget)
	}
}

func TestRuntimeSemanticPlanProviderRejectsMixedDocumentFields(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	request.DefinitionContext = request.SemanticContext.Definition
	if _, err := runtimeProviderRequest(request); err == nil || !strings.Contains(err.Error(), "must not carry operations fields") {
		t.Fatalf("mixed semantic request error = %v", err)
	}
	operations := llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, SemanticManifest: request.SemanticManifest}
	if _, err := runtimeProviderRequest(operations); err == nil || !strings.Contains(err.Error(), "must not carry semantic plan fields") {
		t.Fatalf("mixed operations request error = %v", err)
	}
}

func runtimeSemanticProviderTestRequest(t testing.TB) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver a bounded artifact",
		DoDID: "dod-1", DoDDefinition: "the artifact works", CapabilityContract: []string{runtimecontract.StepKindFileWrite},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: target},
		StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{{Stage: "planning", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-planning", Revision: 1, ContentHash: "soul-planning-hash", SourceProvenance: "test", Instructions: "planning-soul-guidance"}}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
			StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	items := []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "the artifact works", Order: 1}}
	slices := []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(), CoverageItems: items,
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-001", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	ceiling := &runtimecontract.ProviderCeiling{
		Selection: runtimecontract.ProviderInferenceSelection{Target: target}, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 128 * 1024, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	context := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: "ws-1", RunID: "run-1", Definition: definition, DefinitionSeed: seed, Activation: activation, ProviderCeiling: ceiling,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	if err := context.Validate(manifest); err != nil {
		t.Fatal(err)
	}
	return llmmodels.RuntimeCandidateRequest{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity:  runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "semantic-request-1", Sequence: 1, PhysicalCallID: "semantic-call-1"},
		StablePrefixHash: runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		SemanticManifest: manifest, SemanticContext: context,
	}
}

func runtimePromptSectionIDs(prompt *llmmodels.PromptContext) []string {
	if prompt == nil {
		return nil
	}
	ret := make([]string, 0, len(prompt.Sections))
	for _, section := range prompt.Sections {
		ret = append(ret, section.ID)
	}
	return ret
}

func TestRuntimeProviderRequestRejectsUnsealedManifestBeforeProviderConfiguration(t *testing.T) {
	_, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations})
	if err == nil || !strings.Contains(err.Error(), "runtime candidate contract manifest") {
		t.Fatalf("provider request error = %v, want manifest preflight failure", err)
	}
}

func runtimeCandidateManifestForRequest(t *testing.T, decision runtimecontract.RuntimeDecision, context runtimecontract.RuntimeCandidateContext) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	decision = runtimeCandidateDecisionForSchemaTest(decision)
	ownerFacts := runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources)
	if decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		ownerFacts.AssessmentPurpose = runtimecontract.RuntimeAssessmentPurposeDefinitionProof
		ownerFacts.AssessmentPurposeManifestHash = "semantic-manifest/llm-provider-request-test"
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, ownerFacts)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// runtimeCandidateManifestForSchema is deliberately test-only. Production
// renderers receive the manifest carried by the sealed EventHub request and
// never compile one from a decision. Unit tests build the same canonical value
// explicitly so no legacy decision-to-schema fallback survives in production.
func runtimeCandidateManifestForSchema(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	decision = runtimeCandidateDecisionForSchemaTest(decision)
	ownerFacts := runtimecontract.RuntimeCandidateManifestOwnerFacts{TargetSelection: selection}
	if selection != nil {
		ownerFacts.OperationAdmissibilities = runtimeCandidateScopeOwnerFactsForTest()
	}
	if decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		ownerFacts.AssessmentPurpose = runtimecontract.RuntimeAssessmentPurposeDefinitionProof
		ownerFacts.AssessmentPurposeManifestHash = "semantic-manifest/llm-schema-test"
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, ownerFacts)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// runtimeCandidateScopeOwnerFactsForTest models the owner-published target
// policy that production receives through prerequisite acquisition. Shared
// descriptors no longer grant existing-scope authority to File or Shell.
func runtimeCandidateScopeOwnerFactsForTest() []runtimecontract.RuntimeOperationAdmissibility {
	file := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility:file-scope-test", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, ObservedRevision: "file-scope-policy-v1",
	}
	for _, variantID := range []string{"file_write", "text_replace", "unified_diff"} {
		file.OperationVariants = append(file.OperationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
			Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: variantID,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		})
	}
	file.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(file)
	shell := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility:shell-scope-test", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.StepKindShellCommand}, ObservedRevision: "shell-scope-policy-v1",
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindShellCommand,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		}},
	}
	shell.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shell)
	return []runtimecontract.RuntimeOperationAdmissibility{file, shell}
}

// runtimeCandidateDecisionForSchemaTest makes test-only schema fixtures obey
// the same C5 invariant as production authority: a state-changing or
// validation convergence state always carries sealed coverage and effect
// constraints. Production code never fills these fields at render time.
func runtimeCandidateDecisionForSchemaTest(decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeDecision {
	contract := decision.RequiredOutput.Normalized()
	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation:
		if len(contract.CoverageWorkItemIDs) == 0 {
			contract.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-schema"}
		}
		policy, ok := runtimecontract.RuntimeConvergenceEffectPolicyFor(decision.State)
		if ok {
			if len(contract.AcceptedEffectScopes) == 0 {
				contract.AcceptedEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), policy.AllowedOperationEffects...)
			}
			if len(contract.RequiredEffectScopes) == 0 {
				contract.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), policy.AllowedOperationEffects...)
			}
		}
	case runtimecontract.ConvergenceStateNeedAssessment:
		if len(contract.CoverageWorkItemIDs) == 0 {
			contract.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-schema"}
		}
		if len(decision.Focus.CoverageEvidenceBindings) != 0 {
			break
		}
		if decision.AuthoritySnapshot.DefinitionRevision == "" {
			decision.AuthoritySnapshot.DefinitionRevision = "definition-llm-schema-test"
		}
		if decision.Focus.ID == "" {
			decision.Focus.ID = "focus-llm-schema-test"
		}
		decision.Focus.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), contract.CoverageWorkItemIDs...)
		decision.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		decision.Focus.TargetSelectorRef = "selector/llm-schema-test"
		decision.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleShell
		decision.Focus.TargetSelectorSubjectRef = "validation/managed-session"
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(contract.CoverageWorkItemIDs))
		for _, coverageID := range contract.CoverageWorkItemIDs {
			binding := runtimecontract.CoverageEvidenceBinding{
				ID: "binding/llm-schema-test/" + string(coverageID), CoverageWorkItemID: coverageID,
				FocusID: decision.Focus.ID, DefinitionRevision: decision.AuthoritySnapshot.DefinitionRevision,
				Proofs: []runtimecontract.CoverageEvidenceProof{{
					EvidenceKind: runtimecontract.EvidenceKindValidation, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, ValidationRecipeID: "recipe/llm-schema-test",
				}},
				TargetSelectorRef: decision.Focus.TargetSelectorRef, TargetSelectorOwnerRef: decision.Focus.TargetSelectorOwnerRef,
				TargetSelectorSubjectRef: decision.Focus.TargetSelectorSubjectRef, DeclarationRef: "definition/llm-schema-test/" + string(coverageID),
			}
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			bindings = append(bindings, binding)
		}
		decision.Focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	}
	decision.RequiredOutput = contract
	return decision
}

func runtimeCandidateSchemaForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) string {
	t.Helper()
	return runtimeCandidateSchemaForManifest(runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateWireForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest) runtimecontract.RuntimeCandidateWireContract {
	t.Helper()
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func runtimeCandidateOutputContractForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateOutputContractProjection {
	t.Helper()
	return runtimeCandidateOutputContractForManifest(manifest, runtimeCandidateWireForTest(t, manifest))
}

func runtimeCandidateOutputContractForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimeCandidateOutputContractProjection {
	t.Helper()
	return runtimeCandidateOutputContractForTest(t, runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateContextManifest(action runtimecontract.ConvergenceState, kinds ...string) runtimecontract.RuntimeCandidateContractManifest {
	operations := make([]runtimecontract.RuntimeCandidateOperationManifest, 0, len(kinds))
	for _, kind := range kinds {
		operations = append(operations, runtimecontract.RuntimeCandidateOperationManifest{Kind: kind})
	}
	policy, _ := runtimecontract.RuntimeConvergenceEffectPolicyFor(action)
	maxOperationCount := 0
	if action == runtimecontract.ConvergenceStateNeedStateChange || action == runtimecontract.ConvergenceStateNeedValidation {
		maxOperationCount = runtimecontract.RuntimeCandidateOperationMaxCount
	}
	return runtimecontract.RuntimeCandidateContractManifest{State: action, EffectPolicy: policy, MaxOperationCount: maxOperationCount, Operations: operations}
}

func sealRuntimeProviderTestPolicies(facts *runtimecontract.RuntimeFacts) {
	if facts == nil {
		return
	}
	if facts.Activation.ExecutionContract.StateChangePolicy == "" {
		facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
}

func TestRuntimeProviderRequestPreservesStablePrefixAndTypedContext(t *testing.T) {
	skillInstruction := "typed skill instructions"
	runtimeSkills, skillBudget, err := runtimecontract.SealRuntimeSkillContexts([]runtimecontract.RuntimeSkillContext{{
		ID: "skill-1", Instruction: skillInstruction,
		Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: "skill-1", Instruction: skillInstruction}),
		},
	}})
	if err != nil {
		t.Fatalf("seal runtime Skill: %v", err)
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRole: "deliverable",
			TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true,
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ContextWindowTokens:      128000,
			ProviderMaxOutputTokens:  16384,
			EffectiveMaxOutputTokens: 4096,
			MaxInputTokens:           92928,
			MaxBytes:                 96 * 1024,
			CapabilityRevision:       "capability-1",
			ConfigurationRevision:    "config-1",
		}, TargetSnapshots: []runtimecontract.TargetSnapshot{{
			TargetRef: "main.go", Content: "package main", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete,
		}}, CapabilityStates: []runtimecontract.RuntimeCapabilityStateSnapshot{{
			RequirementID: "capability_state:git", SnapshotID: "capability-state-git", AcquisitionKey: "capability-state/ws-1/run-1/capability_state:git/0", QueryReceiptRef: "capability-state-receipt/ws-1/run-1/capability_state:git/0", OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
			ObservedRevision: "repo-absent", ContentHash: "repo-state-hash", Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired,
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
		}}},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			DialogLanguage:    "zh-CN",
			DefaultSoul:       &runtimecontract.RuntimeSoulContext{ID: "soul-default", Revision: 1, ContentHash: "soul-default-hash", SourceProvenance: "test", Instructions: "default-only-soul" + strings.Repeat("d", 22*1024)},
			StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{
				{Stage: "execution", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-execution", Revision: 1, ContentHash: "soul-execution-hash", SourceProvenance: "test", Instructions: "execution-only-soul" + strings.Repeat("e", 22*1024), AllowedSkillRefs: []string{"*"}}},
				{Stage: "validation", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-validation", Revision: 1, ContentHash: "soul-validation-hash", SourceProvenance: "test", Instructions: "validation-only-soul" + strings.Repeat("v", 13*1024)}},
			},
			Skills: runtimeSkills, SkillContextBudget: skillBudget,
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "go-test-observable", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go", RequiredArgs: []string{"test", "-v"},
			}},
			Continuation: runtimecontract.RuntimeContinuationContext{},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				StateChangePolicy:       runtimecontract.RuntimeStateChangeMustProduce,
				CapabilityUsagePolicies: []runtimecontract.RuntimeCapabilityUsagePolicy{{ID: "usage-1", PreferredTools: []string{"lsp_diagnostics"}}},
				ProfileContracts: []runtimecontract.RuntimeProfileContract{{
					ID:                      "software-dev",
					AcceptanceDimensions:    []runtimecontract.RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true}},
					DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"}},
					EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
						{ID: "runtime_entrypoint", Scope: "acceptanceDimension", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
						{ID: "version_control_snapshot", Scope: "deliverable", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}},
					},
				}},
				TemplateStagePolicy: &runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: runtimecontract.RuntimeTemplateStagePolicySchemaCurrent, Stages: []runtimecontract.RuntimeTemplateStage{{
					ID: "execution", SoulBinding: "execution",
					RequiredOutputs: []runtimecontract.RuntimeTemplateStageRequiredOutput{{Kind: "state_change", EvidenceKinds: []string{"state_change"}}},
				}}},
			},
		},
	}
	sealRuntimeProviderTestPolicies(&facts)
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "workspace-relative-new-file-target", SelectorOwnerRef: runtimecontract.RuntimeModuleFile, SelectorSubjectRef: "deliverable",
		SelectorCandidateRefs: []string{"initial-deliverable.txt"},
		Disposition:           runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "file-admissibility-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			},
			SelectorRefs:   []string{"workspace-relative-new-file-target"},
			ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		}},
		SelectorRefs:     []string{"workspace-relative-new-file-target"},
		ConstraintRefs:   []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		ObservedRevision: "file-revision-1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileInventory}
	bindRuntimeCandidateTestSelection(&facts, selection)
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.StepKindFileWrite})
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindFileWrite},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{runtimecontract.StepKindFileWrite},
		ChangeImpact: &runtimecontract.RuntimeDefinitionChangeImpact{
			Version: runtimecontract.RuntimeDefinitionChangeImpactVersionCurrent, Changed: true, Policy: "cumulative", RequiredPreExecutionAssessment: true,
			GoalChange:           runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "goal-1"},
			DoDChange:            runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "dod-1"},
			StrategyRequirements: []string{"identify additive constraints"},
		},
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	rejectedPath := "main.go"
	rejectedShape := runtimecontract.RuntimeRejectedOperationShape{
		Operation:        runtimecontract.RuntimeOperation{ID: "rejected-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{rejectedPath}},
		FailureSignature: "rejected-shape-signature",
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{rejectedShape})
	if candidateContext.RequiredState != decision.State || !slices.Equal(candidateContext.RequiredEffectScopes, decision.RequiredOutput.RequiredEffectScopes) ||
		!slices.Equal(candidateContext.RequiredEvidenceKinds, decision.RequiredOutput.RequiredEvidenceKinds) {
		t.Fatalf("candidate context requirements = %+v, want sealed decision output %+v", candidateContext, decision.RequiredOutput)
	}
	manifest := runtimeCandidateManifestForRequest(t, decision, candidateContext)
	request, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: manifest, DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.ContextBudget == nil || request.ContextBudget.CachePlan == nil || request.ContextBudget.CachePlan.StablePrefixHash != prefix ||
		request.ContextBudget.CachePlan.RenderedStablePrefixHash == "" || len(request.PromptContext.Sections) != 6 {
		t.Fatalf("provider request cache plan = %+v prompt=%+v", request.ContextBudget, request.PromptContext)
	}
	if request.ContextBudget.ActualBytes <= 0 || request.ContextBudget.ActualBytes >= 96*1024 {
		t.Fatalf("provider request actual bytes = %d, want current-stage projection below 96 KiB", request.ContextBudget.ActualBytes)
	}
	if request.PromptContext.Sections[3].Content == "" || request.PromptContext.Sections[4].Content == "" || request.Schema == "" {
		t.Fatalf("provider request lost dynamic context: %+v", request)
	}
	contractSection := request.PromptContext.Sections[0].Content
	for _, value := range []string{"one JSON object", "outputContract is the sole structured-output contract", "manifestHash", "sole canonical decision authority", "requiredEvidenceKinds declares the current evidence obligation"} {
		if !strings.Contains(contractSection, value) {
			t.Fatalf("provider request stable contract lost %q: %s", value, contractSection)
		}
	}
	for _, forbidden := range []string{`"operations"`, `"focusId"`, `"externalDependency"`, "copy that exact value into its top-level targetRefs"} {
		if strings.Contains(contractSection, forbidden) {
			t.Fatalf("provider request stable contract retained manifest-specific rule %q: %s", forbidden, contractSection)
		}
	}
	dynamicSection := request.PromptContext.Sections[3].Content
	executionTail := request.PromptContext.Sections[4].Content
	largeRefs := request.PromptContext.Sections[5].Content
	if request.PromptContext.Sections[3].ID != "C3.runtime-authority" || request.PromptContext.Sections[4].ID != "C4.execution-tail" || request.PromptContext.Sections[5].ID != "C5.large-refs" {
		t.Fatalf("runtime prompt sections = %+v", request.PromptContext.Sections)
	}
	for _, value := range []string{
		`"outputContract"`, `"manifestHash"`, `"structuredOutputInstruction"`, `"$id":"workorch.runtime_candidate.state_change"`, `"forbiddenTopLevelFields"`,
		`"operationContracts":[{"kind":"file/write"`, `"name":"content"`, `"targetCardinality":"one"`,
		`"forbiddenParameters":["input","output","payload","path","targetRef","targetRefs"]`,
		`"targetSelectionInstruction"`, "empty workspace creation scope", `sealed prospective target \"initial-deliverable.txt\"`, `"skillContextBudget"`,
	} {
		if !strings.Contains(dynamicSection, value) {
			t.Fatalf("provider request dynamic output contract lost %q: %s", value, dynamicSection)
		}
	}
	for _, forbidden := range []string{
		`"RequiredArgs"`, `"CommandName"`, `"RecipeArgs"`, `"admissibleOperationKinds"`, `"capabilities"`,
		`"capabilityStates"`, `"repo-state-hash"`, `"operationAdmissibilities"`, `"prerequisiteObservations"`,
		`"evidencePolicyIds"`,
	} {
		if strings.Contains(dynamicSection, forbidden) {
			t.Fatalf("provider request leaked Shell command-construction policy %q: %s", forbidden, dynamicSection)
		}
	}
	if !strings.Contains(request.PromptContext.Sections[1].Content, "identify additive constraints") || !strings.Contains(request.PromptContext.Sections[1].Content, "cumulative") {
		t.Fatalf("provider request lost typed definition change impact: %s", request.PromptContext.Sections[1].Content)
	}
	if !strings.Contains(request.PromptContext.Sections[3].Content, "typed skill instructions") || !strings.Contains(request.PromptContext.Sections[3].Content, "zh-CN") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "execution-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "validation-only-soul") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "default-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "must_produce_state_change") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "lsp_diagnostics") {
		t.Fatalf("provider request lost typed activation context: %s", request.PromptContext.Sections[3].Content)
	}
	for _, forbidden := range []string{"signature-old", "target-old", "RecoveryAction", `"continuation"`} {
		if strings.Contains(executionTail, forbidden) {
			t.Fatalf("C4 leaked continuation/recovery detail %q: %s", forbidden, executionTail)
		}
	}
	capabilitySection := request.PromptContext.Sections[2].Content
	if strings.Contains(capabilitySection, "signature-old") || strings.Contains(capabilitySection, "target-old") {
		t.Fatalf("volatile continuation leaked into C2 stable prefix: %s", capabilitySection)
	}
	for _, value := range []string{"must_produce_state_change", "lsp_diagnostics", "runtime_entrypoint", "version_control_snapshot", "git/write", "commit", "workorch.template_stage_policy", "state_change"} {
		if !strings.Contains(capabilitySection, value) {
			t.Fatalf("provider request lost typed profile contract %q: %s", value, capabilitySection)
		}
	}
	for idx := 0; idx < 3; idx++ {
		if strings.Contains(request.PromptContext.Sections[idx].Content, "rejected-shape-signature") {
			t.Fatalf("rejected shape leaked into stable C%d section: %s", idx, request.PromptContext.Sections[idx].Content)
		}
	}
	if !strings.Contains(executionTail, "rejectedOperationShapes") || !strings.Contains(executionTail, "rejected-shape-signature") {
		t.Fatalf("rejected shape is not rendered in volatile execution tail: %s", executionTail)
	}
	if strings.Contains(dynamicSection, "rejected-shape-signature") || strings.Contains(dynamicSection, "signature-old") || strings.Contains(dynamicSection, `"continuation":{}`) || strings.Contains(largeRefs, "package main") {
		t.Fatalf("volatile or large content leaked into C3/C5: c3=%s c5=%s", dynamicSection, largeRefs)
	}
	for _, testCase := range []struct {
		name           string
		failure        runtimecontract.RuntimeCandidateFailure
		correctionPath string
		alternative    string
		want           []string
	}{
		{
			name: "schema diagnostic",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
				Reason: "raw decoder text must not be rendered: json: unknown field candidate",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					Path: "$.operations[0].parameters[name=summary]", Actual: "summary", Allowed: []string{"command", "args"}, Forbidden: []string{"summary"},
				},
			},
			correctionPath: "$.operations[0].parameters[name=summary]", alternative: "field_absent",
			want: []string{`"class":"schema"`, `"code":"candidate_schema_invalid"`, `"path":"$.operations[0].parameters[name=summary]"`, `"actual":"summary"`},
		},
		{
			name: "null required parameters container",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
				Reason: "raw decoder cause must not be rendered",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					Path: "$.operations[0].parameters", Actual: "null", Allowed: []string{"array"}, Required: []string{"parameters"},
				},
			},
			correctionPath: "$.operations[0].parameters", alternative: runtimecontract.RuntimeCandidateCorrectionFieldPresent,
			want: []string{`"class":"schema"`, `"actual":"null"`, `"rejectedValue":"null"`, "wrong JSON type", "field presence without schema conformance"},
		},
		{
			name: "semantic manifest diagnostic",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid",
				Reason: "raw semantic cause must not be rendered",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					OperationID: "write", Path: "$.operations[0].targetRefs", Actual: "other.go", Required: []string{"main.go"},
				},
			},
			correctionPath: "$.operations[0].targetRefs", alternative: "main.go",
			want: []string{`"class":"contract"`, `"code":"candidate_contract_invalid"`, `"operationId":"write"`, `"path":"$.operations[0].targetRefs"`},
		},
		{
			name: "grounded target rejection",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "candidate-target-outside-grounding",
				Reason: "raw owner explanation must not be rendered", TargetRefs: []string{"main.go"},
			},
			correctionPath: "$.operations[0].targetRefs", alternative: "main.go",
			want: []string{`"code":"candidate_target_outside_grounding"`, `"targetRefs":["main.go"]`},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if diagnostic := testCase.failure.Diagnostic; diagnostic != nil {
				if diagnostic.ManifestHash == "" {
					diagnostic.ManifestHash = manifest.Hash
				}
				if diagnostic.RejectedShapeHash == "" {
					diagnostic.RejectedShapeHash = testCase.failure.Signature
				}
				if testCase.failure.Class == runtimecontract.RuntimeCandidateFailureClassSchema && strings.HasPrefix(diagnostic.Path, "$.operations[0]") {
					diagnostic.WireRule = runtimeCandidateWireRuleIdentityForProviderTest(t, manifest, 0, "", runtimecontract.StepKindFileWrite)
				}
			}
			observation, correction := runtimeCandidateCorrectionPairForProviderTest(testCase.failure, manifest.Hash, testCase.correctionPath, testCase.alternative)
			retryRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
				DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
				Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, CandidateCorrectionObservation: observation, StablePrefixHash: prefix,
				CandidateCorrection: correction,
				CandidateManifest:   manifest, DefinitionContext: definition, CandidateContext: candidateContext,
			})
			if err != nil {
				t.Fatal(err)
			}
			retrySection := retryRequest.PromptContext.Sections[4].Content
			for _, value := range append([]string{`"candidateAttempt":1`, `"previousFailure"`, `"correctionInput"`, `"path":"` + testCase.correctionPath + `"`, `"alternativeValue":"` + testCase.alternative + `"`, "outputContract.structuredOutputInstruction", "sealed manifest hash"}, testCase.want...) {
				if !strings.Contains(retrySection, value) {
					t.Fatalf("provider retry lost correction %q: %s", value, retrySection)
				}
			}
			if strings.Contains(retrySection, testCase.failure.Reason) {
				t.Fatalf("provider retry leaked raw prior failure into C4: %s", retrySection)
			}
			if strings.Contains(retrySection, `"disposition"`) || strings.Contains(retrySection, `"terminalCode"`) {
				t.Fatalf("provider retry rendered Orchestrator disposition state: %s", retrySection)
			}
			var tail struct {
				Retry struct {
					PreviousFailure struct {
						Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic `json:"diagnostic"`
					} `json:"previousFailure"`
				} `json:"retry"`
			}
			if err := json.Unmarshal([]byte(retrySection), &tail); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(retrySection, `"reason"`) {
				t.Fatalf("provider retry serialized a previous-failure reason: %s", retrySection)
			}
			for _, forbidden := range []string{`"Disposition"`, `"Witness"`, `"TerminalCode"`} {
				if strings.Contains(retrySection, forbidden) {
					t.Fatalf("provider retry leaked persistence DTO field %s: %s", forbidden, retrySection)
				}
			}
			if testCase.failure.Diagnostic != nil {
				diagnostic := tail.Retry.PreviousFailure.Diagnostic
				want := observation.Diagnostic.Normalized()
				if diagnostic == nil || !reflect.DeepEqual(diagnostic.Normalized(), want) {
					t.Fatalf("provider retry changed typed correction delta: got=%+v want=%+v", diagnostic, want)
				}
			}
			for idx := 0; idx <= 3; idx++ {
				if retryRequest.PromptContext.Sections[idx].Content != request.PromptContext.Sections[idx].Content {
					t.Fatalf("same-transaction retry changed C%d", idx)
				}
			}
			if retryRequest.PromptContext.Sections[4].Content == request.PromptContext.Sections[4].Content || retryRequest.ContextBudget.CachePlan.StablePrefixHash != request.ContextBudget.CachePlan.StablePrefixHash || retryRequest.PromptContext.Sections[0].Content != contractSection {
				t.Fatalf("retry changed stable provider contract: initial=%+v retry=%+v", request.ContextBudget.CachePlan, retryRequest.ContextBudget.CachePlan)
			}
		})
	}
	freshContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	freshObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "admissibility-file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: "admissibility-v2",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v2"}},
	}
	freshObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(freshObservation)
	freshContext.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{freshObservation}
	freshRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, freshContext), DefinitionContext: definition, CandidateContext: freshContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 3; idx++ {
		if freshRequest.PromptContext.Sections[idx].Content != request.PromptContext.Sections[idx].Content {
			t.Fatalf("freshness metadata drifted stable C%d section", idx)
		}
	}
	if strings.Contains(freshRequest.PromptContext.Sections[3].Content, `"prerequisiteObservations"`) || strings.Contains(freshRequest.PromptContext.Sections[3].Content, "tree-v2") {
		t.Fatalf("raw freshness metadata leaked into manifest-scoped C3: %s", freshRequest.PromptContext.Sections[3].Content)
	}
	if freshRequest.PromptContext.Sections[3].Content != request.PromptContext.Sections[3].Content {
		t.Fatal("freshness-only aggregate change must not change the manifest-scoped provider C3 projection")
	}
	limitedContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	limitedContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 1,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	if _, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, limitedContext), DefinitionContext: definition, CandidateContext: limitedContext,
	}); err == nil || !strings.Contains(err.Error(), "provider ceiling") {
		t.Fatalf("runtimeProviderRequest ceiling err = %v, want typed provider ceiling failure", err)
	}
}

func TestRuntimeProviderRequestProjectsManifestScopedC3AtRunScale(t *testing.T) {
	const (
		criticalTargetBodyMarker = "critical-target-body-08"
		nonManifestTargetMarker  = "nonmanifest-target-body"
		nonManifestCatalogMarker = "nonmanifest-mcp-catalog"
		nonManifestMCPMarker     = "nonmanifest-mcp-activation"
		rawFreshnessMarker       = "raw-freshness-observation"
		rawSequenceMarker        = "raw-capability-sequence"
	)
	targetRefs := make([]string, 0, 17)
	targetSnapshots := make([]runtimecontract.TargetSnapshot, 0, 18)
	for idx := 0; idx < 17; idx++ {
		targetRef := fmt.Sprintf("internal/runtime/target-%02d.go", idx)
		bodyMarker := fmt.Sprintf("target-body-%02d", idx)
		if idx == 8 {
			bodyMarker = criticalTargetBodyMarker
		}
		content := "package runtime\n\n// " + bodyMarker + "\n" + strings.Repeat(fmt.Sprintf("var target%02d = %q\n", idx, bodyMarker), 130)
		targetRefs = append(targetRefs, targetRef)
		targetSnapshots = append(targetSnapshots, runtimecontract.TargetSnapshot{
			TargetRef: targetRef, Content: content, ContentHash: fmt.Sprintf("target-hash-%02d", idx),
			ContentState: runtimecontract.SnapshotContentStateComplete, Size: int64(len(content)), LineCount: 133,
		})
	}
	targetSnapshots = append(targetSnapshots, runtimecontract.TargetSnapshot{
		TargetRef: "unrelated/private.txt", Content: nonManifestTargetMarker + strings.Repeat("x", 8*1024),
		ContentHash: "unrelated-target-hash", ContentState: runtimecontract.SnapshotContentStateComplete,
	})

	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-run-scale", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "implementation", SelectorRef: "file/workspace-runtime-targets", SelectorOwnerRef: "profile/software-dev", SelectorSubjectRef: "runtime",
		SelectorCandidateRefs: append([]string(nil), targetRefs...), Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: append([]string(nil), targetRefs...), CandidateTargetRefs: append([]string(nil), targetRefs...), CandidateCount: len(targetRefs), ObservedRevision: "workspace-tree-run-scale",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)

	catalogs := runtimeCandidateScopeOwnerFactsForTest()
	gitCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility:git-run-scale", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		OperationKinds: []string{runtimecontract.StepKindGitWrite}, ObservedRevision: "git-catalog-run-scale",
	}
	for _, variantID := range []string{"init", "add", "commit"} {
		gitCatalog.OperationVariants = append(gitCatalog.OperationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
			Kind: runtimecontract.StepKindGitWrite, VariantID: variantID,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		})
	}
	gitCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(gitCatalog)
	mcpCapability := runtimecontract.RuntimeMCPCapabilityContext{
		ID: "mcp-unrelated:tool:inspect", ServerID: "mcp-unrelated", Kind: "tool", OperationKind: runtimecontract.StepKindMCPToolCall,
		Name: "inspect", Description: nonManifestCatalogMarker + strings.Repeat("m", 16*1024), Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectWorkspaceDurable), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencyAtMostOnce),
	}
	mcpCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility:mcp-run-scale", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ObservedRevision: "mcp-catalog-run-scale",
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindMCPToolCall,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		}},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{mcpCapability},
	}
	mcpCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(mcpCatalog)
	catalogs = append(catalogs, gitCatalog, mcpCatalog)

	owners := []string{runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeModuleGit, runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeModuleMCP}
	observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, 34)
	for idx := 0; idx < 34; idx++ {
		owner := owners[idx%len(owners)]
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: fmt.Sprintf("unrelated-prerequisite-%02d", idx), Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			OwnerModuleID: owner, ContentHash: fmt.Sprintf("unrelated-content-%02d", idx),
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: owner, SubjectRef: fmt.Sprintf("unrelated-catalog-%02d", idx),
				Revision: fmt.Sprintf("%s-%02d-%s", rawFreshnessMarker, idx, strings.Repeat("f", 768)),
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		observations = append(observations, observation)
	}

	sequence := []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: "patch-required-00", Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff", TargetRef: targetRefs[0]},
		{RequirementID: "patch-required-08", Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff", TargetRef: targetRefs[8], DependsOnRequirementIDs: []string{"patch-required-00"}},
	}
	capabilityState := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "capability-state:run-scale", AcquisitionKey: "capability-state/run-scale", AcquisitionBinding: "capability-state/run-scale",
		OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repository-run-scale", WorkspaceTreeRevision: "workspace-tree-run-scale", ContentHash: rawSequenceMarker,
		Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired, RequiredOperationSequence: sequence, QueryReceiptRef: "query-receipt/run-scale",
	}
	capabilityState.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(capabilityState)

	skillInstruction := "manifest-scoped-skill-marker"
	runtimeSkills, skillBudget, err := runtimecontract.SealRuntimeSkillContexts([]runtimecontract.RuntimeSkillContext{{
		ID: "skill-run-scale", Instruction: skillInstruction,
		Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: "skill-run-scale", Instruction: skillInstruction}),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-run-scale", RunID: "run-008", DefinitionRevision: "definition-run-scale", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-run-scale", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-run-scale"}, TargetRole: "implementation", RequireTargetGrounding: true},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-run-scale", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{
			runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindGitWrite, runtimecontract.StepKindShellCommand, runtimecontract.StepKindMCPToolCall,
		}},
		Resources: runtimecontract.RuntimeResourceFacts{
			Manifest:        &runtimecontract.WorkspaceManifest{Revision: "workspace-tree-run-scale", TargetRefs: append(append([]string(nil), targetRefs...), "unrelated/private.txt")},
			TargetSnapshots: targetSnapshots, TargetSelection: &selection, CapabilityStates: []runtimecontract.RuntimeCapabilityStateSnapshot{capabilityState},
			OperationAdmissibilities: catalogs, PrerequisiteObservations: observations,
			ProviderCeiling: &runtimecontract.ProviderCeiling{
				Selection:           runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
				ContextWindowTokens: 256000, ProviderMaxOutputTokens: 29000, EffectiveMaxOutputTokens: 8192, MaxInputTokens: 180000, MaxBytes: 512 * 1024,
				CapabilityRevision: "capability-run-scale", ConfigurationRevision: "configuration-run-scale",
			},
		},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, DialogLanguage: "zh-CN",
			StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{{Stage: "execution", Soul: runtimecontract.RuntimeSoulContext{
				ID: "soul-run-scale", Revision: 1, ContentHash: "soul-run-scale-hash", SourceProvenance: "test", Instructions: "manifest-scoped-soul-marker",
				AllowedSkillRefs: []string{"*"}, AllowedMCPServerRefs: []string{"*"}, AllowedCapabilityRefs: []string{"*"}, AllowedCapabilityCategories: []string{"*"}, AllowedToolKinds: []string{"*"},
			}}},
			Skills: runtimeSkills, SkillContextBudget: skillBudget,
			MCPServerIDs: []common.MCPServerID{"mcp-unrelated"}, MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
				ID: mcpCapability.ID, ServerID: mcpCapability.ServerID, Kind: mcpCapability.Kind, OperationKind: mcpCapability.OperationKind,
				Name: mcpCapability.Name, Description: nonManifestMCPMarker + strings.Repeat("a", 8*1024), Enabled: true,
				EffectScope: mcpCapability.EffectScope, IdempotencyClass: mcpCapability.IdempotencyClass,
			}},
			OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{{
				ID: "mcp-unrelated-policy", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: string(mcpCapability.ID),
				EffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
			}},
			Memories: []runtimecontract.RuntimeMemoryContext{{ID: "memory-run-scale", MemoryType: "fact", Content: "manifest-scoped-memory-marker"}},
			MemorySelection: &workorchmemorycontract.RecallSelectionEvidence{
				Policy: "test-selection", Status: "selected", CandidateMemoryIDs: []common.MemoryID{"memory-run-scale"},
				SelectedMemoryIDs: []common.MemoryID{"memory-run-scale"}, RetrievedMemoryIDs: []common.MemoryID{"memory-run-scale"},
				SelectionScores:     []workorchmemorycontract.RecallSelectionScore{{ID: "memory-run-scale", Score: 1, Reason: "selected"}},
				RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: "memory-run-scale", Summary: "manifest-scoped-memory-marker"}},
				ContextBudget:       workorchmemorycontract.RecallContextBudget{MaxItems: 1, MaxBytes: 4096, MaxEstimatedTokens: 1024, UsedItems: 1, UsedBytes: 32, EstimatedTokens: 8},
			},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "unrelated-shell-policy", AppliesTo: runtimecontract.StepKindShellCommand, Description: "nonmanifest-shell-policy"}},
		},
	}
	for _, observation := range observations {
		facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
			ID: observation.PrerequisiteID, Kind: observation.Kind, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal,
			OwnerModuleID: observation.OwnerModuleID, SubjectRef: "run-scale-owner-catalog", Reason: "run-scale regression freshness fact",
		})
	}
	sealRuntimeProviderTestPolicies(&facts)
	bindRuntimeCandidateTestSelection(&facts, selection)
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.RuntimeBlockKindPatchApply})
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:             []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff"}},
			AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, RequiredOperationSequence: sequence,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: "startup-input-run-scale", Revision: facts.DefinitionRevision, GoalID: "goal-run-scale", GoalDefinition: "modify the sealed runtime targets",
		DoDID: "dod-run-scale", DoDDefinition: "all sealed targets converge", CapabilityContract: []string{runtimecontract.RuntimeBlockKindPatchApply},
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	manifest := runtimeCandidateManifestForRequest(t, decision, context)
	initial, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: manifest, DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	dynamic := initial.PromptContext.Sections[3].Content
	for _, retained := range []string{criticalTargetBodyMarker, "manifest-scoped-soul-marker", skillInstruction, "manifest-scoped-memory-marker", `"requiredOperationReferences":["patch-required-00","patch-required-08"]`} {
		if !strings.Contains(dynamic, retained) {
			t.Fatalf("manifest-scoped C3 lost %q", retained)
		}
	}
	for _, forbidden := range []string{
		nonManifestTargetMarker, nonManifestCatalogMarker, nonManifestMCPMarker, rawFreshnessMarker, rawSequenceMarker,
		`"admissibleOperationKinds"`, `"capabilities"`, `"operationAdmissibilities"`, `"prerequisiteObservations"`, `"capabilityStates"`,
		`"mcpServerIds"`, `"mcpCapabilities"`, `"evidencePolicyIds"`, "unrelated/private.txt",
	} {
		if strings.Contains(dynamic, forbidden) {
			t.Fatalf("manifest-scoped C3 retained non-manifest/raw value %q", forbidden)
		}
	}
	var rendered struct {
		Context struct {
			Resources struct {
				TargetSnapshots []runtimecontract.TargetSnapshot `json:"targetSnapshots"`
			} `json:"resources"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(dynamic), &rendered); err != nil {
		t.Fatal(err)
	}
	if len(rendered.Context.Resources.TargetSnapshots) != 17 {
		t.Fatalf("manifest-scoped target snapshots = %d, want 17", len(rendered.Context.Resources.TargetSnapshots))
	}

	legacyPayload := map[string]any{}
	if err := json.Unmarshal([]byte(dynamic), &legacyPayload); err != nil {
		t.Fatal(err)
	}
	legacyContext := legacyPayload["context"].(map[string]any)
	legacyResources := legacyContext["resources"].(map[string]any)
	legacyContext["admissibleOperationKinds"] = context.AdmissibleOperationKinds
	legacyContext["capabilities"] = context.Capabilities
	legacyResources["manifest"] = context.Resources.Manifest
	legacyResources["targetSnapshots"] = context.Resources.TargetSnapshots
	legacyResources["capabilityStates"] = context.Resources.CapabilityStates
	legacyResources["operationAdmissibilities"] = context.Resources.OperationAdmissibilities
	legacyResources["prerequisiteObservations"] = context.Resources.PrerequisiteObservations
	legacyActivation := runtimeCandidateActivationForStage(context.Activation, "execution", manifest.State)
	legacyActivation.Continuation = nil
	legacyContext["activation"] = legacyActivation
	legacyDynamic, err := json.Marshal(legacyPayload)
	if err != nil {
		t.Fatal(err)
	}
	legacyRequest := initial
	legacyPrompt := *initial.PromptContext
	legacyPrompt.Sections = append([]llmmodels.PromptSection(nil), initial.PromptContext.Sections...)
	legacyPrompt.Sections[3].Content = string(legacyDynamic)
	legacyRequest.PromptContext = &legacyPrompt
	legacyWire, err := json.Marshal(legacyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if initial.ContextBudget.ActualBytes*5 >= len(legacyWire)*4 {
		t.Fatalf("manifest-scoped request did not shrink materially: projected=%d legacy=%d", initial.ContextBudget.ActualBytes, len(legacyWire))
	}

	retryFailure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "run-scale-retry",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path:         "$.operations[0].parameters",
			ManifestHash: manifest.Hash,
			WireRule:     runtimeCandidateWireRuleIdentityForProviderTest(t, manifest, 0, "patch-required-00", ""),
		},
	}
	retryObservation, retryCorrection := runtimeCandidateCorrectionPairForProviderTest(retryFailure, manifest.Hash, "$.operations[0].parameters", "field_present")
	retry, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1,
		CandidateCorrectionObservation: retryObservation, CandidateCorrection: retryCorrection, StablePrefixHash: prefix,
		CandidateManifest: manifest, DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx <= 3; idx++ {
		if retry.PromptContext.Sections[idx].Content != initial.PromptContext.Sections[idx].Content {
			t.Fatalf("same-contract attempt 1 changed C%d", idx)
		}
	}
}

func TestRuntimeCandidateAuthorityProjectionKeepsFileSelectorOnlyBoundedWorkspaceRefs(t *testing.T) {
	resources := runtimecontract.RuntimeResourceFacts{
		Manifest: &runtimecontract.WorkspaceManifest{Revision: "workspace-v1", TargetRefs: []string{"a.go", "b.go"}},
		TargetSnapshots: []runtimecontract.TargetSnapshot{
			{TargetRef: "a.go", Content: "package a", ContentHash: "hash-a", ContentState: runtimecontract.SnapshotContentStateComplete},
			{TargetRef: "b.go", Content: "package b", ContentHash: "hash-b", ContentState: runtimecontract.SnapshotContentStateComplete},
		},
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
		Kind: runtimecontract.RuntimeBlockKindPatchApply, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"file/workspace"}},
	}}}
	projection := runtimeCandidateAuthorityResources(resources, manifest)
	if projection.Manifest == nil || len(projection.Manifest.TargetRefs) != 2 || len(projection.TargetSnapshots) != 2 {
		t.Fatalf("File selector-only projection lost bounded workspace refs: %+v", projection)
	}

	nonFileManifest := manifest
	nonFileManifest.Operations[0].OwnerModuleID = runtimecontract.RuntimeModuleMCP
	nonFile := runtimeCandidateAuthorityResources(resources, nonFileManifest)
	if nonFile.Manifest != nil || len(nonFile.TargetSnapshots) != 0 {
		t.Fatalf("non-File selector-only projection inherited File inventory: %+v", nonFile)
	}
}

func TestRuntimeCandidateAuthorityActivationKeepsOnlyManifestCatalogMCP(t *testing.T) {
	selected := runtimecontract.RuntimeMCPCapabilityContext{
		ID: "mcp-selected:tool:inspect", ServerID: "mcp-selected", Kind: "tool", OperationKind: runtimecontract.StepKindMCPToolCall,
		Name: "inspect", Enabled: true, EffectScope: string(runtimecontract.RuntimeEffectWorkspaceDurable), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencyAtMostOnce),
	}
	unrelated := selected
	unrelated.ID = "mcp-unrelated:tool:inspect"
	unrelated.ServerID = "mcp-unrelated"
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-selected-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{selected}, ObservedRevision: "mcp-selected-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	activation := runtimecontract.RuntimeActivationContext{
		MCPServerIDs:    []common.MCPServerID{selected.ServerID, unrelated.ServerID},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{selected, unrelated},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{
			{ID: "selected-policy", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: string(selected.ID), EffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce},
			{ID: "unrelated-policy", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: string(unrelated.ID), EffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce},
		},
		Memories: []runtimecontract.RuntimeMemoryContext{{ID: "memory-selected", Content: "memory-marker"}},
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
			Kind: runtimecontract.StepKindMCPToolCall, OwnerModuleID: runtimecontract.RuntimeModuleMCP, AdmissibilitySnapshotRefs: []string{catalog.SnapshotID},
		}},
	}
	projection := runtimeCandidateAuthorityActivationForManifest(activation, runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog}}, manifest)
	if len(projection.MCPCapabilities) != 1 || projection.MCPCapabilities[0].ID != selected.ID ||
		len(projection.MCPServerIDs) != 1 || projection.MCPServerIDs[0] != selected.ServerID {
		t.Fatalf("manifest MCP activation projection = %+v", projection)
	}
	if len(projection.Memories) != 1 || projection.Memories[0].ID != "memory-selected" {
		t.Fatalf("manifest activation lost sealed memory: %+v", projection.Memories)
	}
}

func TestValidateRuntimeCandidateManifestSupportingFactsRequiresCanonicalExactBindings(t *testing.T) {
	capability := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "capability-state-1", AcquisitionKey: "acquisition-1", AcquisitionBinding: "binding-1",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityStateKind: "workspace_tree", SubjectRef: "workspace",
		ObservedRevision: "workspace-v1", ContentHash: "workspace-hash-v1", Disposition: runtimecontract.RuntimeCapabilityStateReady,
		QueryReceiptRef: "query-receipt-1",
	}
	capability.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(capability)
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-file-read", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileRead}, TargetRefs: []string{"main.go"}, ObservedRevision: "workspace-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeCandidateContractManifest{Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
		Kind: runtimecontract.StepKindFileRead, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		PrerequisiteSnapshotRefs: []string{capability.SnapshotID}, AdmissibilitySnapshotRefs: []string{catalog.SnapshotID},
	}}}
	resources := runtimecontract.RuntimeResourceFacts{
		CapabilityStates:         []runtimecontract.RuntimeCapabilityStateSnapshot{capability},
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, resources); err != nil {
		t.Fatalf("canonical supporting facts: %v", err)
	}

	driftedCapability := resources
	driftedCapability.CapabilityStates = append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), resources.CapabilityStates...)
	driftedCapability.CapabilityStates[0].ContentHash = "workspace-hash-drifted"
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, driftedCapability); err == nil || !strings.Contains(err.Error(), "drifted content") {
		t.Fatalf("drifted capability-state error = %v", err)
	}

	wrongOwner := manifest
	wrongOwner.Operations = append([]runtimecontract.RuntimeCandidateOperationManifest(nil), manifest.Operations...)
	wrongOwner.Operations[0].OwnerModuleID = runtimecontract.RuntimeModuleShell
	if err := validateRuntimeCandidateManifestSupportingFacts(wrongOwner, resources); err == nil || !strings.Contains(err.Error(), "does not support manifest owner/kind") {
		t.Fatalf("wrong admissibility owner error = %v", err)
	}
}

func TestRuntimeProviderContractRequiresCompleteCompactBoundedOutput(t *testing.T) {
	contract := runtimeCandidateContractCatalog()
	for _, required := range []string{"exactly one JSON object", "complete", "compact", "finished", "unknown fields", "unrequested operations", "preface", "suffix", "markdown fences", "Do not repeat"} {
		if !strings.Contains(contract, required) {
			t.Fatalf("runtime candidate contract lost %q: %s", required, contract)
		}
	}
	schema := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedStateChange)
	for _, required := range []string{`"maxItems"`, `"maxLength"`, `"additionalProperties":false`} {
		if !strings.Contains(schema, required) {
			t.Fatalf("runtime candidate schema lost bound %s: %s", required, schema)
		}
	}
}

func TestValidateRuntimeCandidateContextIntegrityRejectsProtectedContextLoss(t *testing.T) {
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", DoDID: "dod-1"}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:         runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
	}
	budget := &llmmodels.ProviderContextBudget{RedactionApplied: true, RedactedPaths: []string{"context.resources.targetSnapshots"}}
	prompt := &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{
		{ID: "C0.runtime", Content: "contract"}, {ID: "C1.goal", Content: "definition"}, {ID: "C2.capability", Content: "capability"},
		{ID: "C3.authority", Content: "authority"}, {ID: "C4.execution", Content: "tail"},
	}}
	err := validateRuntimeCandidateContextIntegrity(definition, context, 0, nil, budget, prompt)
	if err == nil || !strings.Contains(err.Error(), "P0.contract") || !strings.Contains(err.Error(), "P4.current_snapshots") || !strings.Contains(err.Error(), "protected_context_redacted") {
		t.Fatalf("integrity error = %v", err)
	}
	if len(budget.IntegrityWarnings) < 3 {
		t.Fatalf("integrity warnings = %v", budget.IntegrityWarnings)
	}
}

func TestRuntimeCandidateRetryExplainsOperationShapeReplayWithoutChangingStablePrefix(t *testing.T) {
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class:     runtimecontract.RuntimeCandidateFailureClassContract,
		Code:      "candidate_replays_rejected_shape",
		Signature: "candidate-replays-rejected-shape",
		Reason:    "runtime candidate replays active rejected operation shape(s)",
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-retry-exact-alternative"}
	observation, directive := runtimeCandidateCorrectionPairForProviderTest(*failure, manifest.Hash, "$.operations[0].targetRefs", "support.go")
	retry := runtimeCandidateRetryForManifest(1, observation, manifest, directive)
	if retry == nil {
		t.Fatal("retry projection is nil")
	}
	for _, want := range []string{
		"rejectedOperationShapes", "noProgressOperationShapes", "C4 execution-tail object", "exact correction alternative",
		`"support.go"`, "Changing only an operation ID",
	} {
		if !strings.Contains(retry.Instruction, want) {
			t.Fatalf("retry instruction lost %q: %s", want, retry.Instruction)
		}
	}
	nonReplayObservation, nonReplayDirective := runtimeCandidateCorrectionPairForProviderTest(runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_admission_invalid", Signature: "candidate-admission-invalid",
	}, manifest.Hash, "$.operations[0].targetRefs", "support.go")
	if retry := runtimeCandidateRetryForManifest(1, nonReplayObservation, manifest, nonReplayDirective); retry == nil || strings.Contains(retry.Instruction, "rejected or no-progress operation shape") {
		t.Fatalf("non-replay retry instruction = %+v, want no replay-specific correction", retry)
	}
}

func TestRuntimeCandidateRetryProjectsParameterEntryReencodeWithoutRawValues(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}, nil)
	encodings := []runtimecontract.RuntimeCandidateParameterEntryEncodingViolation{{
		OperationIndex: 0, ParameterIndex: 1, OperationKind: runtimecontract.RuntimeBlockKindPatchApply, ContainerParameterName: "content",
		MisplacedParameters: []runtimecontract.RuntimeCandidateMisplacedParameter{
			{Name: "baseHash", Type: runtimecontract.RuntimeOperationParameterTypeString},
			{Name: "coverageTargets", Type: runtimecontract.RuntimeOperationParameterTypeStringList},
			{Name: "summary", Type: runtimecontract.RuntimeOperationParameterTypeString},
		},
	}}
	correction := (&llmmodels.RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReencodeParameterEntries},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding, Path: "$.operations",
			RejectedValue:           runtimecontract.RuntimeCandidateParameterEncodingInlineSiblings,
			AlternativeValue:        runtimecontract.RuntimeCandidateParameterEncodingNamedEntries,
			ParameterEntryEncodings: encodings,
		},
	}).Normalized()
	observation := (&runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].parameters[1].baseHash", Actual: "baseHash",
			Allowed:                 []string{"name", "string", "integer", "boolean", "stringList"},
			WireRule:                runtimeCandidateWireRuleIdentityForProviderTest(t, manifest, 0, "", runtimecontract.RuntimeBlockKindPatchApply),
			ViolationKind:           runtimecontract.RuntimeCandidatePayloadViolationParameterEntryEncoding,
			ParameterEntryEncodings: encodings, ManifestHash: manifest.Hash,
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReencodeParameterEntries},
		},
	}).Clone()
	observation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(correction.ManifestHash, correction.Objectives, &correction.Witness)
	if err := correction.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	outputContract := runtimeCandidateOutputContractForTest(t, manifest)
	retry := runtimeCandidateRetryForManifest(1, &observation, manifest, &correction)
	if retry == nil || !reflect.DeepEqual(outputContract, runtimeCandidateOutputContractForTest(t, manifest)) {
		t.Fatalf("parameter-entry C4 changed C3 output contract or produced no retry: %+v", retry)
	}
	if strings.Contains(retry.Instruction, "Remove every field") {
		t.Fatalf("parameter-entry retry was misrendered as legal-field removal: %s", retry.Instruction)
	}
	for _, want := range []string{"every listed logical parameter", "exactly name plus the listed typed wireValueField", "Keep the container parameter"} {
		if !strings.Contains(retry.Instruction, want) {
			t.Fatalf("parameter-entry retry lost %q: %s", want, retry.Instruction)
		}
	}
	if len(retry.CorrectionInput.Witness.ParameterEntryEncodings) != 1 {
		t.Fatalf("parameter-entry correction projection=%+v", retry.CorrectionInput.Witness)
	}
	entries := retry.CorrectionInput.Witness.ParameterEntryEncodings[0].Entries
	wantEntries := []runtimeCandidateParameterEncodingEntryProjection{
		{Name: "baseHash", ContractType: runtimecontract.RuntimeOperationParameterTypeString, WireValueField: "string"},
		{Name: "coverageTargets", ContractType: runtimecontract.RuntimeOperationParameterTypeStringList, WireValueField: "stringList"},
		{Name: "summary", ContractType: runtimecontract.RuntimeOperationParameterTypeString, WireValueField: "string"},
	}
	if !reflect.DeepEqual(entries, wantEntries) || retry.CorrectionInput.Witness.ParameterEntryEncodings[0].SourceEntryPath != "$.operations[0].parameters[1]" {
		t.Fatalf("parameter-entry typed examples=%+v", retry.CorrectionInput.Witness.ParameterEntryEncodings)
	}
	encoded, err := json.Marshal(retry)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"secret complete file content", "secret-owner-hash", "secret change summary"} {
		if strings.Contains(string(encoded), raw) {
			t.Fatalf("C4 parameter-entry projection leaked raw provider value %q: %s", raw, encoded)
		}
	}
}

func TestRuntimeCandidateRetryProjectsTypedOperationSetConsolidation(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:           []string{"main.go"},
		},
	}, nil)
	violation := &runtimecontract.RuntimeCandidateOperationSetViolation{
		OwnerModuleID:    runtimecontract.RuntimeModuleFile,
		TargetRef:        "main.go",
		OperationKinds:   []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindPatchApply},
		OperationIndexes: []int{0, 1},
		Maximum:          1,
	}
	witness := runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension:             runtimecontract.RuntimeCandidateCorrectionWitnessOperationSetCardinality,
		Path:                  "$.operations",
		RejectedValue:         runtimecontract.RuntimeModuleFile + "|main.go",
		AlternativeValue:      "one_operation_per_owner_target",
		RejectedShapeHash:     "candidate-shape-overlap",
		OperationSetViolation: violation,
	}
	objectives := []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConsolidateOperationSet}
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  "candidate_owner_target_effect_overlap", Signature: "candidate-owner-target-effect-overlap",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: "op-second", Path: "$.operations", Actual: runtimecontract.RuntimeModuleFile + "|main.go",
			ManifestHash: manifest.Hash, RejectedShapeHash: witness.RejectedShapeHash,
			OperationSetViolation: violation, Objectives: objectives,
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(manifest.Hash, objectives, &witness),
	}
	correction := llmmodels.RuntimeCandidateCorrectionInput{ManifestHash: manifest.Hash, Objectives: objectives, Witness: witness}
	if err := correction.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	retry := runtimeCandidateRetryForManifest(1, &observation, manifest, &correction)
	if retry == nil || retry.CorrectionInput.Witness.OperationSetViolation == nil ||
		!reflect.DeepEqual(retry.CorrectionInput.Witness.OperationSetViolation.Normalized(), violation.Normalized()) {
		t.Fatalf("typed operation-set retry projection = %+v", retry)
	}
	encoded, err := json.Marshal(retry)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"dimension":"operation_set_cardinality"`,
		`"ownerModuleId":"blocks/file"`,
		`"targetRef":"main.go"`,
		`"operationKinds":["patch/apply","patch/apply"]`,
		`"operationIndexes":[0,1]`,
		"Consolidate every required change",
		"do not use dependsOn",
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("operation-set C4 projection lost %q: %s", want, encoded)
		}
	}
	for _, forbidden := range []string{`"operationIds"`, "op-first", "op-second"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("operation-set C4 projection leaked owner-derived operation identity %q: %s", forbidden, encoded)
		}
	}
}

func TestRuntimeCandidateRetryProjectsOwnerConstraintOnlyInC4WithoutStablePrefixDrift(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:             []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff"}},
			AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
	manifest := runtimeCandidateManifestForSchema(t, decision, nil)
	outputBefore := runtimeCandidateOutputContractForTest(t, manifest)
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: "startup-input-owner-constraint", Revision: "definition-1",
		GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
	}
	execution := runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce}
	prefixBefore := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, execution)

	objectives := []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformOwnerConstraint}
	witness := runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension:        runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint,
		OperationKind:    runtimecontract.RuntimeBlockKindPatchApply,
		Path:             "$.operations[0].parameters[name=diff]",
		ConstraintRef:    "patch/unified_diff/diff/single_file",
		RejectedValue:    string(runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax),
		AlternativeValue: string(runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile),
	}
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_owner_constraint_nonconformant",
		Signature: "candidate-owner-constraint-nonconformant",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: "patch-1", Path: witness.Path, Actual: witness.RejectedValue,
			ConstraintRef: witness.ConstraintRef, RequiredShape: witness.AlternativeValue,
			ManifestHash: manifest.Hash, Objectives: objectives,
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(manifest.Hash, objectives, &witness),
	}
	correction := llmmodels.RuntimeCandidateCorrectionInput{ManifestHash: manifest.Hash, Objectives: objectives, Witness: witness}
	if err := correction.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	retry := runtimeCandidateRetryForManifest(1, &observation, manifest, &correction)
	if retry == nil {
		t.Fatal("owner-constraint retry projection is nil")
	}
	outputAfter := runtimeCandidateOutputContractForTest(t, manifest)
	prefixAfter := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, execution)
	if !reflect.DeepEqual(outputBefore, outputAfter) || prefixBefore == "" || prefixBefore != prefixAfter {
		t.Fatalf("owner-constraint C4 projection changed C3/stable prefix: before=%q after=%q", prefixBefore, prefixAfter)
	}
	payload, err := json.Marshal(struct {
		C3 any                              `json:"c3"`
		C4 *runtimeCandidateRetryProjection `json:"c4"`
	}{C3: outputAfter, C4: retry})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, required := range []string{
		`"candidateConstraints"`, `"constraintRef":"patch/unified_diff/diff/single_file"`,
		`"dimension":"owner_candidate_constraint"`, `"rejectedValue":"invalid_syntax"`,
		`"alternativeValue":"unified_diff_single_file"`, "capability owner rejected one parameter's representation",
	} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("owner-constraint C3/C4 projection lost %q: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{
		"--- a/private-secret.go", "unified diff must include file header", "raw owner parser diagnostic", `"reason"`,
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("owner-constraint C4 leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestRuntimeCandidateExecutionTailOnlyRendersProviderSafeFacts(t *testing.T) {
	secretParameter := "raw-operation-payload-must-not-reach-c4"
	context := runtimecontract.RuntimeCandidateContext{
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_fact", Signature: "owner-signature",
			Message: "raw failure message must not reach c4", TargetRefs: []string{"main.go"},
			GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external", SubjectRef: "dep-1", Reason: "raw governance reason must not reach c4"},
		},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
			ID: "evidence-1", Kind: runtimecontract.EvidenceKindValidation, SubjectRef: "main.go", FailureCode: "validation_failed",
			FailureSignature: "evidence-signature", FailureMessage: "raw evidence failure must not reach c4", Summary: "raw evidence summary must not reach c4",
			EvidenceReason: "raw evidence reason must not reach c4", TargetRefs: []string{"main.go"},
		}}},
		Assessments: []runtimecontract.RuntimeAssessmentFact{{
			ID: "assessment-1", FocusID: "focus-1", RequirementID: "requirement-1", Summary: "raw assessment summary must not reach c4",
			RequirementText: "raw assessment requirement must not reach c4", Disposition: runtimecontract.AssessmentDispositionSatisfied,
		}},
		RejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{
			Operation:   runtimecontract.RuntimeOperation{ID: "op-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"main.go"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &secretParameter}}},
			FailureCode: "rejected_shape", FailureSignature: "shape-signature", FailureMessage: "raw rejected-shape message must not reach c4",
		}},
		Activation: runtimecontract.RuntimeActivationContext{Continuation: runtimecontract.RuntimeContinuationContext{}},
	}
	tail := buildRuntimeCandidateExecutionTailProjection(context, 0, nil, runtimecontract.RuntimeCandidateContractManifest{}, nil)
	encoded, err := json.Marshal(tail)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{
		"raw failure message", "raw governance reason", "raw evidence failure", "raw evidence summary", "raw evidence reason",
		"raw assessment summary", "raw assessment requirement", "raw rejected-shape message", secretParameter,
		"raw-recovery-action", "raw continuation reason", `"repairHints"`, `"continuation"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("C4 leaked non-provider-safe detail %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"code":"owner_fact"`, `"failureCode":"validation_failed"`, `"failureSignature":"shape-signature"`, `"kind":"file/write"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("C4 lost provider-safe fact %q: %s", required, text)
		}
	}
}

func TestRuntimeTargetSelectionInstructionIsLimitedToFileScopes(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange)
	for _, required := range []string{"durable File-owner operation", `sealed prospective target "initial-deliverable.txt"`, "Git, Shell, MCP, or another owner"} {
		if !strings.Contains(got, required) {
			t.Fatalf("creation instruction lost %q: %s", required, got)
		}
	}
	if got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedValidation); got != "" {
		t.Fatalf("validation must not receive creation instruction: %q", got)
	}
	selection.Disposition = runtimecontract.RuntimeTargetSelectionScope
	selection.TargetRefs = []string{"implementation.go", "support.go"}
	selection.CandidateTargetRefs = append([]string(nil), selection.TargetRefs...)
	selection.CandidateCount = len(selection.TargetRefs)
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got = runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange)
	for _, required := range []string{"bounded workspace feasibility scope, not a selected repair target", "Focus, FailureTarget, current failure, or RepairDirective", "may be a new file", "Do not use this scope to authorize Git, Shell, MCP"} {
		if !strings.Contains(got, required) {
			t.Fatalf("scope instruction lost %q: %s", required, got)
		}
	}
	selection.OwnerModuleID = runtimecontract.RuntimeModuleGit
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	if got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange); got != "" {
		t.Fatalf("non-File selection must not receive creation instruction: %q", got)
	}
}

func TestRuntimeCandidateAuthorityProjectionPreservesRepairDirectiveAuthority(t *testing.T) {
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "repair-1", FocusID: "focus-1", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		CurrentFailure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionRejected, Message: "validation failed",
			ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
				SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
				Class:         runtimecontract.ValidationDiagnosticBuildFailed, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetFailureSurface,
				TargetRefs: []string{"main.go"}, StderrPreview: "compile failed",
			},
		},
	}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:         runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		RequiredState: directive.RequiredState, RequiredEffectScopes: directive.RequiredEffectScopes,
		RequiredEvidenceKinds: directive.RequiredEvidenceKinds, RepairDirective: &directive,
		AdmissibleOperationKinds: []string{runtimecontract.StepKindFileWrite},
	}
	projection := runtimeCandidateAuthorityContextProjection(context, runtimeCandidateContextManifest(runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.StepKindFileWrite))
	if projection.RequiredState != runtimecontract.ConvergenceStateNeedStateChange || projection.RepairDirective == nil {
		t.Fatalf("projection lost repair authority: %+v", projection)
	}
	if len(projection.RequiredEffectScopes) != 1 || projection.RequiredEffectScopes[0] != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("projection required effects = %v", projection.RequiredEffectScopes)
	}
	if len(projection.RequiredEvidenceKinds) != 1 || projection.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("projection required evidence = %v", projection.RequiredEvidenceKinds)
	}
	if projection.RepairDirective == nil || projection.RepairDirective.FocusID != "focus-1" {
		t.Fatalf("projection repair directive = %+v", projection.RepairDirective)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ValidationDiagnostic", runtimecontract.ValidationDiagnosticBuildFailed, "compile failed", "main.go"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("C3 projection lost validation diagnostic %q: %s", want, encoded)
		}
	}
}

func TestRuntimeCandidateAuthorityProjectionPreservesManifestScopedTargetGrounding(t *testing.T) {
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRole: "implementation", TargetRefs: []string{"implementation.go"}, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true,
		},
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		Resources: runtimecontract.RuntimeResourceFacts{
			FailureTarget: &runtimecontract.FailureTarget{Signature: "validation-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
			}},
			TargetSnapshots: []runtimecontract.TargetSnapshot{{TargetRef: "implementation.go", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete}, {TargetRef: "support.go", ContentHash: "hash-2", ContentState: runtimecontract.SnapshotContentStateComplete}},
			TargetSelection: &runtimecontract.RuntimeTargetSelection{PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "implementation", SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope, TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "manifest-1"},
		},
	}
	context.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*context.Resources.TargetSelection)
	manifest := runtimeCandidateContextManifest(runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.RuntimeBlockKindPatchApply)
	manifest.TargetRefs = []string{"implementation.go", "support.go"}
	manifest.Operations[0].OwnerModuleID = runtimecontract.RuntimeModuleFile
	manifest.Operations[0].Target = runtimecontract.RuntimeOperationTargetAuthorization{
		Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"implementation.go", "support.go"}, OwnerSelectorRefs: []string{"file/workspace_deliverable"},
	}
	manifest.Operations[0].PrerequisiteSnapshotRefs = []string{context.Resources.TargetSelection.SnapshotID}
	projection := runtimeCandidateAuthorityContextProjection(context, manifest)
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"TargetRole":"implementation"`, `"TargetRefs":["implementation.go"]`, `"TargetDiscoverySources":["workspace_manifest"]`, `"RequireTargetGrounding":true`, `"failureTarget"`, `"signature":"validation-failure"`, `"targetSelection"`, `"scope"`, `"support.go"`} {
		if !strings.Contains(string(encoded), required) {
			t.Fatalf("volatile target-grounding context lost %s: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{`"TargetObservation"`, `"ownerModuleId":"blocks/mcp"`, `"observedRevision":"mcp-server-v1"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("manifest-scoped target projection leaked raw owner observation %s: %s", forbidden, encoded)
		}
	}
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod"}
	base := runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{ID: "profile"}}}
	before := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base)
	context.Focus.TargetRefs = []string{"different.go"}
	after := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base)
	if before != after {
		t.Fatalf("focus target grounding drifted stable C0-C2 prefix: %s != %s", before, after)
	}
}

func TestRuntimeCandidateSchemasAreActionSpecific(t *testing.T) {
	stateChange := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedStateChange)
	validation := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedValidation)
	if stateChange == validation {
		t.Fatal("state-change and validation candidate schemas must be distinct")
	}
	if !strings.Contains(stateChange, "workorch.runtime_candidate.state_change") || !strings.Contains(stateChange, "durable") {
		t.Fatalf("state-change schema = %s", stateChange)
	}
	if !strings.Contains(stateChange, "Every operation") || strings.Contains(stateChange, "At least one operation") {
		t.Fatalf("state-change schema does not render the canonical all-durable effect policy: %s", stateChange)
	}
	if !strings.Contains(validation, "workorch.runtime_candidate.validation") || !strings.Contains(validation, "non-durable") {
		t.Fatalf("validation schema = %s", validation)
	}
	catalog := runtimeCandidateContractCatalog()
	if strings.Contains(catalog, "produce_state_change or produce_validation") || strings.Contains(catalog, "produce only those missing evidence kinds") {
		t.Fatalf("candidate catalog retains conflicting action guidance: %s", catalog)
	}
	if !strings.Contains(catalog, "sole canonical decision authority") || !strings.Contains(catalog, "requiredEvidenceKinds declares the current evidence obligation") {
		t.Fatalf("candidate catalog lost repair authority rules: %s", catalog)
	}
	if !strings.Contains(catalog, "current runtime authority's outputContract") || strings.Contains(catalog, "context.outputContract") || strings.Contains(catalog, "workorch.runtime_candidate.state_change") ||
		strings.Contains(catalog, "dependsOn") || strings.Contains(catalog, "targetRefs") {
		t.Fatalf("stable catalog retained a second operation contract: %s", catalog)
	}
}

func TestRuntimeCandidateSchemaNeverExposesOwnerDerivedOperationAuthority(t *testing.T) {
	testCases := []struct {
		name     string
		decision runtimecontract.RuntimeDecision
	}{
		{
			name: "file",
			decision: runtimecontract.RuntimeDecision{
				State:          runtimecontract.ConvergenceStateNeedStateChange,
				RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
			},
		},
		{
			name: "git dependent sequence",
			decision: runtimecontract.RuntimeDecision{
				State: runtimecontract.ConvergenceStateNeedStateChange,
				RequiredOutput: runtimecontract.RequiredOutputContract{
					AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
					RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
						{RequirementID: "init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
						{RequirementID: "commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"init"}},
					},
				},
			},
		},
		{
			name: "shell",
			decision: runtimecontract.RuntimeDecision{
				State:          runtimecontract.ConvergenceStateNeedStateChange,
				RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindShellCommand}},
			},
		},
		{
			name: "mcp",
			decision: runtimecontract.RuntimeDecision{
				State:          runtimecontract.ConvergenceStateNeedValidation,
				RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall}},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			schema := runtimeCandidateSchemaForDecision(t, testCase.decision, nil)
			for _, property := range []string{"id", "dependsOn", "coverageWorkItemIds"} {
				if runtimeOperationSchemaHasProperty(t, schema, property) {
					t.Fatalf("candidate schema exposed owner-derived operation property %q: %s", property, schema)
				}
			}
		})
	}
}

func runtimeOperationSchemaHasProperty(t *testing.T, encoded, property string) bool {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(encoded), &root); err != nil {
		t.Fatal(err)
	}
	operations, ok := root["properties"].(map[string]any)["operations"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operations: %s", encoded)
	}
	items, ok := operations["items"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operation items: %s", encoded)
	}
	if properties, ok := items["properties"].(map[string]any); ok {
		if _, found := properties[property]; found {
			return true
		}
	}
	for _, rawBranch := range runtimeSchemaArray(items["oneOf"]) {
		branch, _ := rawBranch.(map[string]any)
		properties, _ := branch["properties"].(map[string]any)
		if _, found := properties[property]; found {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateOutputContractUsesActionSpecificForbiddenTopLevelFields(t *testing.T) {
	operation := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
	}, nil)
	if !containsString(operation.ForbiddenTopLevelFields, "dependsOn") || !containsString(operation.ForbiddenTopLevelFields, "parameters") {
		t.Fatalf("operation forbidden fields = %#v", operation.ForbiddenTopLevelFields)
	}
	assessment := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedAssessment}, nil)
	if containsString(assessment.ForbiddenTopLevelFields, "coverageWorkItemIds") {
		t.Fatalf("assessment forbids its required top-level field: %#v", assessment.ForbiddenTopLevelFields)
	}
	governance := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedGovernance}, nil)
	if !containsString(governance.ForbiddenTopLevelFields, "reason") {
		t.Fatalf("governance forbidden fields = %#v", governance.ForbiddenTopLevelFields)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateSchemaDerivesPatchTargetRequirementsFromOperationContract(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, TargetRefs: []string{"main.go", "support.go"},
		},
	}, nil)
	for _, value := range []string{`"const":"patch/apply"`, `"const":"patchType"`, `"targetRefs"`, `"minItems":1`, `"maxItems":1`, `"minLength":1`} {
		if !strings.Contains(schema, value) {
			t.Fatalf("descriptor-driven patch schema lost %q: %s", value, schema)
		}
	}
	if strings.Contains(runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedStateChange), `"const":"patch/apply"`) {
		t.Fatal("volatile operation constraints leaked into the stable generic contract schema")
	}
}

func TestRuntimeCandidateOperationCardinalityIsEquivalentWithJSONSchemaResponseOnOrOff(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			TargetRefs:    []string{"main.go", "support.go"},
		},
	}, nil)
	manifest.MaxOperationCount = 2
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	schema := runtimeCandidateSchemaForManifest(manifest)
	var document map[string]any
	if err := json.Unmarshal([]byte(schema), &document); err != nil {
		t.Fatal(err)
	}
	properties, _ := document["properties"].(map[string]any)
	operationsSchema, _ := properties["operations"].(map[string]any)
	if operationsSchema["maxItems"] != float64(manifest.MaxOperationCount) {
		t.Fatalf("schema maxItems = %#v, want manifest maxOperationCount %d", operationsSchema["maxItems"], manifest.MaxOperationCount)
	}

	operationManifest := manifest.Operations[0]
	bindingRefs := runtimeCandidateBindingAuthorizationRefs(operationManifest.BindingAuthorizations)
	operation := func(target, content string) map[string]any {
		value := map[string]any{
			"kind":       runtimecontract.RuntimeBlockKindPatchApply,
			"targetRefs": []string{target},
			"parameters": []map[string]any{
				{"name": "patchType", "string": "file_write"},
				{"name": "content", "string": content},
			},
		}
		if len(bindingRefs) != 0 {
			value["bindingRefs"] = []string{bindingRefs[0]}
		}
		return value
	}
	payload, err := json.Marshal(map[string]any{"operations": []map[string]any{
		operation("main.go", "package main"), operation("support.go", "package support"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := llmmodels.ProviderRequest{Schema: schema}
	for _, enabled := range []bool{false, true} {
		textConfig := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: enabled}, request)
		if enabled && (textConfig == nil || textConfig.Format == nil || !textConfig.Format.Strict) {
			t.Fatalf("JSONSchemaResponse=true did not dispatch the manifest schema: %+v", textConfig)
		}
		if !enabled && textConfig != nil {
			t.Fatalf("JSONSchemaResponse=false unexpectedly dispatched provider schema: %+v", textConfig)
		}
		candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
		if decodeErr != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 2 {
			t.Fatalf("JSONSchemaResponse=%t local boundary result candidate=%+v err=%v", enabled, candidate, decodeErr)
		}
	}
}

func TestRuntimeCandidateRequiredWireIsMinimalWithJSONSchemaResponseOnOrOff(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindFileWrite},
			TargetRefs:    []string{"go.mod"},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{
				RequirementID: "create-module", Kind: runtimecontract.StepKindFileWrite, TargetRef: "go.mod",
			}},
		},
	}, nil)
	wire := runtimeCandidateWireForTest(t, manifest)
	rule, found := runtimecontract.RuntimeCandidateWireRuleForRequiredOperation(wire, "create-module")
	if !found || !slices.Equal(rule.ProviderFields, []string{"requiredOperationRef", "parameters"}) ||
		!slices.Equal(rule.DerivedTargetRefs, []string{"go.mod"}) {
		t.Fatalf("required operation wire rule = %+v found=%t", rule, found)
	}
	output := runtimeCandidateOutputContractForManifest(manifest, wire)
	if output.WireContractHash != wire.Hash || len(output.OperationContracts) != 1 ||
		output.OperationContracts[0].RequireTargetRefs ||
		!slices.Equal(output.OperationContracts[0].AllowedOperationFields, []string{"parameters", "requiredOperationRef"}) ||
		len(output.OperationContracts[0].Parameters) != 1 || output.OperationContracts[0].Parameters[0].Name != "content" {
		t.Fatalf("minimal required-operation output contract = %+v", output)
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	for _, localField := range []string{"kind", "bindingRefs", "targetRefs"} {
		if runtimeOperationSchemaHasProperty(t, schema, localField) {
			t.Fatalf("required operation schema exposed locally-derived field %q: %s", localField, schema)
		}
	}
	payload := []byte(`{"operations":[{"requiredOperationRef":"create-module","parameters":[{"name":"content","string":"module example"}]}]}`)
	request := llmmodels.ProviderRequest{Schema: schema}
	for _, enabled := range []bool{false, true} {
		textConfig := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: enabled}, request)
		if enabled && (textConfig == nil || textConfig.Format == nil || !textConfig.Format.Strict) {
			t.Fatalf("JSONSchemaResponse=true did not dispatch the minimal required-operation schema: %+v", textConfig)
		}
		if !enabled && textConfig != nil {
			t.Fatalf("JSONSchemaResponse=false unexpectedly dispatched provider schema: %+v", textConfig)
		}
		candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
		if err != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
			t.Fatalf("JSONSchemaResponse=%t minimal wire candidate=%+v err=%v", enabled, candidate, err)
		}
		operation := candidate.StateChange.Operations[0].Operation
		if operation.Kind != runtimecontract.StepKindFileWrite || !slices.Equal(operation.TargetRefs, []string{"go.mod"}) ||
			!slices.Equal(operation.BindingRefs, rule.DerivedBindingRefs) {
			t.Fatalf("JSONSchemaResponse=%t locally compiled operation=%+v", enabled, operation)
		}
	}
}

func TestRuntimeCandidateAuxiliarySingletonWireIsMinimalWithJSONSchemaResponseOnOrOff(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		DefinitionRevision: "definition-auxiliary-wire",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-auxiliary-wire", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-auxiliary-wire"}, TargetRefs: []string{"main.go"},
		},
	}
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.RuntimeBlockKindPatchApply})
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		AuthoritySnapshot: runtimecontract.AuthoritySnapshot{DefinitionRevision: facts.DefinitionRevision},
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:             []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace"}},
			AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:       append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
			TargetRefs:                append([]string(nil), facts.Focus.TargetRefs...),
		},
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	wire := runtimeCandidateWireForTest(t, manifest)
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, runtimecontract.RuntimeBlockKindPatchApply)
	if !found || rule.VariantID != "text_replace" ||
		!slices.Equal(rule.ProviderFields, []string{"kind", "parameters"}) ||
		len(rule.DerivedBindingRefs) != 1 || !slices.Equal(rule.DerivedTargetRefs, []string{"main.go"}) ||
		!slices.Contains(rule.DerivedParameterNames(), "patchType") {
		t.Fatalf("minimal auxiliary wire rule = %+v found=%t", rule, found)
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	for _, localField := range []string{"bindingRefs", "targetRefs"} {
		if runtimeOperationSchemaHasProperty(t, schema, localField) {
			t.Fatalf("auxiliary operation schema exposed locally-derived field %q: %s", localField, schema)
		}
	}
	if strings.Contains(schema, `"const":"patchType"`) || strings.Contains(schema, `"const":"text_replace"`) {
		t.Fatalf("auxiliary operation schema exposed locally-derived variant authority: %s", schema)
	}
	payload := []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`)
	request := llmmodels.ProviderRequest{Schema: schema}
	for _, enabled := range []bool{false, true} {
		textConfig := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: enabled}, request)
		if enabled && (textConfig == nil || textConfig.Format == nil || !textConfig.Format.Strict) {
			t.Fatalf("JSONSchemaResponse=true did not dispatch the minimal auxiliary schema: %+v", textConfig)
		}
		if !enabled && textConfig != nil {
			t.Fatalf("JSONSchemaResponse=false unexpectedly dispatched provider schema: %+v", textConfig)
		}
		candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
		if decodeErr != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
			t.Fatalf("JSONSchemaResponse=%t minimal auxiliary candidate=%+v err=%v", enabled, candidate, decodeErr)
		}
		operation := candidate.StateChange.Operations[0].Operation
		variantDerived := false
		for _, parameter := range operation.Parameters {
			if parameter.Name == "patchType" && parameter.String != nil && *parameter.String == "text_replace" {
				variantDerived = true
				break
			}
		}
		if operation.Kind != runtimecontract.RuntimeBlockKindPatchApply || !variantDerived ||
			!slices.Equal(operation.BindingRefs, rule.DerivedBindingRefs) ||
			!slices.Equal(operation.TargetRefs, rule.DerivedTargetRefs) {
			t.Fatalf("JSONSchemaResponse=%t locally compiled auxiliary operation=%+v", enabled, operation)
		}
	}
}

func TestRuntimeCandidateSchemaRequiresOneSelectionFromNonSingletonSealedBindings(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		DefinitionRevision: "definition-proof-binding",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-proof-binding", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}, TargetRefs: []string{"main.go", "support.go"},
		},
	}
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.RuntimeBlockKindPatchApply})
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		AuthoritySnapshot: runtimecontract.AuthoritySnapshot{DefinitionRevision: facts.DefinitionRevision},
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, TargetRefs: facts.Focus.TargetRefs,
		},
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	branch := runtimeSchemaOperationBranchForKind(t, schema, runtimecontract.RuntimeBlockKindPatchApply)
	properties, _ := branch["properties"].(map[string]any)
	bindingSchema, ok := properties["bindingRefs"].(map[string]any)
	if !ok || bindingSchema["minItems"] != float64(1) || bindingSchema["maxItems"] != float64(1) ||
		!runtimeSchemaStringArrayContains(branch["required"], "bindingRefs") {
		t.Fatalf("proof-bearing schema did not retain one provider binding selection: branch=%+v", branch)
	}
	wire := runtimeCandidateWireForTest(t, manifest)
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, runtimecontract.RuntimeBlockKindPatchApply)
	if !found || len(rule.DerivedBindingRefs) != 0 || !slices.Equal(rule.AllowedBindingRefs, []string{
		"binding/test/llm/focus-proof-binding/cwi-1", "binding/test/llm/focus-proof-binding/cwi-2",
	}) {
		t.Fatalf("proof-bearing wire rule = %+v found=%t", rule, found)
	}
	instruction := runtimeCandidateStructuredOutputInstruction(manifest, wire)
	if !strings.Contains(instruction, "bindingRefs is required and requires exactly one value") || strings.Contains(instruction, "Optional bindingRefs") || strings.Contains(instruction, "may omit bindingRefs") {
		t.Fatalf("proof-bearing instruction = %s", instruction)
	}
}

func TestRuntimeCandidateSchemaRequiresCanonicalTargetRefsForDurableFileOperations(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{TargetRefs: []string{"main.go", "support.go"}, AcceptedKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindFileEdit,
		}},
	}, nil)
	for _, kind := range []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit} {
		if !runtimeSchemaRequiresTopLevelTargetRefs(schema, kind) {
			t.Fatalf("%s schema must require canonical top-level targetRefs", kind)
		}
	}
}

func TestRuntimeCandidateSchemaClosesNamedParameterObjects(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}, TargetRefs: []string{"main.go", "support.go"},
		},
	}, nil)
	patchParameters := runtimeSchemaParametersForKind(t, schema, runtimecontract.RuntimeBlockKindPatchApply)
	if !runtimeSchemaHasClosedNamedParameterBranch(patchParameters, "patchType", "string") {
		t.Fatalf("patch/apply parameter schema does not close patchType as string: %#v", patchParameters)
	}
	if !runtimeSchemaHasClosedNamedParameterBranch(patchParameters, "occurrence", "integer") {
		t.Fatalf("patch/apply parameter schema does not close occurrence as integer: %#v", patchParameters)
	}
	if runtimeSchemaHasNamedParameterBranch(patchParameters, "unknown") {
		t.Fatalf("patch/apply parameter schema widened to an unknown parameter: %#v", patchParameters)
	}
	if !runtimeSchemaRequiresTopLevelTargetRefs(schema, runtimecontract.RuntimeBlockKindPatchApply) {
		t.Fatal("patch/apply schema must require canonical top-level targetRefs")
	}

	shellParameters := runtimeSchemaParametersForKind(t, schema, runtimecontract.StepKindShellCommand)
	if runtimeSchemaHasNamedParameterBranch(shellParameters, "summary") {
		t.Fatalf("shell/command schema admitted summary parameter: %#v", shellParameters)
	}
	if !runtimeSchemaHasClosedNamedParameterBranch(shellParameters, "timeoutMillis", "integer") {
		t.Fatalf("shell/command parameter schema does not close timeoutMillis as integer: %#v", shellParameters)
	}
}

func TestRuntimeCandidateSchemaClosesMCPDynamicParameters(t *testing.T) {
	toolSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall}},
	}, nil)
	toolParameters := runtimeSchemaParametersForKind(t, toolSchema, runtimecontract.StepKindMCPToolCall)
	if !runtimeSchemaHasClosedNamedParameterBranch(toolParameters, "serverId", "string") {
		t.Fatalf("mcp/tool_call lost fixed serverId parameter: %#v", toolParameters)
	}
	if !runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "string") ||
		!runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "integer") ||
		!runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "boolean") {
		t.Fatalf("mcp/tool_call dynamic input schema is not closed scalar union: %#v", toolParameters)
	}
	if runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "stringList") {
		t.Fatalf("mcp/tool_call dynamic input schema wrongly permits stringList: %#v", toolParameters)
	}

	resourceSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPResourceRead}},
	}, nil)
	resourceParameters := runtimeSchemaParametersForKind(t, resourceSchema, runtimecontract.StepKindMCPResourceRead)
	if runtimeSchemaHasDynamicParameterBranch(resourceParameters, "", "string") {
		t.Fatalf("mcp/resource_read must not expose dynamic parameter fields: %#v", resourceParameters)
	}
}

func runtimeSchemaParametersForKind(t *testing.T, encoded string, kind string) map[string]any {
	t.Helper()
	branch := runtimeSchemaOperationBranchForKind(t, encoded, kind)
	properties, ok := branch["properties"].(map[string]any)
	if !ok {
		t.Fatalf("operation branch for %q has no properties: %#v", kind, branch)
	}
	parameters, ok := properties["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("operation branch for %q has no parameter schema: %#v", kind, properties)
	}
	return parameters
}

func runtimeSchemaRequiresTopLevelTargetRefs(encoded, kind string) bool {
	branch := runtimeSchemaOperationBranchForKind(nil, encoded, kind)
	if branch == nil {
		return false
	}
	return runtimeSchemaStringArrayContains(branch["required"], "targetRefs")
}

func runtimeSchemaOperationBranchForKind(t *testing.T, encoded, kind string) map[string]any {
	var root map[string]any
	if err := json.Unmarshal([]byte(encoded), &root); err != nil {
		if t != nil {
			t.Fatal(err)
		}
		return nil
	}
	operations, _ := root["properties"].(map[string]any)["operations"].(map[string]any)
	items, _ := operations["items"].(map[string]any)
	for _, rawBranch := range runtimeSchemaArray(items["oneOf"]) {
		branch, _ := rawBranch.(map[string]any)
		properties, _ := branch["properties"].(map[string]any)
		kindSchema, _ := properties["kind"].(map[string]any)
		if kindSchema["const"] != kind {
			continue
		}
		return branch
	}
	if t != nil {
		t.Fatalf("candidate schema has no operation branch for %q: %#v", kind, items["oneOf"])
	}
	return nil
}

func runtimeSchemaHasClosedNamedParameterBranch(parameters map[string]any, name, valueField string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] != name || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeSchemaStringArrayContains(branch["required"], "name") && runtimeSchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeSchemaHasNamedParameterBranch(parameters map[string]any, name string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] == name {
			return true
		}
	}
	return false
}

func runtimeSchemaHasDynamicParameterBranch(parameters map[string]any, pattern, valueField string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["pattern"] != pattern || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeSchemaStringArrayContains(branch["required"], "name") && runtimeSchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeSchemaParameterBranches(parameters map[string]any) []map[string]any {
	items, _ := parameters["items"].(map[string]any)
	ret := make([]map[string]any, 0)
	for _, rawBranch := range runtimeSchemaArray(items["oneOf"]) {
		if branch, ok := rawBranch.(map[string]any); ok {
			ret = append(ret, branch)
		}
	}
	return ret
}

func runtimeSchemaKindConst(condition map[string]any) string {
	properties, _ := condition["properties"].(map[string]any)
	kind, _ := properties["kind"].(map[string]any)
	value, _ := kind["const"].(string)
	return value
}

func runtimeSchemaArray(value any) []any {
	values, _ := value.([]any)
	return values
}

func runtimeSchemaStringArray(value any) []string {
	values := runtimeSchemaArray(value)
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		if value, ok := raw.(string); ok {
			ret = append(ret, value)
		}
	}
	return ret
}

func runtimeSchemaStringArrayContains(value any, want string) bool {
	for _, value := range runtimeSchemaStringArray(value) {
		if value == want {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateSchemaRequiresNonEmptyCanonicalPatchTarget(t *testing.T) {
	condition := runtimeNonEmptyParameterContainsSchema("targetRef", runtimecontract.RuntimeOperationParameterTypeString)
	contains, ok := condition["contains"].(map[string]any)
	if !ok {
		t.Fatalf("condition = %#v, want contains object", condition)
	}
	properties, ok := contains["properties"].(map[string]any)
	if !ok {
		t.Fatalf("contains = %#v, want properties", contains)
	}
	target, ok := properties["string"].(map[string]any)
	if !ok || target["minLength"] != 1 {
		t.Fatalf("targetRef schema = %#v, want non-empty string", target)
	}
	required, ok := contains["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "name" || required[1] != "string" {
		t.Fatalf("targetRef required = %#v, want name and string", contains["required"])
	}
}

func TestRuntimeCandidateSchemaNarrowsFileOwnerScopeToDurableFileAndDeclaredGroundedKinds(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindShellCommand,
			runtimecontract.RuntimeBlockKindPatchApply,
			runtimecontract.StepKindGitWrite,
		}},
	}
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	schema := runtimeCandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"const":"patch/apply"`) || !strings.Contains(schema, `"const":"shell/command"`) {
		t.Fatalf("scope schema = %s, want owner-published File and Shell scope kinds", schema)
	}
	for _, forbidden := range []string{`"git/write"`} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("scope schema admitted undeclared grounded kind %s: %s", forbidden, schema)
		}
	}
	output := runtimeCandidateOutputContractForTest(t, manifest)
	if len(output.OperationContracts) != 2 || output.OperationContracts[0].Kind != runtimecontract.RuntimeBlockKindPatchApply || output.OperationContracts[1].Kind != runtimecontract.StepKindShellCommand {
		t.Fatalf("scope operation contracts = %#v, want patch/apply and shell/command", output.OperationContracts)
	}
	kinds := runtimeCandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope admissible kinds = %v, want %v", got, want)
	}
}

func TestRuntimeCandidateRendererProjectsSealedTargetPolicy(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds:        []string{runtimecontract.StepKindFileEdit},
		AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"a.go", "b.go"},
		SelectorRefs: []string{"file/workspace_deliverable"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{inventory},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := runtimeCandidateOutputContractForTest(t, manifest)
	if len(output.OperationContracts) != 1 {
		t.Fatalf("operation contracts = %#v", output.OperationContracts)
	}
	contract := output.OperationContracts[0]
	if contract.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		contract.Target.Policy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope ||
		!slices.Equal(contract.Target.OwnerScopeTargetRefs, []string{"a.go", "b.go"}) ||
		!slices.Equal(contract.Target.AllowedTargetRefs, []string{"a.go", "b.go"}) ||
		!slices.Equal(contract.AdmissibilitySnapshotRefs, []string{inventory.SnapshotID}) ||
		!contract.RequireTargetRefs {
		t.Fatalf("sealed target policy projection = %#v", contract)
	}
	schema := string(output.Schema)
	if !strings.Contains(schema, `"enum":["a.go","b.go"]`) {
		t.Fatalf("schema must constrain targetRefs to sealed allowed set: %s", schema)
	}
	if !strings.Contains(output.StructuredOutputInstruction, "TargetPolicy=existing_owner_scope") ||
		!strings.Contains(output.StructuredOutputInstruction, "Allowed targets: a.go, b.go") {
		t.Fatalf("instruction lost sealed target policy: %s", output.StructuredOutputInstruction)
	}
}

func TestRuntimeCandidateRendererProjectsOnlyManagedValidationRecipeIDs(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds:         []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
	}}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: "go-test", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := runtimeCandidateOutputContractForTest(t, manifest)
	if len(output.OperationContracts) != 1 || !slices.Equal(output.OperationContracts[0].ValidationRecipeIDs, []string{"go-test"}) {
		t.Fatalf("validation recipe selection projection = %#v", output.OperationContracts)
	}
	schema := string(output.Schema)
	if !strings.Contains(schema, `"recipeId"`) || !strings.Contains(schema, `"enum":["go-test"]`) {
		t.Fatalf("managed recipe schema lacks sealed recipe ID enum: %s", schema)
	}
	for _, forbidden := range []string{`"command":"go"`, `"args":["test","-v","./..."]`, "test-name filter"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("managed recipe schema leaked Shell execution detail %q: %s", forbidden, schema)
		}
	}
	if !strings.Contains(output.StructuredOutputInstruction, "recipeId must select exactly one sealed validation recipe: go-test") ||
		!strings.Contains(output.StructuredOutputInstruction, "Do not include command, args, timeoutMillis") {
		t.Fatalf("managed recipe instruction = %s", output.StructuredOutputInstruction)
	}
}

func TestRuntimeCandidateManifestDrivesSchemaInstructionAndRetryCorrection(t *testing.T) {
	cases := []struct {
		name     string
		decision runtimecontract.RuntimeDecision
		contains []string
		omits    []string
	}{
		{
			name: "patch target and closed parameters",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.RuntimeBlockKindPatchApply},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
			}},
			contains: []string{"patch/apply", "Locally derived and forbidden in provider output: targetRefs", "Forbidden parameters"},
		},
		{
			name: "shell summary remains forbidden",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.StepKindShellCommand},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			}},
			contains: []string{"shell/command", "command:string"},
			omits:    []string{"summary"},
		},
		{
			name: "required sequence exposes operation-local dependency",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.StepKindGitWrite},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
					{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add"},
					{RequirementID: "commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
				},
			}},
			contains: []string{"git/write", "dependsOn", "requiredOperationRef", "stage=git/write/add", "commit=git/write/commit", "requires=stage"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(testCase.decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
			if err != nil {
				t.Fatal(err)
			}
			output := runtimeCandidateOutputContractForTest(t, manifest)
			if output.ManifestHash != manifest.Hash || output.StructuredOutputInstruction == "" {
				t.Fatalf("output contract did not retain manifest identity: %+v", output)
			}
			instruction := output.StructuredOutputInstruction
			for _, want := range testCase.contains {
				if !strings.Contains(instruction, want) {
					t.Fatalf("manifest instruction lost %q: %s", want, instruction)
				}
			}
			for _, unwanted := range testCase.omits {
				if strings.Contains(instruction, unwanted) {
					t.Fatalf("manifest instruction admitted %q: %s", unwanted, instruction)
				}
			}
			for _, property := range []string{"id", "dependsOn", "coverageWorkItemIds"} {
				if runtimeOperationSchemaHasProperty(t, string(output.Schema), property) {
					t.Fatalf("schema exposed owner-derived operation property %q: %s", property, output.Schema)
				}
			}
			observation, directive := runtimeCandidateCorrectionPairForProviderTest(runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid", Reason: `arbitrary stale error text with parameter "summary"`}, manifest.Hash, "$.operations", "field_present")
			retry := runtimeCandidateRetryForManifest(1, observation, manifest, directive)
			if retry == nil || !strings.Contains(retry.Instruction, manifest.Hash) || !strings.Contains(retry.Instruction, "outputContract.structuredOutputInstruction") || strings.Contains(retry.Instruction, "context.outputContract") || strings.Contains(retry.Instruction, "arbitrary stale error text") {
				t.Fatalf("retry correction was not manifest-derived: %+v", retry)
			}
		})
	}
}

func TestRuntimeCandidateSchemaKeepsExplicitSequenceDuringFileScopeNarrowing(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindGitWrite},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{
				RequirementID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
			}},
		},
	}
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	kinds := runtimeCandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope kinds = %v, want %v", got, want)
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"shell/command"`) || !strings.Contains(schema, `"patch/apply"`) || !strings.Contains(schema, `"initialize-repository"`) {
		t.Fatalf("scope sequence schema = %s", schema)
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := runtimecontract.RuntimeCandidateWireRuleForRequiredOperation(wire, "initialize-repository")
	if !found || rule.Kind != runtimecontract.StepKindGitWrite || rule.VariantID != "init" {
		t.Fatalf("scope required wire rule = %+v found=%t", rule, found)
	}
}

func TestRuntimeCandidateSchemaBindsEachRequiredOperationToItsExactProspectiveTarget(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		DefinitionRevision: "definition-required-targets",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-required-targets", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-required-targets"}, TargetRefs: []string{"go.mod", "workorch_test.go"},
		},
	}
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.StepKindFileWrite})
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		AuthoritySnapshot: runtimecontract.AuthoritySnapshot{DefinitionRevision: facts.DefinitionRevision},
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindFileWrite}, AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
			TargetRefs: facts.Focus.TargetRefs, RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "create-module", Kind: runtimecontract.StepKindFileWrite, TargetRef: "go.mod"},
				{RequirementID: "create-test", Kind: runtimecontract.StepKindFileWrite, TargetRef: "workorch_test.go", DependsOnRequirementIDs: []string{"create-module"}},
			},
		},
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(runtimeCandidateSchemaForManifest(manifest)), &schema); err != nil {
		t.Fatal(err)
	}
	properties, _ := schema["properties"].(map[string]any)
	operations, _ := properties["operations"].(map[string]any)
	conditions := runtimeSchemaArray(operations["allOf"])
	if len(conditions) != 2 {
		t.Fatalf("required operation conditions = %d, want 2: %+v", len(conditions), conditions)
	}
	wantRefs := map[string]struct{}{"create-module": {}, "create-test": {}}
	for _, raw := range conditions {
		condition, _ := raw.(map[string]any)
		contains, _ := condition["contains"].(map[string]any)
		containsProperties, _ := contains["properties"].(map[string]any)
		refSchema, _ := containsProperties["requiredOperationRef"].(map[string]any)
		ref, _ := refSchema["const"].(string)
		_, found := wantRefs[ref]
		if !found {
			t.Fatalf("unexpected required operation schema ref %q", ref)
		}
		if _, exposed := containsProperties["targetRefs"]; exposed || runtimeSchemaStringArrayContains(contains["required"], "targetRefs") {
			t.Fatalf("required operation %q exposed locally-derived targetRefs: %+v", ref, contains)
		}
		delete(wantRefs, ref)
	}
	if len(wantRefs) != 0 {
		t.Fatalf("missing required operation schemas: %+v", wantRefs)
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	wantTargets := map[string]string{"create-module": "go.mod", "create-test": "workorch_test.go"}
	for ref, target := range wantTargets {
		rule, found := runtimecontract.RuntimeCandidateWireRuleForRequiredOperation(wire, ref)
		if !found || !slices.Equal(rule.DerivedTargetRefs, []string{target}) || containsString(rule.ProviderFields, "targetRefs") {
			t.Fatalf("required operation %q wire rule = %+v, want locally-derived target %q", ref, rule, target)
		}
	}
}

func TestRuntimeProviderRequestNarrowsFileScopeAcrossSchemaContextAndRetry(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable", SelectorOwnerRef: "profile/test", SelectorSubjectRef: "deliverable",
		SelectorCandidateRefs: []string{"main.go", "support.go"},
		Disposition:           runtimecontract.RuntimeTargetSelectionScope, CandidateCount: 2,
		TargetRefs: []string{"main.go", "support.go"}, CandidateTargetRefs: []string{"main.go", "support.go"}, ObservedRevision: "workspace-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-scope", RunID: "run-scope", DefinitionRevision: "definition-scope", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-scope", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-scope"}, RequireTargetGrounding: true, TargetRole: "deliverable"},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-scope", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{
			runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply,
		}},
		Resources: runtimecontract.RuntimeResourceFacts{
			TargetSelection:          &selection,
			OperationAdmissibilities: runtimeCandidateScopeOwnerFactsForTest(),
			ProviderCeiling: &runtimecontract.ProviderCeiling{
				Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ContextWindowTokens: 128000,
				ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
				CapabilityRevision: "capability-scope", ConfigurationRevision: "config-scope",
			},
		},
		Activation: runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
	}
	sealRuntimeProviderTestPolicies(&facts)
	bindRuntimeCandidateTestSelection(&facts, selection)
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{
		runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply,
	})
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-scope", GoalID: "goal-scope", GoalDefinition: "goal", DoDID: "dod-scope", DoDDefinition: "dod"}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	manifest := runtimeCandidateManifestForRequest(t, decision, candidateContext)
	candidateRequest := llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix, DefinitionContext: definition,
		CandidateManifest: manifest, CandidateContext: candidateContext,
	}
	request, err := runtimeProviderRequest(candidateRequest)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"git/write"`} {
		if strings.Contains(request.Schema, forbidden) {
			t.Fatalf("scope provider schema widened to %s: %s", forbidden, request.Schema)
		}
	}
	if !strings.Contains(request.Schema, `"const":"patch/apply"`) || !strings.Contains(request.Schema, `"const":"shell/command"`) {
		t.Fatalf("scope provider schema did not retain durable File and declared target-bound operation: %s", request.Schema)
	}
	dynamic := request.PromptContext.Sections[3].Content
	for _, required := range []string{`"operationContracts":[{"kind":"patch/apply"`, `"kind":"shell/command"`, "bounded workspace feasibility scope"} {
		if !strings.Contains(dynamic, required) {
			t.Fatalf("scope provider C3 lost %q: %s", required, dynamic)
		}
	}
	for _, forbidden := range []string{`"kind":"git/write"`, `"admissibleOperationKinds"`, `"capabilities"`, `"operationAdmissibilities"`, `"prerequisiteObservations"`} {
		if strings.Contains(dynamic, forbidden) {
			t.Fatalf("scope provider C3 widened operation contracts to %s: %s", forbidden, dynamic)
		}
	}

	t.Run("missing or drifted manifest supporting facts fail before provider submission", func(t *testing.T) {
		missingSelection := candidateRequest
		missingSelection.CandidateContext.Resources.TargetSelection = nil
		if _, err := runtimeProviderRequest(missingSelection); err == nil || !strings.Contains(err.Error(), "prerequisite snapshot") || !strings.Contains(err.Error(), "is missing") {
			t.Fatalf("missing target-selection snapshot error = %v", err)
		}

		driftedSelection := candidateRequest
		drifted := *candidateContext.Resources.TargetSelection
		drifted.ObservedRevision = "workspace-drifted-with-stale-snapshot-id"
		driftedSelection.CandidateContext.Resources.TargetSelection = &drifted
		if _, err := runtimeProviderRequest(driftedSelection); err == nil || !strings.Contains(err.Error(), "prerequisite snapshot") || !strings.Contains(err.Error(), "is invalid") {
			t.Fatalf("drifted target-selection snapshot error = %v", err)
		}

		admissibilityRef := ""
		for _, operation := range manifest.Operations {
			if len(operation.AdmissibilitySnapshotRefs) != 0 {
				admissibilityRef = operation.AdmissibilitySnapshotRefs[0]
				break
			}
		}
		if admissibilityRef == "" {
			t.Fatal("scope manifest has no admissibility snapshot reference")
		}
		missingCatalog := candidateRequest
		missingCatalog.CandidateContext.Resources.OperationAdmissibilities = slices.DeleteFunc(
			append([]runtimecontract.RuntimeOperationAdmissibility(nil), candidateContext.Resources.OperationAdmissibilities...),
			func(value runtimecontract.RuntimeOperationAdmissibility) bool {
				return value.SnapshotID == admissibilityRef
			},
		)
		if _, err := runtimeProviderRequest(missingCatalog); err == nil || !strings.Contains(err.Error(), "admissibility snapshot") || !strings.Contains(err.Error(), "is missing") {
			t.Fatalf("missing admissibility snapshot error = %v", err)
		}

		generation, err := (&Biz{}).GenerateCandidate(context.Background(), missingCatalog)
		if err == nil || generation.Audit == nil || generation.Audit.ProviderSubmitted || generation.Status != llmmodels.RuntimeCandidateStatusFailed {
			t.Fatalf("missing supporting fact generation = %+v err=%v", generation, err)
		}
	})

	retryManifest := manifest
	retryObservation, retryCorrection := runtimeCandidateCorrectionPairForProviderTest(runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "candidate-target-outside-grounding", Reason: "owner-grounded workspace scope permits only a durable File operation with a concrete target"}, retryManifest.Hash, "$.operations[0].targetRefs", "main.go")
	retryRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1,
		CandidateCorrectionObservation: retryObservation,
		CandidateCorrection:            retryCorrection,
		StablePrefixHash:               prefix, CandidateManifest: retryManifest, DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	retryTail := retryRequest.PromptContext.Sections[4].Content
	if !strings.Contains(retryTail, "outputContract.structuredOutputInstruction") || strings.Contains(retryTail, "context.outputContract") ||
		!strings.Contains(retryTail, "sealed manifest hash") || strings.Contains(retryTail, "do not use Git, MCP, Web, or SSH") {
		t.Fatalf("scope provider retry did not use manifest-bound correction: %s", retryTail)
	}
}

func bindRuntimeCandidateTestSelection(facts *runtimecontract.RuntimeFacts, selection runtimecontract.RuntimeTargetSelection) {
	if facts == nil {
		return
	}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID:            selection.PrerequisiteID,
		Kind:          runtimecontract.PrerequisiteKindTargetSelection,
		Scope:         runtimecontract.RuntimePrerequisiteScopeFocus,
		FocusID:       facts.Focus.ID,
		OwnerModuleID: selection.OwnerModuleID,
		SubjectRef:    selection.TargetRole,
	})
}

// bindRuntimeCandidateTestFocus is a test-fixture compiler for provider
// rendering tests. Production receives activation-sealed bindings and never
// creates them at request-render time. Keeping the fixture explicit makes the
// LLM tests consume the same CWI -> binding manifest path as Runtime.
func bindRuntimeCandidateTestFocus(t *testing.T, facts *runtimecontract.RuntimeFacts, state runtimecontract.ConvergenceState, operationKinds []string) {
	t.Helper()
	if facts == nil || len(facts.Focus.CoverageWorkItemIDs) == 0 {
		return
	}
	focus := facts.Focus.Normalized()
	if selection := facts.Resources.TargetSelection; selection != nil {
		source := selection.Normalized()
		focus.TargetRefs = append([]string(nil), source.TargetRefs...)
		focus.TargetSelectorRef = source.SelectorRef
		focus.TargetSelectorOwnerRef = source.SelectorOwnerRef
		focus.TargetSelectorSubjectRef = source.SelectorSubjectRef
		focus.TargetSelectorCandidateRefs = append([]string(nil), source.SelectorCandidateRefs...)
		if source.ProspectiveTargetRef != "" {
			focus.ProspectiveTargetRefs = []string{source.ProspectiveTargetRef}
		}
	}
	evidenceKind := runtimecontract.EvidenceKindStateChange
	effectScopes := []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
	if state == runtimecontract.ConvergenceStateNeedValidation {
		evidenceKind = runtimecontract.EvidenceKindValidation
		effectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
	}
	if len(focus.RequiredEvidenceKinds) == 0 {
		focus.RequiredEvidenceKinds = []string{evidenceKind}
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, kind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind, AllowedOperationKinds: append([]string(nil), operationKinds...)}
		if kind == runtimecontract.EvidenceKindStateChange {
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
		} else {
			proof.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), effectScopes...)
			proof.ValidationRecipeID = "test-provider-render-recipe"
		}
		proofs = append(proofs, proof)
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID: "binding/test/llm/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
			FocusID: focus.ID, DefinitionRevision: facts.DefinitionRevision, Proofs: proofs,
			TargetRefs: append([]string(nil), focus.TargetRefs...), TargetSelectorRef: focus.TargetSelectorRef,
			TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs: append([]string(nil), focus.TargetSelectorCandidateRefs...), ProspectiveTargetAllOfConstraintRefs: append([]string(nil), focus.ProspectiveTargetAllOfConstraintRefs...), ProspectiveTargetRefs: append([]string(nil), focus.ProspectiveTargetRefs...),
			DeclarationRef: "test/llm-provider-render",
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		bindings = append(bindings, binding)
	}
	focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	facts.Focus = focus
}

func TestRuntimeCandidateSchemaRestrictsVariantsToSealedAuthority(t *testing.T) {
	patchManifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
			}},
		},
	}, nil)
	patchOutput := runtimeCandidateOutputContractForTest(t, patchManifest)
	patchSchema := string(patchOutput.Schema)
	if strings.Contains(patchSchema, `"const":"file_write"`) || strings.Contains(patchSchema, `"const":"text_replace"`) || strings.Contains(patchSchema, `"const":"unified_diff"`) || strings.Contains(patchSchema, `"const":"patchType"`) {
		t.Fatalf("patch schema exposed locally compiled variant authority: %s", patchSchema)
	}
	if len(patchOutput.WireRules) != 1 || patchOutput.WireRules[0].CompiledVariantID != "file_write" ||
		!slices.Contains(patchOutput.WireRules[0].LocallyDerivedFields, "parameters[name=patchType]") ||
		len(patchOutput.OperationContracts) != 1 || len(patchOutput.OperationContracts[0].AllowedVariantIDs) != 0 || len(patchOutput.OperationContracts[0].Variants) != 0 {
		t.Fatalf("patch output did not bind the sole sealed variant locally: %+v", patchOutput)
	}

	commitManifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "commit",
			}},
		},
	}, nil)
	commitOutput := runtimeCandidateOutputContractForTest(t, commitManifest)
	commitSchema := string(commitOutput.Schema)
	if strings.Contains(commitSchema, `"const":"commit"`) || strings.Contains(commitSchema, `"const":"add"`) || strings.Contains(commitSchema, `"const":"init"`) ||
		strings.Contains(commitSchema, `"const":"operation"`) || strings.Contains(commitSchema, `"const":"pathspecs"`) || !strings.Contains(commitSchema, `"const":"message"`) ||
		len(commitOutput.WireRules) != 1 || commitOutput.WireRules[0].CompiledVariantID != "commit" {
		t.Fatalf("commit schema did not keep only provider-selectable commit fields: output=%+v", commitOutput)
	}

	sequenceManifest := runtimeCandidateManifestForSchema(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "commit",
			}},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
				{RequirementID: "stage-changes", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
				{RequirementID: "create-commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage-changes"}},
			},
		},
	}, nil)
	sequenceOutput := runtimeCandidateOutputContractForTest(t, sequenceManifest)
	sequenceSchema := string(sequenceOutput.Schema)
	for _, variant := range []string{"init", "add", "commit"} {
		if strings.Contains(sequenceSchema, `"const":"`+variant+`"`) {
			t.Fatalf("required sequence schema exposed locally-derived variant %q: %s", variant, sequenceSchema)
		}
	}
	wantVariants := map[string]string{"repo-init": "init", "stage-changes": "add", "create-commit": "commit"}
	for _, rule := range sequenceOutput.WireRules {
		if want, found := wantVariants[rule.RequiredOperationRef]; found {
			if rule.CompiledVariantID != want || rule.CompiledKind != runtimecontract.StepKindGitWrite {
				t.Fatalf("required sequence wire rule = %+v, want variant %q", rule, want)
			}
			delete(wantVariants, rule.RequiredOperationRef)
		}
	}
	if len(wantVariants) != 0 {
		t.Fatalf("required sequence wire rules lost variants: %+v", wantVariants)
	}
	if len(sequenceOutput.OperationContracts) != 1 || len(sequenceOutput.OperationContracts[0].AllowedVariantIDs) != 0 ||
		len(sequenceOutput.OperationContracts[0].Variants) != 0 {
		t.Fatalf("required sequence output contract exposed locally-derived variants: %+v", sequenceOutput.OperationContracts)
	}
	for _, parameter := range sequenceOutput.OperationContracts[0].Parameters {
		if parameter.Name == "operation" {
			t.Fatalf("required sequence output contract exposed locally-derived discriminator: %+v", sequenceOutput.OperationContracts[0])
		}
	}

	unknownVariantDecision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "does-not-exist",
			}},
		},
	}
	if _, err := runtimecontract.BuildRuntimeCandidateContractManifest(unknownVariantDecision, runtimecontract.RuntimeCandidateManifestOwnerFacts{}); err == nil {
		t.Fatal("unknown sealed variant must fail manifest compilation instead of producing a widened schema")
	}
}

func TestRuntimeAssessmentRequestPublishesCanonicalEvidenceRefContract(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "runtime-evidence-00000000000000000001",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequireAssessment: true,
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
			ID: "runtime-operation-1", Kind: runtimecontract.EvidenceKindStateChange, SourceRef: "workorch://runtime-operations/dispatch-1/op-1",
			EvidenceRefs: []string{"workorch://workspace/main.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"file/write"}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ContextWindowTokens: 128000,
			ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
			CapabilityRevision: "capability-1", ConfigurationRevision: "config-1",
		}},
	}
	sealRuntimeProviderTestPolicies(&facts)
	facts.Focus.TargetSelectorRef = "selector/assessment-evidence-ref-test"
	facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleFile
	facts.Focus.TargetSelectorSubjectRef = "workspace"
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/assessment-evidence-ref-test", CoverageWorkItemID: "cwi-1", FocusID: facts.Focus.ID,
		DefinitionRevision: facts.DefinitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindStateChange, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		}},
		TargetSelectorRef: facts.Focus.TargetSelectorRef, TargetSelectorOwnerRef: facts.Focus.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef: facts.Focus.TargetSelectorSubjectRef, DeclarationRef: "definition/assessment-evidence-ref-test",
	}
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
	facts.Evidence.Accepted[0].FocusID = facts.Focus.ID
	facts.Evidence.Accepted[0].BindingRefs = []string{binding.ID}
	facts = facts.Normalized()
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedAssessment, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	projection := runtimeCandidateAuthorityContextProjection(context, runtimeCandidateManifestForRequest(t, decision, context))
	wantRefs := runtimecontract.AcceptedEvidenceRefsForFocus(facts)
	if strings.Join(projection.AllowedEvidenceRefs, "|") != strings.Join(wantRefs, "|") || len(wantRefs) != 3 {
		t.Fatalf("allowed evidence refs = %+v, want %+v", projection.AllowedEvidenceRefs, wantRefs)
	}
	request, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, context), DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	dynamic := request.PromptContext.Sections[3].Content
	for _, ref := range wantRefs {
		if !strings.Contains(dynamic, ref) {
			t.Fatalf("assessment context lost allowed evidence ref %q: %s", ref, dynamic)
		}
	}
	schema := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedAssessment)
	for _, contract := range []string{`"required":["focusId","coverageWorkItemIds","disposition","summary"]`, `"const":"satisfied"`, `"required":["evidenceRefs"]`, `"minItems":1`} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("assessment schema lost conditional evidence contract %q: %s", contract, schema)
		}
	}
	retryFailure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "missing evidenceRefs"}
	retryManifest := runtimeCandidateManifestForRequest(t, decision, context)
	retryObservation, retryCorrection := runtimeCandidateCorrectionPairForProviderTest(retryFailure, retryManifest.Hash, "$.evidenceRefs", "field_present")
	retry, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, CandidateCorrectionObservation: retryObservation, StablePrefixHash: prefix,
		CandidateCorrection: retryCorrection,
		CandidateManifest:   retryManifest, DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range wantRefs {
		if !strings.Contains(retry.PromptContext.Sections[3].Content, ref) {
			t.Fatalf("assessment retry lost allowed evidence ref %q", ref)
		}
	}
}

func TestRuntimeCandidateProviderSeparatesAuthorityFromRejectedShapeTail(t *testing.T) {
	path := "main.go"
	operation := runtimecontract.RuntimeOperation{ID: "validate-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}}
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "runtime-evidence-00000000000000000001",
		Focus:        runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileRead}},
		Evidence: runtimecontract.EvidenceLedger{Rejected: []runtimecontract.EvidenceFact{{
			ID: "rejected-1", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-1", OperationID: "validate-1",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000001", FailureSignature: "sig-1",
		}}},
	}.Normalized()
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds: []string{runtimecontract.StepKindFileRead}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{{
		Operation: operation, FailureSignature: "sig-1",
	}})
	manifest := runtimeCandidateManifestForRequest(t, decision, context)
	authority := runtimeCandidateAuthorityContextProjection(context, manifest)
	tail := buildRuntimeCandidateExecutionTailProjection(context, 0, nil, manifest, nil)
	payload, err := json.Marshal(struct {
		Authority runtimeCandidateAuthorityProjection     `json:"authority"`
		Tail      runtimeCandidateExecutionTailProjection `json:"tail"`
	}{Authority: authority, Tail: tail})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"rejectedOperationShapes", "file/read", "sig-1", "main.go"} {
		if !strings.Contains(string(payload), value) {
			t.Fatalf("provider tail lost %q: %s", value, payload)
		}
	}
	authorityPayload, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"rejectedOperationShapes", "admissibleOperationKinds", "validate-1", "sig-1"} {
		if strings.Contains(string(authorityPayload), forbidden) {
			t.Fatalf("C3 authority retained C4/duplicate fact %q: %s", forbidden, authorityPayload)
		}
	}
}
