package configcore

import (
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
)

func DefaultWebSearchProviderTemplates() []workorchconfigurationcontract.WebSearchProviderTemplate {
	return []workorchconfigurationcontract.WebSearchProviderTemplate{
		{
			TemplateID:    webcommon.ProviderHTTP,
			Name:          "HTTP Web Search Provider",
			Description:   "web/search 使用同源 HTTP provider；web/fetch 由 Web owner 在 DNS/redirect/SSRF 治理后直接读取并生成有界证据。",
			ID:            "web-http-main",
			Provider:      webcommon.ProviderHTTP,
			Timeout:       "20s",
			MaxResults:    8,
			MaxFetchBytes: 262144,
			MaxRedirects:  3,
		},
	}
}
