package llm

import (
	"fmt"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Closed, same-request references only; these are a wire encoding, not owner
// receipt lookup or an authority cache. Expansion is checked before submission.
type runtimeChangeReviewFileRef struct {
	Current int    `json:"current"`
	Source  string `json:"source"`
}
type runtimeChangeReviewBodySource struct {
	ref  string
	file rc.RuntimeValidationInputReviewFile
}

const runtimeChangeReviewReferenceRule = "changeReviewFileRefs replace only empty changeReview.current[current].content with the exact same-request source path. Target and contentHash must match; expand all references before review. No external lookup, summary, authority or credit is implied."

func changeReviewBodySources(work *rc.RuntimeImplementationWorkContent, workPath string, contexts []runtimeValidationGenerationContextProjection, generationPath string) []runtimeChangeReviewBodySource {
	var ret []runtimeChangeReviewBodySource
	if work != nil {
		for i, s := range work.Snapshots {
			ret = append(ret, runtimeChangeReviewBodySource{fmt.Sprintf("%s.snapshots[%d].Content", workPath, i), rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, ContentHash: s.ContentHash, Content: s.Content}})
		}
	}
	for gi, g := range contexts {
		if g.Context == nil {
			continue
		}
		for fi, f := range g.Context.Files {
			ret = append(ret, runtimeChangeReviewBodySource{fmt.Sprintf("%s[%d].context.Files[%d].content", generationPath, gi, fi), f})
		}
	}
	return ret
}
func compactChangeReviewFiles(c *rc.RuntimeChangeReviewContext, sources []runtimeChangeReviewBodySource) []runtimeChangeReviewFileRef {
	if c == nil {
		return nil
	}
	var refs []runtimeChangeReviewFileRef
	for i := range c.Current {
		f := &c.Current[i]
		if f.Content == "" {
			continue
		}
		for _, s := range sources {
			if *f == s.file {
				refs = append(refs, runtimeChangeReviewFileRef{i, s.ref})
				f.Content = ""
				break
			}
		}
	}
	return refs
}
func expandChangeReviewFiles(c *rc.RuntimeChangeReviewContext, refs []runtimeChangeReviewFileRef, sources []runtimeChangeReviewBodySource) error {
	seen := map[int]bool{}
	for _, ref := range refs {
		if c == nil || ref.Current < 0 || ref.Current >= len(c.Current) || seen[ref.Current] {
			return fmt.Errorf("change review reference is duplicate or outside request")
		}
		seen[ref.Current] = true
		f := &c.Current[ref.Current]
		source, err := runtimeFileReferenceSource(ref.Source, sources)
		if err != nil {
			return err
		}
		if f.Content != "" || f.TargetRef != source.TargetRef || f.ContentHash != source.ContentHash {
			return fmt.Errorf("change review reference differs from exact source")
		}
		f.Content = source.Content
	}
	return nil
}

func runtimeFileReferenceSource(ref string, sources []runtimeChangeReviewBodySource) (rc.RuntimeValidationInputReviewFile, error) {
	var file rc.RuntimeValidationInputReviewFile
	found := false
	for _, source := range sources {
		if ref != source.ref {
			continue
		}
		if found || source.file.Content == "" || rc.RuntimeValidationInputContentHash(source.file.Content) != source.file.ContentHash {
			return file, fmt.Errorf("file reference source is duplicate or differs from exact content/hash")
		}
		file, found = source.file, true
	}
	if !found {
		return file, fmt.Errorf("file reference source unavailable in request")
	}
	return file, nil
}
