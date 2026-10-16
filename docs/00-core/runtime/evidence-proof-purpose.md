# Evidence Proof Purpose

Status: normative

Type: core-contract

## 历史行为与当前资源

原 capability receipt 的实际读取与 effect 后像是不可变历史。后续合法修改同一资源时，第一份后像必须按真实 owner
观察成为 stale；不能进入当前资源依赖集合，也不能因资源恢复为旧 bytes 而恢复旧验收 credit。

`evidence-applicability-dependencies/v2` 显式声明 `historical-effect-proof/v1`、原 owner、operation kind 和 owner
发布的版本化用途合同。它只允许原 state-change receipt 证明真实发生过的行为，不证明资源当前符合验收。
当前实施的用途合同为 `file/mutation-historical-effect/v1`；operation 用途资格由技术 descriptor 发布，Core
不按 File、路径、文件类型或诊断分支放宽门禁。完整用途进入原 receipt/fact/adjudication/材料身份。

用途只能从原 sealed all-of binding 编译：同一 exact binding 必须同时声明 state-change 与独立普通 validation。
execution context 只传递这一派生声明；receipt admission、独立重载和只读来源反查核对原 dispatch 的完整 binding。
state-change-only、acquisition binding 与旧 v1 receipt 保留原 current 适用性合同，不按 operation kind 自动升级。
dispatch 必须属于普通 Definition state-change；带原 repair commitment 或 implementation-work commitment 的中间
transaction 不获该用途，即使保留原 effect+validation binding 也不例外。执行、receipt、read-source 与重载反查同一限制。
用途声明进入 executable contract revision，旧 binary 下非终态 authority 不自动迁移。

终验必须取得同 binding 的普通 validation、原 actual reads/current dependency pair 及本 evidence revision／
reconciliation generation 的 matching owner observation。历史 effect 与普通证明合取；独立证明缺失、stale、未采样、
未通过完整检查、来源错误或 equivalent-binding 反证失败均不能封存成功材料。旧普通 validation 的 stale 仍精确
reopen CWI、退休 binding／selector 并要求重新绑定和真实验证，不凭历史行为恢复旧 credit。

## 四机与恢复

- SM-1 不变，Run 独占 lifecycle。
- SM-2 区分 behavioral proof eligibility 与 resource currentness。历史 effect 不产生 current-read debt；若只读 query
  观察到旧后像 stale，原 stale 事实仍完整保存，不授权重新执行 effect。
- SM-3 从原 sealed binding 派生用途，receiving owner 生产 actual historical/postcondition pair。原 receipt、effect、
  execution key/fence、预算和 anti-replay 门禁不变；跨 owner 只用 concrete typed EventHub。
- SM-4 只采样当前普通证明并投递已裁决材料。terminal current union 排除历史 effect 的输入和后像，逐项材料完整
  保全历史；相同当前资源的版本冲突严格拒绝，不以“最新一份”或用途声明掩盖冲突。

只读历史查询的 `RuntimeEvidenceApplicabilityRequest.PurposeRef` 必须从原 receipt／fact 的用途派生并进入 request
identity；调用方不能为普通来源自报历史用途，也不能删除原历史用途。提交观察仍须遵守原 state/evidence revision、
reconciliation generation、完整来源、无 active dispatch／未结算 provider 的窗口与同一 SM-2 private event/CAS。
历史观察完整追加保存，真实 changed／missing 仍为 stale，不作为当前资源 proof。它不进入 current applicability semantic
hash，不改变当前 authority transaction，不退役普通未封存 candidate／request，也不重开或恢复旧 credit。
该审计 CAS 只能追加原观察及推进 aggregate revision；不能重跑 focus advancement／decision sealing，甚至不能增加
相同 action 的 attempt。独立 transition gate 按原持久前像反证完整写集，禁止夹带候选、credit、receipt 或预算变化。
普通来源的 current 观察及精确 stale/reopen 门禁保持原样；历史查询不要求普通 drive 新增采样或预算。

