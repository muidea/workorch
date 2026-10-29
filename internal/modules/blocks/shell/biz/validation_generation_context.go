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

// Go-specific selection belongs to the validation adapter, never Core. Observe
// the actual implementation and module definitions, not old write parameters or
// other validation packages. This is generation context, not review or evidence.
func shellValidationGenerationContext(directory string, manifest runtimecontract.WorkspaceManifest, revision string) *runtimecontract.RuntimeValidationGenerationContext {
	c := &runtimecontract.RuntimeValidationGenerationContext{Version: runtimecontract.RuntimeValidationGenerationContextVersion,
		ContractRef: "shell/go-validation-generation-context/v1", ObservedRevision: revision, Outcome: runtimecontract.RuntimeValidationGenerationContextUnavailable}
	var refs []string
	for _, ref := range manifest.TargetRefs {
		if strings.HasPrefix(ref, "workorch_validation/") {
			continue
		}
		if (strings.HasSuffix(ref, ".go") && !strings.HasSuffix(ref, "_test.go")) || ref == "go.mod" || ref == "go.sum" || ref == "go.work" || ref == "go.work.sum" {
			refs = append(refs, ref)
		}
	}
	slices.Sort(refs)
	refs = slices.Compact(refs)
	if len(refs) > runtimecontract.RuntimeValidationGenerationContextMaxFiles {
		c.Outcome = runtimecontract.RuntimeValidationGenerationContextOverCapacity
		return c
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return c
	}
	defer root.Close()
	remaining := int64(runtimecontract.RuntimeValidationGenerationContextMaxBytes)
	var files []runtimecontract.RuntimeValidationInputReviewFile
	for _, ref := range refs {
		info, err := root.Lstat(filepath.FromSlash(ref))
		if err != nil || !info.Mode().IsRegular() {
			return c
		}
		if info.Size() > remaining {
			c.Outcome = runtimecontract.RuntimeValidationGenerationContextOverCapacity
			return c
		}
		f, err := root.Open(filepath.FromSlash(ref))
		if err != nil {
			return c
		}
		content, readErr := io.ReadAll(io.LimitReader(f, remaining+1))
		closeErr := f.Close()
		if readErr != nil || closeErr != nil || !utf8.Valid(content) {
			return c
		}
		if int64(len(content)) > remaining {
			c.Outcome = runtimecontract.RuntimeValidationGenerationContextOverCapacity
			return c
		}
		remaining -= int64(len(content))
		files = append(files, runtimecontract.RuntimeValidationInputReviewFile{TargetRef: ref, Content: string(content), ContentHash: runtimecontract.RuntimeValidationInputContentHash(string(content))})
	}
	c.Files, c.Outcome = files, runtimecontract.RuntimeValidationGenerationContextComplete
	if c.Validate(revision) != nil {
		c.Files, c.Outcome = nil, runtimecontract.RuntimeValidationGenerationContextUnavailable
	}
	return c
}
