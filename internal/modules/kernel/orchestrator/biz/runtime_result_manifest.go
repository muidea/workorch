package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) workspaceDeliverableManifestContent(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	evidenceRefs []string,
	workspace *workspaceModel.Workspace,
	patchDetails []resultPatchDetail,
) string {
	var builder strings.Builder
	builder.WriteString("# Workspace Deliverable Manifest\n\n")
	builder.WriteString(fmt.Sprintf("- Workspace: `%s`\n", run.WorkspaceID))
	builder.WriteString(fmt.Sprintf("- Run: `%s`\n", run.ID))
	builder.WriteString(fmt.Sprintf("- Result: `%s`\n", fmt.Sprintf("%s/result/dod-assessment", run.ID)))
	builder.WriteString(fmt.Sprintf("- Source step: `%s`\n", step.ID))
	if summary, _ := assessment["summary"].(string); strings.TrimSpace(summary) != "" {
		builder.WriteString(fmt.Sprintf("- DoD summary: %s\n", strings.TrimSpace(summary)))
	}
	builder.WriteString("\n## Deliverable Files\n\n")
	files := workspaceDeliverableFilesFromPatchDetails(patchDetails)
	if len(files) == 0 {
		builder.WriteString("- No workspace file deliverables were produced in this run.\n")
	} else {
		builder.WriteString("| Path | Category | Purpose | Size |\n")
		builder.WriteString("| --- | --- | --- | ---: |\n")
		for _, file := range files {
			builder.WriteString(fmt.Sprintf(
				"| `%s` | %s | %s | %d bytes |\n",
				markdownTableCell(file.Path),
				markdownTableCell(file.Category),
				markdownTableCell(artifactPurposeForCategory(file.Category)),
				file.Size,
			))
		}
	}
	writePatchEvidenceSection(&builder, patchDetails)
	builder.WriteString("\n## DoD Relation\n\n")
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		builder.WriteString("| DoD Item | State | Evidence |\n")
		builder.WriteString("| --- | --- | --- |\n")
		for _, item := range items {
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %s |\n",
				markdownTableCell(stringFromMap(item, "item")),
				markdownTableCell(stringFromMap(item, "state")),
				markdownTableCell(strings.Join(evidenceRefsFromValue(item["evidenceRefs"]), ", ")),
			))
		}
	} else {
		builder.WriteString("- No item-level DoD relation was reported by the assessment.\n")
	}
	builder.WriteString("\n## Acceptance Evidence\n\n")
	for _, ref := range evidenceRefs {
		builder.WriteString(fmt.Sprintf("- `%s`\n", ref))
	}
	return strings.TrimSpace(builder.String())
}

