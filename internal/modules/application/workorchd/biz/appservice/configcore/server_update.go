package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func UpdateServerConfiguration(cmd UpdateServerConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	if cmd.DefaultDialogLanguage == nil {
		return ReadServiceConfiguration()
	}
	language := strings.TrimSpace(*cmd.DefaultDialogLanguage)
	if language != "" {
		if !dialoglanguage.IsSupported(language) {
			return ServiceConfiguration{}, fmt.Errorf("%w: defaultDialogLanguage only supports zh-CN or en", common.ErrInvalidArgument)
		}
		language = dialoglanguage.NormalizeTag(language)
	}
	cfg.Server.DefaultDialogLanguage = language
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}
