#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

: "${GOCACHE:=/tmp/workorch-go-build}"
export GOCACHE

go test \
  ./internal/modules/blocks/web/... \
  ./internal/pkg/workorchconfig \
  ./internal/modules/application/workorchd/biz/appservice \
  ./internal/modules/application/workorchd/biz/bootstrap \
  ./internal/modules/kernel/orchestrator/... \
  ./internal/modules/blocks/llm/biz \
  -count=1
