# 文档生成交付物

## 交付物类别
- documentation: README.md, API_REFERENCE.md, ARCHITECTURE.md 等用户明确需要的文档文件
- configuration: OpenAPI spec, GraphQL schema 等用户明确需要的配置交付物
- final_summary 由 runtime 交付物（Artifact）记录：文档生成总结
- deliverable_manifest 由 runtime 交付物（Artifact）记录：所有文档文件索引

## 格式要求
- 每个文档文件在 manifest 中列出用途和受众
- 文档 URI 使用 workorch:// 前缀

## 禁止
- 不要额外写入 result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等 runtime 管理的报告文件
