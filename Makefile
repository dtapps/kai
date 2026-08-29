.PHONY: help icons build-assets dev darwin-build darwin-package darwin-dmg windows-build windows-package linux-build linux-package tidy bindings sqlc swift-build lint-go lint-fe fmt-fe format-swift check-cross

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
	cd frontend && pnpm install

bindings: ## 生成 Wails TypeScript 绑定
	wails3 generate bindings -clean=true -ts -i

icons: ## 生成图标资源（icns/ico/Assets.car）
	wails3 task common:generate:icons

build-assets: ## 同步版本号/应用名到构建资源（Info.plist、Windows 清单）
	wails3 task common:update:build-assets

# wails3 生成命令全集（备查）：
#   通用（全平台）：generate bindings / generate icons / update build-assets  ← 已聚合进下方 wails3-generate
#   Windows 专属：generate syso / generate webview2bootstrapper  ← 由 build/windows/Taskfile.yml 在 build/package 时自动触发
#   Linux  专属：generate appimage / generate .desktop            ← 由 build/linux/Taskfile.yml 在 build/package 时自动触发
# 平台专属命令不在本 Makefile 暴露（非目标平台跑无意义，依赖各自 Taskfile 自动跑）。
wails3-generate: bindings icons build-assets ## 生成 Wails 绑定/图标/构建资源

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
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 dev -port $(PORT)

# ==================== 格式化 / 修复 ====================

format: format-go format-swift format-frontend format-i18n-go format-i18n-frontend ## 格式化和修复（全部）

format-go: ## 格式化 Go 代码
	gofmt -w -s .
	go fmt ./...
	go fix ./...

# 用 Apple 官方 swift-format 原地格式化 Swift 桥接层源码。
# 未安装时给出明确安装提示（brew install swift-format 或走 swift 工具链）。
format-swift: ## 格式化 Swift 桥接层源码（原地，需 swift-format）
	@command -v swift-format >/dev/null 2>&1 || { \
		echo "!! swift-format 未安装，请先安装："; \
		echo "   brew install swift-format"; \
		echo "   或确保 swift 工具链自带 swift-format 在 PATH 中"; \
		exit 1; }
	swift-format --in-place ./pkg/swiftbridge/internal/swift/*.swift
	@echo ">> swift format done"

format-frontend: ## 修复前端代码
	pnpm --dir ./frontend run format

format-i18n-go: ## 格式化 Go 后端 i18n JSON 文件
	pnpm --dir ./frontend exec prettier --write "$(CURDIR)/internal/i18n/locales/**/*.json"

format-i18n-frontend: ## 格式化前端 i18n JSON 文件
# 	pnpm --dir ./frontend exec prettier --write "$(CURDIR)/internal/i18n/*.json"

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

check: lint-go lint-frontend test-go fuzz-go vuln-go check-cross ## 检查和测试（全部）

lint-go: ## Go 代码检查（有 issue 即停止）
	golangci-lint run ./...
	golangci-lint run --fix ./...

lint-frontend: ## 前端 TypeScript 类型检查（类型错误即停止）
	pnpm --dir ./frontend run tsc
# 	pnpm --dir ./frontend run check
	pnpm --dir ./frontend run build:dev

test-go: ## Go 后端测试（测试失败即停止）
	go test -vet=off -v ./internal/... -count=1

fuzz-go: ## Go 模糊测试（make fuzz-go FUZZ=FuzzXxx 时间=30s；失败即停止）
	go test -vet=off -fuzz=$(FUZZ) -fuzztime=$(or $(TIME),30s) ./internal/...

vuln-go: ## Go 依赖漏洞检查（发现漏洞即停止）
	govulncheck -show verbose ./...

# ==================== 构建打包 ====================

darwin-build: ## [macOS] 编译正式二进制 -> bin/Kai（不打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task darwin:build

darwin-package: ## [macOS] 正式打包 -> bin/Kai.app（打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task darwin:package

darwin-package-dmg: ## [macOS] 打包并生成 bin/Kai.dmg 安装包
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task darwin:package:dmg

windows-build: ## [Windows] 编译正式二进制 -> bin/Kai.exe（不打包）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task windows:build

windows-package: ## [Windows] 正式打包 -> bin/Kai.exe + 安装包（nsis/msix）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task windows:package

linux-build: ## [Linux] 编译正式二进制（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task linux:build

linux-package: ## [Linux] 正式打包 -> bin/Kai（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) wails3 task linux:package

# ==================== 其他 ====================

clean: ## 清理构建产物
	rm -rf frontend/dist frontend/bindings
	rm -f kai bin/*

tool-deps: ## 工具依赖
	@echo "==> 安装必要的工具依赖..."

	wails3 version || true
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	-wails3 version
	@echo "==> wails3 工具安装或更新完成"

	sqlc version || true
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	-sqlc version
	@echo "==> sqlc 工具安装或更新完成"

	go install entgo.io/ent/cmd/ent@latest
	go install entgo.io/ent/cmd/entc@latest
	@echo "==> ent 工具安装或更新完成"

	golangci-lint version || true
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	-golangci-lint version
	@echo "==> golangci-lint 工具安装或更新完成"

	govulncheck version || true
	go install golang.org/x/vuln/cmd/govulncheck@latest
	-govulncheck version
	@echo "==> govulncheck 工具安装或更新完成"

deps: ## 安装所有依赖
	@echo "==> 安装所有依赖..."

	go version
	go mod download
	@echo "==> Go 安装所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend install
	@echo "==> pnpm 安装所有依赖完成"

update-deps: ## 更新所有依赖
	@echo "==> 更新所有依赖..."

	go version
	go get -u ./...
	go mod tidy
	@echo "==> Go 更新所有依赖完成"

	pnpm --version
# 	pnpm --dir ./frontend update
	pnpm --dir ./frontend update --latest
	pnpm --dir ./frontend self-update
	@echo "==> pnpm 更新所有依赖完成"

setup: deps bindings ent sqlc ## 完整项目初始化
	@echo "项目初始化完成！运行 'make dev' 启动开发模式"

# ==================== 更新 / 拉取 ====================

sync: ## 拉取 GitHub 最新并以 fast-forward 合并（保留本地未提交改动）
	git fetch origin
	git merge --ff-only origin/master
	@echo "已同步 origin/master 最新代码，本地未提交改动已保留"

pull: sync ## sync 别名（拉取远程最新并保留本地改动）

# ==================== 推送 ====================

push: ## 推送到所有远程仓库
	git push origin HEAD
	git push cnb HEAD
	git push gitea HEAD
	git push gitlab HEAD
	git push gitee HEAD
	git push gitcode HEAD
	@echo "推送完成！"

push-force: ## 强制推送到所有远程仓库（忽略冲突）
	git push --force origin HEAD
	git push --force cnb HEAD
	git push --force gitea HEAD
	git push --force gitlab HEAD
	git push --force gitee HEAD
	git push --force gitcode HEAD
	@echo "强制推送完成！"