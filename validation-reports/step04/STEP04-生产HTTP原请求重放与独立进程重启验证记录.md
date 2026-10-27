# STEP04 生产 HTTP、人工重查与独立进程重启验证记录

日期：2026-10-27（Asia/Shanghai）。基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，加累积未提交修改。

## 本轮关闭的验收缺口

此前回环 HTTP 在 Go 测试进程内运行，owner close/reopen 不能证明独立服务进程重启。
本轮新增 `scripts/verify-workspace-preparation-step04.py`，以当前源码重新构建生产 Web 和二进制，
通过真实 HTTP 驱动既有生产 owner 接线；共启动三个不同 PID 的生产进程，实际 SIGTERM 停机两次后重新启动。
每次启动重新核对构建内容身份及 `/proc/<pid>/exe` 的二进制 bytes；结束后全部验收服务已停止。

本轮是**验收接线收口**，没有新增生产 readiness/M3 状态机或修改 Provider：

1. 精确保存、派生、六审、确认到 eligible，再显式执行 STEP04 检查。
2. GET 未检查状态不能自动执行；原检查保存完整 required 清单、all_of 汇总、来源义务编译及 owner 证据引用。
3. 原 request 的 GET、check 重放和 recover 不增加 attempt、不执行新观察、不改写原 owner 文件；同 ID 异 revision 返回 409。
4. 停机前移走隔离测试配置；重启后原请求仍读取原历史，不把历史升级为当前有效，也不修复配置。
5. 使用**新 request＋当前 revision**显式重查，真实观察配置缺失；人工恢复原文件后再次显式新检，新增结果而不覆盖旧结果。
6. 最后一次 POST 只读取 HTTP status/headers，主动丢弃响应体；随后 GET 对账、再次独立进程重启，按三个原 request 分别 replay/recover。三份原始历史保持一致。

| 显式检查 | required | passed | outstanding |
| --- | --- | --- | --- |
| 原检查 | 11 | 10 | provider_cache |
| 配置缺失后的新检查 | 11 | 9 | managed_configuration、provider_cache |
| 人工恢复后的新检查 | 11 | 10 | provider_cache |

三个操作均为 blocked，`currentValidity=not_evaluated`，未签发 M3。
检查与恢复阶段 Provider HTTP 请求（包括 models GET）为 0，embedding 为 0，Workspace/全局 Run inventory 为 0，业务资源 bytes 不变。
每个新检查恰好执行一对 SkillHub status；GET、已完成重放、恢复及进程重启不重复执行。
首次本地模型缓存来自前置候选准备；重启后 cache miss 准确保留为阻塞，没有刷新或补成功。

## 验收边界

生产二进制、HTTP transport、Workspace/File/Soul/Skill/MCPRegistry/Memory/Run 等 owner 均使用真实接线。
**前置候选准备的 8 次模型响应及空 SkillHub status 是显式本地 fixture**，不是实际模型质量或本机 SkillHub readiness 的证明。
无真实模型配置或凭据读入，无真实网关、收费推理/embedding、MCP 启动、生产 Run 或 STEP05。
有界 Profile 的精确空 Skill/MCP 选择不能外推为非空资源组合也已通过。

新增 status-only fixture 只接受固定 status argv，限制 cwd 在本次隔离根内，不支持 sync/use/apply/repair。
共用 STEP01 进程 helper 仅增加可选的进程环境参数，旧入口默认行为不变；不修改应用的环境或鉴权合同。
按 go-multi-module-dev 保留既有 owner 和 typed EventHub 边界，没有新增运行单元或 Runtime SM-1～SM-4 迁移。

## 代码与自动门禁

- 新验收器与 `scripts/fixtures/step04-skill-hub.py` 纳入 `build_identity.py` 的显式源码输入白名单。
- 新增 verifier 定向单测 8 项，覆盖清单删项/重复/降 required/换 owner、伪造 Provider PASS、汇总计数、来源替换、原历史覆盖、作用域/currentness 替换，以及 CLI 禁用 live/旧二进制和 status fixture 禁写边界。
- `make verify-build-identity-tests` 通过：构建身份 17 项、STEP01 helper 5 项、STEP04 verifier 8 项。
- STEP03 verifier 7 项通过（本轮另行执行，未扩大为收费或浏览器复验）。
- `GOCACHE=/tmp/workorch-go-cache make verify-boundary-contract` 通过。
  首次未指定 GOCACHE 时因默认缓存只读未完成，指定可写缓存后重跑通过；未修改代码绕过门禁。
