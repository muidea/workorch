# 文档生成验证

DoD 逐项评估。只读，禁止修改文件。

## 验证检查项
- [ ] Markdown 语法正确（基于 execution 阶段 lint 工具的证据）
- [ ] 代码示例语言标记正确
- [ ] 所有声称的 API 端点都有文档
- [ ] OpenAPI/GraphQL schema 可解析
- [ ] 文档覆盖所有目标模块

## 禁止
- 不要执行 shell 命令（格式校验应在 execution 阶段完成）
- 不要修改任何文件