历史 effect 不替代当前资源快照、普通 validation、semantic equivalence、原准则、quality、完整 all-of、assessment、
终态材料消费者或有序三 owner ACK。重启只消费原 aggregate，不迁移或补写历史用途。

## 跨 CWI 用途来源合同

`runtime/cross-cwi-evidence-purpose/v1` 是独立的显式 Definition opt-in，不是跨源 work-round／repair lineage guard。
规格的 `EvidencePurposeMappings` 每项含 `ContractRef + HistoricalItemRef + AcceptanceItemRef`。两端只能是不同且唯一
的原 DoD candidate ref；每个历史来源只有一个普通当前验收来源。禁止自映射、歧义、映射链和依赖环。
历史来源保留 owner 发布用途资格的普通 state-change-only 声明；验收来源保留独立的普通 owner-attested validation。
repair／implementation-work 中间事务不能借用这一合同。

完整映射与全部原准则进入 proposal hash、独立六审、用户 exact confirmation、Specification admission hash 及
startup-bound source inventory。映射改变必须重新评审和确认；运行期不得依据文件名、CWI 顺序或文本猜选，
自洽重算当前 hash 不得替代原确认身份。Application 的纯 compiler 只添加“验收后继依赖历史前驱”的 exact 边，
不得添加反向终验依赖、改变原文本或将映射本身作为证据。
pre-seal 同时反证行为进度与终验结算的双用途依赖闭包：仅精确 consumer 的前驱依赖可消费行为完成，其它依赖
仍等待前驱终验。即使原 CWI graph 为 DAG，“consumer 依赖中间项，而中间项等待该历史前驱终验”的隐藏结算
环仍须零执行拒绝；不能让 Runtime 留在没有可继续成员的 active/nonterminal 状态。

SM-2 必须把行为进度与终验 credit 分离：原完整 accepted effect/receipt 仅允许前驱退出行为执行并解锁映射点名的
精确后继；前驱在普通当前证明形成前保持终验未满足，不能以 `satisfied/settled` 冒终验已完成。
后继普通证明必须合取覆盖前驱全部原义务与自身原义务，再由 SM-2 反查原映射、完整 effect lineage 和真实当前
普通证明结算前驱；缺失、歧义、未确认、来源漂移或 stale 均阻塞。普通证明 stale 必须撤销相应终验 credit 并精确
重开验收来源，历史行为保持原不可变事实，不重发 effect、不复活旧 credit。

逐源普通检查由不可变 inventory 与 exact consumer CWI 纯编译：验收来源同时承接所有点名历史来源的原
obligation/verification 以及自己的原 obligation/verification。每项完整 Criterion/Declaration 字节与来源身份保持
原值，check ref 重新绑定验收 CWI，而不能复用前驱的 check ref 冒充后继执行。共享的同一原 verification 只要求
一次，不同原 verification 均必须合取；不按文本相似性去重。完整映射已进入 inventory hash，编译结果进入 seed、
ordinary validation requirement、owner native mapping、输入逐项评审和正式逐源材料；独立重载重编全部原来源，
不能通过协调重算 check/requirement/seed hash 隐藏任何一项。评审必须逐 check 使用该源原 Criterion，不用后继
CWI 的共同文本替代历史准则。这些检查声明仍不证明历史 effect、当前资源、assessment 或终验 settlement 已成立。

seed、用途映射与正式 closeout 的完整逐源反证可在一次纯调用内复用已完整验证的同一 detached inventory，再逐 CWI
编译检查。不得缓存成功、只信调用方 hash 或跨调用／恢复复用；公开检查编译入口每次完整验证原 inventory。
复用不能改变来源、原文、检查顺序、缺项／冲突的拒绝顺序或任何 credit／预算／owner 事实。

