package llm

import (
	"encoding/json"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProviderValidationRejectsMalformedDiagnosticExtension(t *testing.T) {
	for name, mutate := range map[string]func(*runtimecontract.RuntimeValidationObservation){
		"protocol": func(o *runtimecontract.RuntimeValidationObservation) { o.FailureDetailProtocolRef = "private-secret" },
		"code": func(o *runtimecontract.RuntimeValidationObservation) {
			o.CheckFailures[0].ReportedAssertion.Code = "private-secret"
		},
		"expected": func(o *runtimecontract.RuntimeValidationObservation) {
			o.CheckFailures[0].ReportedAssertion.ExpectedShape = "private-secret"
		},
		"observed": func(o *runtimecontract.RuntimeValidationObservation) {
			o.CheckFailures[0].ReportedAssertion.ObservedShape = "private-secret"
		},
		"comparison": func(o *runtimecontract.RuntimeValidationObservation) {
			o.CheckFailures[0].ComparisonHash = "private-secret"
		},
		"unversioned": func(o *runtimecontract.RuntimeValidationObservation) { o.FailureDetailProtocolRef = "" },
	} {
		t.Run(name, func(t *testing.T) {
			o := runtimecontract.RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: "owner/checks/v1", FailedChecks: []string{"check"},
				FailureDetailProtocolRef: runtimecontract.RuntimeValidationFailureDetailProtocolV2,
				CheckFailures: []runtimecontract.RuntimeValidationCheckFailure{{CheckRef: "check", DiagnosticHash: strings.Repeat("a", 64), ComparisonHash: strings.Repeat("c", 64),
					ReportedAssertion: runtimecontract.RuntimeValidationReportedAssertion{Code: "shape_mismatch", ExpectedShape: "object", ObservedShape: "array"}}}}
			mutate(&o)
			projected := runtimecontract.ProjectRuntimeValidationExecutionFeedback(&o)
			encoded, err := json.Marshal(projected)
			if err != nil || strings.Contains(string(encoded), "private-secret") || len(projected.CheckFailures) != 0 || !projected.FailureDetailsIncomplete || len(projected.FailedChecks) != 1 {
				t.Fatal("malformed diagnostic leaked to provider or disguised a failed check")
			}
		})
	}
}
