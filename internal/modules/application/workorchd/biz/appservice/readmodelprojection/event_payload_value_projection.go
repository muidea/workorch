package readmodelprojection

func projectEventPayloadValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= EventPayloadStringLimit {
			return typed, true
		}
		return typed[:EventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []string:
		return projectEventPayloadStringListValue(typed), true
	case []any:
		return projectEventPayloadAnyListValue(typed)
	default:
		return nil, false
	}
}
