# STEP04 Skill 正式库只读完整性观察与验证记录

日期：2026-10-22（Asia/Shanghai）。本轮是在既有未提交 STEP04 工作树上继续实现，不是整个第四步的退出报告；不进入 STEP05。

## 本轮结论

F03 新增 Skill 正式库的完整、有界、只读观察，以及 Application 的精确封存绑定和两次完整重观察。
新增 8 个测试函数；最终受影响累计回归、定向 race、vet、格式与边界检查通过。
**不据此宣称完整 Skill 选择、全部资源 readiness、M3、STEP04 或 live 验证已经通过。**

按 go-multi-module-dev v2.6.0 保持 Skill owner / Application 组合分工：没有新运行单元、跨 owner repository/service/callback，
不变更 Runtime SM-1～SM-4 的迁移或权限。Skill 新入口只有只读事实返回，没有准备写账本或执行授权。

## 实现与边界

- Skill owner 新增 `/skill/startup-preparation/registry-catalog/query`，使用具体 `ObserveRegistryCatalogCommand/Result`，
  仅接受 exact `application/workorchd` source，核验请求 Workspace scope、取消与返回类型。
  typed message/delivery、订阅与关闭期只读 drain、模块生命周期测试同步；生产 topic 清单从 317 增至 318，有对应合同与发送方，不是仅改数量。
- 每次在 owner 内真实调用一次完整 repository List；memory/file 在同一次读锁中取完整 map，禁止用不同时点分页拼接。
  底层 List 对 map key/记录 ID 不同或 nil 记录返回 conflict 和零列表，不补写、不修复；file 原字节回归覆盖拒绝和重开后不变。
- 观察校验 canonical 完整内容、UTF-8/字段边界、完整 provenance 和内容 hash；排序后拒绝重复 ID。
  最多 1024 条、整个 typed 结果最多 8 MiB；同时限制 material 与编码后字节，超限不给部分“完整结果”。空正式库返回显式完整空列表。
  既有 create/get/list 的内容上限纯函数移至 owner models 复用，不放宽原校验。
- 返回真实观察时间、owner 重建即变化的随机 epoch 和完整目录 hash；epoch 不持久化、不跨重建授信，hash 仅作内容身份。
- `ObservePreparationSkillRegistry`：精确 seal → 两次真实完整 Registry query → 完整记录/epoch 比较 → 精确 seal 复核；整体 3 秒。
  独立校验 DTO、scope、上下限、canonical 原值、hash、观察时间；不以 hash 命中替代完整记录比较。
  失败保留已成功取得的首次独立事实及失败类别，原 owner error 向受限内部调用者返回；不自动重试、fallback、refresh。
- 该子用例不调用 SkillHub/provider，不读指令 fallback 路径，不写 Workspace/准备账本，不接 public HTTP GET/SSE。
  返回的是 owner canonical 记录的事实，不声称原持久 JSON 的跨 owner 原子字节快照、长期 freshness、完整合并目录或精确选择证明。

主要入口：

- `internal/modules/blocks/skill/biz/preparation_catalog.go`
- `internal/modules/blocks/skill/pkg/events/preparation_catalog.go`
- `internal/modules/blocks/skill/pkg/models/validation.go`
- `internal/modules/application/workorchd/biz/activation_preparation_skills.go`

## 验证与证据身份

受限原证据根：`/tmp/workorch-step04-skill-registry-b53SKf`。原始日志不提交 Git；目录由 mktemp 隔离创建。

最终累计回归命令（`GOCACHE=/tmp/workorch-go-cache`）：

```bash
go test ./internal/modules/blocks/skill/... \
  ./internal/modules/application/workorchd/biz \
  ./internal/modules/kernel/workspace/biz ./internal/pkg/events \
  ./internal/pkg/repository/... -count=1
```

