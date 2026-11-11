package shell

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Execution provenance belongs to the capability owner, not the input semantic
// reviewer. Read exact test bytes on the isolated operation snapshot. The review
// envelope is per applicable subject; it is not a cumulative project byte cap.
// The existing native-source count, per-file and actual execution bounds remain.
func shellProjectExecutionSources(directory string, targets []string) ([]rc.RuntimeValidationDiagnosticSource, []rc.RuntimeValidationInputReviewFile, string) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, "validation_execution_sources_unavailable"
	}
	defer root.Close()
	var files []rc.RuntimeValidationInputReviewFile
	for _, ref := range targets {
		if ref != "go.mod" && !strings.HasSuffix(ref, "_test.go") {
			continue
		}
		info, err := root.Lstat(filepath.FromSlash(ref))
		if err != nil || !info.Mode().IsRegular() {
			return nil, nil, "validation_execution_sources_unavailable"
		}
		if info.Size() > rc.RuntimeValidationDiagnosticSourceMaxBytes {
			return nil, nil, "validation_execution_source_capacity_exceeded"
		}
		file, err := root.Open(filepath.FromSlash(ref))
		if err != nil {
			return nil, nil, "validation_execution_sources_unavailable"
		}
		body, err := io.ReadAll(io.LimitReader(file, rc.RuntimeValidationDiagnosticSourceMaxBytes+1))
		file.Close()
		if err != nil || !utf8.Valid(body) {
			return nil, nil, "validation_execution_sources_unavailable"
		}
		if len(body) > rc.RuntimeValidationDiagnosticSourceMaxBytes {
			return nil, nil, "validation_execution_source_capacity_exceeded"
		}
		files = append(files, rc.RuntimeValidationInputReviewFile{TargetRef: ref, Content: string(body), ContentHash: rc.RuntimeValidationInputContentHash(string(body))})
	}
	sources, code := shellProjectDiagnosticSourceInventory(files)
	if code != "" {
		return nil, nil, code
	}
	for i := range sources {
		for _, file := range files {
			if sources[i].InputTargetRef == file.TargetRef {
				sources[i].InputContentHash = file.ContentHash
			}
		}
	}
	return sources, files, ""
}

// Only sources actually implicated by an original failure are applicable to
// its new independent input review. Exact bodies already in the sealed subject
// are not repeated; all other implicated bodies must fit the existing envelope.
func shellProjectFailedSourceInputs(directory string, sources []rc.RuntimeValidationDiagnosticSource, stages []shellProjectStageFact, reviewed []rc.RuntimeValidationInputReviewFile) ([]rc.RuntimeValidationInputReviewFile, string) {
	var refs []string
	for _, stage := range stages {
		for _, source := range sources {
			if !slices.Contains(stage.FailedSourceRefs, source.SourceRef) || slices.Contains(refs, source.InputTargetRef) {
				continue
			}
			included := false
			for _, file := range reviewed {
				included = included || file.TargetRef == source.InputTargetRef && file.ContentHash == source.InputContentHash
			}
			if !included {
				refs = append(refs, source.InputTargetRef)
			}
		}
	}
	slices.Sort(refs)
	var inputs []rc.RuntimeValidationInputReviewFile
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "validation_failure_input_snapshot_unavailable"
	}
	defer root.Close()
	remaining := rc.RuntimeValidationDiagnosticSourceMaxBytes
	for _, ref := range refs {
		file, err := root.Open(filepath.FromSlash(ref))
		if err != nil {
			return nil, "validation_failure_input_snapshot_unavailable"
		}
		body, err := io.ReadAll(io.LimitReader(file, int64(remaining)+1))
		file.Close()
		if err != nil || !utf8.Valid(body) {
			return nil, "validation_failure_input_snapshot_unavailable"
		}
		if len(body) > remaining {
			return nil, "validation_failure_input_capacity_exceeded"
		}
		remaining -= len(body)
		input := rc.RuntimeValidationInputReviewFile{TargetRef: ref, Content: string(body), ContentHash: rc.RuntimeValidationInputContentHash(string(body))}
		for _, source := range sources {
			if source.InputTargetRef == ref && source.InputContentHash != input.ContentHash {
				return nil, "validation_failure_input_changed"
			}
		}
		inputs = append(inputs, input)
	}
	return inputs, ""
}
