package readmodelprojection

func projectReadModelScalarMapValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= eventPayloadStringLimit {
			return typed, true
		}
		return typed[:eventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case map[string]any:
		ret := map[string]any{}
		for key, item := range typed {
			if projected, ok := projectReadModelScalarMapValue(item); ok {
				ret[key] = projected
			}
		}
		if len(ret) == 0 {
			return nil, false
		}
		return ret, true
	default:
		return nil, false
	}
}
