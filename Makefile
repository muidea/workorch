SHELL := /usr/bin/env bash

GO ?= go
APP := ./application/workorchd
PKGS := ./...

.PHONY: help fmt test vet build run ci

help:
	@echo "Targets:"
	@echo "  fmt   - go fmt $(PKGS)"
	@echo "  test  - go test $(PKGS)"
	@echo "  vet   - go vet $(PKGS)"
	@echo "  build - go build $(APP)"
	@echo "  run   - go run $(APP)"
	@echo "  ci    - fmt check + vet + test + build"

fmt:
	$(GO) fmt $(PKGS)

test:
	$(GO) test $(PKGS)

vet:
	$(GO) vet $(PKGS)

build:
	$(GO) build $(APP)

run:
	$(GO) run $(APP)

ci:
	@fmt_out="$$(gofmt -l .)"; \
	if [[ -n "$$fmt_out" ]]; then \
		echo "Files need gofmt:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi
	$(GO) vet $(PKGS)
	$(GO) test $(PKGS)
	$(GO) build $(APP)
