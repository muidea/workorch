package content

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ParseRuntimeRawRef(ref string) (RuntimeRawRef, error) {
	ref = strings.TrimSpace(ref)
	const prefix = "workorch://workspaces/"
	if ref == "" || !strings.HasPrefix(ref, prefix) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	rest := strings.TrimPrefix(ref, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 6 || !SafeRuntimeRawRefPart(parts[0]) || parts[1] != "runs" || !SafeRuntimeRawRefPart(parts[2]) || !SafeRuntimeRawRefPart(parts[3]) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	collection := parts[3]
	fileName := parts[len(parts)-1]
	if !AllowedRuntimeRawContentFile(collection, fileName) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	itemParts := parts[4 : len(parts)-1]
	if len(itemParts) == 0 {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	for _, part := range itemParts {
		if !SafeRuntimeRawRefPart(part) {
			return RuntimeRawRef{}, common.ErrInvalidArgument
		}
	}
	return RuntimeRawRef{
		Ref:         ref,
		WorkspaceID: common.WorkspaceID(parts[0]),
		RunID:       common.RunID(parts[2]),
		Collection:  collection,
		ItemPath:    strings.Join(itemParts, "/"),
		FileName:    fileName,
	}, nil
}

func SafeRuntimeRawRefPart(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.Contains(value, "/") && !strings.Contains(value, "\\")
}
