package configcore

import webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"

func DefaultWebSearchProviderTemplates() []WebSearchProviderTemplate {
	return []WebSearchProviderTemplate{
		{
			TemplateID:    webcommon.ProviderHTTP,
			Name:          "HTTP Web Search Gateway",
			Description:   "通过受治理的 HTTP 搜索/读取网关执行 web/search 与 web/fetch；网关负责真实搜索源和网页正文抽取。",
			ID:            "web-http-main",
			Provider:      webcommon.ProviderHTTP,
			Timeout:       "20s",
			MaxResults:    8,
			MaxFetchBytes: 262144,
			MaxRedirects:  3,
		},
	}
}
