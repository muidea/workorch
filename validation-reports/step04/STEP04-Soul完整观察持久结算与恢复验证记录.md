# STEP04 Soul 完整观察持久结算与恢复验证记录

日期：2026-10-23。基线 `834aa350cc4e77379819666540b9912a7c22b64c` 加此前及本轮未提交修改。
本文件是实施/验证记录，不是设计权威、整个 STEP04/M3、发布或全 CI 通过声明。

## 本轮交付与边界

将已有独立 Soul 精确观察接到生产“原请求接纳 → Application 观察 → Workspace 私有结算/CAS → GET/原操作恢复”路径。

1. 生产 preparation port 从实际 session exports 注入进程配置 accessor，不使用 `ConfigLoader()` 的空配置兼容 fallback。
   缺 accessor 或读取失败不当成已观察的空 system default；不修改 live 配置。
2. 六阶段选择沿用 stage binding、Workspace default、system default 优先级及原 default lookup 规则。
   纯选择规则归 Workspace，Application 直接使用 focused owner 模型，不建立类型 alias facade，
   不改变既有 activation routing。原 activation 路由对照回归保留。
3. `PreparationSoulFacts` 保存完整所选 typed Soul records 的两轮实际读取、前后 system default、
   Workspace 默认/有序绑定、六阶段来源、exact seal/configuration hash、Workspace revision 和观察时间。
   records 保留 revision/provenance、全部列表及指令原值；反证完整值，不以 content hash 代替完整相等。
   该合同不包含 `Reobserved`/`SealBindingCurrent` 成功标志；旧 helper 标志不作为 owner 输入。
   最多六阶段加独立 default 的七个唯一 Soul，单 record 沿用 Soul owner 96 KiB 总内容上限，整体 helper 三秒。
4. Workspace settlement 独立重建选择，反查 accepted 原 request/payload/revision/configuration、
   当前 Workspace 配置及 exact seal；不接受 caller 选择的快照或成功凭据。
   完整前后值与随机引用在原 blocked 结算同一次 CAS 保存，不在资源 I/O 期间持有 definition/reducer 锁。
5. 新私有 `WorkspacePreparationSoulEvidence` 有版本、原意图身份、完整 facts 与完整性 hash。
   file/memory/scoped repository 深克隆、重开、0600 和 append-only backstop 同步接通；
   删材料、换引用、重算 hash 后替换历史均拒绝。
   Soul typed clone 及仓库 Soul clone 使用保留 nil/空列表形状的深克隆；不改变 Soul owner 的既有规范化规则。
6. 公共 record/HTTP/Web 只投影 `soulEvidenceRef`（`soul-observation:` 加 16 字节独立随机数的小写 hex），
   不输出指令、列表或私有完整性 hash。引用只表示完整历史材料已保存，不表示仍然 current、ready 或 M3。
7. 完整相等仍为 `blocked/souls_observed_remaining_checks`。
   缺配置、缺资源、错误回执、漂移或不完整观察为 `soul_observation_unavailable_not_saved`：
   明确本次完整材料未保存，不将部分 records/失败资料冒充完整 archive。
   当前只保存这一负向分类，尚未新增失败原文的受限诊断读取合同，不能称失败材料已完整归档或准确逐因展示。
8. CAS 写失败不部分保存；丢回复用原身份读回；revision/epoch 漂移只保留历史并 binding_changed/stale。
   GET、同请求重放、显式恢复及 owner 重开不重新读取 Soul/进程配置，不补造未提交材料。
9. stateownercheck 只给 `PreparationSoulSelection.Stage` 登记不可变路由事实字段，
   不把整个 DTO 加入宽泛白名单；新增 sibling State/TargetStage/DesiredState 的反例仍拒绝。
   未增加 EventHub topic、运行单元或新的 Runtime 状态迁移，未改 SM-1～SM-4、STEP05 或执行授权。

这里的“原值”是 Soul owner 返回的 typed record，不声称读取了未经 owner 规范化的底层文件原文。
Soul allowlist 不是 Skill/MCP 当前可用性；空选择也不是 Profile 适用性或 readiness 的最终裁决。
跨 owner 的时点读取不是读取至 Workspace CAS 的全资源原子快照。

## 验证与受限证据索引

受限证据根：`/tmp/workorch-step04-soul-evidence-8ffygy`。临时原日志/二进制不提交 Git，不能替代长期证据保管。

