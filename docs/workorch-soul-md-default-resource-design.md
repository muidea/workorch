# WorkOrch Soul MD 默认资源拆分设计

## 背景

当前内置 Soul 定义集中维护在 `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json` 中。该文件同时包含结构化元数据与大段 `soulMD` 正文，导致后续维护、审查、差异比较和局部修改成本较高。

本设计只调整 workorchd 内置默认资源的组织方式，不改变 Soul 运行时模型、HTTP API、持久化模型、workspaceTemplate 绑定关系或 orchestrator 阶段语义。

## 目标

- `souls.json` 只保留结构化元数据和正文引用。
- 每个内置 Soul 的自然语言正文单独放入一个 `.md` 文件。
- bootstrap 加载时把 `.md` 内容解析回现有 `SoulMD` 字段，保持下游使用方式不变。
- 文件命名、路径和加载校验保持简单、可审计、可测试。

## 非目标

- 不调整 Soul API 请求/响应字段，外部仍使用 `soulMd`。
- 不调整数据库或仓储结构。
- 不把软件开发 Soul 的行为约束下沉到核心 runtime。
- 不引入模板引擎、include 指令、变量替换或多文件拼接。
- 当前阶段不考虑旧格式兼容；内置 `souls.json` 必须使用 `soulMDRef`。

## 目录结构

```text
internal/modules/application/workorchd/biz/bootstrap/defaults/
  souls.json
  souls/
    generic-governed/
      planning.md
      execution.md
      decision.md
      approval.md
      validation.md
      artifact.md
    software-dev/
      planning.md
      execution.md
      ...
```

## JSON 格式

内置 `souls.json` 的单项结构调整为：

```json
{
  "id": "soul-software-dev-execution",
  "name": "软件开发-执行",
  "description": "编码实现、运行构建和测试。允许 shell 命令、文件写入和补丁。",
  "businessDomains": ["软件开发"],
  "allowedSkillRefs": ["*"],
  "allowedMCPServerRefs": ["*"],
  "allowedToolKinds": ["file/list", "file/read", "shell/command", "patch/apply"],
  "soulMDRef": "souls/software-dev/execution.md"
}
```

约束：

- `soulMDRef` 必填。
- `soulMD` 不再允许出现在内置默认 `souls.json` 中。
- `soulMDRef` 必须是相对路径，且必须位于 `souls/` 目录下。
- `soulMDRef` 必须按 template 分组，格式为 `souls/<template>/<stage>.md`。
- `soulMDRef` 必须以 `.md` 结尾。
- `soulMDRef` 不能包含 `..`、绝对路径或空路径段。
- `<template>` 与 `<stage>` 应从 `id=soul-<template>-<stage>` 机械推导，避免所有 Markdown 文件堆在同一级目录。

## 加载流程

1. `go:embed` 同时嵌入 `defaults/souls.json` 和 `defaults/souls/*/*.md`。
2. `loadEmbeddedSouls` 读取 `souls.json` 并解析结构化元数据。
3. 对每个 Soul：
   - 校验 `id` 非空。
   - 校验 `soulMDRef` 合法。
   - 从 `defaults/<soulMDRef>` 读取 Markdown 正文。
   - 正文 trim 后写入 `embeddedSoul.SoulMD`。
4. 后续 `EnsureBuiltinDefaults` 继续按现有路径 upsert `soulmodel.Soul`。

## 错误处理

以下情况必须返回错误并阻止默认资源加载：

- `soulMDRef` 为空。
- `soulMDRef` 指向 `souls/` 之外，或未使用 `souls/<template>/<stage>.md` 两级分类。
- `soulMDRef` 使用绝对路径或包含 `..`。
- Markdown 文件不存在或内容 trim 后为空。
- `souls.json` 仍包含内联 `soulMD`。

## 测试要求

- 所有内置 Soul 均能通过 `loadEmbeddedSouls` 加载出非空 `SoulMD`。
- `souls.json` 中每个 `soulMDRef` 与 `id` 一一对应，路径为 `souls/<template>/<stage>.md`。
- `souls.json` 中不再出现 `soulMD` 字段。
- 现有软件开发 Soul 内容断言继续通过，证明拆分后没有丢失关键行为约束。
- 新增非法 `soulMDRef` 单元测试，覆盖空值、越界路径、非 md 文件等。

## 收口清单

- [x] 新增本文档。
- [x] 调整 `embeddedSoul` 结构，增加 `SoulMDRef`。
- [x] 调整 `go:embed` 范围，嵌入 `defaults/souls/*/*.md`。
- [x] 调整 `loadEmbeddedSouls`，按 `soulMDRef` 读取 Markdown。
- [x] 按 template/stage 拆分 `souls.json` 中 36 个内置 Soul 的 `soulMD` 到独立 Markdown 文件。
- [x] 删除 `souls.json` 中的内联 `soulMD`。
- [x] 补充 bootstrap 测试。
- [x] 执行格式、单元测试和 diff 检查。
