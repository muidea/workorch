package integration_test

import (
	"fmt"
	"path"
	"slices"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The scripted independent reviewer recognizes only the untouched original
// production assertions and its previously proposed exact CWI input bytes.
// This fixture does not infer a consistency verdict from names or execution.
func (p *implementationWorkFullflowProvider) reviewProjectDiagnosticSource(source runtimecontract.RuntimeValidationDiagnosticSource, sources []runtimecontract.RuntimeValidationDiagnosticSource, files []runtimecontract.RuntimeValidationInputReviewFile, refs []string) (runtimecontract.RuntimeValidationDiagnosticSourceReview, error) {
	content := ""
	for _, file := range files {
		if file.TargetRef == source.InputTargetRef {
			content = file.Content
		}
	}
	if content == "" || source.SourceRef == "" {
		return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("diagnostic source has no exact original input")
	}
	native := path.Base(source.NativeCaseRef)
	switch {
	case source.InputTargetRef == "internal/logic/logic_test.go":
		if content != implementationWorkFullflowProjectFiles()[source.InputTargetRef] || !slices.Contains([]string{"TestDeliveredValueProduction", "TestDeliveredValueConcurrent"}, native) || !strings.HasPrefix(source.NativeCaseRef, "example.com/workorch-fullflow/internal/logic/") {
			return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("production project test changed or unknown")
		}
	case strings.HasPrefix(source.InputTargetRef, "workorch_validation/"):
		criterion, found := p.inputCriteria[source.InputTargetRef]
		if !found {
			return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("project native has no original proposed criterion")
		}
		var natives []string
		for _, member := range sources {
			if member.InputTargetRef == source.InputTargetRef {
				natives = append(natives, path.Base(member.NativeCaseRef))
			}
		}
		// Inventory order is source-hash order, whereas input declaration order is
		// original owner mapping order; compare the independently inspected member
		// bodies without rewriting either input.
		if !strings.Contains(content, fmt.Sprintf("func %s(t *testing.T) { verifyDelivery(t, %q) }", native, criterion)) {
			return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("project native does not preserve original criterion")
		}
		tail := implementationWorkFullflowInput(source.InputTargetRef, nil, criterion)
		prefix := "package checks\nimport (\"testing\"; \"os\"; \"os/exec\"; \"path/filepath\"; \"strings\"; \"example.com/workorch-fullflow/internal/logic\")\n"
		declarations := ""
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "func Test") {
				declarations += line + "\n"
			}
		}
		if len(natives) == 0 || strings.Replace(content, declarations, "", 1) != tail || !strings.HasPrefix(content, prefix) {
			return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("unknown project native/helper/fixture bytes")
		}
	default:
		return runtimecontract.RuntimeValidationDiagnosticSourceReview{}, fmt.Errorf("unknown project test source; cannot guess compatible semantics")
	}
	return runtimecontract.RuntimeValidationDiagnosticSourceReview{SourceRef: source.SourceRef, Conclusion: runtimecontract.RuntimeValidationInputConsistent, Reason: "Exact original assertion/helper bytes independently checked: real production value=4/concurrency or original engineering/CLI criterion; compatible additional all-of constraint, no credit or repair authority.", InputTargetRefs: []string{source.InputTargetRef}}, nil
}
