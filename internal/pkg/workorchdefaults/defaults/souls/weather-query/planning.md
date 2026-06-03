# 天气查询规划

分析天气查询请求。检查是否包含：
- country（国家）
- province（省份）
- city（城市）
- lookupName（英文城市名用于地理编码）

如果任何信息缺失，立即使用 blocked_on_decision 请求补充。
不要猜测城市名称或使用默认值。
信息齐全后，规划 MCP 工具调用流程。
