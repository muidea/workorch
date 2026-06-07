package setupdraftcore

import (
	"encoding/json"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ParseCompletion(payload []byte) (Draft, error) {
	var envelope struct {
		Draft *Draft `json:"draft"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Draft{}, fmt.Errorf("%w: invalid workspace setup draft response: %v", common.ErrInvalidArgument, err)
	}
	if envelope.Draft != nil {
		if HasContent(*envelope.Draft) {
			return NormalizeDraft(*envelope.Draft), nil
		}
	}
	var topLevel Draft
	if err := json.Unmarshal(payload, &topLevel); err != nil {
		return Draft{}, fmt.Errorf("%w: invalid workspace setup draft response: %v", common.ErrInvalidArgument, err)
	}
	if HasContent(topLevel) {
		return NormalizeDraft(topLevel), nil
	}
	return parseCompletionAliases(payload)
}
