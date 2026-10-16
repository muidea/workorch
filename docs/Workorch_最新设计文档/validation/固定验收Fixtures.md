# 固定验收 Fixtures

- 包版本：`VAL-2026-10-12.1`；状态：设计输入，全部未执行
- 唯一场景规格：[验收规格](验收规格.md)。本页不是生产状态快照、自动测试运行结果或待填模板
- 所有 ID、时间、版本号、预算值、路径和效果数均是本包冻结的目标 fixture 标签。仅明确标成“基线常量”的值来自指定源码；不能将样例 ID 误报为生产事实
- Workorch 基线：`5360caf36e081717087557f34d2af3af1c16dc16`；组织规范：`6ba50452fccd33c912b07a0648fbb97acf15d756`、v2.6.0
- 此处是可独立审阅的持久记录语义投影。装配成真实 Go 值须由[具体合同目录](../contracts/组件具体合同目录.md)的 typed 字段映射完成；不新增 FixtureEnvelope、万能 payload 或运行期注册表。目标新增记录只在 NC01–NC05 的所属 owner 落地

## 固定身份简写对照

场景说明中的短称只允许按此表展开，不能作为不同ID发送。所有实际typed输入使用右列完整值：

| 短称 | 完整fixture身份 |
|---|---|
| ws1 / Run1 / Run2 | ws-fixture-01 / run-fixture-01 / run-fixture-02 |
| def1 / def2 / input1 / input2 | def-01/r1 / def-02/r2 / 原始输入revision1 / 合法修订输入revision2 |
| candidate1 / candidate2 | candidate-01/r3 / candidate-02/r1 |
| permit17 / intent1 / reservation generation1 | activation-permit-17 / initial/ws-fixture-01/intent-1 / WorkspaceRunGeneration1 |
| grant17 / use1 | exec-grant-17/r1 / 该grant对intent1恰一次使用记录 |
| material17 / seal17 / result17 / closeout17 / delivery17 | material-17 / seal-17 / result-17 / closeout-17 / delivery-17 |
| ticket17 / approve18 / confirm17 | ticket-change-17/r2 / decision-approve-18/r1（NC04正式决议） / confirm-17 |
| auth17 / binding17 / binding16 / fence5 | auth-17 / binding-17/hBinding1 / binding-16/hBinding0 / file-fence/5 |
| memory17r2 | memory-17/revision2 |

source/program/report的r41/r11/r9是资源revision，不是Run StateRevision或Runtime revision。便捷shortlabel本身不作为hash原文。

## 0. 固定比较与版本

