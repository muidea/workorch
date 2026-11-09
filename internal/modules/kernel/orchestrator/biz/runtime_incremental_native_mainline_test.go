package orchestrator

import (
	"regexp"
	"strings"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// No test entry is prewired. Real owners carry structural maintenance,
// helper-only partial effects and new semantic conflicts across multiple files,
// then the native/build/behavior failure sequence and final acceptance.
func TestDriveRuntimeIncrementalNativeMaintenanceAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol, DynamicProjectObservations: true, SequentialProjectFailures: true, NativeProjectSources: true, IncrementalNativeSources: true}
	baseline := runValidationInputReviewMainline(t, "input_revision_after_failure", "", options)
	for _, cut := range []string{"after_partial_native_helper", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "input_revision_after_failure", cut, options); calls != baseline {
				t.Fatalf("partial maintenance recovery repeated a consumed provider request: %d != %d", calls, baseline)
			}
		})
	}
}

func incrementalNativeIncomplete(body string) bool {
	for _, stage := range []string{"legacy", "bare", "helper"} {
		if strings.Contains(body, "// native-adoption:"+stage) {
			return true
		}
	}
	return false
}

func incrementalNativeInput(t *testing.T, body, stage string) string {
	t.Helper()
	return incrementalNativeFromWired(nativeSourceChannelMainlineInput(t, body), stage)
}

func incrementalNativeFromWired(wired, stage string) string {
	wired = strings.TrimSuffix(wired, "\n// native-adoption:wired\n")
	const call = " if managedNativeSource(t){return}"
	body := wired
	switch stage {
	case "legacy", "bare":
		body = wired[:strings.Index(wired, "\nfunc managedNativeSource(")]
		body = strings.ReplaceAll(body, call, "") + "\nvar _ = os.Getenv\n"
		if stage == "legacy" {
			pattern := regexp.MustCompile(`(func Test[[:alnum:]_]+\(t \*testing.T\) \{)`)
			body = pattern.ReplaceAllString(body, "$1\n if legacyNativeSource(t){return}")
			body += `
func legacyNativeSource(t *testing.T)bool{
 t.Helper()
 exe,err:=os.Executable();if err!=nil{t.Fatal(err)}
 if err:=exec.Command(exe,"-test.run=^"+t.Name()+"$").Run();err!=nil{t.Fatal(err)}
 return true
}
`
		}
	case "helper":
		body = strings.ReplaceAll(body, call, "")
	}
	return body + "\n// native-adoption:" + stage + "\n"
}

func incrementalNativeRevisionTarget(v *rc.RuntimeValidationInputRevision) string {
	if v == nil {
		return ""
	}
	for _, target := range v.InputTargetRefs {
		for _, file := range v.InputFiles() {
			if file.TargetRef == target && incrementalNativeIncomplete(file.Content) {
				return target
			}
		}
	}
	return ""
}

func incrementalNativeSemanticDocument(d *rc.RuntimeSemanticPlanDocument, c llmevents.RequestRuntimeCandidateCommand) {
	for _, o := range c.SemanticManifest.ProofObligations {
		if w := o.ImplementationWork; w != nil && !w.PurposeChoiceOnly && d.PlanStep == rc.RuntimeSemanticPlanStepProofBatch {
			if target := incrementalNativeRevisionTarget(w.Scope.Request.InputRevision); target != "" {
				d.Proposals[0].Target.OwnerScopeTargetRefs = []string{target}
			}
		}
	}
}

func incrementalNativeCandidate(t *testing.T, c llmevents.RequestRuntimeCandidateCommand, finals map[string]string) (rc.RuntimeCandidate, bool) {
	t.Helper()
	v := c.CandidateManifest.ImplementationWorkingBinding.InputRevision
	target := incrementalNativeRevisionTarget(v)
	if target == "" {
		return rc.RuntimeCandidate{}, false
	}
	for _, f := range v.InputFiles() {
		if f.TargetRef != target {
			continue
		}
		stage := "wired"
		if strings.Contains(f.Content, "// native-adoption:legacy") {
			stage = "bare"
		} else if strings.Contains(f.Content, "// native-adoption:bare") {
			stage = "helper"
		}
		if finals[target] == "" {
			t.Fatal("maintenance did not bind an actual original input", target)
		}
		before, after := f.Content, incrementalNativeFromWired(finals[target], stage)
		return rc.RuntimeCandidate{StateChange: &rc.StateChangeCandidate{Operations: []rc.StateChangingOperation{{Operation: rc.RuntimeOperation{ID: "adopt-native-input", Kind: rc.StepKindFileEdit, TargetRefs: []string{target}, RequiredOperationRef: c.CandidateManifest.RequiredOperationSequence[0].RequirementID, Parameters: []rc.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}}}}}}, true
	}
	t.Fatal("exact maintenance input was not supplied")
	return rc.RuntimeCandidate{}, false
}