- 生产 Web/Go 构建和内容身份校验通过；Vite 既有大 chunk 告警仍保留。
- `git diff --check` 通过。未执行全仓 Go/全 CI，不能称全 CI 或发布验收通过。

## 受限证据索引

最终证据根（已被 Git 忽略）：
`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-process-20261027-c`。
新目录权限 0700，文件按 0077 umask 创建；原始材料和配置不提交 Git。

| 证据 | 内容 |
| --- | --- |
| `result.json` | 三个 PID、原请求、三次 attempt、零调用/零 Run 和明确的 ready/M3/browser false 边界 |
| `http-trace.json`、`http-*-request.bin`、`http-*-response.bin` | 正常 HTTP 的原始请求/响应 bytes 与状态索引 |
| `lost-response-request.bin`、`lost-response-headers.json` | 原请求 bytes 与实际 status/headers；明确 bodyRead=false，未保存的响应体不称完整证据 |
| `original-preparation.json`、`missing-config-preparation.json`、`manually-restored-preparation.json` | 三个操作的完整公共投影 |
| `original-owner-record.bin`、`final-owner-record.bin`、`data/` | 原始持久文件 bytes、完整 owner 历史及证据归档；不以摘要替代原材料 |
| `process-1.log`～`process-3.log` | 三个生产进程原日志 |
| `provider-http-paths.json`、`provider-*.bin`、`skillhub-calls.jsonl` | 前置 fixture 的原始请求/响应和精确调用序列 |
| `console-build.log`、`go-build.log`、`build-commands.json`、`pre-go-build.json`、`build-identity.json`、`workorch` | 本轮受控构建与实际运行产物 |
| `verifier-tests.log`、`boundary.log` | 最终自动门禁输出 |

最终构建内容身份：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，加本轮构建时全部未提交输入 |
| buildInputSha256 | `49677db393ff86544f61f9227b7a019e0e2294f385800ef73e51ad14f9f36493` |
| consoleTreeSha256 | `7d6f3b3d9d7a583b2b457775ba9c8b86105e34e575a9719f4d757bbfaf431f33` |
| binarySha256 | `3191f13430a32423d6c5ffa4e288e0847c254cd2edd89ec8e2517f1fdf42ba46` |

验收器在构建后、每次进程启动前及最后调用公开 Python `build_identity.verify()`，不是声称另行执行生产证据的 CLI verify。
身份仅是本机内容绑定，不是可信构建 attestation，也不表示最终 Git 提交或外部部署已独立实测。
前两次开发验收根 `step04-process-20261027-a/b` 仍保留，不冒充最终源码验收。

复跑命令（目录必须不存在；不接受 live 配置、旧二进制或收费选项）：

```sh
GOCACHE=/tmp/workorch-go-cache python3 scripts/verify-workspace-preparation-step04.py --evidence-dir /absolute/new/restricted/evidence-directory
```

## 剩余主体项

本轮关闭的是非 Provider **生产 HTTP／恢复／人工重查／独立 OS 进程重启**的阻塞主链验收缺口，
不是“除 Provider 外所有内容完成”。仍需继续：

1. 完整 PreparedBootstrap 的生产输入接线和各 owner 有效性合取；已保存来源义务编译和历史 PASS 不能代替它。
2. F07 唯一历史 M3 的成功结算及原 final fence/CAS 消费。Provider 仍被排除时不构造成功事实；这些未实现代码也不统称为 Provider 问题。
3. 实际生产浏览器交互验收，以及具备真实 required 事实后成功分支的 HTTP/重启复验。本轮构建 Web 不等于实际浏览器已通过。

本轮未提交 Git，保留所有既有累积修改。