func workspaceDeliverableFilesFromPatchDetails(patchDetails []resultPatchDetail) []workspaceDeliverableFile {
	if len(patchDetails) == 0 {
		return nil
	}
	files := make([]workspaceDeliverableFile, 0, len(patchDetails))
	seen := map[string]struct{}{}
	for _, patch := range patchDetails {
		target := strings.TrimSpace(patch.TargetRef)
		if target == "" {
			continue
		}
		target = filepath.ToSlash(filepath.Clean(target))
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		size := int64(len(strings.TrimSpace(patch.Content)))
		if size == 0 {
			size = int64(len(strings.TrimSpace(patch.Diff)))
		}
		files = append(files, workspaceDeliverableFile{
			Path:     target,
			Size:     size,
			Category: artifactCategoryForWorkspaceFile(target),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func writePatchEvidenceSection(builder *strings.Builder, patchDetails []resultPatchDetail) {
	if builder == nil || len(patchDetails) == 0 {
		return
	}
	builder.WriteString("\n## Code Change Evidence\n\n")
	builder.WriteString("| Patch | Type | Target | State | Summary |\n")
	builder.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, patch := range patchDetails {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | %s | `%s` | %s | %s |\n",
			markdownInlineCode(string(patch.ID)),
			markdownTableCell(patch.PatchType),
			markdownInlineCode(patch.TargetRef),
			markdownTableCell(string(patch.State)),
			markdownTableCell(patch.Summary),
		))
	}
	if hasPatchDeliverableContent(patchDetails) {
		builder.WriteString("\n- Full change content is stored in the Code Change Diff artifact.\n")
	}
}

func expectedCompletionArtifactRefs(run *runModel.Run, patchDetails []resultPatchDetail, webSourceDetails []resultWebSourceDetail) []common.ArtifactID {
	if run == nil || run.ID == "" {
		return nil
	}
	ret := []common.ArtifactID{
		common.ArtifactID(fmt.Sprintf("%s/artifact/result-summary", run.ID)),
		common.ArtifactID(fmt.Sprintf("%s/artifact/deliverable-manifest", run.ID)),
	}
	if hasPatchDeliverableContent(patchDetails) {
		ret = append(ret, patchDiffArtifactID(run))
	}
	if len(webSourceDetails) > 0 {
		ret = append(ret, webSourceIndexArtifactID(run))
	}
	return ret
}

func patchDiffArtifactID(run *runModel.Run) common.ArtifactID {
	if run == nil {
		return ""
	}
	return common.ArtifactID(fmt.Sprintf("%s/artifact/code-change-diff", run.ID))
}

func hasPatchDeliverableContent(patchDetails []resultPatchDetail) bool {
	for _, patch := range patchDetails {
		if strings.TrimSpace(patch.Diff) != "" || strings.TrimSpace(patch.Content) != "" {
			return true
		}
	}
	return false
}

func patchDiffArtifactContent(patchDetails []resultPatchDetail) string {
	var builder strings.Builder
	builder.WriteString("# Code Change Diff\n")
	for _, patch := range patchDetails {
		if strings.TrimSpace(patch.Diff) == "" && strings.TrimSpace(patch.Content) == "" {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(markdownHeadingText(firstNonEmptyOrchestratorString(patch.TargetRef, string(patch.ID))))
		builder.WriteString("\n\n")
		builder.WriteString(fmt.Sprintf("- Patch: `%s`\n", markdownInlineCode(string(patch.ID))))
		builder.WriteString(fmt.Sprintf("- Type: `%s`\n", markdownInlineCode(patch.PatchType)))
		builder.WriteString(fmt.Sprintf("- Target: `%s`\n", markdownInlineCode(patch.TargetRef)))
		builder.WriteString(fmt.Sprintf("- State: `%s`\n", markdownInlineCode(string(patch.State))))
		if patch.Summary != "" {
			builder.WriteString(fmt.Sprintf("- Summary: %s\n", patch.Summary))
		}
		if strings.TrimSpace(patch.Diff) != "" {
			builder.WriteString("\n```diff\n")
			builder.WriteString(strings.TrimRight(patch.Diff, "\n"))
			builder.WriteString("\n```\n")
			continue
		}
		builder.WriteString("\n```text\n")
		builder.WriteString(strings.TrimRight(patch.Content, "\n"))
		builder.WriteString("\n```\n")
	}
	return strings.TrimSpace(builder.String())
}

type workspaceDeliverableFile struct {
	Path       string
	Size       int64
	Executable bool
	Category   string
}

func workspaceManifestFiles(deliverableDirectory string) ([]workspaceDeliverableFile, error) {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	if deliverableDirectory == "" {
		return nil, common.ErrInvalidArgument
	}
	root, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, common.ErrInvalidArgument
	}
	const maxManifestFiles = 200
	files := make([]workspaceDeliverableFile, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if shouldSkipWorkspaceManifestPath(rel, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(files) >= maxManifestFiles {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, workspaceDeliverableFile{
			Path:       filepath.ToSlash(rel),
			Size:       info.Size(),
			Executable: info.Mode().Perm()&0o111 != 0,
			Category:   artifactCategoryForWorkspaceFile(rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func shouldSkipWorkspaceManifestPath(rel string, entry os.DirEntry) bool {
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(rel)))
	if normalized == "." || normalized == "" {
		return false
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		switch part {
		case ".git", ".workorch-data", "node_modules", "vendor", "dist", "build", ".next", ".cache", "coverage", "tmp", "temp":
			return true
		}
	}
	if entry != nil && entry.IsDir() {
		base := filepath.Base(normalized)
		if strings.HasPrefix(base, ".git") {
			return true
		}
	}
	return false
}

func artifactCategoryForWorkspaceFile(path string) string {
	normalized := strings.ToLower(filepath.ToSlash(path))
	base := pathBaseLower(normalized)
	switch {
	case strings.Contains(normalized, "/testdata/"),
		strings.HasSuffix(base, "_test.go"),
		strings.HasSuffix(base, ".test.js"),
		strings.HasSuffix(base, ".spec.js"),
		strings.Contains(base, "test-report"),
		strings.Contains(base, "coverage"):
		return resultcommon.ArtifactCategoryTestReport
	case strings.HasSuffix(base, ".md"),
		strings.HasSuffix(base, ".adoc"),
		strings.HasSuffix(base, ".rst"),
		strings.HasPrefix(base, "readme"),
		strings.HasPrefix(base, "docs"):
		return resultcommon.ArtifactCategoryDocumentation
	case strings.HasSuffix(base, ".toml"),
		strings.HasSuffix(base, ".yaml"),
		strings.HasSuffix(base, ".yml"),
		strings.HasSuffix(base, ".json"),
		strings.HasSuffix(base, ".env"),
		strings.Contains(base, "config"),
		strings.Contains(base, "lock"):
		return resultcommon.ArtifactCategoryConfiguration
	default:
		return resultcommon.ArtifactCategorySource
	}
}

func artifactPurposeForCategory(category string) string {
	switch category {
	case resultcommon.ArtifactCategoryConfiguration:
		return "Configuration required to run or integrate the deliverable."
	case resultcommon.ArtifactCategoryDocumentation:
		return "Documentation or usage guidance for acceptance review."
	case resultcommon.ArtifactCategoryTestReport:
		return "Verification material related to the DoD."
	case resultcommon.ArtifactCategoryRuntimeEvidence:
		return "Runtime evidence produced during execution."
	case resultcommon.ArtifactCategoryFinalSummary:
		return "Final user-facing acceptance summary."
	default:
		return "Source or implementation artifact for the accepted outcome."
	}
}

func pathBaseLower(path string) string {
	path = filepath.ToSlash(path)
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