1. 每个 case 重新从声明的 fixture 装载；不继承上一 case 的成功状态。未显式列入写集的全部权威字段、历史记录和预算维度必须逐字段不变
2. `H(x)` 在本页仅表示 UTF-8 精确字节的 SHA-256（小写十六进制），无 BOM，换行 LF。下面给出的资源内容 hash 可直接计算。生产的 Definition/request/operation/material hash 必须使用 owner 卡所固定的 canonical 格式及前缀，由独立参考编码器生成并随执行证据记录；不能拿 `H("标签")` 冒充合法生产 seal
3. `hDef1/hCandidate1/hManifest1/hBinding1/hDispatch1/hReceipt1/hMaterial1` 是指向下述完整语义对象及 owner 固定 codec 的内容寻址符号，不是任意可填写值。正常例使用对应对象唯一 hash；否定例必须只改指定字节并保留旧 hash。若实现 adapter 不能提供对象完整编码与独立 hash 比较，该例 BLOCKED，不可以 skipped/PASS
4. fixture 时钟 `t0=2026-10-12T02:00:00Z`；默认现在 `t1=02:00:10Z`；迟到 `tLate=02:10:01Z`。时钟只控制模型时间，SG2 停机/kill/排空必须另读真实任务与进程事实
5. 基线版本白名单：GoalSpecification.Version=`1`；DefinitionAdmissionCandidate.SchemaVersion=`4`；compiler=`workorch-definition-admission-v4`；CWI schema=`workorch.runtime_cwi_ledger`；DefinitionSeed=`workorch.runtime_definition_seed`；semantic manifest=`workorch.runtime_semantic_plan_manifest`；provider semantic document=`workorch.runtime_semantic_plan_document`；feasibility=`workorch.runtime_semantic_feasibility`；semantic ledger=`workorch.runtime_semantic_planning_ledger`；acceptance slice=`workorch.acceptance_slice_policy`
6. 负例明确值：GoalSpecification.Version=`99`；candidate schema=`99`；CWI schema=`workorch.runtime_cwi_ledger.future99`；损坏对象仅将原 `hI1` 首字节翻转而不改保存的 hash。白名单外的旧持久版本亦不得默默按当前编译器重写。其他 owner 的版本/空值白名单以它的固定 SHA 合同卡 C05 为准；没有旧版本读取证明即“不支持此组合”，并非默认兼容
7. 运行预算固定为独立子账而非第二份产品余额：provider physical-call 上限2、已承诺0；operation physical-effect 上限2、已承诺0；proof logical-attempt 上限2、已承诺0；typed reconciliation probe 上限2、已承诺0。单元预算负例仅把对应维度上限改为1。usage 未知保留 reservation，不从 physical-call 数推算 token/金额为0
8. 所有例默认 actor=`user-fixture-17`、Workspace scope=`ws-fixture-01`。自动政策主体=`policy-fixture-auto-1`，必须另记 delegation principal=`user-fixture-17`，不能伪装人工点击

<a id="fx-g"></a>
## FX-G：原 Goal、独立源清单、有限输入与验收 oracle

### G.1 原文与来源

原 Goal G1（下列整段 UTF-8 原文，source=`goal-original-1`、revision=`1`）：

> 为指定不可变 CSV 输入实现导入校验并产出 JSON 报告：每行校验 email 格式；发现重复 email 必须逐项报告；不得修改源 CSV；所有本 Run 调度的处理/验证步骤不得访问外网。若源 CSV 存在非 ASCII 姓名，额外产出 UTF-8 姓名处理统计。交付程序版本、报告与可复核测试记录。

独立审查者在候选生成前固定源清单 `source-inventory-1`；候选/LLM输出不得反向决定它：

| SourceRef | Requirement | Obligation | Required | DoD | 必需原始判据 |
|---|---|---|---|---|---|
| goal-original-1/email | R-A | O-A | true | DOD-AB | 对完整 hI1 每行按 VP-Email-1 判格式，并跑边界断言 EA01–EA06 |
| goal-original-1/duplicates | R-B | O-B | true | DOD-AB | 精确字节相同 email 的全部重复行；断言 EB01–EB04 |
| goal-original-1/source-readonly | R-C | O-C | true | DOD-C | 全执行区间只读强制及覆盖无缺口；最终 hash 仅作补充 |
| goal-original-1/no-egress | R-D | O-D | true | DOD-D | 全派发处理/验证区间均有禁外网强制及控制观察记录 |
| goal-original-1/conditional-name | R-E | O-E | true，条件性 | DOD-E | 全量扫描精确输入；非 ASCII 为 TRUE 要统计，FALSE 才有证据 N/A，解析失败 UNKNOWN |
| goal-original-1/delivery | R-F | O-F | true | DOD-F | 持久程序、报告、验证记录与最终被验证版本一致且用户可读 |

合法共享：O-A、O-B 共用 DOD-AB、一个任务和一个执行 receipt，但 terminal inventory 必须保留两项独立 obligation 身份、文本 hash、assertion 集合和 closure 判据。`runtime-obligation-A..F` 分别精确引用 O-A..F；任务拆合不影响其 all-of。

### G.2 输入与独立有限 oracle

源资源 `file:source/input.csv`，owner=`file`，resource revision=`41`，只读；准确字节为下面代码框内容加末尾 LF：

```csv
row,name,email
1,Ada,ada@example.test
2,Bo,bo.example.test
3,Cal,ada@example.test
4,Dee,Dee@example.test
```

