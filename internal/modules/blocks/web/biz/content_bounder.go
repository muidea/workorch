package web

// boundWebContent preserves the provider's truncation observation and applies
// the owner byte ceiling before content enters a typed result. Byte slicing is
// intentional and preserves the existing wire behavior.
func boundWebContent(content string, maxBytes int64, providerTruncated bool) (string, bool) {
	raw := []byte(content)
	if maxBytes <= 0 || int64(len(raw)) <= maxBytes {
		return content, providerTruncated
	}
	return string(raw[:maxBytes]), true
}
