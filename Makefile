SHELL := /usr/bin/env bash

GO ?= go
NPM ?= npm
APP := ./application/workorchd
PKGS := ./...
GOFMT ?= gofmt
GOTEST_FLAGS ?=
BUILD_OUT ?= ./bin/workorchd
CONSOLE_WEB := ./web/workorch-console

.PHONY: help fmt fmt-check test test-race vet console-build build clean run run-file verify-service verify-service-file verify-llm verify-llm-file docker-build docker-run docker-run-file docker-verify docker-verify-file ci

help:
	@echo "Targets:"
	@echo "  fmt        - gofmt -w + go fmt for project sources"
	@echo "  fmt-check  - fail if any Go file needs gofmt"
	@echo "  test       - go test $(PKGS)"
	@echo "  test-race  - go test -race $(PKGS)"
	@echo "  vet        - go vet $(PKGS)"
	@echo "  console-build - build the embedded workorch management console assets"
	@echo "  build      - go build -o $(BUILD_OUT) $(APP)"
	@echo "  clean      - remove build/demo artifacts"
	@echo "  run        - go run $(APP) as HTTP service"
	@echo "  run-file   - clean data, then run $(APP) as HTTP service with file-backed repository"
	@echo "  verify-service      - build, start, and verify the HTTP service with memory repository"
	@echo "  verify-service-file - build, start, and verify the HTTP service with file-backed repository and restart-read validation"
	@echo "  verify-llm      - build, start, and verify LLM-driven workspace execution with a local mock provider"
	@echo "  verify-llm-file - verify LLM-driven workspace execution with file-backed repository and restart-read validation"
	@echo "  docker-build    - docker build local workorchd image"
	@echo "  docker-run      - run workorchd container with in-memory repository"
	@echo "  docker-run-file - run workorchd container with file-backed repository"
	@echo "  docker-verify      - build image and verify the container with memory repository"
	@echo "  docker-verify-file - build image and verify the container with file-backed repository and restart-read validation"
	@echo "  ci         - fmt-check + vet + test + build + file-backed service verification"

fmt:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -n "$$files" ]]; then \
		$(GOFMT) -w $$files; \
	fi
	$(GO) fmt $(PKGS)

fmt-check:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -z "$$files" ]]; then \
		exit 0; \
	fi; \
	fmt_out="$$($(GOFMT) -l $$files)"; \
	if [[ -n "$$fmt_out" ]]; then \
		echo "Files need gofmt:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi

test:
	$(GO) test $(GOTEST_FLAGS) $(PKGS)

test-race:
	$(GO) test -race $(GOTEST_FLAGS) $(PKGS)

vet:
	$(GO) vet $(PKGS)

console-build:
	@cd $(CONSOLE_WEB) && \
	if [[ ! -d node_modules ]]; then $(NPM) install; fi && \
	$(NPM) run build

build: console-build
	@mkdir -p $(dir $(BUILD_OUT))
	$(GO) build -o $(BUILD_OUT) $(APP)

clean:
	rm -rf ./bin ./.workorch-data ./workorchd

run:
	$(GO) run $(APP) -config-dir config

run-file:
	rm -rf ./.workorch-data
	$(GO) run $(APP) -config-dir config/file

verify-service:
	WORKORCH_VERIFY_ADDR=127.0.0.1:18080 ./scripts/verify-workorchd-http.sh

verify-service-file:
	WORKORCH_VERIFY_ADDR=127.0.0.1:18082 WORKORCH_VERIFY_BACKEND=file ./scripts/verify-workorchd-http.sh

verify-llm:
	WORKORCH_LLM_VERIFY_ADDR=127.0.0.1:18180 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh

verify-llm-file:
	WORKORCH_LLM_VERIFY_ADDR=127.0.0.1:18182 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18183 WORKORCH_LLM_VERIFY_BACKEND=file ./scripts/verify-llm-workspace-execution.sh

docker-build:
	docker build -t workorchd:local .

docker-run:
	docker run --rm -p 8080:8080 workorchd:local

docker-run-file:
	mkdir -p ./.workorch-data
	chmod 0777 ./.workorch-data
	docker run --rm -p 8080:8080 \
		-v $(CURDIR)/config/container-file/application.toml:/etc/workorch/application.toml:ro \
		-v $(CURDIR)/.workorch-data:/var/lib/workorch \
		workorchd:local

docker-verify:
	WORKORCH_VERIFY_PORT=18081 ./scripts/verify-workorchd-container.sh

docker-verify-file:
	WORKORCH_VERIFY_PORT=18083 WORKORCH_VERIFY_BACKEND=file ./scripts/verify-workorchd-container.sh

ci: fmt-check vet test build verify-service-file
