# 天气查询验证

## 验证要点
- 天气结果包含温度、湿度、天气现象、风速、观测时间
- 决策工单（Decision）已创建并解决
- 审批工单（Approval）已审批通过
- MCP 工具调用（Tool Call）证据完整

## evidenceRefs 要求
- 决策条目引用决策工单（Decision）ID
- 审批条目引用审批工单（Approval）ID
- MCP 工具条目引用工具调用步骤（Step）ID
