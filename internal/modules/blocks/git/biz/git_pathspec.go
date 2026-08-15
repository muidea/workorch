package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func optionalPathspecs(input []string) ([]string, error) {
	return parsePathspecs(input, false, true)
}

func requiredPathspecs(input []string) ([]string, error) {
	return parsePathspecs(input, true, false)
}

func parsePathspecs(input []string, required bool, allowRepositoryRoot bool) ([]string, error) {
	if required && len(input) == 0 {
		return nil, fmt.Errorf("%w: pathspecs are required", common.ErrInvalidArgument)
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		pathspec, err := normalizePathspec(item, allowRepositoryRoot)
		if err != nil {
			return nil, err
		}
		if pathspec != "" {
			ret = append(ret, pathspec)
		}
	}
	return ret, nil
}

func normalizePathspec(value string, allowRepositoryRoot bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: empty pathspec is not allowed", common.ErrInvalidArgument)
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: absolute pathspec is not allowed", common.ErrInvalidArgument)
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, ":") {
		return "", fmt.Errorf("%w: git option or magic pathspec is not allowed", common.ErrInvalidArgument)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." && allowRepositoryRoot {
		return "", nil
	}
	if cleaned == "." || cleaned == "*" || cleaned == ":/" {
		return "", fmt.Errorf("%w: broad pathspec is not allowed", common.ErrInvalidArgument)
	}
	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == ".." {
			return "", fmt.Errorf("%w: parent pathspec is not allowed", common.ErrInvalidArgument)
		}
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git"+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: .git pathspec is not allowed", common.ErrInvalidArgument)
	}
	return filepath.ToSlash(cleaned), nil
}

func pathspecArgs(pathspecs []string) []string {
	if len(pathspecs) == 0 {
		return nil
	}
	ret := []string{"--"}
	ret = append(ret, pathspecs...)
	return ret
}
