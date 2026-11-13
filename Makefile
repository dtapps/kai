# 设置默认目标
.DEFAULT_GOAL := setup

.PHONY: help icons build-assets dev darwin-build darwin-package darwin-dmg windows-build windows-package linux-build linux-package tidy bindings sqlc swift-build lint-go lint-fe fmt-fe format-swift check-cross

# 环境变量优先级高于 .env：include 的赋值会覆盖环境值，先暂存一份
ifeq ($(origin GIT_REMOTE),environment)
    GIT_REMOTE_ENV := $(GIT_REMOTE)
endif

# 加载 .env 文件（如果存在）
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

ifneq ($(GIT_REMOTE_ENV),)
    GIT_REMOTE := $(GIT_REMOTE_ENV)
endif

# 同步/推送目标远程，逗号分隔：GIT_REMOTE=origin,github,gitlab
# 可通过 .env / 环境变量 / 命令行覆盖，未配置时默认 origin。
GIT_REMOTE ?= origin
export GIT_REMOTE

# 前端项目目录
FRONTEND_DIR = frontend

# 编译输出目录
BUILD_DIR=bin

# ==================== 构建配置 ====================

# 开发服务器端口
PORT ?= 9247

# 根目录有 .env 就载入其中的构建变量（KEY=val 形式，# 注释与空行自动忽略）
# 载入后下方 VERSION/... 用 ?= 定义，.env 同名值优先、缺失则回退默认值；命令行 make KEY=val 可再覆盖。
ifneq (,$(wildcard $(CURDIR)/.env))
include $(CURDIR)/.env
export
endif

# 版本号
VERSION     ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
# 构建时间(UTC)
BUILD_TIME  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# 提交短哈希
GIT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
# GitHub API Token
GITHUB_TOKEN ?=
# CNB API Token
CNB_TOKEN   ?=
# PostHog Project ID
POSTHOG_PROJECT_ID   ?=
# PostHog Project token
POSTHOG_TOKEN   ?=
# 开发标记，true 为开发态，false 为正式构建
DEV         ?= false

# 合并 stderr 到 stdout
FILTER := 2>&1

# 默认目标
help: ## 显示帮助信息
	@echo "Kai 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' Makefile | sort | \
		sed -E 's/^([a-zA-Z0-9_-]+):.*## (.*)$$/\1|\2/' | \
		awk -F'|' '{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	pnpm --dir ./frontend install

# -clean=true 生成前清理输出目录
# -ts 生成 TypeScript
# -i 使用 TS 接口
# -names 使用名称而非 ID
bindings: ## 生成 Wails 绑定
	wails3 generate bindings -clean=true -ts -i

icons: ## 生成图标资源
	wails3 task common:generate:icons

build-assets: ## 同步版本号/应用名到构建资源
	wails3 task common:update:build-assets

wails3-generate: bindings icons build-assets ## 生成 Wails 绑定/图标/webview/构建资源

sqlc: ## 生成 sqlc 代码（internal/historystore internal/configstore internal/httplogstore）
	cd internal/historystore && sqlc generate
	cd internal/configstore && sqlc generate
	cd internal/httplogstore && sqlc generate

i18n: i18n-go i18n-frontend ## 合并所有 i18n 拆分文件到主文件

# 检测操作系统（Windows 用 cmd 脚本，*nix 用 sh 脚本）
ifeq ($(OS),Windows_NT)
  I18N_GO_CMD   = scripts\\merge-go-i18n.cmd
  I18N_FE_CMD   = scripts\\merge-frontend-i18n.cmd
else
  I18N_GO_CMD   = ./scripts/merge-go-i18n.sh
  I18N_FE_CMD   = ./scripts/merge-frontend-i18n.sh
endif

i18n-go: ## 合并 Go 后端 i18n 拆分文件
	$(I18N_GO_CMD)

i18n-frontend: ## 合并前端 i18n 拆分文件
	$(I18N_FE_CMD)

swift-build: ## 手动编译 Swift 桥接动态库（libkai_bridge.dylib，含翻译与辅助功能）
	cd pkg/swiftbridge/scripts && bash ./build.sh