| 验证 | 实际结果与证据 |
| --- | --- |
| 最终受影响 Go 包完整回归 | `affected-go-final.log`：17 个有测试包通过、2 个包无测试，`-count=1`；覆盖 Workspace、Soul、Application biz/server、repository 三后端、workorchrepo、公共 events/management contract 及 stateownercheck |
| 原操作定向测试 | `soul-targeted.log`：真实 Soul owner、完整观察结算、三种 CAS 切点、原输入反查通过；最终完整回归再次覆盖 |
| 定向 race | `race.log`：Workspace biz/models、repository 与 Application biz 的 Soul、repository 历史及 HTTP workflow 用例通过 |
| 真实回环 HTTP | 最终 Application 完整包及 race 内包含 `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen`；真实 production transport/port、Workspace/File/Soul/Run owners 与文件持久库 |
| Web 组件 | `web-components.log`：13 文件、141 项通过，含新增 Soul 历史引用、缺引用/错格式/错状态/伪 PASS 反例 |
| Web 规则 | `web-rules.log`：11 项通过 |
| 生产 Web 构建 | `console-build.log`：通过，保留已有大 chunk 提示 |
| vet / fmt / boundary | `vet-final.log`、`fmt-final.log`、`boundary-final.log`：退出 0；boundary 保留已有 pycache binary-match 提示 |
| Go 二进制 | `build.log`：退出 0，仍有只读 module stat-cache 写入告警，不称干净日志 |
| 内容身份 | `source-snapshot.json`、`build-identity.json`、`identity-produce.log`：capture → build → produce 完成；归档后另 capture 对比 |

新增/扩展覆盖：两轮完整值、相同 hash 的列表分区漂移、revision/provenance、nil/空列表形状、缺 loader、
缺 Soul、system default 漂移、unbound、原 request/config/seal/revision/time 错绑、三后端 clone/reopen、
不可改写材料/引用、CAS 写失败/丢回复/版本漂移、原操作恢复不重读，以及 public 不输出材料。

真实 HTTP 前置 authoring 为 8 次固定本地 provider 文档 fixture 调用；STEP04 新增推理 0、embedding 0。
cache-only fixture 读取 1 次；所选 Soul 实际 Get 两次，未使用 system default 不读取。
首次检查后删除测试 Soul 和托管配置文件，GET、同请求 POST、recover、owner/HTTP transport 重开后
结果与原值保持不变，Soul 仍总计两次读取，Run inventory 为 0。
这是进程内 owner/transport 重开，不是独立 OS 进程 stop/start；Web 组件不是实际 Chrome。
二进制只构建，未部署或用它启动服务，不把 Go 测试 HTTP 结果记成该二进制的部署验收。

首次局部命令因 sandbox 禁止 socket 失败；后续允许 loopback 的最终完整包与 race 已通过。
第一次完整 Go 回归还发现旧断言把未接通 Soul 原因固定为 `check_pipeline_incomplete`，
已按新负向分类同步，未改生产结果凑绿，最终完整命令重跑通过。
边界初测拒绝类型 alias、未登记的 Stage 与未经审阅的跨 owner Stage；改为直接引用 focused owner 类型、
登记不可变字段及精确字段审阅测试后通过。另一次边界运行受默认只读 GOCACHE 限制，
最终统一 `GOCACHE=/tmp/workorch-go-cache` 完成，不放宽检查器。

未执行全仓 CI、收费 live、实际 Chrome、独立 OS 进程重启、新二进制部署或 Runtime fullflow。
继承 fullflow/profile 断言与 orchestrator 累计超时本轮未重跑/修复，不称全 CI 通过。

## 构建内容身份

以下仅关联本地 dirty tree 构建输入与产物字节，不是纯基线 SHA 的验证结果、构建因果证明或可信 attestation。

| 身份 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` 加未提交修改 |
| buildInputSha256 | `856e3f3a46b7c1cfd08483d9322404dadf60e908020d48e6f65d7321a4c24ac7` |
| consoleTreeSha256 | `5583b569b96ead133503e100da89fc47481d983cce974ad621d48d563f61ece6` |
| binarySha256 | `ce5c7efeaecda1701d458ad25f0583b3cc21d429048fc9fabde0e41e03ad52b2` |

## 尚未满足的退出门槛

1. Skill/SkillHub 精确观察尚未汇入原操作持久阶段；保留用户接受的现行 status 效果，
   效果前持久封存、unknown 不自动重发。Soul 不完整/失败材料的受限原文诊断与逐因展示仍需补强。
2. MCP/Memory/provider 的原模式、精确选择、完整性、版本与 freshness；provider cache/history
   目前仍是负向分类而非完整原始事实归档。不得 probe、embedding 或自动降级凑 PASS。
3. 所有 required 真实事实合取、纯编译、最终 reservation/epoch/完整 Run 历史 CAS 和历史 M3 正例。
   当前 v2 仍只有 accepted/unknown/blocked/failed，没有 readiness/M3 正状态。
4. 主体正例后补生产 HTTP/Web 正负/故障恢复、独立 OS 进程重启及最终源码/产物/20 项退出矩阵。

严格限 STEP04；未进入 STEP05、Runtime 派发/执行授权或收费验证；未提交 Git，保留此前全部修改。