`hI1=56212ee9d9f4a3c01a089dddb2110f9ed80f8080da99cd8745e59c1b6a62e61c`。VP-Email-1 在此有限fixture中定义：一个非空本地部分、恰好一个 @、域至少一个点、两端和中间无空白；不做 trim、大小写折叠或 Unicode规范化。本例不宣称 RFC 全覆盖。逐行冻结预期：row1 valid；row2 invalid；row3 valid；row4 valid。duplicate 组恰为 `ada@example.test:[1,3]`；row4与任何小写值不隐式等同；非ASCII姓名计数0，条件FALSE。

边界断言独立固定：EA01 `a@b.test` true；EA02 `ab.test` false；EA03 `@b.test` false；EA04 `a@@b.test` false；EA05 ` a@b.test` false；EA06 `a@b` false。EB01相同两行都报告；EB02大小写不同不归组；EB03唯一email不误报；EB04三次重复三行均报告。

报告的业务投影固定为：`inputHash=hI1; rows=[(1,true),(2,false),(3,true),(4,true)]; duplicateGroups=[("ada@example.test",[1,3])]; nameCondition=FALSE`。JSON字段序/空白不决定业务真值，交付实际原始字节仍需另有 hash。独立 oracle读取全部四行及十条边界断言，不调用被测生成器、reducer或同一验收validator。

I2仅将row4姓名`Dee`改为`李`，其他字节相同：`hI2=950d9ffb89e8bb70b49874eed66415c528738c2169a9d4dd2ba4baca01db1e2b`；nameCondition=TRUE、非ASCII姓名行数1。I2不是自动获准替换I1，资源/Definition允许后才用于新 binding。

### G.3 F2 六类 required scope

候选=`candidate-01/r3/hCandidate1`，specification=`spec-01/v1/hSpec1`。以下6×6矩阵是本fixture的独立 expected set；每格记录review kind、R/O、candidate hash、判据范围、审查者来源、证据引用、状态，正常值均PASS：

| 实际 review kind | A | B | C | D | E | F |
|---|---|---|---|---|---|---|
| dimension_coverage | email | duplicates | read-only | no-egress | condition | delivery |
| negative_path | malformed-email | missed-duplicate | write-attempt | egress-attempt | parse-error | missing-artifact |
| regression | EA01–06 | EB01–04 | original-bytes | all-dispatches | I1/I2-switch | final-version |
| resource_environment | exact-I1 | exact-I1 | source-mount | isolation-boundary | full-scan | durable-destination |
| traceability | source-A→O-A | source-B→O-B | source-C→O-C | source-D→O-D | source-E→O-E | source-F→O-F |
| verifier_feasibility | line-oracle | group-oracle | interval-control | egress-control | scanner | artifact-readback |

每格证据名固定为 `review/<kind>/R-<letter>/hCandidate1`。当前owner已按每一种review的RequirementIDs检查所有required Requirement（源码定位见验收规格§1.4）；adapter须将每条review映射到其精确scope，不能把一条PASS扩展解释成六种review。集合全覆盖仍不证明审查结论充分。坏版本 `review-bad-1` 仍保留六个PASS，但全部RequirementIDs只有R-A；坏版本2只漏`negative_path/R-B`；两者均缺scope。

N/A不能用来遮掩未知：本fixture上述36格全适用。若另一个支持的用途确实有不适用scope，须先固定该用途适用性政策、理由、依赖及独立证据；不能运行后因为检查失败才改N/A。条件O-E在I1上的有据N/A不是免去对R-E审查。

### G.4 Oracle边界与保留目标

源清单完整性、IDs/引用/集合覆盖、hash/版本/预算/实际调用次数、有限数据预期可机器复核。任意自然语言目标的语义完备性不可由schema、六个LLM PASS或本fixture自动证明；候选生成前由独立业务审查者审阅原文→原子承诺→判据的充分性，并将身份、时间、适用用途、争议及签署版本保存为业务review证据。不能改目标、删禁网义务、删断言或把未知变N/A取得通过。当前执行环境没有全区间只读/禁网强制时，FX-G正常全环为BLOCKED，展示能力缺口。

