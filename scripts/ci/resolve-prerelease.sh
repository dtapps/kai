#!/usr/bin/env bash
#
# 版本号归一化 + 预发布判定。
#
# 事实来源：GitHub Actions 的 .github/workflows/release.yml 与 CNB 的 .cnb.yml
# 都 source 这一份实现，避免各处各写一套 grep 正则、判断口径慢慢跑偏
# （此前 GitHub / CNB / GitCode / Gitee 各写了一遍「补 v」和「猜预发布」）：
#
#   v1.2.3 / 1.2.3        -> TAG=v1.2.3        PRERELEASE=false   正式版
#   v1.2.3-rc.1           -> TAG=v1.2.3-rc.1   PRERELEASE=true    预发布
#   v1.2.3-beta.1         -> TAG=v1.2.3-beta.1 PRERELEASE=true    预发布
#   v1.2.3-alpha.1        -> TAG=v1.2.3-alpha.1 PRERELEASE=true   预发布
#
# 只用版本号后缀判定，不接受「是不是预发布」这种布尔输入：布尔输入会和版本号打架。
#
# 用法（被 source）：
#   . scripts/ci/resolve-prerelease.sh
#   resolve_version "${VERSION}"
#   printf '%s %s\n' "${TAG}" "${PRERELEASE}"
#
# 直接执行可以跑内置用例自检（改了正则以后先跑它）：
#   bash scripts/ci/resolve-prerelease.sh --self-test
#
# ⚠️ 本文件必须保持 POSIX sh 语法：CNB 的 stage 默认用 `sh`（可能是 dash/ash）执行，
# GitHub Actions 用 bash。不要引入 [[ ]]、数组、BASH_SOURCE、$'...' 这类 bash 专有写法。

# derive_packaging_versions
#   从 TAG 派生各打包平台能接受的版本串，全部 export：
#
#   VERSION_BASE   X.Y.Z          三段纯数字。用于 build/config.yml 的 info.version。
#                  这个值会流到两处严格格式的地方：
#                    - Windows NSIS 的 VIProductVersion/VIFileVersion：模板自己补一个 ".0"，
#                      必须是 4 段纯数字，传进 "0.2.8-rc.1" 就拼成 0.2.8-rc.1.0，
#                      makensis 直接报 "invalid VIFileVersion format, should be X.X.X.X"
#                    - macOS 的 CFBundleShortVersionString：同样只接受 X.Y.Z
#
#   VERSION_PKG    X.Y.Z~rc.1     deb / rpm / arch 用（把 '-' 换成 '~'）。
#                  rpm 的 Version 字段不允许 '-'（那是 Version 与 Release 的分隔符），
#                  deb 里 '~' 又正好表示「排序低于正式版」（rc 升级正式版不会被判成降级），
#                  arch 的 pkgver 同样禁止 '-'。一个串同时满足三种格式。
#
# 注意：注入二进制的 buildinfo.Version 仍用带后缀的原始版本（用户看到的版本），不受这里影响。
derive_packaging_versions() {
  _ver="${TAG#v}"
  _ver="${_ver#V}"

  # 取前三段数字；不足三段补 0，多余部分（如 0.2.8.1 的第 4 段）丢弃
  _core="$(printf '%s' "${_ver}" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*$/\1/')"
  case "${_core}" in
    *.*.*) ;;
    *.*) _core="${_core}.0" ;;
    *) _core="${_core}.0.0" ;;
  esac

  VERSION_BASE="${_core}"
  VERSION_PKG="$(printf '%s' "${_ver}" | sed 's/-/~/g')"
}