原完整 effect 的 evidence CAS 同时保存 `RuntimeHistoricalBehaviorCompletion`，携带原用途来源、binding ref、
唯一 completed dispatch 与完整 evidence member refs。独立 admission／重载必须重放原 accepted semantic batch、
provider document、receiving-owner feasibility、execution key/fence、receipt 正文与 processed evidence fence；
局部 binding hash 不证明原来源。前驱保持 `bound/open`，退出行为执行的 focus 仍保留在 canonical completed queue，
不进入 future planning，也不因此成为 terminal satisfied。共享同一验收后继的其它历史来源仍按自己的原准入执行；
后继尚缺这些来源时，真实 effect CAS 不能因为后继暂未就绪而丢失历史进度，不能借前驱终验 credit 解锁其它依赖。

`RuntimeHistoricalAcceptanceProof` 从原后继普通 Definition assessment 的封存输入纯编译，反查前驱全部 effect
不晚于原 evidence cutoff、后继原 accepted semantic binding、完整逐源 requirement/native-check 映射、真实 owner
receipt 与原 applicability observation prefix。acquisition／repair／work 或 producer satisfied claim 不构成终验。
SM-2 只能在原普通 assessment CAS 内追加这一 proof 并同时结算两端；前驱 evidence refs 合取行为与普通验收来源，
不能把后继回执改写成前驱 owner 输出。proof history 按原 assessment chronology 有界 append-only；旧 proof 不因
新 generation／观察而改写。验收来源 reopened 或 current binding／assessment 身份失效时，同 CAS 撤销前驱 credit，
保留历史行为及全部结算 proof，前驱回到原 `bound/open` 而不重新进入 effect 执行。针对前驱验收的 reopen 必须路由到
原唯一普通 consumer；不得重开 create-only 行为。SM-4 逐源材料保留原前驱 check/text identity，但检查其独立 consumer
check；普通 evidence/current binding 仍指向真实后继。成功材料另要求 current revision/reconciliation generation 的
fresh owner observation，不能用原 assessment prefix 代替这一次采样。

正式 Result 的逐源索引在含映射的 inventory 中使用 `source-check/<原 CWI checkRef>`，以区分同一原 verification
在历史来源与验收来源中的独立条目；未声明映射的 inventory 保持既有索引身份。索引只标识原来源，不改变
Criterion/Declaration、普通检查映射、evidence owner 或验收 credit。

SM-3 只能从原 sealed Definition mapping 与普通 semantic binding 编译 receiving-owner 用途，原 receipt 与 fresh-load
均反查同一来源。SM-4 成功材料逐项保全历史及当前来源，但 current union 只消费真实普通当前证明，原资源版本冲突
门禁不放宽；所有原准则仍 all-of，三 owner ACK 顺序不变。SM-1 不新增生命周期。

历史 endpoint 的 `HistoricalAcceptance` 声明由原 inventory 纯编译，携带 exact 两端原 candidate/CWI identity、
inventory hash、原两端全文 bytes hash 与后继完整逐源检查 hash；原 seed、semantic obligation、sealed binding 和
semantic proof identity 均保全这一声明。历史 obligation 仍为 ordinary state-change-only，原逐源检查只保全来源，
不能伪造本 CWI 已执行普通验收；其后继完整检查仍独立必需。acquisition、repair、implementation-work 不得携带
此声明。等价 binding 比较必须比较完整用途来源，换后继或原 checks 不能复用旧行为 credit。
receiving-owner 用途仅从原合法普通 dispatch 派生；mapped receipt 必须生产实际读取／后像与版本化历史用途，
缺失或降级 v1 直接拒绝。独立重载与回执准入使用同一原 dispatch purpose gate；原 v1 或未确认 state-change-only
来源不升级。封存声明与取得历史 receipt 仍不表示前驱终验通过，也不替代延期结算消费者。

声明／确认的结构合法性不等于可执行 availability。映射执行入口必须在协调行为进度、终验消费者及恢复不变量
均可用后开放；不具备这些消费者的 binary 在 bootstrap／seed 边界明确拒绝，不能降级为旧 same-binding 用途。

