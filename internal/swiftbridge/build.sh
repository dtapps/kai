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
rm -f "$OUT" *.o

echo ">> building $OUT for $TARGET"
# 桥接层源码已拆分为多个 Swift 文件（bridge_errors / bridge_common / bridge_log /
# apple_translate / apple_accessibility / apple_ocr），均以 *.swift 通配一次性喂给 swiftc
# （Swift 单编译单元，不支持逐文件编译再 ar）。-c 模式会为每个源文件产出同名 .o，
# 再由 ar 全部打包成静态库供 cgo 链接。-parse-as-library 让 swiftc 以库模式处理
# （不注入默认 _main 入口），避免与 Go 的 main 符号冲突。
# 仅编译真实源文件，排除 .bak/.tmp 等备份（否则旧 kai_ocr 等符号会被重复编译进 .a）。
swiftc -c -parse-as-library $(ls *.swift | grep -v '\.bak$') \
    -framework Translation \
    -framework ApplicationServices \
    -framework AppKit \
    -framework Vision \
    -framework CoreGraphics \
    -target "$TARGET" \
    -O

ar rcs "$OUT" *.o
rm -f *.o
echo ">> done: $DIR/$OUT"
