# mcpregistry：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页11个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

MCPRegistry是server/capability注册资产owner，目录名mcpregistry与wire catalog label可不同；CT ID用稳定目录owner。注册数据必须包含typed配置/schema/revision/provenance和支持边界；不把shared mutable registry或credential返回给caller。

注册/发现/能力测试事实不等于实际操作许可，远端执行必须走MCP。调用方封存exact server/capability snapshot；schema改变导致fresh admission，不悄悄适配旧sealed operation。

## 逐topic合同

<a id="ct-mcpregistry-mcpcommandcreatecapability"></a>

### CT-mcpregistry-MCPCommandCreateCapability

- 边界：`TopicMCPCommandCreateCapability` = `/mcp/command/create-capability`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L287)
- 接收provider：mcpregistry；Command [CreateCapabilityCommand](#schema-createcapabilitycommand) → Result [CreateCapabilityResult](#schema-createcapabilityresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [CreateMCPCapability](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_capability_commands.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandcreateserver"></a>

### CT-mcpregistry-MCPCommandCreateServer

- 边界：`TopicMCPCommandCreateServer` = `/mcp/command/create-server`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L283)
- 接收provider：mcpregistry；Command [CreateServerCommand](#schema-createservercommand) → Result [CreateServerResult](#schema-createserverresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [CreateMCPServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_server_commands.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandensuredefaults"></a>

### CT-mcpregistry-MCPCommandEnsureDefaults

- 边界：`TopicMCPCommandEnsureDefaults` = `/mcp/command/ensure-defaults`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L293)
- 接收provider：mcpregistry；Command [EnsureDefaultsCommand](#schema-ensuredefaultscommand) → Result [EnsureDefaultsResult](#schema-ensuredefaultsresult)；target合法caller：kernel/system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：system [ensureMCPDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L104)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandgetcapability"></a>

### CT-mcpregistry-MCPCommandGetCapability

- 边界：`TopicMCPCommandGetCapability` = `/mcp/command/get-capability`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L288)
- 接收provider：mcpregistry；Command [GetCapabilityCommand](#schema-getcapabilitycommand) → Result [GetCapabilityResult](#schema-getcapabilityresult)；target合法caller：application/workorchd/context-registry、application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [ReadMCPCapability](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_capability_commands.go#L65)；workorchd [ReadMCPCapabilityByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_mcp_reads.go#L30)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandgetserver"></a>

### CT-mcpregistry-MCPCommandGetServer

- 边界：`TopicMCPCommandGetServer` = `/mcp/command/get-server`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L284)
- 接收provider：mcpregistry；Command [GetServerCommand](#schema-getservercommand) → Result [GetServerResult](#schema-getserverresult)；target合法caller：application/workorchd/context-registry、application/workorchd/appservice、blocks/mcp
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [ReadMCPServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_server_commands.go#L104)；workorchd [ReadMCPServerByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_mcp_reads.go#L14)；mcp [GetServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L910)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandlistcapabilities"></a>

### CT-mcpregistry-MCPCommandListCapabilities

- 边界：`TopicMCPCommandListCapabilities` = `/mcp/command/list-capabilities`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L289)
- 接收provider：mcpregistry；Command [ListCapabilitiesCommand](#schema-listcapabilitiescommand) → Result [ListCapabilitiesResult](#schema-listcapabilitiesresult)；target合法caller：application/workorchd/context-registry、application/workorchd/read-model
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [ListMCPCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_capability_commands.go#L78)；workorchd [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelports/mcp_readers.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandlistservers"></a>

### CT-mcpregistry-MCPCommandListServers

- 边界：`TopicMCPCommandListServers` = `/mcp/command/list-servers`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L285)
- 接收provider：mcpregistry；Command [ListServersCommand](#schema-listserverscommand) → Result [ListServersResult](#schema-listserversresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [ListMCPServers](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_server_commands.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandreadregistrysnapshot"></a>

### CT-mcpregistry-MCPCommandReadRegistrySnapshot

- 边界：`TopicMCPCommandReadRegistrySnapshot` = `/mcp/query/registry-snapshot`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L291)
- 接收provider：mcpregistry；Command [ReadRegistrySnapshotCommand](#schema-readregistrysnapshotcommand) → Result [RegistrySnapshotResult](#schema-registrysnapshotresult)；target合法caller：blocks/mcp
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：mcp [RegistrySnapshot](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L940)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandreadselectionsnapshot"></a>

### CT-mcpregistry-MCPCommandReadSelectionSnapshot

- 边界：`TopicMCPCommandReadSelectionSnapshot` = `/mcp/query/selection-snapshot`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L292)
- 接收provider：mcpregistry；Command [ReadSelectionSnapshotCommand](#schema-readselectionsnapshotcommand) → Result [SelectionSnapshotResult](#schema-selectionsnapshotresult)；target合法caller：application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [ReadMCPSelectionSnapshot](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_mcp_reads.go#L46)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandupdatecapability"></a>

### CT-mcpregistry-MCPCommandUpdateCapability

- 边界：`TopicMCPCommandUpdateCapability` = `/mcp/command/update-capability`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L290)
- 接收provider：mcpregistry；Command [UpdateCapabilityCommand](#schema-updatecapabilitycommand) → Result [UpdateCapabilityResult](#schema-updatecapabilityresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [UpdateMCPCapability](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_capability_commands.go#L35)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-mcpregistry-mcpcommandupdateserver"></a>

### CT-mcpregistry-MCPCommandUpdateServer

- 边界：`TopicMCPCommandUpdateServer` = `/mcp/command/update-server`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L286)
- 接收provider：mcpregistry；Command [UpdateServerCommand](#schema-updateservercommand) → Result [UpdateServerResult](#schema-updateserverresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L53) → `handleCommand`（mcpregistry）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)
- 已观察外部引用：workorchd [UpdateMCPServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/mcp_server_commands.go#L31)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)；[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-capability"></a>

### Capability

- 源码：[internal/modules/blocks/mcpregistry/pkg/models/mcp_capability.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/models/mcp_capability.go#L31)
- 字段：`ID common.MCPCapabilityID`；`ServerID common.MCPServerID`；`Kind string`；`Name string`；`DisplayName string`；`Description string`；`Category string`；`Tags []string`；`UsageHint string`；`InputSummary string`；`OutputSummary string`；`StageHints []string`；`RiskLevel string`；`EffectScope string`；`IdempotencyClass string`；`ValidationProofApplicability string`；`Schema workorcheventpayload.Payload`；`URI string`；`Enabled bool`；`ApprovalPolicy string`；`Source string`；`SourceRef string`；`Revision uint64`
- 字段引用类型：[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)；[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)
- 实际方法：[CloneCapability](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/models/mcp_capability.go#L65)；[CloneCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/models/mcp_capability.go#L76)

<a id="schema-capabilityquery"></a>

### CapabilityQuery

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:204](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L204)
- 字段：`ServerID common.MCPServerID`；`Kind string`；`Category string`；`Stage string`；`RiskLevel string`；`Enabled *bool`；`Query string`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)

<a id="schema-createcapabilitycommand"></a>

### CreateCapabilityCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:179](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L179)
- 字段：`CapabilityID common.MCPCapabilityID`；`ServerID common.MCPServerID`；`Kind string`；`Name string`；`DisplayName string`；`Description string`；`Category string`；`Tags []string`；`UsageHint string`；`InputSummary string`；`OutputSummary string`；`StageHints []string`；`RiskLevel string`；`EffectScope string`；`IdempotencyClass string`；`ValidationProofApplicability string`；`Schema workorcheventpayload.Payload`；`URI string`；`Enabled bool`；`ApprovalPolicy string`
- 字段引用类型：[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)；[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-createcapabilityresult"></a>

### CreateCapabilityResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:249](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L249)
- 字段：`Capability *mcpmodel.Capability`；`RegistryRevision string`
- 字段引用类型：[Capability](#schema-capability)

<a id="schema-createservercommand"></a>

### CreateServerCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:97](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L97)
- 字段：`ServerID common.MCPServerID`；`Name string`；`Transport string`；`Endpoint string`；`Command string`；`Args []string`；`EnvironmentRefs []EnvironmentReference`；`Enabled bool`；`AllowedTools []string`；`AllowedResources []string`；`AllowedPrompts []string`；`DefaultApprovalPolicy string`；`SessionPolicy string`；`TimeoutMillis int`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[EnvironmentReference](#schema-environmentreference)

<a id="schema-createserverresult"></a>

### CreateServerResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:232](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L232)
- 字段：`Server *MCPServer`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)

<a id="schema-ensuredefaultscommand"></a>

### EnsureDefaultsCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:230](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L230)
- 声明：`type EnsureDefaultsCommand struct{}`

<a id="schema-ensuredefaultsresult"></a>

### EnsureDefaultsResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:276](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L276)
- 字段：`Ensured bool`；`RegistryRevision string`

<a id="schema-environmentreference"></a>

### EnvironmentReference

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L21)
- 字段：`Name string`；`Reference string`

<a id="schema-getcapabilitycommand"></a>

### GetCapabilityCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:219](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L219)
- 字段：`CapabilityID common.MCPCapabilityID`
- 字段引用类型：[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)

<a id="schema-getcapabilityresult"></a>

### GetCapabilityResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:253](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L253)
- 字段：`Capability *mcpmodel.Capability`；`RegistryRevision string`
- 字段引用类型：[Capability](#schema-capability)

<a id="schema-getservercommand"></a>

### GetServerCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:214](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L214)
- 字段：`ServerID common.MCPServerID`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)

<a id="schema-getserverresult"></a>

### GetServerResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:236](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L236)
- 字段：`Server *MCPServer`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)

<a id="schema-listcapabilitiescommand"></a>

### ListCapabilitiesCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:220](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L220)
- 字段：`Query CapabilityQuery`；`CursorAfterID common.MCPCapabilityID`；`Limit int`
- 字段引用类型：[CapabilityQuery](#schema-capabilityquery)；[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)

<a id="schema-listcapabilitiesresult"></a>

### ListCapabilitiesResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:257](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L257)
- 字段：`Capabilities []*mcpmodel.Capability`；`NextCursor common.MCPCapabilityID`；`RegistryRevision string`
- 字段引用类型：[Capability](#schema-capability)；[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)

<a id="schema-listserverscommand"></a>

### ListServersCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:215](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L215)
- 字段：`CursorAfterID common.MCPServerID`；`Limit int`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)

<a id="schema-listserversresult"></a>

### ListServersResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:240](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L240)
- 字段：`Servers []*MCPServer`；`NextCursor common.MCPServerID`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)；[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)

<a id="schema-mcpserver"></a>

### MCPServer

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:116](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L116)
- 字段：`ID common.MCPServerID`；`Name string`；`Transport string`；`Endpoint string`；`Command string`；`Args []string`；`EnvironmentRefs []EnvironmentReference`；`Enabled bool`；`AllowedTools []string`；`AllowedResources []string`；`AllowedPrompts []string`；`DefaultApprovalPolicy string`；`SessionPolicy string`；`TimeoutMillis int`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[EnvironmentReference](#schema-environmentreference)

<a id="schema-readregistrysnapshotcommand"></a>

### ReadRegistrySnapshotCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:225](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L225)
- 声明：`type ReadRegistrySnapshotCommand struct{}`

<a id="schema-readselectionsnapshotcommand"></a>

### ReadSelectionSnapshotCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:226](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L226)
- 字段：`ServerIDs []common.MCPServerID`；`CapabilityIDs []common.MCPCapabilityID`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[MCPCapabilityID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L27)

<a id="schema-registrysnapshotresult"></a>

### RegistrySnapshotResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:266](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L266)
- 字段：`Servers []*MCPServer`；`Capabilities []*mcpmodel.Capability`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)；[Capability](#schema-capability)

<a id="schema-selectionsnapshotresult"></a>

### SelectionSnapshotResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:271](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L271)
- 字段：`Servers []*MCPServer`；`Capabilities []*mcpmodel.Capability`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)；[Capability](#schema-capability)

<a id="schema-updatecapabilitycommand"></a>

### UpdateCapabilityCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:202](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L202)
- 实际alias/同形类型：[CreateCapabilityCommand](#schema-createcapabilitycommand)
- 字段引用类型：[CreateCapabilityCommand](#schema-createcapabilitycommand)

<a id="schema-updatecapabilityresult"></a>

### UpdateCapabilityResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:262](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L262)
- 字段：`Capability *mcpmodel.Capability`；`RegistryRevision string`
- 字段引用类型：[Capability](#schema-capability)

<a id="schema-updateservercommand"></a>

### UpdateServerCommand

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:114](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L114)
- 实际alias/同形类型：[CreateServerCommand](#schema-createservercommand)
- 字段引用类型：[CreateServerCommand](#schema-createservercommand)

<a id="schema-updateserverresult"></a>

### UpdateServerResult

- 源码：[internal/modules/blocks/mcpregistry/pkg/events/messages.go:245](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/events/messages.go#L245)
- 字段：`Server *MCPServer`；`RegistryRevision string`
- 字段引用类型：[MCPServer](#schema-mcpserver)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L127)：`!mcpRegistryCommandSourceAllowed(ev.ID(), ev.Source())`
- [mcpRegistryCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L242)：来源集合由switch/返回表达式判定，见完整函数
  - helper完整分支：`func mcpRegistryCommandSourceAllowed(topic, source string) bool { switch topic { case mcpevents.TopicMCPCommandCreateServer, mcpevents.TopicMCPCommandUpdateServer, mcpevents.TopicMCPCommandCreateCapability, mcpevents.TopicMCPCommandUpdateCapability: return source == applicationContextRegistrySource case mcpevents.TopicMCPCommandListServers: return source == applicationContextRegistrySource case mcpevents.TopicMCPCommandGetServer: return source == applicationContextRegistrySource || source == applicationOwnerReadSource || source == mcpExecutionSource case mcpevents.TopicMCPCommandListCapabilities: return source == applicationContextRegistrySource || source == applicationReadModelSource case mcpevents.TopicMCPCommandGetCapability: return source == applicationContextRegistrySource || source == applicationOwnerReadSource case mcpevents.TopicMCPCommandReadRegistrySnapshot: return source == mcpExecutionSource case mcpevents.TopicMCPCommandReadSelectionSnapshot: return source == applicationOwnerReadSource case mcpevents.TopicMCPCommandEnsureDefaults: return source == systemCommandSource default: return false } }`
  - 当前来源常量：`applicationContextRegistrySource = "application/workorchd/context-registry"`；`applicationOwnerReadSource = "application/workorchd/appservice"`；`mcpExecutionSource = "blocks/mcp"`；`applicationReadModelSource = "application/workorchd/readmodel"`；`systemCommandSource = "kernel/system"`