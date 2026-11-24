package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestChangeReviewLocalReferencesAreExactAndRejectMissingForeignAndDuplicateSources(t *testing.T) {
	body := "完整当前事实\r\n"
	f := rc.RuntimeValidationInputReviewFile{TargetRef: "resource", Content: body, ContentHash: rc.RuntimeValidationInputContentHash(body)}
	context := &rc.RuntimeChangeReviewContext{WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition", Current: []rc.RuntimeValidationInputReviewFile{f}}
	sources := []runtimeChangeReviewBodySource{{"C3.workContext.content.snapshots[0].Content", f}}
	for _, name := range []string{"exact", "missing", "duplicate", "foreign", "changed-body", "changed-target"} {
		t.Run(name, func(t *testing.T) {
			c := context.Clone()
			refs := compactChangeReviewFiles(c, sources)
			if len(refs) != 1 || c.Current[0].Content != "" || context.Current[0].Content != body {
				t.Fatal("not lossless/detached")
			}
			src := append([]runtimeChangeReviewBodySource(nil), sources...)
			switch name {
			case "missing":
				src = nil
			case "duplicate":
				refs = append(refs, refs[0])
			case "foreign":
				refs[0].Source = "another-request/source"
			case "changed-body":
				src[0].file.Content += "tampered"
			case "changed-target":
				src[0].file.TargetRef = "foreign"
			}
			err := expandChangeReviewFiles(c, refs, src)
			if name == "exact" {
				if err != nil || !reflect.DeepEqual(c, context) {
					t.Fatal("lossy reference", err)
				}
			} else if err == nil {
				t.Fatal("invalid reference reached provider")
			}
		})
	}
}

func TestChangeReviewRenderedIntegrityAndFrozenRecovery(t *testing.T) {
	r, base := frozenProviderFixture(t)
	c := &rc.RuntimeChangeReviewContext{WorkspaceID: r.SemanticContext.WorkspaceID, RunID: r.SemanticContext.RunID, DefinitionRevision: r.SemanticContext.Definition.Revision}
	for _, s := range r.SemanticContext.WorkContext.Content.Snapshots {
		c.Current = append(c.Current, rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, ContentHash: s.ContentHash, Content: s.Content})
	}
	preimage := rc.RuntimeValidationInputReviewFile{TargetRef: c.Current[0].TargetRef, Content: strings.Repeat("exact auxiliary preimage\r\n", 100)}
	preimage.ContentHash = rc.RuntimeValidationInputContentHash(preimage.Content)
	body := c.Current[0].Content
	op := rc.RuntimeOperation{ID: "auxiliary", Kind: rc.StepKindFileEdit, TargetRefs: []string{preimage.TargetRef}, Parameters: []rc.OperationParameter{{Name: "oldText", String: &preimage.Content}, {Name: "newText", String: &body}}}
	c.Changes = []rc.RuntimeChangeReviewOperation{{Operation: op, OperationHash: rc.StableRuntimeOperationHash(op), ExecutionKey: "original-key", Fence: 1, DispatchID: "original-dispatch", OperationID: op.ID, OwnerModuleID: rc.RuntimeModuleFile, ReceiptRef: "runtime-receipt/original-key", ReceiptHash: "original-receipt-hash", TargetRef: preimage.TargetRef, BeforeHash: preimage.ContentHash, AfterHash: c.Current[0].ContentHash, Baseline: &preimage}}
	r.SemanticContext.ChangeReview = c
	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if base.PromptContext.Sections[i].Content != p.PromptContext.Sections[i].Content {
			t.Fatalf("volatile change facts moved into C%d", i)
		}
	}
	metrics := runtimeProviderContextSectionMetrics(p.PromptContext.Sections[4], p.PromptContext)
	var expanded runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &expanded); err != nil {
		t.Fatal(err)
	}
	reviewSources := changeReviewBodySources(r.SemanticContext.WorkContext.Content, "C3.workContext.content", nil, "")
	if err := expandChangeReviewFiles(expanded.ChangeReview, expanded.ChangeReviewFileRefs, reviewSources); err != nil {
		t.Fatal(err)
	}
	if err := expandChangeReviewBaselinesAndValues(expanded.ChangeReview, expanded.ChangeReviewBaselineRefs, expanded.ChangeReviewValueRefs, reviewSources); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expanded.ChangeReview, c) {
		t.Fatal("rendered projection lost original auxiliary preimage or operation")
	}
	refCount := len(expanded.ChangeReviewFileRefs) + len(expanded.ChangeReviewBaselineRefs) + len(expanded.ChangeReviewValueRefs)
	expanded.ChangeReviewFileRefs = nil
	expanded.ChangeReviewBaselineRefs = nil
	expanded.ChangeReviewValueRefs = nil
	expandedJSON, _ := json.Marshal(expanded)
	if metrics.LocalReferences != refCount || metrics.BodyBytes != len(p.PromptContext.Sections[4].Content) || metrics.ExpandedBytes != len(expandedJSON) {
		t.Fatal("actual/expanded accounting is not exact", metrics)
	}
	var tail runtimeSemanticTailProjection
	if json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &tail) != nil || len(tail.ChangeReviewFileRefs) != len(c.Current) || len(tail.ChangeReviewValueRefs) == 0 {
		t.Fatal("body duplicated instead of exactly referenced")
	}
	for _, mutate := range []string{"omitted", "foreign", "changed", "lost-ref", "lost-value-ref", "foreign-value-ref", "mixed-value", "instruction"} {
		candidate := p
		prompt := *p.PromptContext
		prompt.Sections = append(prompt.Sections[:0:0], p.PromptContext.Sections...)
		candidate.PromptContext = &prompt
		var v runtimeSemanticTailProjection
		json.Unmarshal([]byte(prompt.Sections[4].Content), &v)
		switch mutate {
		case "omitted":
			v.ChangeReview = nil
		case "foreign":
			v.ChangeReview.WorkspaceID = "other-workspace"
		case "changed":
			v.ChangeReview.Current[0].ContentHash = "another-version"
		case "lost-ref":
			v.ChangeReviewFileRefs = nil
		case "lost-value-ref":
			v.ChangeReviewValueRefs = nil
		case "foreign-value-ref":
			v.ChangeReviewValueRefs[0].SourceIndex = len(v.ChangeReview.Changes)
		case "mixed-value":
			v.ChangeReview.Changes[0].Operation.Parameters[0].String = &body
		case "instruction":
			prompt.Sections[3].Content = strings.Replace(prompt.Sections[3].Content, "changeReview contains", "changeReview does not contain", 1)
		}
		if mutate != "instruction" {
			raw, _ := json.Marshal(v)
			prompt.Sections[4].Content = string(raw)
		}
		if validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, candidate.ContextBudget, candidate.PromptContext) == nil {
			t.Fatal("corrupt source/instruction admitted", mutate)
		}
	}
	frozen, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = frozen, ref
	original, _ := json.Marshal(sectionMessages(p, false))
	r.SemanticContext.ChangeReview.Current[0].Content = "new current fact must wait for a new request"
	r.SemanticContext.ChangeReview.Changes[0].Baseline.Content = "new preimage must wait for a new request"
	replay, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(sectionMessages(replay, false))
	if !bytes.Equal(original, actual) {
		t.Fatal("recovery rewrote frozen source or requested fresh model sampling")
	}
}