### 确认的 Profile deliverable 来源域

`runtime/profile-deliverable-evidence-purpose/v2` 是独立来源域版本，不能用 v1 将 Profile 交付来源冒充原 DoD。
每项除两端 ref 外，必须封存 `HistoricalProfileID + HistoricalDeliverableID + HistoricalProfileHash + HistoricalText`。
历史 ref 固定为 `profile/<ProfileID>/deliverable/<DeliverableID>`；快照 hash 由单个完整 Profile 的既有 hash compiler
生成，全文必须精确等于该 required deliverable 的原 `Description`。全 Profile 集合内 deliverable source ID 必须唯一，
不能 first-match；该来源不添加到原 DoD inventory，不生成假 DoD、假 owner fact 或运行期目标。
v1 wire 禁止携带这些扩展字段（包括空值）；v2 缺项、错版本、未知字段、null 或错误类型仍由严格 decoder 拒绝。

完整来源域字段进入规格内容／六审／确认／admission／startup inventory hash，确认前 UI 展示原全文和快照身份。
Workspace pre-seal 与 Runtime bootstrap／fresh-load 分别反查精确 Profile 快照、required 声明及唯一原 DoD consumer。
Profile 变化不能只重算 mapping hash 延续原确认，须重新评审和确认。仅精确 consumer 可消费历史行为以继续执行；
其它源依赖仍要求前驱终验，双用途依赖闭包继续拒绝隐藏结算环。

历史来源仍是 owner 发布用途资格的普通 state-change-only 声明；repair／work 中间 transaction 不获得历史用途。
consumer 可另有已封存工作策略，但该策略不代替普通 Definition validation：work commitment／回执本身零终验 credit，
最终必须使用普通 purpose 的独立 all-of assessment、真实当前读取和完整 source checks。独立检查 compiler 为 Profile
全文生成绑定 inventory／原来源／exact CWI 的检查，consumer 还必须合取全部自己的原 DoD checks；原交付义务与全部
DoD 仍保留，不能以相似文本去重。材料同时保全真实历史 effect 和验收 consumer，current union 不含历史后像。
SM-1、原 receipt/key/fence、anti-replay、预算及三 owner ACK 不变；SM-2/3/4 复用原用途来源和私有 CAS，不增加状态机。

## 验证合同

共享纯合同验证版本、owner/kind、完整绑定、缺普通证明、原读取／后像保全、stale 不提升 current、JSON reload、
clone 隔离和 current union 冲突。File producer 保留原 actual-byte oracle，未声明用途仍输出 v1。
共享文件与多焦点 drive 必须保留后续真实修改；重开只能由 exact owner stale observation 和原 binding lineage 证明。
新 focus 不是旧 selector 复活；CAS 中断、响应丢失、duplicate／restart 不重复 effect、receipt、预算或 credit。
历史审计的 prepared／document-complete／pending／proposal 窗口须覆盖 CAS 前失败、CAS 后响应丢失与原 JSON 重载，
保持普通 authority／candidate／semantic／reconciliation 原值，且协调 request 重哈希不能改变原用途。
共享文件主链另取真实 File 终态只读回查：旧后像仍 stale，原 behavioral eligibility 不冒 current；查询前后原 aggregate、
receipt、effect、budget 与正式材料完全不变。终态只读查询不是观察 writer 的 CAS 或 fresh OS-process 验证。
脚本 provider／private startup 的合同验证不等于公开 fullflow 或 live。

跨 CWI 用途须另覆盖完整映射 wire/schema、原文字节、hash／clone／重载、未确认及协调重哈希替换、歧义／环、
无 owner 用途／无普通验收的零执行拒绝。Runtime 正例必须保留真实 create→edit→Git 和行为修复；前驱延期窗口零
终验 credit，后继 all-of 后才结算，CAS 前后／响应丢失／fresh restart 零重复 effect／provider／receipt／预算。
pre-seal、UI 或 availability 反例测试不能代替该完整 Runtime 闭环。
