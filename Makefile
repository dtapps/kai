.PHONY: help icons build-assets dev darwin-build darwin-package darwin-dmg windows-build windows-package linux-build linux-package tidy bindings sqlc swift-build lint-go lint-fe fmt-fe check-cross

# 开发端口固定为 9247
PORT ?= 9247

# 将 stderr 合并到 stdout（便于查看完整输出）。注意：此处「绝不」接 grep 管道，
# 否则会吞掉 golangci-lint 的退出码（grep 匹配到噪声即返回 0），导致 make check
# 在 lint 失败时仍继续（"失败还在继续"）。噪声可裸打印，不影响正确性。
# 用法：<命令> $(FILTER)
FILTER := 2>&1

# 默认目标
help: ## 显示帮助信息
	@echo "Kai 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	cd frontend && pnpm install

bindings: ## 生成 Wails TypeScript 绑定
	wails3 generate bindings -clean=true -ts -i

icons: ## 生成图标资源（icns/ico/Assets.car）
	task common:generate:icons

build-assets: ## 同步版本号/应用名到构建资源（Info.plist、Windows 清单）
	task common:update:build-assets

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

i18n-go: ## 合并 Go 后端 i18n 拆分文件
	./scripts/merge-go-i18n.sh

i18n-frontend: ## 合并前端 i18n 拆分文件
# 	./scripts/merge-frontend-i18n.sh

swift-build: ## 手动编译 Swift 桥接静态库（libkai_translate.a，含翻译与辅助功能）
	cd internal/swiftbridge && bash ./build.sh

dev: i18n ## 运行 Wails 开发模式
	wails3 dev -port $(PORT)

# ==================== 格式化 / 修复 ====================

format: format-go format-frontend format-i18n-go format-i18n-frontend ## 格式化和修复（全部）

format-go: ## 格式化 Go 代码
	gofmt -w -s .
	go fmt ./...
	go fix ./...

format-frontend: ## 修复前端代码
	pnpm --dir ./frontend run format

format-i18n-go: ## 格式化 Go 后端 i18n JSON 文件
	pnpm --dir ./frontend exec prettier --write "$(CURDIR)/internal/i18n/locales/**/*.json"

format-i18n-frontend: ## 格式化前端 i18n JSON 文件
# 	pnpm --dir ./frontend exec prettier --write "$(CURDIR)/internal/i18n/*.json"

# ==================== 检查 / 测试 ====================

check: lint-go lint-frontend test-go fuzz-go vuln-go check-cross ## 检查和测试（全部）

check-cross: ## 交叉编译验证（darwin/arm64 开 CGO + windows CGO=0；过滤 ld:warning）
	@echo "==> 交叉编译 darwin/arm64 (CGO)"
	GOOS=darwin GOARCH=arm64 go build ./... 2>&1 | grep -v 'ld: warning' || true
# 	@echo "==> 交叉编译 darwin/amd64 (CGO)"
# 	GOOS=darwin GOARCH=amd64 go build ./... 2>&1 | grep -v 'ld: warning' || true
	@echo "==> 交叉编译 windows/arm64 (CGO_ENABLED=0)"
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ./... 2>&1 | grep -v 'ld: warning' || true
	@echo "==> 交叉编译 windows/amd64 (CGO_ENABLED=0)"
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./... 2>&1 | grep -v 'ld: warning' || true
	@echo "==> 交叉编译验证完成"

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

# 版本/构建时间默认值：git tag 优先，无 tag 用 dev；构建时间取当前 UTC。
# 通过同名变量传给 task（task 的 {{.VERSION}} / {{.BUILD_TIME}} 会读取），可在命令行覆盖：
#   make darwin-package VERSION=1.2.0 BUILD_TIME=2026-08-12T00:00:00Z
VERSION     ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
BUILD_TIME  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
GITHUB_TOKEN ?=
CNB_TOKEN   ?=
DEV         ?= false

darwin-build: ## [macOS] 编译正式二进制 -> bin/Kai（不打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task darwin:build

darwin-package: ## [macOS] 正式打包 -> bin/Kai.app（打包成 .app）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task darwin:package

darwin-package-dmg: ## [macOS] 打包并生成 bin/Kai.dmg 安装包
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task darwin:package:dmg

windows-build: ## [Windows] 编译正式二进制 -> bin/Kai.exe（不打包）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task windows:build

windows-package: ## [Windows] 正式打包 -> bin/Kai.exe + 安装包（nsis/msix）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task windows:package

linux-build: ## [Linux] 编译正式二进制（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task linux:build

linux-package: ## [Linux] 正式打包 -> bin/Kai（本地联调用）
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) GITHUB_TOKEN=$(GITHUB_TOKEN) CNB_TOKEN=$(CNB_TOKEN) DEV=$(DEV) task linux:package

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

# 	pnpm --dir ./frontend update --latest
update-deps: ## 更新所有依赖
	@echo "==> 更新所有依赖..."

	go version
	go get -u ./...
	go mod tidy
	@echo "==> Go 更新所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend update
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