package llm

import (
	"encoding/json"
	"fmt"
	"reflect"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateWorkFileRef struct {
	Snapshot int    `json:"snapshot"`
	Source   string `json:"source"`
}

const runtimeCandidateFileReferenceRule = "workFileRefs replace only empty C4.implementationWorkContent.snapshots[snapshot].Content with the exact source path in this request's C3 generation corpus or operationContracts inputRevision input files. Target, hash and complete bytes must match. Expand workFileRefs first, then changeReviewFileRefs (which may reference the expanded C4 work snapshots), then changeReviewValueRefs. No external lookup or cycles are allowed. Restore all original inputs and operations before proposing changes; references grant no execution or validation credit."

// Sources are enumerated from the closed provider projection, never resolved
// through arbitrary JSON paths, external state or another request. C3 bodies
// remain inline; C4 work can reference C3, and review can reference C3 or work.
// This ordering is acyclic even when a current review body and a frozen work
// body happen to be identical to an original input-review preimage.
func runtimeCandidateAuthorityBodySources(output runtimeCandidateOutputContractProjection, contexts []runtimeValidationGenerationContextProjection) []runtimeChangeReviewBodySource {
	sources := changeReviewBodySources(nil, "", contexts, "C3.context.validationGenerationContexts")
	for oi, operation := range output.OperationContracts {
		r := operation.InputRevision
		if r == nil {
			continue
		}
		path := fmt.Sprintf("C3.outputContract.operationContracts[%d].inputRevision", oi)
		if r.Observation != nil {
			path += ".ownerObservation.Files"
		} else {
			path += ".Binding.InputReview.Files"
		}
		for fi, f := range r.inputFiles() {
			sources = append(sources, runtimeChangeReviewBodySource{fmt.Sprintf("%s[%d].content", path, fi), f})
		}
	}
	return sources
}

// This is source/projection integrity, not another semantic review of the
// accepted input report. Bind the complete original recipe, report, files and
// source identities to the sealed manifest before any of them can be reused.
func runtimeCandidateInputRevisionSourcesMatch(manifest rc.RuntimeCandidateContractManifest, actual []runtimeCandidateOperationContractProjection) bool {
	// The sealed manifest already owns membership and canonical ordering. Do
	// not rebuild its unrelated owner catalogs to compare this narrow view.
	operations := manifest.Operations
	if len(operations) != len(actual) {
		return false
	}
	for i, operation := range operations {
		if actual[i].Kind != operation.Kind || actual[i].OwnerModuleID != operation.OwnerModuleID || !reflect.DeepEqual(actual[i].InputRevision, inputRevisionForProvider(operation.InputRevision)) {
			return false
		}
	}
	return true
}

func compactRuntimeCandidateFiles(tail *runtimeCandidateExecutionTailProjection, sources []runtimeChangeReviewBodySource) {
	// Capture the complete work sources before replacing their bodies. The
	// decoder restores work before using it as a review source.
	reviewSources := append(changeReviewBodySources(candidateReviewWorkSource(tail), "C4.implementationWorkContent", nil, ""), sources...)
	tail.ChangeReviewValueRefs = compactChangeReviewValues(tail.ChangeReview)
	tail.ChangeReviewFileRefs = compactChangeReviewFiles(tail.ChangeReview, reviewSources)
	compactRuntimeCandidateWorkFiles(tail, sources)
}

func candidateReviewWorkSource(tail *runtimeCandidateExecutionTailProjection) *rc.RuntimeImplementationWorkContent {
	if tail.ImplementationWorkContent == nil {
		return nil
	}
	return &rc.RuntimeImplementationWorkContent{Snapshots: tail.ImplementationWorkContent.Snapshots}
}

func compactRuntimeCandidateWorkFiles(tail *runtimeCandidateExecutionTailProjection, sources []runtimeChangeReviewBodySource) {
	if tail.ImplementationWorkContent == nil {
		return
	}
	for si := range tail.ImplementationWorkContent.Snapshots {
		s := &tail.ImplementationWorkContent.Snapshots[si]
		if s.Content == "" {
			continue
		}
		for _, source := range sources {
			f := source.file
			if s.TargetRef == f.TargetRef && s.ContentHash == f.ContentHash && s.Content == f.Content {
				tail.WorkFileRefs = append(tail.WorkFileRefs, runtimeCandidateWorkFileRef{si, source.ref})
				s.Content = ""
				break
			}
		}
	}
	if len(tail.WorkFileRefs) != 0 {
		tail.ReferenceRule = runtimeCandidateFileReferenceRule
	}
}

func expandRuntimeCandidateWorkFiles(tail *runtimeCandidateExecutionTailProjection, sources []runtimeChangeReviewBodySource) error {
	if len(tail.WorkFileRefs) == 0 {
		if tail.ReferenceRule != "" {
			return fmt.Errorf("unexpected work file reference rule")
		}
		return nil
	}
	if tail.ReferenceRule != runtimeCandidateFileReferenceRule || tail.ImplementationWorkContent == nil {
		return fmt.Errorf("work file references lack exact rule/source")
	}
	seen := map[int]bool{}
	for _, ref := range tail.WorkFileRefs {
		if ref.Snapshot < 0 || ref.Snapshot >= len(tail.ImplementationWorkContent.Snapshots) || seen[ref.Snapshot] {
			return fmt.Errorf("work file reference duplicate or outside request")
		}
		seen[ref.Snapshot] = true
		s := &tail.ImplementationWorkContent.Snapshots[ref.Snapshot]
		f, err := runtimeFileReferenceSource(ref.Source, sources)
		if err != nil {
			return err
		}
		if s.Content != "" || s.TargetRef != f.TargetRef || s.ContentHash != f.ContentHash {
			return fmt.Errorf("work file reference differs from exact target/hash/content")
		}
		s.Content = f.Content
	}
	return nil
}

func runtimeCandidatePromptBodySources(prompt *models.PromptContext) ([]runtimeChangeReviewBodySource, error) {
	if prompt == nil {
		return nil, fmt.Errorf("file references lack request")
	}
	var authority struct {
		Context        runtimeCandidateAuthorityProjection      `json:"context"`
		OutputContract runtimeCandidateOutputContractProjection `json:"outputContract"`
	}
	count := 0
	for _, s := range prompt.Sections {
		if s.ID != "C3.runtime-authority" {
			continue
		}
		if err := json.Unmarshal([]byte(s.Content), &authority); err != nil {
			return nil, err
		}
		count++
	}
	if count != 1 {
		return nil, fmt.Errorf("file references require unique C3")
	}
	return runtimeCandidateAuthorityBodySources(authority.OutputContract, authority.Context.ValidationGenerationContexts), nil
}

// The renderer, integrity gate, metrics and correction composition all expand
// the same encoding, in the same order, without rebuilding owner authority.
func expandRuntimeCandidateFiles(tail *runtimeCandidateExecutionTailProjection, prompt *models.PromptContext) error {
	var sources []runtimeChangeReviewBodySource
	if len(tail.WorkFileRefs)+len(tail.ChangeReviewFileRefs) != 0 {
		var err error
		sources, err = runtimeCandidatePromptBodySources(prompt)
		if err != nil {
			return err
		}
	}
	if err := expandRuntimeCandidateWorkFiles(tail, sources); err != nil {
		return err
	}
	sources = append(changeReviewBodySources(candidateReviewWorkSource(tail), "C4.implementationWorkContent", nil, ""), sources...)
	if err := expandChangeReviewFiles(tail.ChangeReview, tail.ChangeReviewFileRefs, sources); err != nil {
		return err
	}
	return expandChangeReviewValues(tail.ChangeReview, tail.ChangeReviewValueRefs)
}
