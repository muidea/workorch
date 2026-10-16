package content

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
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
	if decoder.Decode(&command) != nil || decoder.Decode(new(any)) != io.EOF || command.Validate() != nil ||
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
	return RuntimeRawContent{
		Ref: parsed.Ref, FileName: parsed.FileName, ContentType: "application/octet-stream",
		Content: append([]byte(nil), value.Raw...), ContentSHA256: value.SavedSHA256, OutputIdentity: &identity,
	}, nil
}
