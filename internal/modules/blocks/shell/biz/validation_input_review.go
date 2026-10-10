package shell

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The adapter supplies bytes, never a consistency verdict. Include the entire
// v3 input namespace, or the v4 check's exact package, and module definitions;
// exceeding the bounded review envelope is explicit unavailability, not a
// truncated or inferred review. Delivery code is still proved by execution.
func shellVersionedInputReview(directory string, targets []string, binding runtimecontract.RuntimeValidationProofRequirementBinding) (*runtimecontract.RuntimeValidationInputReviewSubject, string) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "validation_input_review_source_unavailable"
	}
	defer root.Close()
	prefix := "workorch_validation/v3/"
	pkg, scopeErr := shellIsolatedValidationPackage(binding.CheckAttestations)
	if scopeErr != nil {
		return nil, "validation_input_review_mapping_unsupported"
	}
	if pkg != "" {
		prefix = pkg + "/"
		if code := shellIsolatedInputScopeCode(targets, pkg); code != "" {
			return nil, code
		}
		if code := shellIsolatedInputSourceSetCode(root, targets, pkg); code != "" {
			return nil, code
		}
	}
	var refs []string
	for _, target := range normalizeShellStrings(targets) {
		if strings.HasPrefix(target, prefix) || target == "go.mod" || target == "go.sum" || target == "go.work" || target == "go.work.sum" {
			refs = append(refs, target)
		}
	}
	if len(refs) == 0 || len(refs) > runtimecontract.RuntimeValidationInputReviewMaxFiles {
		return nil, "validation_input_review_capacity_exceeded"
	}
	slices.Sort(refs)
	subject := &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}
	if pkg != "" {
		subject.ScopeContractRef, subject.ScopeRequirements = shellIsolatedReviewScopeContractRef, shellIsolatedReviewScopeRequirements
		if binding.CheckAttestations[0].ObservationProtocolRef == shellGoConjunctiveEntrypointObservationProtocolRef {
			subject.ScopeContractRef, subject.ScopeRequirements = shellConjunctiveReviewScopeContractRef, shellConjunctiveReviewScopeRequirements
		}
	}
	remaining := int64(runtimecontract.RuntimeValidationInputReviewMaxBytes)
	for _, ref := range refs {
		info, err := root.Lstat(filepath.FromSlash(ref))
		if err != nil || !info.Mode().IsRegular() {
			return nil, "validation_input_review_source_unavailable"
		}
		if info.Size() > remaining {
			return nil, "validation_input_review_capacity_exceeded"
		}
		file, err := root.Open(filepath.FromSlash(ref))
		if err != nil {
			return nil, "validation_input_review_source_unavailable"
		}
		content, readErr := io.ReadAll(io.LimitReader(file, remaining+1))
		file.Close()
		if readErr != nil || !utf8.Valid(content) {
			return nil, "validation_input_review_source_unavailable"
		}
		if int64(len(content)) > remaining {
			return nil, "validation_input_review_capacity_exceeded"
		}
		remaining -= int64(len(content))
		subject.Files = append(subject.Files, runtimecontract.RuntimeValidationInputReviewFile{TargetRef: ref, Content: string(content), ContentHash: runtimecontract.RuntimeValidationInputContentHash(string(content))})
	}
	for _, check := range binding.CheckAttestations {
		if !slices.Contains(refs, shellVersionedNativeCheckTarget(check.NativeCheckRef)) {
			return nil, "validation_input_review_source_unavailable"
		}
		if pkg != "" && check.SupplementalNativeCheckRef != "" && !slices.Contains(refs, shellSupplementTargetRef(check.NativeCheckRef)) {
			return nil, "validation_input_review_source_unavailable"
		}
	}
	subject.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*subject, binding)
	if subject.ValidateAgainstBinding(binding) != nil {
		return nil, "validation_input_review_source_unavailable"
	}
	return subject, ""
}