code-generate: sqlc i18n swift-build ##  代码生成 sqlc i18n swift

dev: ## 运行 Wails 开发模式
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 dev -port $(PORT)

# ==================== 构建打包 ====================

darwin-build: ## [macOS] 编译正式二进制 -> bin/Kai（不打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task darwin:build

darwin-package: ## [macOS] 正式打包 -> bin/Kai.app（打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task darwin:package

darwin-package-dmg: ## [macOS] 打包并生成 bin/Kai.dmg 安装包
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task darwin:package:dmg

windows-build: ## [Windows] 编译正式二进制 -> bin/Kai.exe（不打包）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task windows:build

windows-package: ## [Windows] 正式打包 -> bin/Kai.exe + 安装包（nsis/msix）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task windows:package

linux-build: ## [Linux] 编译正式二进制（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task linux:build

linux-package: ## [Linux] 正式打包 -> bin/Kai（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) POSTHOG_PROJECT_ID=$(POSTHOG_PROJECT_ID) POSTHOG_TOKEN=$(POSTHOG_TOKEN) DEV=$(DEV) wails3 task linux:package

build-frontend: ## 构建前端
	pnpm --dir $(FRONTEND_DIR) build

# ==================== 其他 ====================

clean: ## 清理构建产物
	rm -rf frontend/dist frontend/bindings
	rm -f kai bin/*

tool-deps: ## 安装必要的工具
	@echo "==> 安装必要的工具..."

	wails3 version || true
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	-wails3 version
	@echo "==> wails3 工具安装或更新完成"

	sqlc version || true
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	-sqlc version
	@echo "==> sqlc 工具安装或更新完成"

	golangci-lint version || true
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	-golangci-lint version
	@echo "==> golangci-lint 工具安装或更新完成"

	govulncheck version || true
	go install golang.org/x/vuln/cmd/govulncheck@latest
	-govulncheck version
	@echo "==> govulncheck 工具安装或更新完成"

	gojq --version || true
	go install github.com/itchyny/gojq/cmd/gojq@latest
	-gojq --version
	@echo "==> gojq 工具安装或更新完成"

	yamlfmt -version || true
	go install github.com/google/yamlfmt/cmd/yamlfmt@latest
	-yamlfmt -version
	@echo "==> yamlfmt 工具安装或更新完成"

	shfmt -version || true
	go install mvdan.cc/sh/v3/cmd/shfmt@latest
	-shfmt -version
	@echo "==> shfmt 工具安装或更新完成"

	dockerfmt version || true
	go install github.com/reteps/dockerfmt@latest
	-dockerfmt version
	@echo "==> dockerfmt 工具安装或更新完成"

	markdownfmt -version || true
	go install github.com/Kunde21/markdownfmt/v3/cmd/markdownfmt@latest
	-markdownfmt -version
	@echo "==> markdownfmt 工具安装或更新完成"

setup: deps ## 一键设置项目（默认执行）
	@echo "🎉 Project setup completed!"

.PHONY: deps
deps: deps-go deps-node ## 安装全部依赖
	@echo "[Deps] 完成。"

.PHONY: deps-go
deps-go: ## 安装全部Go依赖
	@echo "[Deps] 安装 Go 模块依赖..."
	go mod tidy
	go mod download

# --frozen-lockfile (冻结锁文件)
# --prefer-offline (优先使用离线缓存)
.PHONY: deps-node
deps-node: ## 安装全部前端依赖
	@echo "[Deps] 安装前端依赖..."
	pnpm --dir $(FRONTEND_DIR) install --frozen-lockfile --prefer-offline

# --latest (无视版本限制，强制升最新)
.PHONY: update-deps
update-deps: update-deps-go update-deps-node ## 更新全部依赖
	@echo "[Deps] 完成。"

.PHONY: update-deps-go
update-deps-go: ## 更新全部Go依赖
	@echo "[Deps] 更新 Go 模块依赖..."
	go get -u ./...
	go mod tidy

# --latest (无视版本范围，强制升最新，并改写 package.json 的版本范围)
.PHONY: update-deps-node
update-deps-node: ## 更新全部前端依赖
	@echo "[Deps] 更新前端依赖..."
	pnpm --dir $(FRONTEND_DIR) update --latest
	pnpm --dir $(FRONTEND_DIR) prune

.PHONY: update-deps-pnpm
update-deps-pnpm: ## 更新 pnpm 自身的版本锁定（package.json 的 packageManager 字段）
	@echo "[Deps] 更新 pnpm 版本锁定..."
	@latest="$$(npm view pnpm version)"; \
	echo "[Deps] packageManager → pnpm@$${latest}"; \
	pnpm --dir $(FRONTEND_DIR) pkg set packageManager=pnpm@$${latest}

.PHONY: deps-clean
deps-clean: ## 删除未使用的依赖：knip --fix 把它们从 package.json 摘掉，
	@echo "[Deps] 删除未使用的依赖..."
	pnpm --dir $(FRONTEND_DIR) run deps:clean
	pnpm --dir $(FRONTEND_DIR) install --lockfile-only
	pnpm --dir $(FRONTEND_DIR) prune

# ==================== 检查 / 测试 ====================

check-cross: ## 交叉编译验证（darwin/arm64 开 CGO + windows CGO=0；过滤 ld:warning）
	@echo "==> 交叉编译 darwin/arm64 (CGO)"
	GOOS=darwin GOARCH=arm64 go build ./... 2>&1 | grep -v 'ld: warning' || true
# 	@echo "==> 交叉编译 darwin/amd64 (CGO)"
# 	GOOS=darwin GOARCH=amd64 go build ./... 2>&1 | grep -v 'ld: warning' || true
# 	@echo "==> 交叉编译 windows/arm64 (CGO_ENABLED=0)"
# 	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ./... 2>&1 | grep -v 'ld: warning' || true
	@echo "==> 交叉编译 windows/amd64 (CGO_ENABLED=0)"
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./... 2>&1 | grep -v 'ld: warning' || true
	@echo "==> 交叉编译验证完成"

#####################
## 格式化 相关
#####################

UNAME_S := $(shell uname -s)

# 用 Apple 官方 swift-format 原地格式化 Swift 桥接层源码。
# 未安装时给出明确安装提示（brew install swift-format 或走 swift 工具链）。
format-swift: ## 格式化 Swift 桥接层源码（原地，需 swift-format，仅限 macOS）
ifeq ($(UNAME_S),Darwin)
	@command -v swift-format >/dev/null 2>&1 || { \
		echo "!! swift-format 未安装，请先安装："; \
		echo "   brew install swift-format"; \
		echo "   或确保 swift 工具链自带 swift-format 在 PATH 中"; \
		exit 1; }
	swift-format --in-place ./pkg/swiftbridge/internal/swift/*.swift
	@echo ">> swift format done"
else
	@echo ">> Skipping swift-format (non-macOS system)"
endif

.PHONY: format-go ## 格式化后端文件
format-go:
	@echo "[Format] 格式化后端文件 (GO)..."
	gofmt -w -s .
	go fmt ./...
	go fix ./...

.PHONY: format-node ## 格式前端文件
format-node:
	@echo "[Format] 格式化前端文件 (PNPM)..."
	pnpm --dir $(FRONTEND_DIR) format

.PHONY: format-yaml
format-yaml:
	@bash scripts/format.sh yaml ## 格式化 YAML 文件

.PHONY: format-json
format-json:
	@bash scripts/format.sh json ## 格式化 JSON 文件

.PHONY: format-sql
format-sql:
	@bash scripts/format.sh sql ## 格式化 SQL 文件

.PHONY: format-markdown
format-markdown:
	@bash scripts/format.sh markdown ## 格式化 Markdown 文件

.PHONY: format-docker
format-docker: ## 格式化 Dockerfile 文件
	@bash scripts/format.sh docker

.PHONY: format-shell
format-shell: ## 格式化 Shell 脚本
	@bash scripts/format.sh shell

.PHONY: format
format: format-node format-go format-swift format-yaml format-json format-sql format-markdown format-docker format-shell ## 格式化

#####################
## 检查 / 测试 相关
#####################

# 统一定义构建命令
# CGO_ENABLED=0: 禁用 CGO，实现完全静态链接
# CGO_ENABLED=1: 启用 CGO，实现动态链接
LINT_ENV = CGO_ENABLED=0

# 定义 Lint 过程中需要激活的 Go Build Tags
LINT_TAGS = goolm,stdjson,purego

# Lint 扫描的目标平台，默认当前平台；CI 通过 LINT_PLATFORMS=... 覆盖为全平台
LINT_PLATFORMS ?= $(shell go env GOOS)/$(shell go env GOARCH)

# -printf=false 关闭 printf 分析，避免 i18n.T() 作日志 format 时触发错误
.PHONY: check-go
check-go: ## 检查后端代码
	@echo "[Check] 运行 go vet 静态检查..."
	go vet -printf=false ./...

	@echo "[Check] 运行 golangci-lint 扫描 (平台: $(LINT_PLATFORMS), Tags: $(LINT_TAGS))..."
	@for os_arch in $(LINT_PLATFORMS); do \
		os=$$(echo $$os_arch | cut -d'/' -f1); \
		arch=$$(echo $$os_arch | cut -d'/' -f2); \
		echo "  -> $$os 平台 ($$arch)"; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint fmt; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint run --build-tags $(LINT_TAGS); \
	done

	@echo "[Check] 自动修复代码风格问题..."
	$(LINT_ENV) golangci-lint run --fix --build-tags $(LINT_TAGS)

	@echo "[Check] 运行 Go 单元测试..."
	go test $(GO_COVER_TEST_FLAGS) $(GO_COVER_SCAN)

	@echo "[Check] 运行 Go 赖漏洞检查..."
	govulncheck -show verbose ./...

	@echo "[Check] 完成。"

.PHONY: check-node
check-node: ## 检查前端代码
	@echo "[Check] 检查前端依赖与质量..."
	@echo "  -> 同步依赖 (确保环境一致)..."
	pnpm --dir $(FRONTEND_DIR) install --frozen-lockfile --prefer-offline

	@echo "  -> 检查安全漏洞 (仅高危阻断)..."
	pnpm --dir $(FRONTEND_DIR) audit --production --audit-level high --registry=https://registry.npmjs.org

	@echo "  -> 检查过时依赖..."
	pnpm --dir $(FRONTEND_DIR) outdated

	@echo "  -> 检查代码风格 (Lint)..."
	pnpm --dir $(FRONTEND_DIR) run lint

	@echo "  -> 检测未使用的依赖 (knip)..."
	pnpm --dir $(FRONTEND_DIR) run lint:unused

	@echo "  -> 运行前端单元测试..."
	pnpm --dir $(FRONTEND_DIR) exec vitest run

	@echo "[Check] 前端检查完成。"

.PHONY: check
check: check-node check-go ## 检查代码

#####################
## 更新 / 拉取 / 推送 相关
#####################

.PHONY: sync
sync: ## 按 GIT_REMOTE 顺序先全部拉取合并，再统一推送（保留本地未提交改动）
	@IFS=,; for r in $$GIT_REMOTE; do \
		git fetch "$$r" && git merge --ff-only "$$r/master" || exit 1; \
	done
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "同步完成！"

.PHONY: push
push: ## 推送到所有远程仓库
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "推送完成！"

.PHONY: push-force
push-force: ## 强制推送到所有远程仓库（忽略冲突）
	@IFS=,; for r in $$GIT_REMOTE; do git push --force "$$r" HEAD || exit 1; done
	@echo "强制推送完成！"

#####################
## 测试覆盖率 相关
#####################

# ── 清单配置：新增目录 / 生成代码只改这里，下面的命令不散落路径 ──────────────
# Go 扫描范围：业务代码全在 internal/ 下，main.go 只是可执行入口，不计入分母。
GO_COVER_SCAN = ./internal/...
# Go 排除目录（包路径前缀，空格分隔）。本项目 internal/ 下没有独立生成代码包
#（sqlc 产物与业务代码同包，无法按包前缀排除），需要时在此追加，如 /internal/ent。
GO_COVER_EXCLUDE =
# 关闭 vet 检查，加快测试与覆盖率统计速度
GO_COVER_TEST_FLAGS = -vet=off

# 前端目录与产物（相对仓库根）
NODE_DIR = $(FRONTEND_DIR)
NODE_COVER_DIR = $(NODE_DIR)/coverage
NODE_COVER_LCOV = $(NODE_COVER_DIR)/lcov.info
NODE_JUNIT_XML = $(NODE_COVER_DIR)/junit.xml
# 前端扫描 / 排除清单（glob 相对 vitest root=frontend/，由 frontend/vitest.config.ts 读取）：
#   NODE_TEST_SCAN      测试用例
#   NODE_COVER_SCAN     计入覆盖率的源码
#   NODE_COVER_EXCLUDE  不计入的测试桩 / 类型声明
# 多个 glob 用空格分隔（不要用逗号，也不要用 {a,b} 花括号，会被拆分导致匹配不到文件）。
NODE_TEST_SCAN = src/**/*.test.ts
NODE_COVER_SCAN = src/**/*.ts src/**/*.svelte
NODE_COVER_EXCLUDE = src/test/** src/**/*.d.ts
NODE_VITEST_ENV = VITEST_TEST_INCLUDE="$(NODE_TEST_SCAN)" VITEST_COVER_INCLUDE="$(NODE_COVER_SCAN)" VITEST_COVER_EXCLUDE="$(NODE_COVER_EXCLUDE)"