<a id="fx-c"></a>
## FX-C：创建、授权与 Definition 变更

| 持久记录 | 固定值与阶段 |
|---|---|
| 创建请求 | request=`create-17`，canonical input=`G1+template-csv/r2+resource-I1+manual`；owner workspace；状态从未接受到metadata→binding/index→resources→authoring→confirmed→M3；各阶段事实由owner journal保存 |
| Workspace | `ws-fixture-01`，revision=20，ActiveRunID空，WorkspaceRunGeneration=0；完整metadata+binding+按ID读取+列表索引均关联同ID |
| 候选/正式定义 | candidate-01/r3/hCandidate1；Definition=`def-01/r1/hDef1`；GoalSpecification v1，admission candidate schema4；startupInput=`startup-01/hStartup1` |
| 配置/资源 | template-csv/r2；soul/r4；skill-csv/r3；llm-config/r7；MCP schema/r5；source file/r41；output directory/r9；configHash=hConfig1 |
| 确认 journal | `confirm-17`，expectedWorkspaceRev20，exact candidate r3/hCandidate1，普通初创使用human exact confirmation=`human-confirm-17`（不强制Decision ticket/resolution）；只有NC04变更变体才绑定decision-approve-18/r1正式决议；完整after-image seal与索引可见性屏障未完成前不对外宣称正式定义有效 |
| NC01 readiness | `ready-17/r1`，ws/def/startup/config以上精确身份；owner检查file/r41与output/r9、shell-isolation/r6；checkedAt=t0，expiresAt=02:05:00Z，policy=`ready-policy/r1`；creationCompletion=`m3-17`只引用此历史事实 |
| 默认M3 | completion=m3-17；M3提交瞬间Run inventory空；不存在execution activation intent；authoring provider request及usage可存在 |
| NC02自动grant | `exec-grant-17/r1`，principal=user-fixture-17，delegate=policy-fixture-auto-1；inputHash=hInput1；allowed candidate predicate=保持G1的O-A..F及VP-Email-1、不得减少输出；actions=accept_exact_candidate+initial_activation；resource={I1只读,output dir写入}；runLimit=1；operation上限2/provider上限2；validFrom=t0，expiresAt=02:05:00Z；useLimit=1；revokedAt空；policyHash=hAutoPolicy1 |
| NC03激活身份 | 逻辑键`(ws-fixture-01,initialactivation)`；intent标签`initial/ws-fixture-01/intent-1`由Workspace持久分配；manual-request-1及auto-request-9只是传输request；同intent固定映射run-fixture-01；终态后仍保留映射 |
| NC04变更ticket | `ticket-change-17/r2`：原def-01/r1；原因“业务明确只需重复组计数”，影响O-B与报告结构、旧证据失效；final candidate=`candidate-02/r1/hCandidate2`；新def-02/r2/hDef2；精确approve=`decision-approve-18/r1`，须绑定完整原因/影响/最终候选，是否授权执行另行判定 |
| 旧历史 | def-01/r1与run-fixture-01 bootstrap/evidence/results不可改；新定义应用journal=`apply-18`；无前驱时Run数仍0直到NC02/03；有前驱时必须NC05 |

差分：过期grant用now=02:05:01Z；撤销用revokedAt=02:00:09Z/reason=`user_revoked_fixture`；未审candidate用hCandidate2但旧approve hCandidate1；readiness漂移用file revision42；部分物化只存在Goal文件、不存在DoD文件及visibility seal。

<a id="fx-r"></a>
## FX-R：四机 aggregate 与 lease

正常运行预像：Run=`run-fixture-01`、SM-1=`active`、Run revision=7、WorkspaceRunGeneration=1、DefinitionFocus=`def-01/r1/hDef1`；runtime record ref=`runtime/run-fixture-01`、StateRevision=100；公开Query/Store通过typed `RuntimeStateRecord`传递，Run负责持久化与owner fence，Orchestrator拥有SM-2/3/4语义；不能把原“opaque”称呼解释为当前公开DTO有opaque字段。

