package git

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
)

func parsePorcelainStatus(stdout string) ([]GitFileChange, GitAheadBehind, string, string) {
	var files []GitFileChange
	aheadBehind := GitAheadBehind{}
	branch := ""
	head := ""
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# branch.head ") {
			branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			continue
		}
		if strings.HasPrefix(line, "# branch.oid ") {
			head = strings.TrimSpace(strings.TrimPrefix(line, "# branch.oid "))
			continue
		}
		if strings.HasPrefix(line, "# branch.ab ") {
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) >= 2 {
				aheadBehind.Ahead = int64(trimSignedInt(fields[0]))
				aheadBehind.Behind = int64(trimSignedInt(fields[1]))
			}
			continue
		}
		if strings.HasPrefix(line, "? ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "? "))
			files = append(files, GitFileChange{Path: path, Status: "untracked", Untracked: true})
			continue
		}
		if strings.HasPrefix(line, "1 ") {
			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}
			xy := fields[1]
			path := strings.Join(fields[8:], " ")
			files = append(files, GitFileChange{Path: path, Status: statusFromXY(xy), Staged: len(xy) > 0 && xy[0] != '.', Unstaged: len(xy) > 1 && xy[1] != '.'})
			continue
		}
	}
	return files, aheadBehind, branch, head
}

func parseNameStatus(stdout string, staged bool) []GitFileChange {
	var files []GitFileChange
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := statusFromCode(fields[0])
		path := fields[len(fields)-1]
		item := GitFileChange{Path: path, Status: status, Staged: staged}
		if strings.HasPrefix(fields[0], "R") && len(fields) >= 3 {
			item.OldPath = fields[1]
		}
		files = append(files, item)
	}
	return files
}

func parseLogCommits(stdout string) []GitCommit {
	var commits []GitCommit
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x1f", 5)
		if len(fields) != 5 {
			continue
		}
		commits = append(commits, GitCommit{Hash: fields[0], ShortHash: fields[1], Author: fields[2], AuthorTime: fields[3], Subject: fields[4]})
	}
	return commits
}

func parseBranches(stdout string) []GitBranch {
	var branches []GitBranch
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		item := GitBranch{Name: fields[0], Commit: fields[1], Current: fields[2] == "*"}
		if len(fields) > 3 && fields[3] != "" {
			item.Upstream = fields[3]
		}
		branches = append(branches, item)
	}
	return branches
}

func logLimit(limit int64) (int, bool) {
	if limit <= 0 {
		return defaultLogLimit, false
	}
	if limit > maxLogLimit {
		return maxLogLimit, true
	}
	return int(limit), false
}

func statusFromXY(xy string) string {
	if strings.Contains(xy, "U") {
		return "conflicted"
	}
	for _, ch := range xy {
		switch ch {
		case 'A':
			return "added"
		case 'M':
			return "modified"
		case 'D':
			return "deleted"
		case 'R':
			return "renamed"
		case 'C':
			return "copied"
		}
	}
	return "modified"
}

func statusFromCode(code string) string {
	switch {
	case strings.HasPrefix(code, "A"):
		return "added"
	case strings.HasPrefix(code, "M"):
		return "modified"
	case strings.HasPrefix(code, "D"):
		return "deleted"
	case strings.HasPrefix(code, "R"):
		return "renamed"
	case strings.HasPrefix(code, "C"):
		return "copied"
	case strings.HasPrefix(code, "U"):
		return "conflicted"
	default:
		return strings.ToLower(code)
	}
}

func classifyGitError(result gitCommandResult, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return gitcommon.ErrorTimeout
	}
	text := strings.ToLower(result.Stderr + "\n" + err.Error())
	switch {
	case strings.Contains(text, "not a git repository"):
		return gitcommon.ErrorNotGitRepository
	case strings.Contains(text, "author identity unknown") ||
		strings.Contains(text, "unable to auto-detect email address") ||
		strings.Contains(text, "作者身份未知") ||
		strings.Contains(text, "未提供邮件地址") ||
		strings.Contains(text, "请告诉我您是谁"):
		return gitcommon.ErrorIdentityMissing
	default:
		return gitcommon.ErrorGitCommandFailed
	}
}

func truncateString(value string, limit int64) (string, bool) {
	if limit <= 0 || int64(len(value)) <= limit {
		return value, false
	}
	n := int(limit)
	if limit < 32 {
		return value[:n], true
	}
	return value[:n] + "\n[truncated]", true
}

func limitChangedFiles(files []GitFileChange) []GitFileChange {
	if len(files) <= maxChangedFiles {
		return files
	}
	return files[:maxChangedFiles]
}

func trimSignedInt(value string) int {
	value = strings.TrimLeft(strings.TrimSpace(value), "+")
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func samePath(left string, right string) bool {
	leftEval, leftErr := filepath.EvalSymlinks(left)
	rightEval, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil {
		left = leftEval
	}
	if rightErr == nil {
		right = rightEval
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
