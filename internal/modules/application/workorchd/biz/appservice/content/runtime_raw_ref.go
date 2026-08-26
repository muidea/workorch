package content

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"

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
	if len(parts) < 6 || !safeContentRefPart(parts[0]) || parts[1] != "runs" || !safeContentRefPart(parts[2]) || !safeContentRefPart(parts[3]) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	collection := parts[3]
	fileName := parts[len(parts)-1]
	if !AllowedRuntimeRawContentFile(collection, fileName) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	itemParts := parts[4 : len(parts)-1]
	// Runtime detail links carry the owner identity as one canonical base64url
	// token.  The application must not reinterpret an owner ID as a private
	// repository path, even when that ID itself contains slashes.
	if len(itemParts) != 1 || !safeContentRefPart(itemParts[0]) {
		return RuntimeRawRef{}, common.ErrInvalidArgument
	}
	itemID, err := decodeRuntimeRawItemID(itemParts[0])
	if err != nil {
		return RuntimeRawRef{}, err
	}
	return RuntimeRawRef{
		Ref:         ref,
		WorkspaceID: common.WorkspaceID(parts[0]),
		RunID:       common.RunID(parts[2]),
		Collection:  collection,
		ItemID:      itemID,
		FileName:    fileName,
	}, nil
}

func decodeRuntimeRawItemID(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 2048 || strings.Contains(token, "=") {
		return "", common.ErrInvalidArgument
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) == 0 || len(decoded) > 1024 || !utf8.Valid(decoded) ||
		base64.RawURLEncoding.EncodeToString(decoded) != token {
		return "", common.ErrInvalidArgument
	}
	itemID := string(decoded)
	if strings.TrimSpace(itemID) != itemID || itemID == "" || strings.IndexFunc(itemID, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", common.ErrInvalidArgument
	}
	return itemID, nil
}

func safeContentRefPart(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.Contains(value, "/") && !strings.Contains(value, "\\")
}