SM-2当前authority transaction=`auth-17`、semantic generation=2、focus=`focus-17`、manifest=`manifest-17/hManifest1`；CWI-A/B/C/D/E/F均未结清；proof binding=`binding-17/hBinding1`精确引用source-I1/r41、program-P1/r11、manifest与obligation；旧binding=`binding-16/hBinding0`只在history中。SM-3当前logical request=`request-plan-17`、stable prefix=`prefix-17`、session=`session-17`；尚未出站时session不含submitted。SM-4 drive lease=`lease-17`、generation=5、epoch=2、token=`lease-token-17`、expiresAt=02:01:00Z。旧worker=`generation4/epoch1/token-old`。

完整 identity 比较集：Workspace/Run/DefinitionFocus/authority transaction/manifest/logical request/stable prefix/session/dispatch/operation/execution key/fence/receipt ref+hash/evidence cutoff/material seal/ResultID+Closeout/WorkspaceRunGeneration/lease generation+epoch+token。各case只使用相关子集，不给每个消息增加公共超大头。

变体R-CAS：并发A/B都读runtime revision100；A提交允许写集after-image101；B携不同payload仍expected100。oracle期望只能一个101，B不得私自把expected改101重放旧计划。

变体R-REPLACEMENT：前驱run-fixture-01已terminal且Result=`result-17`；replacement=`replace-17`、request=`restart-17`、phase=`successor_materialization`；恢复失败计数7，下一次失败为8；状态exhausted后durable不重置、不装新timer、不激活pending child；同request重放仍`replacement_pending`。

<a id="fx-p"></a>
## FX-P：provider会话与真实消费

调用方可为authoring或runtime，但不能混用账：

- authoring：Workspace=`ws-fixture-01`，candidate-01/r3、phase=`proposal`或精确六类critic；request=`authoring-provider-17`，RunID空；Workspace request ledger及LLM usage存在是合法M3前事实
- runtime：FX-R、request=`request-plan-17`、session=`session-17`、target=model-fixture-1/provider-fixture-1、expected configuration revision=`llm-config/r7`；完整typed request hash由CompleteProviderDocumentCommand.RequestHash规则独立计算，前缀=`provider-document-sha256:`
- 正向受控返回：exact RequestIdentity/RequestHash/Target/ConfigurationRevision；ProviderSubmitted=true；唯一Document arm；content Observed=true/Complete=true；usage(input=120,output=80)；记录一条物理call和一条usage结算
- 截断返回：唯一Incomplete arm、Code=`output_limit`或`stream_ended_before_completion`、ObservedDocument.Complete=false；两个code各跑；physical=1；schema/semantic correction attempt仍0
- 提交未知：session durable已submitted、没有document/outcome；provider物理sink计数1；恢复后追加调用数0，session terminal/outcome=`provider_submission_unknown`，按Core终态交付；未知usage reservation不释放
- usage迟到：receipt=`usage-17`绑定同session/request；input120/output80在新generation到达仍结算原账；同receipt再到重复no-op。不是新Run的消费
- 结果坏形：Document+Failure两个arm；nil result；正确payload伴error；ConfigurationRevision=`llm-config/r8`；RequestHash翻转一字节。逐个隔离注入，不把合并坏形造成的一个error当五例通过
- preflight阻塞点：preflight开始时runtime rev100；期间intent退休并提交101，或唯一provider预算被其他session耗用，或lease升generation6；preflight返回后必须fresh-load，再session CAS；三变体均新增call=0

<a id="fx-o"></a>
## FX-O：sealed效果、receipt、proof与working/no-credit

