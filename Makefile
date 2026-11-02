SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

export GOTOOLCHAIN := go1.27.1
export CGO_ENABLED := 0

GO      ?= go
PKGS    ?= ./...
BUILD   ?= build
BIN     ?= $(BUILD)/omnic
CI      ?=

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev-dirty")
LDFLAGS ?= -X main.version=$(VERSION)

# Benchmark knobs (make bench BENCH=Percentile COUNT=10 BENCHTIME=2s NAME=old).
BENCH     ?= .
COUNT     ?= 6
BENCHTIME ?= 1s
NAME      ?= new
BENCHDIR  ?= $(BUILD)/bench
OLD       ?= $(BENCHDIR)/old.txt
NEW       ?= $(BENCHDIR)/new.txt

.PHONY: all help install-tools build build-debug build-handset build-phone build-stress test bench bench-compare profile qa fix fmt vet lint nilaway arch-go fieldalignment vuln loc infra-build infra-rebuild infra-up infra-debug infra-down clean

help: ## List targets
	@grep -hE '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/' | expand -t24

install-tools: ## Download deps and build the pinned go tools into the module cache
	$(GO) mod download
	$(GO) tool golangci-lint version
	$(GO) tool gofumpt --version
	$(GO) tool arch-go --version
	$(GO) tool govulncheck -version >/dev/null 2>&1 || true
	$(GO) tool nilaway -V=full >/dev/null 2>&1 || true
	$(GO) tool fieldalignment -V=full >/dev/null 2>&1 || true
	$(GO) tool benchstat -h >/dev/null 2>&1 || true
	$(GO) tool dlv version
	$(GO) tool sqlc version

all: build build-handset ## Build the emulation server and handset

build: ## Build the service binary
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN) ./cmd/omnic

build-debug: ## Build an unoptimized server binary and the pinned dlv for the VS Code "Debug Server" launch
	$(GO) build -gcflags='all=-N -l' -ldflags="$(LDFLAGS)" -o $(BUILD)/omnic-debug ./cmd/omnic
	$(GO) build -o $(BUILD)/dlv github.com/go-delve/delve/cmd/dlv

build-handset: ## Build the omnic-handset binary
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BUILD)/omnic-handset ./cmd/omnic-handset

build-phone: build-handset ## Alias for build-handset
build-stress: build-handset ## Alias for build-handset

test: ## Run tests with the race detector
	CGO_ENABLED=1 $(GO) test -race -count=1 $(PKGS)

bench: ## Run benchmarks, save to build/bench/$(NAME).txt (BENCH=regex COUNT=6 BENCHTIME=1s PKGS=./... NAME=new)
	@mkdir -p $(BENCHDIR)
	$(GO) test -run '^$$' -bench '$(BENCH)' -benchmem -benchtime $(BENCHTIME) -count $(COUNT) $(PKGS) | tee $(BENCHDIR)/$(NAME).txt

bench-compare: ## benchstat OLD vs NEW (defaults build/bench/old.txt vs new.txt; run `make bench NAME=old`, change code, `make bench`)
	$(GO) tool benchstat $(OLD) $(NEW)

profile: ## Build, load-test and capture pprof concurrency profiles into build/profile (MODE=local|docker)
	scripts/profile.sh $(or $(MODE),local)

qa: fix fmt vet lint nilaway arch-go loc fieldalignment ## Full static-analysis gate: fix -> fmt -> vet -> lint -> nilaway -> arch-go -> loc -> fieldalignment

fix: ## go fix modernizers (bounded by the go.mod version): rewrite locally, fail in CI
ifeq ($(CI),)
	$(GO) fix $(PKGS)
else
	$(GO) fix -diff $(PKGS)
endif

fmt: ## gofumpt: rewrite locally, fail in CI
ifeq ($(CI),)
	$(GO) tool gofumpt -w .
else
	@out=$$($(GO) tool gofumpt -l .); if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi
endif

vet: ## go vet
	$(GO) vet $(PKGS)

lint: ## golangci-lint
	$(GO) tool golangci-lint run $(PKGS)

nilaway: ## NilAway nil-panic analysis
	$(GO) tool nilaway $(PKGS)

arch-go: ## architecture rules from arch-go.yml
	$(GO) tool arch-go

fieldalignment: ## struct field ordering
	$(GO) tool fieldalignment $(PKGS)

loc: ## Fail on any Go file over 1500 lines of code, comments and blanks excluded (MAXLOC= override)
	scripts/loc.sh -m $(or $(MAXLOC),1500)

vuln: ## govulncheck against the Go vuln DB (needs network; not part of qa)
	$(GO) tool govulncheck $(PKGS)

infra-build: ## Build the image (uses layer cache)
	docker compose build

infra-rebuild: ## Rebuild the image from scratch (no layer cache, fresh base image)
	docker compose build --no-cache --pull

infra-up: ## Start the local stack (server, Prometheus :9090, Grafana :3000 no-login) and wait for health
	docker compose up -d --wait

infra-debug: ## Start the local stack with dlv headless on 127.0.0.1:2345 (see docker-compose.debug.yml)
	docker compose -f docker-compose.yml -f docker-compose.debug.yml up -d --build --wait

infra-down: ## Stop the local stack, keep volumes
	docker compose down

clean: ## Remove build output
	rm -rf $(BUILD)
