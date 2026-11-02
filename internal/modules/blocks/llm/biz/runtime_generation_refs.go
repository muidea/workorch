package llm

import (
	"fmt"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticGenerationFileRef struct {
	Generation       int     `json:"generation"`
	File             int     `json:"file"`
	Snapshot         *int    `json:"snapshot,omitempty"`
	SourceReview     *[3]int `json:"sourceReview,omitempty"`
	SourceGeneration *int    `json:"sourceGeneration,omitempty"`
	SourceFile       int     `json:"sourceFile,omitempty"`
}

func sameGenerationFile(a, b rc.RuntimeValidationInputReviewFile) bool {
	return a.TargetRef == b.TargetRef && a.ContentHash == b.ContentHash && a.Content == b.Content && len(a.Content) >= 256
}
func compactSemanticGenerationFiles(p *runtimeSemanticAuthorityProjection) []runtimeSemanticGenerationFileRef {
	var refs []runtimeSemanticGenerationFileRef
	for gi, g := range p.Manifest.ValidationGenerationContexts {
		if g.Context == nil {
			continue
		}
		for fi := range g.Context.Files {
			f := &g.Context.Files[fi]
			var ref *runtimeSemanticGenerationFileRef
			if p.WorkContext != nil && p.WorkContext.Content != nil {
				for si, s := range p.WorkContext.Content.Snapshots {
					if sameGenerationFile(*f, rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, ContentHash: s.ContentHash, Content: s.Content}) {
						n := si
						ref = &runtimeSemanticGenerationFileRef{Generation: gi, File: fi, Snapshot: &n}
						break
					}
				}
			}
			if ref == nil {
				for pi, proof := range p.Manifest.ProofObligations {
					if proof.ValidationProofRequirement == nil {
						continue
					}
					for ri, review := range proof.ValidationProofRequirement.InputReviews {
						for si, s := range review.Files {
							if sameGenerationFile(*f, s) {
								source := [3]int{pi, ri, si}
								ref = &runtimeSemanticGenerationFileRef{Generation: gi, File: fi, SourceReview: &source}
								break
							}
						}
						if ref != nil {
							break
						}
					}
					if ref != nil {
						break
					}
				}
			}
			if ref == nil {
				for previous := 0; previous < gi; previous++ {
					source := p.Manifest.ValidationGenerationContexts[previous].Context
					if source == nil {
						continue
					}
					for si, s := range source.Files {
						if sameGenerationFile(*f, s) {
							n := previous
							ref = &runtimeSemanticGenerationFileRef{Generation: gi, File: fi, SourceGeneration: &n, SourceFile: si}
							break
						}
					}
					if ref != nil {
						break
					}
				}
			}
			if ref != nil {
				f.Content = ""
				refs = append(refs, *ref)
			}
		}
	}
	return refs
}
func expandSemanticGenerationFiles(p *runtimeSemanticAuthorityProjection, refs []runtimeSemanticGenerationFileRef) error {
	for _, r := range refs {
		contexts := p.Manifest.ValidationGenerationContexts
		if r.Generation < 0 || r.Generation >= len(contexts) || contexts[r.Generation].Context == nil || r.File < 0 || r.File >= len(contexts[r.Generation].Context.Files) {
			return fmt.Errorf("generation reference destination outside local context")
		}
		target := &contexts[r.Generation].Context.Files[r.File]
		var source rc.RuntimeValidationInputReviewFile
		count := 0
		if r.Snapshot != nil {
			count++
			if p.WorkContext == nil || p.WorkContext.Content == nil || *r.Snapshot < 0 || *r.Snapshot >= len(p.WorkContext.Content.Snapshots) {
				return fmt.Errorf("generation snapshot reference outside work scope")
			}
			s := p.WorkContext.Content.Snapshots[*r.Snapshot]
			source = rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, ContentHash: s.ContentHash, Content: s.Content}
		}
		if r.SourceReview != nil {
			count++
			a := *r.SourceReview
			if a[0] < 0 || a[0] >= len(p.Manifest.ProofObligations) {
				return fmt.Errorf("generation review source outside proof scope")
			}
			proof := p.Manifest.ProofObligations[a[0]].ValidationProofRequirement
			if proof == nil || a[1] < 0 || a[1] >= len(proof.InputReviews) || a[2] < 0 || a[2] >= len(proof.InputReviews[a[1]].Files) {
				return fmt.Errorf("generation review file source outside proof scope")
			}
			source = proof.InputReviews[a[1]].Files[a[2]]
		}
		if r.SourceGeneration != nil {
			count++
			n := *r.SourceGeneration
			if n < 0 || n >= r.Generation || contexts[n].Context == nil || r.SourceFile < 0 || r.SourceFile >= len(contexts[n].Context.Files) {
				return fmt.Errorf("generation reference is forward/cyclic or outside local context")
			}
			source = contexts[n].Context.Files[r.SourceFile]
		}
		if count != 1 || target.Content != "" || source.Content == "" || target.TargetRef != source.TargetRef || target.ContentHash != source.ContentHash {
			return fmt.Errorf("generation reference requires one exact original source without mixed content")
		}
		target.Content = source.Content
	}
	return nil
}
