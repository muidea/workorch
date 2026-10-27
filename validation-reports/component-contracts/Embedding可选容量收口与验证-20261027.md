# Embedding 可选容量收口与验证

日期：2026-10-27。基线：`6d0276300c1a41279ea949dcd7c55fb0f7184ce7` 加本轮工作区修改。
本记录是本地验证记录，不是最终提交的 CI、构建 attestation 或真实模型调用证明。

## 核对结论及实现

AetherRelay 当前合同将模型容量元数据定义为可选；精确模型及 `/v1/embeddings` endpoint 决定协议支持。
WorkOrch 原先把未知 `contextWindowTokens` 等同不可调用，导致 endpoint-only Embedding 被禁选。

- LLM owner 允许 Embedding 容量缺失／0，原值保持未知；负容量仍拒绝。
- `workorch.embedding-transport/v1` 独立限制原输入 64 KiB、编码后的 JSON 请求 512 KiB、响应 4 MiB。
  正 context 容量按原利用率及 token/byte 估算进一步收紧输入。字节限制不宣称已知模型 token 容量。
- 增加 owner 投影 `embeddingReady`。混合模型缺 Responses 容量时仍不能成为对话候选；目录资格不证明远端授权。
- 策略版本进入 catalog/capability revision 和 Embedding audit profile mode。审计区分输入字节上限与 JSON 请求上限。
- Application adapter、LLM owner、HTTP adapter 及 typed result 按原输入绑定；不 trim、不截断，非法 UTF-8 明确拒绝。
- 配置页允许显式选择 owner 接纳的容量未知模型，保存精确模型身份，不自动设置默认值。
  owner readme、provider/Web 当前规范及嵌入式生产资源同步更新。

没有修改 AetherRelay、真实配置或当前运行进程；不修改 Runtime SM-1～SM-4 的迁移或执行授权。

## 验证

以下 Go 命令统一使用 `GOCACHE=/tmp/workorch-go-cache GOPROXY=off GOSUMDB=off`。
HTTP 测试只使用隔离回环 fixture，没有访问真实 provider。

| 验证 | 结果 |
| --- | --- |
| LLM 全部包、configcore、activationports、Workspace models、workorchport、management contract、CLI 全部包回归 | 通过 |
| Embedding／catalog／typed result 定向 race | 通过；包含真实执行的 concrete result 输入绑定用例 |
| Web `npm test` | 11 项通过 |
| Web `npm run test:components` | 14 文件、204 项通过；配置页定向 7 项包含实际选择及保存 |
| Web `npm run build` | 通过，生产 assets 已重新生成 |
| `make verify-boundary-contract` | 通过 |
| 受影响 LLM／Application adapter／配置投影／management contract 的 `go vet` | 通过 |
| 当前工作区 `go build -o /tmp/workorch-embedding-closure-20261027 ./application/workorch` | exit 0；未启动该产物 |
| `git diff --check` 与变更 Go 文件 gofmt 核对 | 通过 |

主要命令：

```bash
go test ./internal/modules/blocks/llm/... \
  ./internal/modules/application/workorchd/biz/appservice/configcore \
  ./internal/modules/application/workorchd/biz/appservice/activationports \
  ./internal/modules/kernel/workspace/pkg/models \
  ./internal/pkg/workorchport ./internal/pkg/workorchmanagementcontract \
  ./internal/services/workorch/... -count=1 -timeout=3m
go test -race ./internal/modules/blocks/llm/biz ./internal/modules/blocks/llm/pkg/events \
  -run 'Embedding|CatalogSeparates|ProviderCommandsPublishAudit|ConcreteProviderResults' -count=1 -timeout=3m
go test -race ./internal/modules/application/workorchd/biz/appservice/activationports \
  ./internal/modules/application/workorchd/biz/appservice/configcore \
  -run 'Embedding|CatalogProjection' -count=1 -timeout=3m
```

断言覆盖：endpoint-only 两个模型可解析且 token 容量仍为 0；混合模型隔离；负容量拒绝；512-token 已知容量
收紧至 384 estimated input tokens／1536 input bytes；未知及大容量输入受本地上限限制；input/wire 精确边界、
JSON 转义、中文与首尾空白、非法 UTF-8；完整响应恰好上限通过／多一字节拒绝；上游 413 保持 typed HTTP
status 且只调用一次；超限 owner command 零物理提交；原文、向量、usage 及审计身份保持绑定。
既有协议测试继续覆盖媒体类型、usage、超时及 redirect 门禁。

构建时出现只读 Go module stat cache 写入警告，但命令 exit 0；Web 有 Ant Design 测试警告及大 chunk 提示，均未导致失败。
本轮不宣称全仓 CI、生产浏览器验收、远端模型权限、真实 Embedding 成功或 STEP04/M3 完成。
运行服务需使用重新构建的版本；配置保存后的 active owner 切换仍遵循原进程重启合同。