operation=`op-write-17`；dispatch=`dispatch-17/hDispatch1`；execution key=`exec/run-fixture-01/op-write-17`；target=`file:output/report.json`；target before revision=8、before hash=hEmpty；effect fence=`file-fence/5`；expected after revision=9、content=`{"fixture":"report-17"}\n`，内容hash=`35f94112fb3daa64ee9f1cfb00858a28c7bc3edb88fe23fe3930b39edb07a7aa`。payload只有sealed单一来源；不允许旁路再传可变Data。receipt=`receipt-17/hReceipt1`包含上述全套identity、实际owner=file、effect outcome、before/after hash与revision；receipt由capability提供typed事实，经Orchestrator合同持久提交，不假设File另有权威receipt库。

调用计数分开记录：physical execute API次数、effect sink成功写入次数、receipt接受次数、预算reservation/settlement次数、credit次数。正常首次执行=(1,1,1,1/1,0)；普通proof后续独立evidence准入才可使相应credit+1；重复返回后effect sink始终1。

普通proof变体：operation phase依次sealed→claimed→owner_committed→evidence_committed；在owner_committed与evidence_committed之间的崩溃可恢复，不能提前credit，也不能为了补evidence重做effect。

working变体：ImplementationWorkPolicy显式声明；当前基线admission必须返回`ErrRuntimeImplementationWorkUnavailable`，物理效果0。未来关闭此缺口后的fixture：同CWI-A的working binding=`working-17/hWorking1`，不等于CoverageEvidenceBinding；OperationReceiptObserved一次CAS100→101同时更新effect/round、execution、working evidence/no-credit、预算结算和相关recovery retirement；SM-2 coverage credit仍0，ordinary proof binding未凭空新增。下一步fresh observation→fresh proof才可取得credit。禁止拆为100→101仅execution、101→102补working/evidence。

跨Run变体：前驱op-write-17仍unknown/in_flight、预算已承诺1且未确定结算；后继run-fixture-02拟写同一canonical资源，execution key不同不构成安全证明。owner支持的对账未证明静默/不冲突前新effect=0；可继续另一个独立只读目标须另过原授权/预算，不能由本变体推成全局停止。

<a id="fx-t"></a>
## FX-T：封存、ordered ACK与lineage

终态准备预像沿FX-R，runtime revision=120；all-of evidence cutoff=`cutoff-17`；当前各binding均binding-17 lineage，资源真实owner revisions={source-I1:file/41,program-P1:file/11,report:file/9,shell-isolation:shell/6}。各依赖owner独立query和资源读回都能佐证；runtime_authority合成revision不能替这四个值。

terminal decision=`completed`只在FX-G所有义务关闭时有效；material=`material-17/hMaterial1`，seal=`seal-17`、seal规范编码hash=`hTerminalSeal1`（与material内容hash hMaterial1分开核对），delivery=`delivery-17`；原Definition、source inventory、evidence cutoff、current bindings、资源revision与closeout都被seal固定。Result创建原ID=`result-17`、closeout=`closeout-17`。禁止恢复时用新资源版本重算同seal。

持久前缀与唯一合法续步：

| fixture | 已完成durable ACK | 下一动作 | 非法提前动作 |
|---|---|---|---|
| T0 | 无；material已seal | Result同delivery/material | Workspace/Run terminal |
| T1 | Result，result-17/closeout-17 | Workspace exact terminal delivery | 重建Result或先Run |
| T2 | Result+Workspace；ActiveRunID空 | Run exact terminal delivery | 开放不同的restart/successor新意图、声称Run terminal（原initialactivation重复仍查询返回原Run） |
| T3 | Result+Workspace+Run且适用post-terminal assertions成立 | 完成交付、读回用户结果 | 重开authority或修改seal |

普通closeout无后继时不得要求SuccessorRunID或后继双端lineage；只核对其适用的post_terminal_delivery assertions。NC05后继专属变体的lineage要求：旧Run.TerminalResultID=result-17；后继run-fixture-02.PredecessorRunID=run-fixture-01、PredecessorResultID=result-17；旧Run.SuccessorRunID=run-fixture-02与LineageState按Run owner事务一致；generation由owner分配2；后继先workspace_full reconciliation，不拷旧credit。三个ACK正确但适用的post-terminal assertion错、ResultID错或closeout不匹配均不算T3；不存在后继的普通closeout不因缺SuccessorRunID被阻塞。