结果：10 个有测试包通过，2 个包无测试文件。使用既有本机 HTTP fixture 和临时数据，不使用真实收费 provider。
首次沙箱执行在既有 HTTP fixture 的回环监听处被拒绝；完整失败日志保留在 `affected-go-test.log`，不记为通过。
允许回环监听后重跑通过；冻结最终源码后的接受日志为 `affected-go-test-accepted.log`，不是仅引用前次运行。

定向 race：

```bash
go test -race ./internal/modules/blocks/skill/... \
  ./internal/modules/kernel/workspace/biz \
  ./internal/pkg/repository/file ./internal/pkg/repository/memory \
  -run 'TestPreparationRegistry|TestPreparationSkillRegistry|TestRegistryCatalog|TestSkillListRejects|TestSkillContract|TestOwnerLifecycle' -count=1
```

覆盖真实 sender + Skill/Workspace owner、file owner 重开、独立 clone 与零持久改写、scope/source、错误 typed reply、
重复/乱序/损坏/非 canonical/超限、取消、原诊断、首份事实保全、增项/内容/epoch/封存漂移、关闭 drain 与停止后零读取。
Skill module 根包在此筛选下没有匹配测试；不能把该行当作模块全部 race 测试通过。
另执行上述受影响路径的 `go vet`、`make fmt-check`、`git diff --check`、`make verify-boundary-contract`，均通过。
边界检查曾命中 events 测试中的函数回调和同一行 `*...Registry...` 的写法，已改成显式场景分支及独立 count；
未修改、跳过或放宽边界扫描。首次及中间失败日志保留，最终使用 `boundary-final.log`。

| 原证据 | SHA256 |
| --- | --- |
| `affected-go-test-accepted.log` | `6ab6ef7f5adf3df8bf4878018df7fcda055cc0773040540db216868a3c04b1bd` |
| `targeted-race-accepted.log` | `2659993547a66c1876a92c31937cae47e38b22f0f389cec7f7877d2c13a72fd7` |
| `boundary-final.log` | `2677a33ae83f0f514d29bf97e3763e17fe01aadafcb69775cc3c775f03dceb62` |

源码身份由现有 `build_identity.py capture` 生成，`source-snapshot-accepted.json`：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` 加本轮及前轮未提交源码，不能仅用 HEAD 代表被测树 |
| buildInputSha256 | `78e0f428b9d9f06d0d70d04f0073f23f3034a2fd3dcb7cdf5e294096e8abc195` |
| consoleTreeSha256 | `dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c` |

本轮没有构建/启动生产二进制，没有新的 binarySha256、CLI verify、生产浏览器或独立服务 stop/start 证据。
file owner 的重开测试不等同服务进程重启。上述内容指纹不是可信构建 attestation，也不引用前轮 binary 为本轮源码背书。
没有重跑全仓 CI、公开 fullflow 或收费模型；已有 CI 阻塞不能在此改写为通过。

## 后续仍需收口

1. F03：SkillHub 已安装指令的完整真实读取，以及在完整 Registry + SkillHub 目录上的精确选择；不能只拿本轮正式库当完整合并目录。
2. F03：目录/权限与托管配置、MCP/Memory 原模式/版本/完整性/freshness，以及剩余 required 项的准确阻塞。
3. F05/F08：显式检查受理、精确幂等、bounded attempts、private reducer/CAS、原观察/诊断持久化及原检查恢复。
4. F07：所有 required 真实事实、完整 no-Run/epoch 观察、final CAS、唯一首次 M3 及竞争/unknown/retirement/crash/late 矩阵。
5. 增量三：HTTP 检查/恢复、Web 分轴与迟到响应 fence、生产构建真实 HTTP/浏览器与独立服务 stop/start。

STEP05 的 Start/Restart 准备回执消费仍不在本轮范围。继承的 integration 默认 Profile 断言、Orchestrator 套件累计超时及 build_identity CLI verify 问题继续单列，不默认扩展本轮修复范围。
