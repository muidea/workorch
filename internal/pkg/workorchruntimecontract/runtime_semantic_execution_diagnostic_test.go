package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSemanticExecutionDiagnosticPreservesFactsWithoutRawOrAuthority(t *testing.T) {
	f := RuntimeFeedback{Domain: RuntimeFeedbackDomainOperation, StableCategory: RuntimeFeedbackCategorySemanticRejection,
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell,
			OriginalClass: "contract", OriginalCode: "validation_failed", OriginalMessage: "OWNER_MESSAGE_NOT_FOR_PROVIDER", Signature: "signature",
			ExpectedRevision: "expected", ObservedRevision: "observed", Truncated: true, Redacted: true, OmittedReason: "OMISSION_REASON_NOT_FOR_PROVIDER",
			ValidationDiagnostic: &ValidationFailureDiagnostic{Class: ValidationDiagnosticBuildFailed, OwnerCode: "build_failed",
				CommandKind: "managed_validation", ExitCode: 2, TimedOut: true, Reason: "REASON_NOT_FOR_PROVIDER",
				RecommendedEvidenceTargetType: "AUTHORITY_NOT_FOR_PROVIDER"},
		}}
	before, _ := json.Marshal(f)
	d, err := ProjectRuntimeSemanticExecutionDiagnostic(&f, RuntimeModuleShell)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || !d.SummaryOnly || d.FeedbackHash != StableRuntimeFeedbackHash(f) || d.OriginalCode != "validation_failed" || d.ExpectedRevision != "expected" || d.ObservedRevision != "observed" || !d.Truncated || !d.Redacted ||
		d.ValidationDiagnostic == nil || d.ValidationDiagnostic.Class != ValidationDiagnosticBuildFailed || d.ValidationDiagnostic.OwnerCode != "build_failed" || d.ValidationDiagnostic.ReportedExitCode != 2 || !d.ValidationDiagnostic.TimedOut {
		t.Fatalf("lost owner diagnostic: %+v", d)
	}
	data, _ := json.Marshal(d)
	if strings.Contains(string(data), "NOT_FOR_PROVIDER") {
		t.Fatal("raw diagnostic or maintenance authority escaped")
	}
	var restored RuntimeSemanticExecutionDiagnostic
	if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(d, &restored) || restored.Validate(RuntimeModuleShell) != nil {
		t.Fatalf("same-version reload changed diagnostic: %v", err)
	}
	clone := d.Clone()
	clone.ValidationDiagnostic.ReportedExitCode = 0
	if d.ValidationDiagnostic.ReportedExitCode != 2 {
		t.Fatal("clone aliases diagnostic")
	}
	if after, _ := json.Marshal(f); string(after) != string(before) {
		t.Fatal("projection rewrote owner original")
	}
	changed := f.CloneOriginal()
	changed.OriginalDiagnostic.OriginalMessage = "another safe omitted message"
	other, err := ProjectRuntimeSemanticExecutionDiagnostic(&changed, RuntimeModuleShell)
	if err != nil || other.FeedbackHash == d.FeedbackHash {
		t.Fatal("omitted original content lost its exact identity")
	}
	for name, mutate := range map[string]func(*RuntimeFeedback){
		"owner":          func(v *RuntimeFeedback) { v.OriginalDiagnostic.OwnerModuleID = RuntimeModuleFile },
		"missing source": func(v *RuntimeFeedback) { v.OriginalDiagnostic = nil },
		"unsafe code":    func(v *RuntimeFeedback) { v.OriginalDiagnostic.OriginalCode = "https://private.example" },
		"unsafe command": func(v *RuntimeFeedback) { v.OriginalDiagnostic.ValidationDiagnostic.CommandKind = "password=secret" },
		"raw preview":    func(v *RuntimeFeedback) { v.OriginalDiagnostic.ValidationDiagnostic.StdoutPreview = "raw" },
	} {
		t.Run(name, func(t *testing.T) {
			value := f.CloneOriginal()
			mutate(&value)
			if _, err := ProjectRuntimeSemanticExecutionDiagnostic(&value, RuntimeModuleShell); err == nil {
				t.Fatal("invalid source entered provider observation")
			}
		})
	}
	f.OriginalDiagnostic.ValidationDiagnostic = nil
	d, err = ProjectRuntimeSemanticExecutionDiagnostic(&f, RuntimeModuleShell)
	if err != nil || d.ValidationDiagnostic != nil {
		t.Fatal("missing process diagnostic became a zero-exit observation")
	}
	if missing, err := ProjectRuntimeSemanticExecutionDiagnostic(nil, RuntimeModuleShell); err != nil || missing != nil {
		t.Fatal("missing feedback became an invented diagnostic")
	}
}
