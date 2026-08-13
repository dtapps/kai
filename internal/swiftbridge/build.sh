#!/usr/bin/env bash
# 编译 Swift 桥接层为静态库，供 cgo 链接。
# 仅在 macOS（darwin）下需要执行，由 Taskfile / go:generate 调用。
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

# 自动取 SDK 版本；低于 26 则兜底 26.0（系统翻译引擎 API 要求 macOS 26+）。
SDK_VER="$(xcrun --show-sdk-version 2>/dev/null || echo '26.0')"
SDK_MIN="$(awk -v v="$SDK_VER" 'BEGIN { print (v+0 >= 26) ? v : "26.0" }')"

# 桥接库架构必须与当前编译目标 GOARCH 对齐（amd64→x86_64，arm64→arm64），
# 否则链接 x86_64 二进制时找不到 arm64 切片里的 C 符号。优先用显式 KAI_BRIDGE_TARGET，
# 否则按 GOARCH（CI/本地 `GOARCH=xxx` 已设置）推导，再回退 arm64。
case "${KAI_BRIDGE_TARGET:-__auto__}" in
  __auto__)
    case "${GOARCH:-$(go env GOARCH 2>/dev/null)}" in
      amd64) BRIDGE_ARCH="x86_64" ;;
      arm64) BRIDGE_ARCH="arm64" ;;
      *)     BRIDGE_ARCH="arm64" ;;
    esac
    TARGET="${BRIDGE_ARCH}-apple-macosx${SDK_MIN}"
    ;;
  *) TARGET="$KAI_BRIDGE_TARGET" ;;
esac
OUT="libkai_bridge.a"

# 先清理旧产物，确保每次都重新编译（避免源码已改但链接到带 bug 的旧 .a）。
rm -f "$OUT" kai_bridge.o

echo ">> building $OUT for $TARGET"
# 单一 swift 源文件（kai_bridge.swift 内已含翻译 + 辅助功能 + 共享日志），-c 只产目标文件；
# -parse-as-library 让 swiftc 以库模式处理（不注入默认 _main 入口），避免与 Go 的 main 符号冲突。
# 再用 ar 打包成静态库供 cgo 链接。
swiftc -c -parse-as-library -o kai_bridge.o kai_bridge.swift \
    -framework Translation \
    -framework ApplicationServices \
    -framework AppKit \
    -framework Vision \
    -framework CoreGraphics \
    -target "$TARGET" \
    -O

ar rcs "$OUT" kai_bridge.o
rm -f kai_bridge.o
echo ">> done: $DIR/$OUT"
