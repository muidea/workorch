# STEP04 File 最终重观察与重启失效验证记录

日期：2026-10-27。基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，包含累积未提交改动；未执行 Git 提交。本记录不是完整 STEP04／M3 通过报告。

## 本轮实际收口

1. File owner 在原显式准备操作的其他观察结束后，真实重读原目录、完整有序规格资源和托管配置。新增具体 typed revalidation／epoch query，分别登记 owner、消息与只读 lane；EventHub 数量由 325 增至 327。File 合同和错误 DTO／重复 Start／Teardown 矩阵也覆盖这两个 topic，没有放宽 concrete graph 审计。
2. 观察绑定 Workspace、原 request／payload、revision、两个根目录和完整有序目标集合，保存完整原文前后像。年龄窗口为开始读取后 30 秒；随机 File owner epoch 在重建后变化。它不是文件 lease，也不保证窗口内不发生外部改写。
3. Workspace 独立反查原材料、scope、数量、顺序、时序及完整原值。后像变化、到期（含等于到期时刻）、时钟回退、缺当前 epoch 或 epoch 变化均只投影 stale。GET／重放／恢复不重新读文件、不续期、不回填旧历史、不修改单项结论。仅最新操作参与当前有效性，旧操作到期不污染新 accepted 操作。
4. 私有 `CompilationSource` 改为无 map 的具体 typed DTO；显式转换和深复制，消费者继续独立执行原持久材料与纯编译校验。完整诊断 bytes、错误原文、字节身份、历史、nil／空配置边界保全，不使用普通公开 metadata 代替原材料；`json:"-"` 不输出 HTTP，也不接受回传成功凭据。
5. 补上 HTTP 回归的分轴断言：进程重建后原账本完全相等，但 currentValidity 为 stale。不能用整个展示对象恒等阻止合法失效。

本轮依照 go-multi-module-dev v2.6 的 owner／typed EventHub 约束实施，不新增运行单元、不改变 Runtime SM-1～SM-4，不创建 Run，不接入 STEP05。

## 自动验证

- File 全包、Workspace `pkg/...`、`internal/pkg/events` 全包通过，包含 concrete message graph。
- Workspace Biz 的 `Test.*Preparation|TestOriginalPreparedBootstrap` 定向集通过（61.149 秒）。首次未授权回环 listener 的沙箱运行失败，获准隔离回环后重新执行通过；不把前者记为业务缺陷或通过。
- workorchd Biz／server 的准备定向集通过（14.361／0.445 秒）。扩大回归曾发现重启展示恒等断言不适配 epoch 失效；修正分轴断言后通过。
- File／Workspace 模型／Workspace Biz 的 File 重观察和私有 DTO／Bootstrap 定向 race 通过；eventmapping 的无损、深复制、nil／空及重复／空 binding 反例通过。
- `go vet`（File、Workspace、workorchd Biz 子树）、`make verify-boundary-contract`、`git diff --check` 通过。
- STEP04 verifier 15 项、build identity 17 项通过。
- 对 a 批完整三操作原归档，typed DTO 往返后独立 persistence gate 与完整 Bootstrap 结构输出逐项相等；不只比较摘要。c 批最终完成后另行复验。

未运行全仓 `make test`、全 CI 或收费 Provider；继承 integration fullflow 与 orchestrator 包耗时问题不在本轮通过清单中。

## 生产证据与构建身份

受限原证据保存在项目忽略目录 `.workorch-data/`，不提交配置、诊断原文或产物到 Git。

- a：`step04-file-currentness-20261027-a`。File 观察接线的 HTTP／两次 OS 进程重启已通过；后续私有 DTO 改动不在该构建。其最后 reload 截图取于加载中，不能作为重载恢复的通过证据。
- b：`step04-file-currentness-20261027-b`。真实浏览器检查、关闭／重开／只读刷新和整页重载已观察到原身份与 stale；但验收期间修改了 HTTP 测试源码，build identity 在后续进程启动前正确拒绝 buildInput 漂移。该批整体失败，不把其证据算到最终源码。
- c：`step04-file-currentness-20261027-c`。冻结源码重建，最终结果及原证据索引见下节。