func TestChangeReviewCompactsAndValidatesExcludedReviewSnapshots(t *testing.T) {
	r := runtimeSemanticProviderTestRequest(t)
	ctx := &r.SemanticContext
	target := "checks/input_test.go"
	mainTarget := "main.go"
	manifestTargets := []string{target, mainTarget}
	beforeBody := strings.Repeat("before input body\r\n", 40)
	afterBody := strings.Repeat("after input body\r\n", 40)
	mainBody := "original work bytes"
	snap := func(ref, body string) rc.TargetSnapshot {
		return rc.TargetSnapshot{
			TargetRef:    ref,
			Content:      body,
			ContentHash:  rc.RuntimeValidationInputContentHash(body),
			ContentState: rc.SnapshotContentStateComplete,
			Size:         int64(len(body)),
		}
	}

	policy := rc.RuntimeImplementationWorkPolicy{
		ProtocolRef:                 rc.RuntimeImplementationWorkProtocolV1,
		OwnerModuleID:               rc.RuntimeModuleFile,
		AllowedOperations:           []rc.RuntimeOperationVariantRef{{Kind: rc.StepKindFileWrite}},
		AllowedTargetPolicies:       []rc.RuntimeOperationTargetPolicy{rc.RuntimeOperationTargetPolicyExistingOwnerScope},
		TargetSelectorRef:           "work-scope",
		TargetSelectorOwnerRef:      "profile/work",
		TargetSelectorSubjectRef:    "implementation",
		TargetSelectorCandidateRefs: manifestTargets,
		MaxRepeatedWorkStates:       2,
		MaxTargetsPerRound:          1,
		RequireFreshObservation:     true,
		RequireFreshValidation:      true,
		ProtectValidationInputs:     true,
	}
	ctx.Activation.ExecutionContract.ProfileContracts = []rc.RuntimeProfileContract{{
		ID: "work",
		EvidenceRequirements: []rc.RuntimeProfileEvidenceRequirement{{
			ID:                       "*",
			Scope:                    "dodItem",
			RequiredEvidenceKinds:    []string{rc.EvidenceKindValidation},
			AcceptedStepKinds:        []string{rc.RuntimeBlockKindManagedValidationSession},
			ImplementationWorkPolicy: &policy,
		}},
	}}
	item := ctx.DefinitionSeed.CoverageItems[0]
	focus := rc.RuntimeDefinitionFocus{
		ID:                       "focus-work",
		SliceID:                  item.SliceID,
		AssessmentScope:          item.SourceScope,
		AssessmentID:             item.SourceID,
		AssessmentText:           item.TargetText,
		CoverageWorkItemIDs:      []common.CoverageWorkItemID{item.ID},
		EvidenceDeclarationRef:   "definition/dodItem/" + item.SourceID,
		RequiredEvidenceKinds:    []string{rc.EvidenceKindValidation},
		AcceptedStepKinds:        []string{rc.RuntimeBlockKindManagedValidationSession},
		ImplementationWorkPolicy: &policy,
	}

	fileCatalog := r.SemanticManifest.OwnerCatalogs[0].Clone()
	fileCatalog.SelectorRefs = []string{"work-scope"}
	fileCatalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	shellCatalog := inputProtectionProviderCatalog()
	shellCatalog.ValidationSurfaces[0].InputTargetRefs = []string{target}
	shellCatalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)

	manifestObs := rc.RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-work",
		Kind:           rc.PrerequisiteKindManifest,
		OwnerModuleID:  rc.RuntimeModuleFile,
		ContentHash:    fileCatalog.ObservedRevision,
		Dependencies: []rc.RuntimeFreshnessDependency{{
			OwnerModuleID: rc.RuntimeModuleFile,
			Domain:        rc.RuntimeResourceWorkspaceTree,
			SubjectRef:    "workspace_deliverable",
			Revision:      fileCatalog.ObservedRevision,
		}},
	}
	manifestObs.ObservationID = rc.StableRuntimePrerequisiteObservationID(manifestObs)
	scopeRequest, err := rc.CompileRuntimeImplementationWorkScopeRequest(
		ctx.WorkspaceID,
		ctx.RunID,
		"authority-work",
		ctx.DefinitionSeed,
		[]rc.RuntimeDefinitionFocus{focus},
		ctx.Activation.ExecutionContract.ProfileContracts,
		item.ID,
		rc.RuntimeWorkspaceManifestBinding{
			Manifest:    rc.WorkspaceManifest{Revision: fileCatalog.ObservedRevision, TargetRefs: manifestTargets},
			Observation: manifestObs,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := rc.RuntimeTargetSelection{
		PrerequisiteID:        scopeRequest.RequestID,
		OwnerModuleID:         policy.OwnerModuleID,
		TargetRole:            "implementation_work",
		SelectorRef:           policy.TargetSelectorRef,
		SelectorOwnerRef:      policy.TargetSelectorOwnerRef,
		SelectorSubjectRef:    policy.TargetSelectorSubjectRef,
		SelectorCandidateRefs: policy.TargetSelectorCandidateRefs,
		Disposition:           rc.RuntimeTargetSelectionScope,
		TargetRefs:            manifestTargets,
		CandidateTargetRefs:   manifestTargets,
		CandidateCount:        len(manifestTargets),
		ObservedRevision:      fileCatalog.ObservedRevision,
	}
	selection.SnapshotID = rc.StableRuntimeTargetSelectionSnapshotID(selection)
	selectionObs := manifestObs
	selectionObs.PrerequisiteID = scopeRequest.RequestID
	selectionObs.Kind = rc.PrerequisiteKindTargetSelection
	selectionObs.ContentHash = selection.SnapshotID
	selectionObs.ObservationID = rc.StableRuntimePrerequisiteObservationID(selectionObs)

	priorScope := rc.RuntimeImplementationWorkScopeObservation{
		Request:     scopeRequest,
		Selection:   selection,
		Observation: selectionObs,
		Content: &rc.RuntimeImplementationWorkContent{
			ProtocolRef:      rc.RuntimeImplementationWorkContentV1,
			ManifestRevision: selection.ObservedRevision,
			Snapshots:        []rc.TargetSnapshot{snap(target, beforeBody), snap(mainTarget, mainBody)},
		},
	}
	priorRound := rc.RuntimeImplementationWorkRound{
		WorkspaceID:             ctx.WorkspaceID,
		RunID:                   ctx.RunID,
		StartupInputHash:        ctx.Definition.StartupInputHash,
		DefinitionSeedHash:      ctx.DefinitionSeed.Hash,
		CoverageWorkItemID:      item.ID,
		PolicyHash:              rc.StableRuntimeImplementationWorkPolicyHash(policy),
		Ordinal:                 1,
		DispatchID:              "dispatch-1",
		CompletedReceiptSetHash: "receipt-set-1",
		ScopeRequest:            scopeRequest,
		ScopeObservation:        &priorScope,
	}
	priorRound.Hash = rc.StableRuntimeImplementationWorkRoundHash(priorRound)
	priorIntent := &rc.RuntimeImplementationIntent{
		DeclarationRef:          focus.EvidenceDeclarationRef,
		ContentHash:             priorScope.Content.Hash(),
		SelectedRequirementRefs: []string{"req-1"},
	}
	priorRound.WorkingBinding = &rc.RuntimeImplementationWorkingBinding{
		IntentBinding: &rc.RuntimeImplementationWorkIntentBinding{
			Source: rc.RuntimeImplementationWorkPurposeChoice{
				CoverageWorkItemID:   item.ID,
				ScopeRequestID:       scopeRequest.RequestID,
				SemanticManifestHash: "prior-manifest-hash",
				LogicalRequestID:     "prior-logical-req",
				DocumentHash:         "prior-doc-hash",
				Purpose:              rc.RuntimeImplementationWorkPurposeEffect,
			},
			Intent: priorIntent,
		},
		Proposals: []rc.RuntimeSemanticProofProposal{{
			CoverageWorkItemID: item.ID,
			Target: rc.RuntimeOperationTargetAuthorization{
				Policy:               rc.RuntimeOperationTargetPolicyExistingOwnerScope,
				OwnerScopeTargetRefs: []string{target},
			},
		}},
	}

	scope := rc.RuntimeImplementationWorkScopeObservation{
		Request:     scopeRequest,
		Selection:   selection,
		Observation: selectionObs,
		Content: &rc.RuntimeImplementationWorkContent{
			ProtocolRef:        rc.RuntimeImplementationWorkContentV1,
			ManifestRevision:   selection.ObservedRevision,
			ExcludedTargetRefs: []string{target},
			Snapshots:          []rc.TargetSnapshot{snap(mainTarget, mainBody)},
			ReviewSnapshots:    []rc.TargetSnapshot{snap(target, afterBody)},
		},
	}
	manifest, err := rc.CompileRuntimeSemanticImplementationPurposeManifest(
		ctx.WorkspaceID,
		ctx.RunID,
		ctx.Definition,
		ctx.DefinitionSeed,
		[]rc.RuntimeDefinitionFocus{focus},
		ctx.Activation.ExecutionContract,
		[]rc.RuntimeOperationAdmissibility{fileCatalog, shellCatalog},
		rc.RuntimeImplementationWorkScopePreview{Request: scopeRequest, Observation: &scope},
		[]rc.RuntimeImplementationWorkRound{priorRound},
	)
	if err != nil {
		t.Fatal(err)
	}
	r.SemanticManifest = manifest
	ctx.PlanStep = rc.RuntimeSemanticPlanStepCoverageOrder
	ctx.BatchID = ""
	ctx.BatchSequence = 0
	ctx.BatchCoverageWorkItemIDs = nil

	beforeHash := rc.RuntimeValidationInputContentHash(beforeBody)
	afterHash := rc.RuntimeValidationInputContentHash(afterBody)
	mainHash := rc.RuntimeValidationInputContentHash(mainBody)
	op := rc.RuntimeOperation{
		ID:         "edit-input-round-1",
		Kind:       rc.StepKindFileEdit,
		TargetRefs: []string{target},
		Parameters: []rc.OperationParameter{
			{Name: "oldText", String: &beforeBody},
			{Name: "newText", String: &afterBody},
		},
	}
	ctx.WorkContext = &rc.RuntimeSemanticWorkContext{
		Content: scope.Content.Clone(),
		PriorRounds: []rc.RuntimeSemanticWorkRound{{
			Ordinal:        1,
			DispatchID:     priorRound.DispatchID,
			ReceiptSetHash: priorRound.CompletedReceiptSetHash,
			Intent:         priorIntent.Clone(),
			Targets: []rc.RuntimeSemanticWorkTarget{{
				OperationID: op.ID,
				TargetRefs:  []string{target},
				ReceiptRef:  "runtime-receipt/exec-round-1",
				ReceiptHash: "receipt-hash-1",
			}},
		}},
	}
	ctx.CoverageProgress, err = rc.BuildRuntimeSemanticCoverageProgress(
		*ctx,
		manifest,
		rc.RuntimeFacts{
			WorkspaceID:        ctx.WorkspaceID,
			RunID:              ctx.RunID,
			DefinitionRevision: ctx.Definition.Revision,
			DefinitionSeed:     ctx.DefinitionSeed,
			EvidenceRevision:   "evidence-1",
			Coverage:           rc.InitialRuntimeCoverageFacts(ctx.DefinitionSeed.CoverageItems),
		},
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx.ExecutionHistory, err = rc.BuildRuntimeSemanticExecutionHistory(*ctx, manifest, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := rc.BuildRuntimeExecutionBudgetView(ctx.WorkspaceID, ctx.RunID, 1, rc.RuntimeExecutionBudgetLedger{Policy: rc.DefaultRuntimeExecutionBudgetPolicy()}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx.ExecutionBudget, err = rc.BuildRuntimeSemanticExecutionBudget(*ctx, manifest, budget, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.StablePrefixHash = rc.StableRuntimeCandidatePrefixHashWithExecutionContract(ctx.Definition, ctx.Activation.ExecutionContract)
	if r.RequestIdentity.LogicalRequestID == "" {
		r.RequestIdentity.LogicalRequestID = "frozen-logical"
	}
	expectedDoc := rc.RuntimeExpectedDocumentContract{
		SchemaVersion:           rc.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:            r.DocumentKind,
		PlanStep:                ctx.PlanStep,
		SemanticContextEncoding: rc.RuntimeSemanticContextEncoding,
		ManifestHash:            manifest.Hash,
		DefinitionSeedHash:      manifest.DefinitionSeedHash,
		LogicalRequestID:        r.RequestIdentity.LogicalRequestID,
		StablePrefixHash:        r.StablePrefixHash,
		MaxOutputTokens:         ctx.ProviderCeiling.EffectiveMaxOutputTokens,
		MaxRecoveryAttempts:     1,
	}
	expectedDoc.Hash = rc.StableRuntimeExpectedDocumentHash(expectedDoc)
	r.ExpectedDocument = &expectedDoc

	preimage := rc.RuntimeValidationInputReviewFile{TargetRef: target, ContentHash: beforeHash, Content: beforeBody}
	ctx.ChangeReview = &rc.RuntimeChangeReviewContext{
		WorkspaceID:        ctx.WorkspaceID,
		RunID:              ctx.RunID,
		DefinitionRevision: ctx.Definition.Revision,
		Current: []rc.RuntimeValidationInputReviewFile{
			{TargetRef: mainTarget, ContentHash: mainHash, Content: mainBody},
			{TargetRef: target, ContentHash: afterHash, Content: afterBody},
		},
		Changes: []rc.RuntimeChangeReviewOperation{{
			Operation:     op,
			OperationHash: rc.StableRuntimeOperationHash(op),
			ExecutionKey:  "exec-round-1",
			Fence:         1,
			DispatchID:    priorRound.DispatchID,
			OperationID:   op.ID,
			OwnerModuleID: rc.RuntimeModuleFile,
			ReceiptRef:    "runtime-receipt/exec-round-1",
			ReceiptHash:   "receipt-hash-1",
			TargetRef:     target,
			BeforeHash:    beforeHash,
			AfterHash:     afterHash,
			Baseline:      &preimage,
		}},
	}

	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatalf("runtimeProviderRequest with ReviewSnapshots: %v", err)
	}
	if !strings.Contains(p.PromptContext.Sections[3].Content, `"reviewSnapshots":`) || !strings.Contains(p.PromptContext.Sections[3].Content, `"reviewSnapshotObservations":`) {
		t.Fatalf("C3 missing reviewSnapshots/reviewSnapshotObservations: %s", p.PromptContext.Sections[3].Content)
	}
	var tail runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &tail); err != nil {
		t.Fatal(err)
	}
	foundReviewRef := false
	for _, ref := range tail.ChangeReviewFileRefs {
		if ref.Source == "C3.workContext.content.reviewSnapshots[0].Content" {
			foundReviewRef = true
		}
	}
	if !foundReviewRef {
		t.Fatalf("C4 ChangeReviewFileRefs = %+v, want ref to C3.workContext.content.reviewSnapshots[0].Content", tail.ChangeReviewFileRefs)
	}
	if err := validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, p.ContextBudget, p.PromptContext); err != nil {
		t.Fatalf("validateRuntimeSemanticContextIntegrity rejected ReviewSnapshots ChangeReview: %v", err)
	}
	frozen, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = frozen, ref
	original, _ := json.Marshal(sectionMessages(p, false))
	replay, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(sectionMessages(replay, false))
	if !bytes.Equal(original, actual) {
		t.Fatal("freeze/replay altered frozen ReviewSnapshots ChangeReview provider messages")
	}
}
