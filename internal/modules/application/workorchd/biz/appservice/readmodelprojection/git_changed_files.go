package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func mapGitChangedFiles(value any) []readmodelcore.GitChangedFile {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.GitChangedFile, 0, len(items))
	for _, item := range items {
		path := readModelStringValue(item["path"])
		if path == "" {
			continue
		}
		ret = append(ret, readmodelcore.GitChangedFile{
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