# resolve_version <版本号>
#   归一化版本号（补 v 前缀）并判定是否预发布；
#   结果写入全局变量 TAG / PRERELEASE / VERSION_TYPE / PRE_NOTE 并导出给后续步骤。
#   版本号为空时返回 1，由调用方决定是 `set -e` 直接失败还是自行处理。
resolve_version() {
  local input="${1:-}"

  # 去掉所有空白字符：workflow_dispatch 输入框粘贴时经常带上首尾空格/换行
  input="$(printf '%s' "${input}" | tr -d '[:space:]')"

  if [ -z "${input}" ]; then
    printf '错误: 版本号不能为空。示例: v1.2.3 / v1.2.3-rc.1 / v1.2.3-beta.1 / v1.2.3-alpha.1\n' >&2
    return 1
  fi

  # 字符白名单：版本号最终会变成 git tag、Docker tag 和 JSON 字段值。
  # 含引号/反斜杠/换行的输入既不是合法版本号，拼进 JSON 时还会破坏结构（甚至注入字段），
  # 所以在这里一次性挡掉。允许的字符与 Docker tag 的字符集一致。
  if printf '%s' "${input}" | grep -qvE '^[0-9A-Za-z._+-]+$'; then
    printf '错误: 版本号含非法字符（只允许字母、数字、. _ + -）: %s\n' "${input}" >&2
    return 1
  fi

  # 统一补 v 前缀：输入 1.2.3 与 v1.2.3 等价；大写 V 前缀也算已经有了，
  # 否则会拼出 vV1.2.3 这种既不像 tag 又不像版本号的东西
  case "${input}" in
    v* | V*) TAG="${input}" ;;
    *) TAG="v${input}" ;;
  esac

  # 后缀判定：-rc / -beta / -alpha（大小写不敏感，rc1 / rc.1 / 裸 rc 都算预发布）
  # 匹配要求后缀出现在版本号末尾，避免 v1.2.3-source、v1.2.3-hotfix 这类被误判
  if printf '%s' "${TAG}" | grep -qiE -- '-(rc|beta|alpha)([.-]?[0-9A-Za-z.-]*)$'; then
    PRERELEASE="true"
    VERSION_TYPE="预发布"
  else
    PRERELEASE="false"
    VERSION_TYPE="正式版"
  fi

  # 派生各打包平台能接受的版本串（见 derive_packaging_versions）
  derive_packaging_versions

  PRE_NOTE="版本号: ${TAG}（${VERSION_TYPE}，prerelease=${PRERELEASE}，打包版本=${VERSION_BASE} / ${VERSION_PKG}）"
  export TAG PRERELEASE VERSION_TYPE PRE_NOTE VERSION_BASE VERSION_PKG

  return 0
}

# 下面是「直接执行」才跑的自检，被 source 时只导出函数就收工。
#
# 判定方式不能用 `[ "${BASH_SOURCE[0]}" = "${0}" ]`：数组下标是 bash 专有语法，CNB 的 stage
# 默认用 sh（dash）执行，`${BASH_SOURCE[0]:-}` 会直接 "Bad substitution" 让整个 stage 挂掉。
# 这里改用 $0 的 basename：直接执行时 $0 是本文件名，被 source 时 $0 是调用方
# （bash / sh / /bin/dash 或调用脚本），在 POSIX 范围内可靠。
# 另外放行显式的 --self-test：source 的调用方不会传这个参数，不会误触发。
if [ "${0##*/}" != 'resolve-prerelease.sh' ] && [ "${1:-}" != '--self-test' ]; then
  # `return` 在被 source 时生效；万一本文件被 `bash < file` 之类方式执行（$0 不是文件名、
  # return 会失败），退回 exit 0。shellcheck 认为 return 不会失败，故显式关掉该提示。
  # shellcheck disable=SC2317
  return 0 2>/dev/null || exit 0
fi

if [ "${1:-}" != "--self-test" ]; then
  printf '用法: %s --self-test\n通常不需要单独执行，CI 里是 source 使用：%s\n' "${0}" '. scripts/ci/resolve-prerelease.sh' >&2
  exit 2
fi

failures=0

check_version() {
  local input="$1" want_tag="$2" want_pre="$3"

  if ! resolve_version "${input}" 2>/dev/null; then
    printf '  ✗ %-20s 解析失败（期望 %s）\n' "${input:-<空>}" "${want_tag}"
    failures=$((failures + 1))
    return 0
  fi

  if [ "${TAG}" = "${want_tag}" ] && [ "${PRERELEASE}" = "${want_pre}" ]; then
    printf '  ✓ %-20s -> %-20s prerelease=%s\n' "${input}" "${TAG}" "${PRERELEASE}"
  else
    printf '  ✗ %-20s -> %-20s prerelease=%s（期望 %s / %s）\n' \
      "${input}" "${TAG}" "${PRERELEASE}" "${want_tag}" "${want_pre}"
    failures=$((failures + 1))
  fi
}