c 构建身份：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，含未提交构建输入 |
| buildInputSha256 | `f517879a1836e3e0930de1fb0fbc673fe809442c4677d9f39c19f59afe8646ed` |
| consoleTreeSha256 | `558280f9f714068c07b1c0f15ce1122e0484cbca022657ea35547220d6f4aa28` |
| binarySha256 | `b963099721df795410f0b9ff019fd197ea8f13b16c6640c231fe7885ca952a44` |

这些身份用于字节一致性检查，不是可信构建 attestation，也不把 Git HEAD 单独当作已测源码快照。

### c 批最终结果与索引

`result.json` 为 `pass=true`、`ready=false`、`M3Issued=false`。真实浏览器证据独立记录，脚本的
`browserVerified=false` 保持原值，不以 `browser-done` 标记冒充页面验证。

- 首次生产 Web 只显式检查一次：request `225c2ebd-787b-47c8-90c8-caa737fbb106`，revision 22→24；原检查 10/11 通过，唯一 outstanding 为 Provider。完整结构编译摘要 `prepared-bootstrap-sha256:fd37131539df20d36052c035354413766ca2b2c171b0657bda3382a417bcdc08`，页面明确不是 ready／M3／授权。
- `browser-explicit-check.txt/png`：真实检查及完整结构编译。`browser-stale-refresh.txt/png`：关闭／重开后只读刷新，原 request 与原证据保留，currentValidity 为 stale。
- `browser-reloaded.txt/png`：整页 reload 返回业务总览后，重新进入同一 Workspace 并打开准备面板；已等待显示原 request、stale 和 10/11，而非加载中的中间页。未提交新检查，也没有从浏览器持久存储恢复。
- `original-preparation.json`、`original-owner-record.bin`、`final-owner-record.bin`、`http-trace.json`：原请求重放／recover 不改历史；缺原配置时新操作 9/11，人工恢复原配置后再显式检查回到 10/11；共三个操作。恢复编译始终使用各原操作归档，不借当前文件补造旧事实。
- 两次独立 OS stop/start：PID `3960972`、`3963052`、`3963129`，重启读回、三个原操作完整 Bootstrap 重编、丢 body 对账均通过。结束后 read-only `ps` 确认三个进程均已退出。
- STEP04 增量 `step04ProviderHTTPRequests=0`、`embeddingCalls=0`、`runCount=0`，浏览器额外 SkillHub inventory status 配对 0。前序候选准备的 8 次 loopback fixture 推理单独计数，不是真实模型或 STEP04 推理。
- 完整原始 `provider-http-paths.json`、`skillhub-calls.jsonl`、进程日志和请求／响应留在同一受限根；构建身份在最终读回时再次核对通过。业务目录无 STEP04 改写。

这证明有界阻塞主链及本轮 File 失效行为，不证明所有非 Provider owner 的持续有效性，也没有签发 M3。

## 当前 live 边界与剩余项

允许且已在实施的验证是无收费、无 Run 的有界阻塞型生产 HTTP／Web／进程重启验证。不能称“除 Provider 外已全部完成”或完整 readiness／M3 正向 live 已通过。

主体出口仍须继续：

1. Soul／Skill（含安装材料）／Memory／MCP 等 required 原观察的完整当前有效性合取与最终 owner 重核；本轮 File 窗口不能代替其他 owner 证明。
2. 全 required 事实及完整 Bootstrap 满足后，Workspace 成功结算和 creation epoch 唯一历史 M3 的 final CAS；覆盖竞争、丢回复及重启。不以既有 no-Run 单项 fence 冒充成功事务。
3. 非空 Skill／MCP 与 semantic Memory 的原模式证据消费及边界验证；不把未支持或未授权场景自动降为空选择或 list-only。
4. Provider 授权合同仍独立 blocked；本轮未修改、未调用真实 Provider，不能用 API key／缓存／结构编译代替授权。

不能因有界用例原时点显示 10/11 单项通过，就跳过前两项主体接线。后续按上述主体出口继续，不扩展自动安装、MCP 启动、外部 probe 或执行授权。
