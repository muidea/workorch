# Workspace Soul MD Default Resource

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 workspace 相关的 Soul Markdown 默认资源约定。

## Stable Rules

### Positioning

- 本能力只调整内置默认 Soul 资源的组织方式
- 不改变 Soul 运行时模型、HTTP API、持久化模型、workspaceTemplate 绑定关系或 orchestrator 阶段语义
- `soulMd` 仍然是对外公开字段；拆分只发生在内置默认资源组织层

### Resource Layout

推荐目录结构：

```text
internal/modules/application/workorchd/biz/bootstrap/defaults/
  souls.json
  souls/
    <template>/
      <stage>.md
```

含义：

- `souls.json` 只保留结构化元数据和正文引用
- 每个内置 Soul 的自然语言正文单独放在 `.md` 文件中
- 文件路径按 `template/stage` 分组，避免所有默认 Soul 正文堆叠在同一级目录

### JSON Contract

默认 `souls.json` 中每个内置 Soul 应使用：

- `id`
- `name`
- `description`
- 结构化允许字段
- `soulMDRef`

约束：

- `soulMDRef` 必填
- 内置默认 `souls.json` 不再允许内联 `soulMD`
- `soulMDRef` 必须是相对路径
- `soulMDRef` 必须位于 `souls/` 目录下
- `soulMDRef` 必须使用 `souls/<template>/<stage>.md` 两级格式
- `soulMDRef` 不得包含 `..`、绝对路径或空路径段

### Loading Contract

加载默认资源时应遵循：

1. 同时 embed `souls.json` 与 `souls/*/*.md`
2. 先解析结构化元数据
3. 按 `soulMDRef` 读取 Markdown 正文
4. trim 后写回运行时 `SoulMD` 字段
5. 后续 `EnsureBuiltinDefaults` 继续按现有 `Soul` 模型 upsert

### Failure Rules

以下情况必须阻止默认资源加载：

- `soulMDRef` 为空
- `soulMDRef` 越界到 `souls/` 之外
- `soulMDRef` 不是 `.md`
- 正文文件不存在
- 正文 trim 后为空
- `souls.json` 仍包含内联 `soulMD`

## Verification Focus

- 所有内置 Soul 都能加载出非空 `SoulMD`
- `soulMDRef` 与 `id` 的 template/stage 结构一一对应
- `souls.json` 中不再出现 `soulMD`
- 拆分后关键 Soul 行为约束不丢失

## Bootstrap / Module Mapping

该能力的稳定落点应限定在 bootstrap 默认资源层：

- `application/workorchd` bootstrap defaults
- embed 资源装载
- builtin Soul upsert / ensure 逻辑

更细实现映射方向：

- `bootstrap/defaults`
  - 持有 `souls.json` 与 `souls/*/*.md` 的 embed 资源
- defaults loader
  - 解析结构化 metadata
  - 读取 `soulMDRef`
  - 校验路径与正文完整性
  - 组装运行时 `SoulMD`
- `EnsureBuiltinDefaults`
  - 把默认 Soul 继续 upsert 到正式 Soul 模型

这些实现都属于应用层 bootstrap，不应下沉到 runtime core / kernel state / product schema。

它不应外溢到：

- runtime stage contract
- HTTP response schema 变更
- workspace template 持久化模型变更

## API / Product Mapping

虽然默认资源组织只发生在 bootstrap 层，但对外产品面仍应保持一致：

- `GET /souls`
- `GET /souls/:soulId`
- `workspace template` / `soul-bindings` 管理界面

对外保持不变的含义：

- API 继续暴露 `soulMd`
- Web/TUI 不需要理解 `soulMDRef`
- `soulMDRef` 只属于默认资源装载实现细节

## Verification Mapping

当前与默认 Soul 资源拆分直接相关的验证入口：

- `make verify-workspace-template-soul`
- `go test` 覆盖 bootstrap/defaults 加载逻辑

更细 fixture 映射方向：

| 验证入口 | 当前主要证明 |
| --- | --- |
| `make verify-workspace-template-soul` | 默认 Soul 资源可被模板初始化、绑定与对外 API 正常消费 |
| `defaults_test.go` | `souls.json` / `soulMDRef` / Markdown 正文装载与路径校验 |
| `GET /souls` / `GET /souls/:soulId` | 对外仍暴露完整 `soulMd`，而不泄漏 `soulMDRef` 细节 |

这些验证共同回答：

- 默认资源是否能被稳定加载
- API 暴露的 `soulMd` 是否仍然完整
- 模板和绑定流程是否未因资源拆分而退化

## Cross-Layer Mapping

- workspace routing：
  `soul-routing.md`
- application/bootstrap：
  `../../30-module-architecture/application-workorchd.md`
- 服务验收：
  `../../60-verification/service-acceptance.md`

## Source Documents

- `soul-routing.md`