<a id="fx-d"></a>
## FX-D：Workspace删除/退休

scope=ws-fixture-01；deletion request=`delete-17`；mode分别retain/purge；删除之前必须没有被认可的live active资格。Run retirement fence=`retire-17`；Orchestrator quiescence=`quiesce-17`；owner inventory固定为两个受管文件`output/report.json`、`output/program.py`，一个外部只读源`source/input.csv`（保留集，永不删除）。首次journal计数managed=2/preserved=1，proof=`purge-proof-17`。Eventstream position floor=900、旧最大position=899。

物理purge后owner完成CAS前崩溃：重载pending journal必须保留首次2/1与原proof，不从已空资源枚举重算0/1。retain后仅删除/封禁受管引用及相应owner记录，不冒称物理purge。迟到receipt/notification仍携原scope和旧revision；不得复生、分配旧position或删除源文件。

<a id="fx-l"></a>
## FX-L：进程、Hub与可变引用

装配顺序fixture：single persistence handle=`store-handle-1`；Initiator一次建立；owner Setup完成后各自登记具体typed订阅。正常任务清单：HTTP input1、Hub handlers2、provider calls0、effect calls0、timer1。停机顺序：停止接收HTTP→拒绝新业务→在途handler/effect有界收敛→取消订阅且确认→timer/task归零→owner Teardown→Initiator最后释放store。

失败变体：owner#2 Setup失败；unsubscribe第二次才成功；handler-2超过5秒drain deadline。前两种按已完成阶段恢复；第三种报告未排空并保留依赖store，不能为“teardown成功”先断持久句柄。重复Teardown不得double-close。

Hub同步路径固定为A lane→B lane，保留父context；重入B→A或独立A/B循环触发实际框架拒绝；不能通过background context规避。Post通知result=nil应安全返回、无result.Set依赖。clone样例：owner返回IDs=["row-A","row-B"]、嵌套SourceRefs=["source-I1"]；caller将第一个改成evil并append，fresh query仍原值。没有slice/pointer的具体值合同对此变异不适用，不能编造可变字段。

## 附加差分 FX-R-ADMIN / FX-T-LATE

