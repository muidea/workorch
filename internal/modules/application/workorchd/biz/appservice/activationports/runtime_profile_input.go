package activationports

import "github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"

// Profile 原始形状在合并与 typed admission 前由同一配置合同校验。
func validateRuntimeProfileInputShape(input map[string]any) error {
	return runtimeprofiledecode.ValidateInputShape(input)
}

type runtimeProfileInputFields struct {
	text, boolean, stringList, integer string
	arrays                             map[string]string
}

func runtimeProfileFields(kind string) runtimeProfileInputFields {
	fields := runtimeprofiledecode.Fields(kind)
	return runtimeProfileInputFields{fields.Text, fields.Boolean, fields.StringList, fields.Integer, fields.Arrays}
}

func validateRuntimeProfileObject(object map[string]any, kind, subject string) error {
	return runtimeprofiledecode.ValidateObject(object, kind, subject)
}