# -coverpkg 只收「扫描范围内且不在排除列表」的包。用 $$(...) 让 go list 在配方执行时才跑，
# 避免任何 make 目标都触发一次包扫描；grep 参数由排除列表生成，加目录不用改命令。
# 排除列表为空时 grep 会因缺 pattern 报错，用 '^$$'（只匹配空行，实际不会过滤任何包）兜底。
GO_COVER_EXCLUDE_ARGS = $(if $(strip $(GO_COVER_EXCLUDE)),$(foreach d,$(GO_COVER_EXCLUDE),-e '$(d)'),-e '^$$')
GO_COVER_PKGS = $$(go list $(GO_COVER_SCAN) | grep -v $(GO_COVER_EXCLUDE_ARGS) | paste -sd, -)
GO_COVER_ARGS = $(GO_COVER_TEST_FLAGS) -coverpkg=$(GO_COVER_PKGS) -coverprofile=coverage.out $(GO_COVER_SCAN)

.PHONY: cover-go
cover-go: ## Go 覆盖率（golang 格式）→ coverage.out；CNB 的 testing:coverage 与 Codecov 都读这份
	@echo "[Coverage] Go 测试覆盖率 → coverage.out"
	go test $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

.PHONY: junit-go
junit-go: ## Go 覆盖率 + JUnit（Codecov Test Analytics 用），依赖 CI 镜像预装的 gotestsum
	@echo "[Coverage] Go 测试 → coverage.out + go-test-report.xml"
	gotestsum --junitfile go-test-report.xml -- $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

