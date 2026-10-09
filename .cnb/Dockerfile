# 使用 golang 镜像作为基础
FROM golang:1.27.2-trixie

# ==========================================
# 定义版本变量
# https://github.com/nodejs/node/releases
# https://github.com/wailsapp/wails/releases
# https://github.com/sqlc-dev/sqlc/releases
# https://github.com/golangci/golangci-lint/releases
# https://go.googlesource.com/vuln
# https://github.com/mvdan/sh/releases
# https://github.com/reteps/dockerfmt/releases
# https://github.com/google/yamlfmt/releases
# https://github.com/itchyny/gojq/releases
# ==========================================
ARG NODE_VERSION=26.11.1
ARG WAILS3_VERSION=v3.0.0-beta.28
ARG SQLC_VERSION=v1.31.1
ARG GOLANGCI_LINT_VERSION=v2.14.0
ARG GOVULNCHECK_VERSION=v1.8.0
ARG SHFMT_VERSION=v3.14.1
ARG DOCKERFMT_VERSION=v0.5.4
ARG YAMLFMT_VERSION=v0.21.0
ARG GOJQ_VERSION=v0.12.19

# 1. 设置 Go 环境变量
ENV GO111MODULE=on
ENV GOPROXY=https://mirrors.cloud.tencent.com/go/,direct
ENV GOINSECURE=go.dtapp.net
ENV GOPATH=/go
ENV PATH="$GOPATH/bin:$PATH"

# 2. 替换 Debian APT 加速源
RUN sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
    || sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list

# 3. 安装基础构建工具
# 注意：libgtk-4-dev / libwebkitgtk-6.0-dev 是 wails3 CLI 从源码 `go install` 及
# 后续 go build/test 编译 webview 所必须的（提供 gtk4.pc / webkitgtk-6.0.pc），
# 否则 make tool-deps / make check 会因 pkg-config 找不到头文件而失败。
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    xz-utils \
    ca-certificates \
    curl \
    gnupg \
    zip \
    unzip \
    pkg-config \
    libgtk-4-dev \
    libwebkitgtk-6.0-dev \
    jq

# 4. 安装 Node.js 工具
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then NODE_ARCH="x64"; \
    elif [ "$ARCH" = "aarch64" ]; then NODE_ARCH="arm64"; \
    else echo "Unsupported architecture: $ARCH" && exit 1; fi && \
    \
    # 构建下载地址
    NODE_DIR="node-v${NODE_VERSION}-linux-${NODE_ARCH}" && \
    NODE_URL="https://nodejs.org/dist/v${NODE_VERSION}/${NODE_DIR}.tar.xz" && \
    \
    cd /tmp && \
    wget -q -O node.tar.xz "${NODE_URL}" && \
    tar -xJf node.tar.xz -C /usr/local --strip-components=1 && \
    \
    # 配置 npm/pnpm 使用腾讯云源
    npm config set registry https://mirrors.cloud.tencent.com/npm/ && \
    npm install -g pnpm && \
    pnpm config set registry https://mirrors.cloud.tencent.com/npm/ && \
    pnpm config set ignore-scripts false && \
    \
    # 清理
    rm -rf node.tar.xz && \
    echo "--- Node.js installed ---" && \
    node -v && npm -v && pnpm -v

# 5 预安装 Go 工具链（版本已锁定）
RUN echo "==> 预安装 Go 工具链（版本已锁定）..." \
    && go install github.com/wailsapp/wails/v3/cmd/wails3@${WAILS3_VERSION} \
    && echo "==> wails3 安装完成 (${WAILS3_VERSION})" \
    && go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION} \
    && echo "==> sqlc 安装完成 (${SQLC_VERSION})" \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && echo "==> golangci-lint 安装完成 (${GOLANGCI_LINT_VERSION})" \
    && go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION} \
    && echo "==> govulncheck 安装完成 (${GOVULNCHECK_VERSION})" \
    && go install mvdan.cc/sh/v3/cmd/shfmt@${SHFMT_VERSION} \
    && echo "==> shfmt 安装完成 (${SHFMT_VERSION})" \
    && go install github.com/reteps/dockerfmt@${DOCKERFMT_VERSION} \
    && echo "==> dockerfmt 安装完成 (${DOCKERFMT_VERSION})" \
    && go install github.com/google/yamlfmt/cmd/yamlfmt@${YAMLFMT_VERSION} \
    && echo "==> yamlfmt 安装完成 (${YAMLFMT_VERSION})" \
    && go install github.com/itchyny/gojq/cmd/gojq@${GOJQ_VERSION} \
    && echo "==> gojq 安装完成 (${GOJQ_VERSION})" \
    && echo "--- Go 工具链安装完成 ---" \
    && ls -l $GOPATH/bin

# 6. 卸载不需要的工具，并清空所有 apt 缓存
RUN apt-get purge -y wget xz-utils \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /root/.npm

CMD ["bash"]