唯一版本依据[运行期目标增量卡](../contracts/运行期目标增量合同.md#target-versions)：AdministrativeCloseoutRecord、AdministrativeTerminalMaterialIdentity、LateOperationReceiptFact、LateProviderUsageFact、RuntimeLateSettlementAppendRecord全部目标schemaVersion=1；负例0/99。行政RecordKind=administrative_only、AuthoritySource=RunAdministrativeIntent、outcome仅cancelled；不改变基线Runtime schema。

这两个差分仅用于目标新增合同，不是当前生产schema。FX-R-ADMIN取FX-C permit17/use1/reservation generation1，Run1分别pending或active但Runtime未初始化；runtime expected revision=0，明确Cancel cancel-preinit-17；同Run initialization/dispatch fence的birth/init-commit marker/runtime record generation/authoritative dispatch、claim与operation authorization inventory证明必须完整，空查询本身不等于证明。正常初始化与admin-only CAS竞争双方各先赢一次，完整expected见VS-NC03-CANCEL。

FX-T-LATE在FX-T/T3原terminal字节上另有目标append-only reconciliation partition revision0；原operation为op-write-17/key/fence5，原provider为session-17/request-plan-17/provider-request-17/llm-config/r7。provider没有operation身份。cons-provider-17预算scope原Run/原grant累计，currency USD、unit microUSD、reserved100000/confirmed0；partial usage60000→confirmed60000/remaining40000；可信最终100000→remaining0；独立超额120000分支如实记120000并阻止超额新调用。usage与receipt producer事实分别使用各自typed输入/ACK，完整cutpoint/去重/zero-effect见VS-NC05-LATE。

FX-R-ADMIN补齐target typed值：CancellationIntent后的ExpectedRunRevision=8；ReservationID=activation-reservation-17；PermitID=activation-permit-17/PermitHash=hPermit17；BootstrapHash=hBootstrap1；InitializationFence=17、DispatchAdmissionFence=29来自对应owner持久资格；ZeroBusinessAdmissionProof方法版本1并保存原inventory来源/revision/hash。缺任一证明就blocked。FX-T-LATE首次StoreRuntimeLateSettlement的ExpectedAppendRevision为非nil指针值0、AppendRevision1、PreviousRecordHash空、RecordHash独立canonicalhash、原seal-17/hTerminalSeal1；opaque最大1048576字节，1048577负例拒绝。正常第二条append要求expected1→2且PreviousRecordHash精确等第一条RecordHash。


<a id="fx-current-adapter"></a>
## FX-CURRENT 固定SHA的执行适配边界（2026-10-12）

本节只固定上述fixture如何进入当前实现，不另定义公共合同。代码路径/行号和静态测试边界见[验收规格§1.4](验收规格.md#current-source-evidence)。

| Fixture | 当前生产形状与装载入口 | 单变量坏像、观测与不得误判 |
|---|---|---|
| FX-R | `RuntimeStateRecord.StateRevision=100`与Run自身revision7分开；Store的ExpectedRuntimeStateRevision是非nil指针100，Query按WorkspaceID/RunID及可选exact ref/revision查；返回StatePresent与RuntimeState配对 | 未映射旧opaque blob字段不是产品FAIL，应先修测试adapter；原record Validate失败不能通过Clone/Normalized洗白；accepted/CAS mismatch/semantic rejection/owner transient四类按真实feedback字段断言 |
| FX-G/F1 | `RuntimeSourceObligationInventory` schema=`workorch.runtime_source_obligation_inventory/v1`；exact CandidateHash/SpecificationHash/DefinitionRevision/StartupInputHash；source、resource、requirement、obligation、verification五个集合完整装载 | O-A/B共享DOD-AB仍产生独立checkRef与原criterion字节；删B、跨Definition/CWI、改criterion或inventory hash分别拒绝。此inventory是候选的不可变来源投影，不能替代候选之前独立固定的原文充分性oracle |
| FX-G/F2 | 每一种review以其RequirementIDs覆盖R-A..F；正常36格不依赖Review数组恰有6元素这一计数 | 缺一required×kind、UNKNOWN、旧candidate hash、无据N/A仍各自独立；当前ValidateReviewCoverage具体error是覆盖缺项诊断，不能要求它已返回目标NC_REVIEW_INCOMPLETE enum |
| FX-C/D2 | 先Decision精确RegisterContractChangeProposal，再Workspace PrepareDefinitionChange及独立candidate review；原StartupInput/seal/authoring ledger保留 | ActiveRunID非空、proposal.RunID非空、原permit未结算目前准备路径拒绝；无Run可准备不等于NC04的运行中matching/formal resolve/adoption已接通。旧review不能给新candidate确认；预算历史不重置 |
| FX-O/working | a公开准入保留Unavailable负例；b内部同CWI工作fixture允许观察真实File receipt CAS后进程退出及fresh-process恢复 | b须标private-seeded、不可升级成公开M3→Start→交付SG3；两轮receipt、预算与allocation按原身份保留，working receipt不给terminal credit；integration公开全流文件存在仍不是运行PASS |
| FX-L | application admission gate、HTTP accepted connection/handler ledger、owner lifecycle各有真实退出点 | 关闭连接不等于handler退出；超时保留未清理ledger/store；反复Stop/Teardown只重试未完成项，所有owner切点继续继承V35/36 |
| FX-C/template | 父child以ParentTemplateIDs和EffectiveSources形成冻结输入；旧Run保留原snapshot | 改父Goal与子冲突或删被引用父必须拒绝；合法父描述更新影响下一次resolved snapshot hash，不热改旧Run |

当前普通RuntimeStateRecord是typed记录；FX-T-LATE中的1MiB opaque append是目标新合同的显式字段，不能套用于当前普通记录。FX-R-ADMIN及late settlement仍使用目标合同版本，正常Runtime记录不得假扮行政收尾或独立追加分区。
