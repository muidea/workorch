package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func mapGitChangedFiles(value any) []workorchprojectioncontract.GitChangedFile {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]workorchprojectioncontract.GitChangedFile, 0, len(items))
	for _, item := range items {
		path := readModelStringValue(item["path"])
		if path == "" {
			continue
		}
		ret = append(ret, workorchprojectioncontract.GitChangedFile{
			Path:      path,
			OldPath:   readModelStringValue(item["oldPath"]),
			Status:    readModelStringValue(item["status"]),
			Staged:    readModelBoolValue(item["staged"]),
			Unstaged:  readModelBoolValue(item["unstaged"]),
			Untracked: readModelBoolValue(item["untracked"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
