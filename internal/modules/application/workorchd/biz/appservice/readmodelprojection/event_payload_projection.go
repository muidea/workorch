package readmodelprojection

const (
	EventPayloadStringLimit = 512
	EventPayloadListLimit   = 16
)

func projectEventPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	ret := map[string]any{}
	for key, value := range payload {
		if !isEventPayloadAllowedKey(key) {
			continue
		}
		if projected, ok := projectEventPayloadValue(value); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
