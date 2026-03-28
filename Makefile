SHELL := /usr/bin/env bash

GO ?= go
APP := ./application/workorchd
PKGS := ./...
GOFMT ?= gofmt
GOTEST_FLAGS ?=
BUILD_OUT ?= ./bin/workorchd

.PHONY: help fmt fmt-check test test-race vet build clean run run-file ci

help:
	@echo "Targets:"
	@echo "  fmt        - gofmt -w + go fmt for project sources"
	@echo "  fmt-check  - fail if any Go file needs gofmt"
	@echo "  test       - go test $(PKGS)"
	@echo "  test-race  - go test -race $(PKGS)"
	@echo "  vet        - go vet $(PKGS)"
	@echo "  build      - go build -o $(BUILD_OUT) $(APP)"
	@echo "  clean      - remove build/demo artifacts"
	@echo "  run        - go run $(APP)"
	@echo "  run-file   - clean demo data, then run $(APP) with file-backed repository"
	@echo "  ci         - fmt-check + vet + test + build + file-backed acceptance"

fmt:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*')"; \
	if [[ -n "$$files" ]]; then \
		$(GOFMT) -w $$files; \
	fi
	$(GO) fmt $(PKGS)

fmt-check:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*')"; \
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

build:
	@mkdir -p $(dir $(BUILD_OUT))
	$(GO) build -o $(BUILD_OUT) $(APP)

clean:
	rm -rf ./bin ./.workorch-data ./workorchd

run:
	$(GO) run $(APP)

run-file:
	rm -rf ./.workorch-data
	WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data $(GO) run $(APP)

ci: fmt-check vet test build run-file
