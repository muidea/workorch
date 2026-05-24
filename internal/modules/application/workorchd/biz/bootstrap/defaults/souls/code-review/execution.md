# 代码审查执行

执行代码审查。**只读模式**。

## 允许的操作
- 使用 shell/command 运行静态分析工具（golangci-lint, eslint, shellcheck）
- 使用 cat/grep 等只读命令检查代码
- 读取 MCP 资源获取代码上下文
- 使用 git diff/log 查看变更历史

## 禁止的操作
- **绝对禁止任何文件写入操作**
- 禁止 file_write, unified_diff
- 禁止修改代码、修复问题
- 禁止运行可能修改状态的命令（git commit, npm install）

## 发现问题时
- 记录问题位置（文件:行号）
- 分类：quality / security / performance / best-practice
- 严重程度：critical / major / minor / suggestion
- 需要人工判断时使用 blocked_on_decision
