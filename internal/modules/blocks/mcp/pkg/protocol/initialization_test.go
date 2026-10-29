package protocol

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNegotiatedVersionUsesExplicitSupportedSet(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		err       error
	}{
		{`{"protocolVersion":"2024-11-05","capabilities":{}}`, "2024-11-05", nil},
		{`{"protocolVersion":"2025-11-25","capabilities":{}}`, "2025-11-25", nil},
		{`{"protocolVersion":"2099-01-01"}`, "", ErrUnsupportedVersion},
		{`{"protocolVersion":"2025-03-26"}`, "", ErrUnsupportedVersion},
		{`{"protocolVersion":" 2024-11-05"}`, "", ErrUnsupportedVersion},
		{`{"protocolVersion":""}`, "", ErrInvalidInitialization},
		{`{"protocolVersion":null}`, "", ErrInvalidInitialization},
		{`{"protocolVersion":20241105}`, "", ErrInvalidInitialization},
		{`{}`, "", ErrInvalidInitialization},
		{`null`, "", ErrInvalidInitialization},
		{`[]`, "", ErrInvalidInitialization},
		{`{`, "", ErrInvalidInitialization},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			raw := json.RawMessage(tc.raw)
			version, err := NegotiatedVersion(raw)
			if version != tc.want || !errors.Is(err, tc.err) || string(raw) != tc.raw {
				t.Fatalf("got %q, %v; want %q, %v; raw=%s", version, err, tc.want, tc.err, raw)
			}
		})
	}
}
