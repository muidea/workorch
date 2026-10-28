package skillhubpreparation

import "testing"

func TestProjectNotFoundRequiresWholeUnambiguousError(t *testing.T) {
	valid := `{"command_error":{"code":"PROJECT_NOT_FOUND","message":"原错误"}}`
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{valid, true},
		{`{"command_error":{"code":"PROJECT_NOT_FOUND","message":""}}`, false},
		{`{"command_error":{"code":"OTHER","message":"PROJECT_NOT_FOUND"}}`, false},
		{`{"command_error":{"code":"PROJECT_NOT_FOUND","Code":"OTHER","message":"x"}}`, false},
		{`{"command_error":{"code":"PROJECT_NOT_FOUND","message":"x"},"items":[]}`, false},
		{`{"command_error":null}`, false},
		{valid + `{}`, false},
		{"prompt\n" + valid, false},
	} {
		if got := ProjectNotFound([]byte(tc.raw)); got != tc.want {
			t.Fatalf("classification %q = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
