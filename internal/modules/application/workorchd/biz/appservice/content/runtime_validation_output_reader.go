package content

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func decodeRuntimeValidationOutputSelector(token string) (string, error) {
	if len(token) > 11000 {
		return "", common.ErrInvalidArgument
	}
	body, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(body) == 0 || len(body) > 8192 || !utf8.Valid(body) || base64.RawURLEncoding.EncodeToString(body) != token {
		return "", common.ErrInvalidArgument
	}
	return string(body), nil
}

func (r *Reader) readRuntimeValidationOutput(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	var command orchestratorevents.QueryRuntimeValidationOutputCommand
	decoder := json.NewDecoder(strings.NewReader(parsed.ItemID))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&command) != nil || decoder.Decode(new(any)) != io.EOF || command.Validate() != nil || command.Offset != 0 ||
		command.WorkspaceID != parsed.WorkspaceID || command.RunID != parsed.RunID || parsed.FileName != command.Stream+".bin" {
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
	canonical, err := command.ContentRef()
	if err != nil || canonical != parsed.Ref {
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
	value, err := ownerreadports.ReadRuntimeValidationOutput(ctx, r.eventHub, command)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	identity := value.Identity.Clone()
	savedSHA256 := value.SavedSHA256
	raw := append([]byte(nil), value.Raw...)
	for int64(len(raw)) < identity.SavedBytes {
		command.Offset = int64(len(raw))
		next, err := ownerreadports.ReadRuntimeValidationOutput(ctx, r.eventHub, command)
		if err != nil {
			return RuntimeRawContent{}, err
		}
		if !reflect.DeepEqual(next.Identity, identity) || next.SavedSHA256 != savedSHA256 {
			return RuntimeRawContent{}, common.ErrConflict
		}
		raw = append(raw, next.Raw...)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != savedSHA256 {
		return RuntimeRawContent{}, common.ErrConflict
	}
	return RuntimeRawContent{
		Ref: parsed.Ref, FileName: parsed.FileName, ContentType: "application/octet-stream",
		Content: raw, ContentSHA256: savedSHA256, OutputIdentity: &identity,
	}, nil
}
