package llm

import (
	"encoding/json"
	"fmt"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateWorkFileRef struct {
	Snapshot   int `json:"snapshot"`
	Generation int `json:"generation"`
	File       int `json:"file"`
}

const runtimeCandidateFileReferenceRule = "workFileRefs replace only empty C4.implementationWorkContent.snapshots[snapshot].Content with exact C3.context.validationGenerationContexts[generation].context.Files[file].content, with identical target/hash. Expand before proposing operations. These references are local to this frozen request and grant no execution or validation credit."

func compactRuntimeCandidateWorkFiles(tail *runtimeCandidateExecutionTailProjection, contexts []runtimeValidationGenerationContextProjection) {
	if tail.ImplementationWorkContent == nil {
		return
	}
	for si := range tail.ImplementationWorkContent.Snapshots {
		s := &tail.ImplementationWorkContent.Snapshots[si]
		if s.Content == "" {
			continue
		}
		found := false
		for gi, g := range contexts {
			if g.Context == nil {
				continue
			}
			for fi, f := range g.Context.Files {
				if s.TargetRef == f.TargetRef && s.ContentHash == f.ContentHash && s.Content == f.Content {
					tail.WorkFileRefs = append(tail.WorkFileRefs, runtimeCandidateWorkFileRef{si, gi, fi})
					s.Content = ""
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if len(tail.WorkFileRefs) != 0 {
		tail.ReferenceRule = runtimeCandidateFileReferenceRule
	}
}

func expandRuntimeCandidateWorkFiles(tail *runtimeCandidateExecutionTailProjection, prompt *models.PromptContext) error {
	if len(tail.WorkFileRefs) == 0 {
		if tail.ReferenceRule != "" {
			return fmt.Errorf("unexpected work file reference rule")
		}
		return nil
	}
	if tail.ReferenceRule != runtimeCandidateFileReferenceRule || tail.ImplementationWorkContent == nil || prompt == nil {
		return fmt.Errorf("work file references lack exact rule/source")
	}
	var authority struct {
		Context runtimeCandidateAuthorityProjection `json:"context"`
	}
	count := 0
	for _, s := range prompt.Sections {
		if s.ID != "C3.runtime-authority" {
			continue
		}
		if err := json.Unmarshal([]byte(s.Content), &authority); err != nil {
			return err
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("work file references require unique C3")
	}
	for _, ref := range tail.WorkFileRefs {
		if ref.Snapshot < 0 || ref.Snapshot >= len(tail.ImplementationWorkContent.Snapshots) || ref.Generation < 0 || ref.Generation >= len(authority.Context.ValidationGenerationContexts) {
			return fmt.Errorf("work file reference outside request")
		}
		g := authority.Context.ValidationGenerationContexts[ref.Generation].Context
		if g == nil || ref.File < 0 || ref.File >= len(g.Files) {
			return fmt.Errorf("work file reference lacks corpus file")
		}
		s, f := &tail.ImplementationWorkContent.Snapshots[ref.Snapshot], g.Files[ref.File]
		if s.Content != "" || f.Content == "" || s.TargetRef != f.TargetRef || s.ContentHash != f.ContentHash || rc.RuntimeValidationInputContentHash(f.Content) != f.ContentHash {
			return fmt.Errorf("work file reference differs from exact target/hash/content")
		}
		s.Content = f.Content
	}
	return nil
}
