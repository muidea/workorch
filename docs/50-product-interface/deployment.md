# WorkOrch Deployment

Status: active

Type: product-interface

Last Updated: 2026-05-30

## Purpose

本文件承接 WorkOrch 的部署与运行入口说明。

## Scope

- container image
- local/file backend startup
- configuration file entry
- deployment prerequisites and startup mode selection

## Responsibility Boundary

本文件只回答：

- 镜像如何构建
- 服务如何启动
- 配置如何挂载
- memory/file backend 如何选择

本文件不回答：

- 当前服务行为是否已经被证明正确
- 哪些 API / read-model / governance / raw-content 路径已经通过验收
- 哪些 deployed script / fixture / container verification 证明了行为成立

这些验证问题统一转入：

- `../60-verification/service-acceptance.md`

## Deployment Vs Acceptance Matrix

| 主题 | `deployment.md` 负责 | `../60-verification/service-acceptance.md` 负责 |
| --- | --- | --- |
| container image | 如何构建、如何运行 | 镜像启动后哪些行为必须通过验证 |
| local startup | `go run` / config-dir 入口 | 启动后 `healthz` / `readyz` / `metrics` / 最小业务链如何验收 |
| file backend | 如何挂载 `application.toml` 与 data dir | file backend 重启、replay、raw/detail 打开如何验收 |
| configuration surface | 配置分区和主要字段入口 | 配置加载后哪些行为与 API 读取已被证明 |
| deployed instance | 如何连接现有配置和进程 | 哪些 `make verify-*` / script 证明现有实例行为成立 |

## Deployment Entry

### Container Image

构建镜像：

```bash
docker build -t workorch:local .
```

或使用：

```bash
make docker-build
```

### In-Memory Startup

```bash
docker run --rm -p 8080:8080 workorch:local
```

### File-Backed Startup

```bash
mkdir -p .workorch-runtime
chmod 0777 .workorch-runtime
docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/config/container-file/application.toml:/etc/workorch/application.toml:ro" \
  -v "$(pwd)/.workorch-runtime:/var/lib/workorch" \
  workorch:local
```

file backend 下：

- internal runtime state 位于 `/var/lib/workorch/.workorch-data/internal/workspaces/<workspace-id>/`
- 默认 deliverable root 位于 `/var/lib/workorch/.workorch-workspaces/<workspace-id>/`

## Local Server Startup

本地默认配置：

```bash
go run ./application/workorch --server -config-dir config
```

本地 file backend：

```bash
go run ./application/workorch --server -config-dir config/file
```

`CONFIG_PATH` 也可指向配置目录，遵循 magicCommon configuration 约定。

## Configuration Surface

主要配置分区：

- `[server]`：监听地址、timeout、`maxConcurrentRuns`
- `[repository]`：backend 与 `dataDir`
- `[runtime]`：repair / retry / shell timeout 等 runtime 默认值
- `[llm]`：default provider 与 provider 列表

## Startup Decision Guide

典型选择方向：

- 只验证最小本地启动
  - 用 `go run ./application/workorch --server -config-dir config`
- 需要验证 file backend 持久化布局
  - 用 `go run ./application/workorch --server -config-dir config/file`
- 需要验证容器镜像和挂载行为
  - 用 `docker run ...` 或 `make docker-build` 后的容器入口

一旦进入“是否行为正确、是否通过验收”的问题，应切换到
`../60-verification/service-acceptance.md`。

## Source Documents

- `../deployment.md`
