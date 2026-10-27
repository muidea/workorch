# STEP04 非 Provider 来源义务编译接线与验证记录

日期：2026-10-26（Asia/Shanghai）。基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，包含累积未提交修改。

## 已完成的非 Provider 主体接线

原流程此前已保存完整单项检查和 all_of 汇总，但来源义务编译仍主要停留在独立纯函数验证。
本轮将 Provider 无关的完整来源义务编译接入**同一次原结算 CAS**：

精确封存/原 manifest → 完整 candidate/startup/admission → 来源义务 inventory 纯编译 →
同次保存有界摘要 → typed HTTP/Web → 原身份重放/恢复/owner 重开。

新摘要 `DefinitionCompilation` 使用 `workorch-preparation-definition-compiler/v1`，保存原
snapshot/manifest identity、sourceInventory/profile hash，以及 source/requirement/obligation/verification 数量。
Provider 仍 blocked 时可以独立完成此编译，不填虚构模型、不调用 activation port、不新建 Runtime。
公共摘要不含原 Goal/DoD、资源或配置 bytes。

File/memory repository 对带摘要的历史在写入、读取时都从完整原 seal 及来源重新编译并精确比较全部值；
不接受 hash 命中或 caller 已验证标志。旧记录无摘要不回填；原中断、漂移不补编译，完成后不能删除或改写摘要。
原材料和诊断不改写，新增字段 clone 独立。原 no-Run、epoch、reservation、版本与 CAS 门禁不放松。

按 go-multi-module-dev 的既有 owner 边界落地：只扩展 Workspace 私有准备事务与纯编译、repository backstop 和显示投影；
没有新增运行单元、跨 owner 直写或 Runtime SM-1～SM-4 迁移。规范入口 Workspace readme/service-api 已同步。

## 结果边界

这是完整来源义务图的纯编译，**不是完整 PreparedBootstrap、ready、有效期租约、M3 或执行权限**。
真实回环 HTTP 有界 Profile 仍为 `11 required / 10 passed / outstanding=provider_cache`，原身份重放、显式恢复和 owner 重开一致。
非空 MCP 等其他场景保留原阻塞，未从空选择规则推导通用通过。

本轮没有改 Provider 逻辑、访问网关、发推理/embedding、启动 MCP、创建生产 Run 或进入 STEP05。
不能宣称“除 Provider 外所有项已经完成”。明确的剩余主线为：

1. 完整成功 bootstrap 的生产接线及各 owner 有效性汇总（现有时点事实不能升级成持续有效租约）。
2. F07 唯一历史 M3 成功结算：完整 required 与有效性满足后，最终 CAS 重核 exact seal、creation/preparation epoch、pending 与完整 no-Run 事实；保留历史与当前失效分轴。
3. 最终生产构建的实际浏览器、独立服务进程 stop/start 与对应正向/阻塞/恢复验收。

Provider 缺可信授权不允许跳过，但上述未完成代码/验收也不一概归因于 Provider；本轮没有伪造正例掩盖它们。

## 验证证据

受限本机原证据根：`/tmp/workorch-step04-definition-mainline-8SOEXu`。原始材料与二进制不提交 Git；临时路径不代表长期归档。

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 受影响 Go 完整回归 | 16 个有测试包通过，2 个包无测试文件 | `affected-go.log` |
| 定向纯编译 | 原 Goal/DoD/Profile/manifest、无模型输入、零写入断言通过 | `pure-compile.log` |
| 最终有界真实 HTTP | 完整生产 transport 与持久 owner 链，Provider blocked；原摘要重放/恢复/重开一致 | `http-bounded.log` |
| 定向 race | Application biz（含六场景 HTTP）、Workspace biz、repository、management 四包通过 | `race.log` |
| Web 组件 | 13 文件、195 项通过 | `web-components.log` |
| Web 定向 | 准备面 66 项通过 | `web-targeted.log` |
| Web 规则 | 11 项通过 | `web-rules.log` |
| vet | 退出 0 | `vet.log` |
| 完整边界 | 退出 0 | `boundary.log` |
| 生产 Web 构建 | 退出 0；原大 chunk 告警保留 | `console-build.log` |
| Go 二进制构建 | capture 后构建，退出 0 | `binary-build.log`、`workorch` |
| 构建身份 CLI verify | 固定完整四字段身份，退出 0 | `source-snapshot.json`、`build-identity.json`、`expected-identity.json`、`build-identity-verify.log` |

完整 Go 命令：

```sh
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchmanagementcontract ./internal/pkg/repository/... ./internal/pkg/workorchrepo -count=1
```

race 在 Application biz、Workspace biz、repository、management 执行：
`-run 'TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestPreparedBootstrapPureCompilationIsNotReadiness|TestPreparationDefinitionCompilation' -race -count=1`。

新反例覆盖原 snapshot/manifest/Profile 替换、伪造 inventory hash/count/version、删除 seal、无原来源导入、clone 隔离；
HTTP 实际使用文件 owner，独立 backstop 拒绝篡改后 CAS。Web 显示编译与阻塞独立，不自动 POST 或执行。
既有 context 缺失历史反例明确去除本轮新摘要以模拟旧版本，仍不允许替换已完成历史。

HTTP listener 只用回环地址；候选准备、SkillHub status、Provider cache 等测试适配器使用既有本地 fixture，
不是收费 live 或真实模型质量验证。owner 重开不等同独立 OS 进程重启。未执行全仓 CI、实际浏览器或部署验证。

## 最终本地内容身份

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，加当前未提交构建输入 |
| buildInputSha256 | `178848a1b4b9be0b41d45f76b3c2c03ca9281854e4e86096980af4c6caad180a` |
| consoleTreeSha256 | `7d6f3b3d9d7a583b2b457775ba9c8b86105e34e575a9719f4d757bbfaf431f33` |
| binarySha256 | `3191f13430a32423d6c5ffa4e288e0847c254cd2edd89ec8e2517f1fdf42ba46` |

先完成 Web 构建，再 capture，随后构建二进制并 produce/verify。身份只证明本机内容及构建前后输入一致，
不证明可信构建 attestation、最终 Git 提交已测或部署运行通过。本轮未提交 Git，已有累积修改全部保留。