check_version 'v1.2.3' 'v1.2.3' 'false'
check_version '1.2.3' 'v1.2.3' 'false'
check_version 'v1.2.3-rc.1' 'v1.2.3-rc.1' 'true'
check_version 'v1.2.3-beta.1' 'v1.2.3-beta.1' 'true'
check_version 'v1.2.3-alpha.1' 'v1.2.3-alpha.1' 'true'
check_version 'v1.2.3-rc' 'v1.2.3-rc' 'true'
check_version 'V1.2.3-RC.1' 'V1.2.3-RC.1' 'true'
check_version 'v1.2.3-alpha.1.2' 'v1.2.3-alpha.1.2' 'true'
check_version ' v1.2.3 ' 'v1.2.3' 'false'
# 不带 rc/beta/alpha 后缀的一律是正式版
check_version 'v1.2.3-hotfix' 'v1.2.3-hotfix' 'false'
check_version 'v2.0.0-core' 'v2.0.0-core' 'false'

# 打包版本派生：Windows NSIS / macOS plist 必须是 X.Y.Z；deb/rpm/arch 不能出现 '-'
check_pkg() {
  local input="$1" want_base="$2" want_pkg="$3"

  if ! resolve_version "${input}" 2>/dev/null; then
    printf '  ✗ %-20s 解析失败（期望 base=%s）\n' "${input}" "${want_base}"
    failures=$((failures + 1))
    return 0
  fi

  if [ "${VERSION_BASE}" = "${want_base}" ] && [ "${VERSION_PKG}" = "${want_pkg}" ]; then
    printf '  ✓ %-20s → base=%-8s pkg=%s\n' "${input}" "${VERSION_BASE}" "${VERSION_PKG}"
  else
    printf '  ✗ %-20s → base=%s pkg=%s（期望 %s / %s）\n' \
      "${input}" "${VERSION_BASE}" "${VERSION_PKG}" "${want_base}" "${want_pkg}"
    failures=$((failures + 1))
  fi
}

check_pkg 'v0.2.8' '0.2.8' '0.2.8'
check_pkg '0.2.8' '0.2.8' '0.2.8'
check_pkg 'v0.2.8-rc.1' '0.2.8' '0.2.8~rc.1'
check_pkg 'v0.2.8-beta.2' '0.2.8' '0.2.8~beta.2'
check_pkg 'v0.2.8-alpha.1.2' '0.2.8' '0.2.8~alpha.1.2'
check_pkg 'v0.2.8-hotfix' '0.2.8' '0.2.8~hotfix'
# 段数不足要补齐，否则 VIProductVersion 拼不出 4 段
check_pkg 'v1.2' '1.2.0' '1.2'

# 非法输入必须被拒绝，不能漏到 git tag / Docker tag / JSON 里：
# 空版本号历史上会打出 :v 这种非法标签；引号/分号会破坏 optionsFrom 的 JSON 结构。
# 注意不要把 "v1.2.3 beta" 当非法输入：空白会先被去掉（粘贴常带空格），结果 v1.2.3beta 合法
for bad_input in '' 'v1.2.3"x' 'v1.2.3;rm -rf /' 'v1.2.3@1' 'v1/2/3'; do
  if resolve_version "${bad_input}" 2>/dev/null; then
    printf '  ✗ %-20s 应当报错，实际通过（TAG=%s）\n' "${bad_input:-<空>}" "${TAG}"
    failures=$((failures + 1))
  else
    printf '  ✓ %-20s 被拒绝（返回非 0）\n' "${bad_input:-<空>}"
  fi
done

if [ "${failures}" -ne 0 ]; then
  printf '\n自检失败: %d 项\n' "${failures}" >&2
  exit 1
fi

printf '\n自检通过\n'