# gocover-cobertura 由统一 CI 镜像预装；-strict 让「包/模块信息缺失被静默跳过」直接失败。
.PHONY: cover-go-cobertura
cover-go-cobertura: cover-go ## Coveralls 的 golang parser 会把包前缀压成 basename，改用 Cobertura 保留完整 filename 路径
	@echo "[Coverage] coverage.out → coverage.xml（Cobertura）"
	gocover-cobertura -by-files -strict < coverage.out > coverage.xml

.PHONY: cover-node
cover-node: ## 前端覆盖率（Vitest v8）→ frontend/coverage/lcov.info
	@echo "[Coverage] 前端覆盖率 → $(NODE_COVER_LCOV)"
	cd $(NODE_DIR) && $(NODE_VITEST_ENV) pnpm exec vitest run --coverage

.PHONY: junit-node
junit-node: ## 前端 JUnit（Codecov Test Analytics 用）；outputFile 用绝对路径避开 vitest root 的相对路径歧义
	@echo "[Coverage] 前端 JUnit → $(NODE_JUNIT_XML)"
	@mkdir -p $(NODE_COVER_DIR)
	cd $(NODE_DIR) && $(NODE_VITEST_ENV) pnpm exec vitest run --reporter=junit --outputFile=$(CURDIR)/$(NODE_JUNIT_XML)