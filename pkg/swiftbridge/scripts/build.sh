#!/usr/bin/env bash
# 编译 Swift 桥接层为静态库 libkai_bridge.a 与动态库 libkai_bridge.dylib。
# 动态库供纯 Go 端（pkg/swiftbridge）用 purego 在运行时 Dlopen 动态加载（零 cgo，不静态链接进主二进制）；
# 静态库一并产出到 pkg/swiftbridge/ 备用。两者每次生成前都会先删除旧文件，确保重编即最新。
# 仅在 macOS（darwin）下需要执行，由 Taskfile / Makefile 调用。
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# 脚本位于 pkg/swiftbridge/scripts/，源码在同级 internal/swift/，dylib 产出到 pkg/swiftbridge/。
SRC_DIR="$(cd "${DIR}/../internal/swift" && pwd)"
PKG_DIR="$(cd "${DIR}/.." && pwd)"
ROOT_DIR="$(cd "${DIR}/../../.." && pwd)"
# 将绝对路径转成相对项目根（用于精简日志输出，macOS 无 realpath --relative-to）。
rel() { echo "${1#"$ROOT_DIR"/}"; }
cd "$SRC_DIR"

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
DYLIB_OUT="libkai_bridge.dylib"
A_OUT="libkai_bridge.a"

# 先清理旧产物，确保每次都重新编译（避免源码已改但加载到带 bug 的旧 dylib / 旧 .a）。
# 同时删除 pkg/swiftbridge/ 下已分发的旧 .a 与 .dylib，防止「重编后 app 仍是旧代码」。
echo ">> cleaning stale build artifacts"
rm -fv "$DYLIB_OUT" "$A_OUT" *.o
echo "removing $(rel "${PKG_DIR}/${DYLIB_OUT}")"
rm -f "${PKG_DIR}/${DYLIB_OUT}"
echo "removing $(rel "${PKG_DIR}/${A_OUT}")"
rm -f "${PKG_DIR}/${A_OUT}"

echo ">> building ${A_OUT} + ${DYLIB_OUT} for $TARGET"
# 桥接层源码已拆分为多个 Swift 文件（bridge_errors / bridge_common / bridge_log /
# apple_translate / apple_accessibility / apple_ocr），均以 *.swift 通配一次性喂给 swiftc
# （Swift 单编译单元，不支持逐文件编译再链接）。-parse-as-library 让 swiftc 以库模式处理
# （不注入默认 _main 入口），避免与 Go 的 main 符号冲突。
# 仅编译真实源文件，排除 .bak/.tmp 等备份。
# 第一步 swiftc -c 产出 .o（库模式），随后复用 .o 既打包静态库 .a（ar rcs），
# 又链接动态库 .dylib（swiftc -emit-library + -undefined dynamic_lookup，
# framework 符号在运行时由宿主进程 Kai.app 提供）。
swiftc -c -parse-as-library $(ls *.swift | grep -v '\.bak$') \
    -target "$TARGET" \
    -O

ar rcs "$A_OUT" *.o

swiftc -emit-library \
    -target "$TARGET" \
    -Xlinker -undefined -Xlinker dynamic_lookup \
    -o "${DYLIB_OUT}" \
    *.o \
    -framework Translation \
    -framework ApplicationServices \
    -framework AppKit \
    -framework Vision \
    -framework CoreGraphics \
    -framework Foundation

# 同步 .a 与 dylib 到 pkg/swiftbridge（纯 Go 动态加载器目录），避免「重编后 app 仍是旧代码」的疑虑：
# 改 Swift 后只需重编本脚本，pkg/swiftbridge 即持有最新 .a / dylib，运行时 Dlopen 加载即为最新。
mkdir -p "${PKG_DIR}"
cp -f "${DYLIB_OUT}" "${PKG_DIR}/${DYLIB_OUT}"
cp -f "$A_OUT" "${PKG_DIR}/${A_OUT}"

# 清理源码目录下的编译产物（.o / .a / .dylib），只保留已 cp 到 PKG_DIR 的那份，
# 确保 internal/swift/ 仅含 .swift 源码，不被编译产物污染。必须在 cp 之后执行。
echo ">> removing local build intermediates (keeping copies in $(rel "$PKG_DIR"))"
rm -fv *.o "$A_OUT" "$DYLIB_OUT"

echo ">> done: $(rel "${PKG_DIR}/${DYLIB_OUT}")"
echo ">> done: $(rel "${PKG_DIR}/${A_OUT}")"
