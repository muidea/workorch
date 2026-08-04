package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
)

func UpdateServerConfiguration(cmd UpdateServerConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if cmd.DefaultDialogLanguage == nil {
		return ReadServiceConfiguration()
	}
	language := strings.TrimSpace(*cmd.DefaultDialogLanguage)
	if language != "" {
		if !dialoglanguage.IsSupported(language) {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: defaultDialogLanguage only supports zh-CN or en", common.ErrInvalidArgument)
		}
		language = dialoglanguage.NormalizeTag(language)
	}
	cfg.Server.DefaultDialogLanguage = language
	if err := workorchconfig.Save(cfg); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}
