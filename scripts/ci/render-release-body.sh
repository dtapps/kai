#!/bin/sh
#
# 生成 Release 正文（Markdown）。
#
# 事实来源：本项目的 CNB / Gitee 两个平台都镜像 GitHub Release 的正文，
# 所以正文格式只在 .github/workflows/release.yml 调用的这份脚本里维护一次。
#
# 只用环境变量传参（不解析位置参数），这样任何一条流水线都能直接
# `sh scripts/ci/render-release-body.sh`：
#
#   VERSION       必填，归一化后的版本号（带 v 前缀），如 v1.2.3-rc.1
#   PRERELEASE    必填，true / false，决定是否加「预发布」警示块
#   DL_BASE       必填，附件下载基址，如 https://github.com/o/r/releases/download/v1.2.3
#   STABLE_URL    选填，正式版列表页 URL，预发布时提示用户去哪找稳定版
#   ARTIFACT_DIR  选填，产物目录，默认 bin
#   OUT_FILE      选填，输出文件，默认 release_body.md
#   CHANGELOG     选填，更新内容（可多行）；为空时输出「暂无更新内容」
#   COMMIT_SHA    选填，提交 SHA；有则加一行提交链接
#   COMMIT_URL    选填，提交链接前缀，拼成 ${COMMIT_URL}/<sha>
#                 GitHub 是 ${REPO_URL}/commit，CNB 是 ${REPO_URL}/-/commit
#   REPO_URL      选填，仓库地址（仅作 COMMIT_URL 缺省时的回退）
#   BUILD_TIME    选填，构建时间，默认取当前 UTC 时间
#
# 自检（改了格式先跑它）：
#   sh scripts/ci/render-release-body.sh --self-test
#
# ⚠️ 必须保持 POSIX sh 语法：脚本可能在 dash/ash 下执行（CNB 的 stage 默认用 sh）。

set -eu

# 下载表格的附件解析与说明文案统一由 scripts/ci/describe-artifact.sh 提供（跨仓库通用），
# source 进来后直接用它的 parse_artifact_name / describe_artifact 渲染表格行，
# 本文件不再内嵌重复实现，避免两处文案跑偏。
# 先 parse_artifact_name <文件名> 写 PLAT / ARCH 等全局变量，再无参 describe_artifact 取文案。
# 用 $0 定位脚本目录，保证从任意 cwd 调用都能找到同目录的 describe-artifact.sh。
_ci_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=scripts/ci/describe-artifact.sh
. "${_ci_dir}/describe-artifact.sh"

# 自动更新专用包带 updater- 前缀（updater-Kai-...），不参与下载表格，由调用方决定是否发布。

# 是否属于「可下载产物」。以下三类不算：
#   - SHA256SUMS：校验文件本身，单独在「校验」节说明
#   - BUILD_TIME / GIT_COMMIT：仅预发布版本附带，供更新器比较，用户不需要下载
#   - updater-*：自动更新专用包（必须是 .app 归档/免安装包），表格里列出来只会让人误下载
is_artifact() {
  case "$1" in
    SHA256SUMS | BUILD_TIME | GIT_COMMIT) return 1 ;;
    updater-*) return 1 ;;
  esac
  return 0
}

render_body() {
  : "${VERSION:?VERSION 为空}"
  : "${PRERELEASE:?PRERELEASE 为空}"
  : "${DL_BASE:?DL_BASE 为空}"

  artifact_dir="${ARTIFACT_DIR:-bin}"
  out_file="${OUT_FILE:-release_body.md}"

  if [ ! -d "${artifact_dir}" ]; then
    printf '❌ 产物目录不存在: %s\n' "${artifact_dir}" >&2
    return 1
  fi

  if [ "${PRERELEASE}" = 'true' ]; then
    release_type='🚧 预发布版本 (Pre-release)'
  else
    release_type='✅ 正式发布版本 (Stable)'
  fi
  build_time="${BUILD_TIME:-$(date -u '+%Y-%m-%d %H:%M:%S UTC')}"

  # 提交号保留完整 SHA 作为**链接目标**（缩写长度随仓库规模变化，链接里用缩写
  # 部分平台会 404）；另算一个短号只用于**显示**，和 GitHub 页面习惯一致。
  if [ -n "${COMMIT_SHA:-}" ]; then
    COMMIT_SHA="$(printf '%s' "${COMMIT_SHA}" | tr -d '[:space:]')"
    COMMIT_SHORT="$(printf '%s' "${COMMIT_SHA}" | cut -c1-7)"
  fi

  # 统计可发布的附件
  artifact_count=0
  # 校验节要给 Windows 用户写出具体文件名，这里顺手把 Windows 产物收集起来，
  # 避免在正文里硬编码某个架构的文件名（架构变了正文就会写错）。
  windows_artifacts=''
  for f in "${artifact_dir}"/*; do
    [ -e "${f}" ] || continue
    [ -d "${f}" ] && continue
    fname="$(basename "${f}")"
    is_artifact "${fname}" || continue
    artifact_count=$((artifact_count + 1))
    case "${fname}" in
      *-windows-*) windows_artifacts="${windows_artifacts}${windows_artifacts:+
}${fname}" ;;
    esac
  done

  {
    # 预发布在最前面单独警示：正文往下滚才发现是 rc，人已经点了下载
    if [ "${PRERELEASE}" = 'true' ]; then
      echo '## ⚠️ 预发布版本'
      echo ''
      echo '> 这是预发布版本，可能包含未充分测试的功能，不建议用于生产环境。'
      if [ -n "${STABLE_URL:-}" ]; then
        printf '> 如需稳定版本，请使用 [最新正式版](%s)。\n' "${STABLE_URL}"
      fi
      echo ''
    fi

    echo '## 发布信息'
    echo ''
    echo '| 项目 | 内容 |'
    echo '| --- | --- |'
    printf '| 版本号 | %s |\n' "${VERSION}"
    printf '| 发布类型 | %s |\n' "${release_type}"
    # 提交链接前缀：COMMIT_URL 优先（CNB 的 Web 路径带 /-/，和 GitHub 不同），
    # 没传时退回 GitHub 风格的 ${REPO_URL}/commit。
    _commit_url="${COMMIT_URL:-}"
    if [ -z "${_commit_url}" ] && [ -n "${REPO_URL:-}" ]; then
      _commit_url="${REPO_URL}/commit"
    fi
    if [ -n "${COMMIT_SHA:-}" ] && [ -n "${_commit_url}" ]; then
      printf '| 提交 | [%s](%s/%s) |\n' "${COMMIT_SHORT}" "${_commit_url}" "${COMMIT_SHA}"
    fi
    printf '| 构建时间 | %s |\n' "${build_time}"
    echo ''

    echo '## 下载'
    echo ''
    if [ "${artifact_count}" -eq 0 ]; then
      echo '> 本次未产出可下载的附件。'
    else
      echo '| 平台 | 架构 | 文件 | 大小 | 说明 |'
      echo '| --- | --- | --- | --- | --- |'
      for f in "${artifact_dir}"/*; do
        [ -e "${f}" ] || continue
        [ -d "${f}" ] && continue
        fname="$(basename "${f}")"
        is_artifact "${fname}" || continue
        # 用 wc -c 取字节数再换算，避免 ls 在非 ASCII 文件名下的解析问题（SC2012）
        fbytes="$(wc -c <"${f}" | tr -d ' ')"
        if [ "${fbytes}" -ge 1048576 ]; then
          fsize="$((fbytes / 1048576))M"
        elif [ "${fbytes}" -ge 1024 ]; then
          fsize="$((fbytes / 1024))K"
        else
          fsize="${fbytes}B"
        fi
        parse_artifact_name "${fname}"
        DESC="$(describe_artifact)"
        printf '| %s | %s | [%s](%s/%s) | %s | %s |\n' \
          "${PLAT}" "${ARCH}" "${fname}" "${DL_BASE}" "${fname}" "${fsize}" "${DESC}"
      done
    fi
    echo ''

    # 有 SHA256SUMS 才写校验说明，避免正文承诺一个不存在的文件
    if [ -f "${artifact_dir}/SHA256SUMS" ]; then
      # 这里一律用 printf '%s\n' 而不是 echo：
      # dash 的 echo 会解释反斜杠转义，Windows 路径的 `.\xxx` 里若出现 \b 会被吞成退格符
      # （`.\Kai-...` 显示成 `.Kai-...` 还算好，但 `.\bin\...` 这类会直接被吃掉字符），
      # printf 不做任何转义解释。反引号是 Markdown 代码标记，不是命令替换。
      # shellcheck disable=SC2016
      printf '%s\n' '## 校验'
      printf '\n'
      # shellcheck disable=SC2016
      printf '%s\n' '附件含 `SHA256SUMS`（`sha256sum` 标准格式，每行「哈希 + 两个空格 + 文件名」）。'
      # shellcheck disable=SC2016
      printf '%s\n' '把 `SHA256SUMS` 和要校验的文件下载到同一目录，然后按平台执行：'
      printf '\n'
      # shellcheck disable=SC2016
      printf '%s\n' '- **Linux**：`sha256sum -c SHA256SUMS`'
      # shellcheck disable=SC2016
      printf '%s\n' '- **macOS**：`shasum -a 256 -c SHA256SUMS`'
      if [ -n "${windows_artifacts}" ]; then
        # 逐个文件给出命令：PowerShell 一次只校验一个文件，只写一个会漏掉另一个架构
        while IFS= read -r wf; do
          [ -n "${wf}" ] || continue
          printf '%s\n' "- **Windows（PowerShell）**：\`Get-FileHash .\\${wf} -Algorithm SHA256\`"
        done <<EOF
${windows_artifacts}
EOF
        # shellcheck disable=SC2016
        printf '%s\n' '  PowerShell 输出为全大写，与 `SHA256SUMS` 里的小写哈希等价，逐位比对即可。'
        # shellcheck disable=SC2016
        printf '%s\n' '  也可用 `certutil -hashfile <文件> SHA256`。'
      fi
      printf '\n'
    fi

    echo '## 更新内容'
    echo ''
    if [ -n "${CHANGELOG:-}" ]; then
      printf '%s\n' "${CHANGELOG}"
    else
      echo '暂无更新内容。'
    fi
  } >"${out_file}"

  cat "${out_file}"
}

# 直接执行才渲染/自检；被 source 时只导出函数。
# 判定用 $0 的 basename（不能用 BASH_SOURCE，dash 会 Bad substitution）。
if [ "${0##*/}" != 'render-release-body.sh' ] && [ "${1:-}" != '--self-test' ]; then
  # shellcheck disable=SC2317
  return 0 2>/dev/null || exit 0
fi

if [ "${1:-}" = '--self-test' ]; then
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' EXIT

  mkdir -p "${tmp_dir}/bin"
  : >"${tmp_dir}/bin/Kai-0.2.8-darwin-arm64.zip"
  : >"${tmp_dir}/bin/Kai-0.2.8-darwin-amd64.zip"
  : >"${tmp_dir}/bin/Kai-0.2.8-windows-amd64-install.exe"
  : >"${tmp_dir}/bin/Kai-0.2.8-windows-arm64-install.exe"
  : >"${tmp_dir}/bin/Kai-0.2.8-linux-amd64.AppImage"
  : >"${tmp_dir}/bin/Kai-0.2.8-linux-amd64.deb"
  : >"${tmp_dir}/bin/Kai-0.2.8-linux-amd64.rpm"
  : >"${tmp_dir}/bin/Kai-0.2.8-linux-amd64.pkg.tar.zst"
  : >"${tmp_dir}/bin/updater-Kai-0.2.8-darwin-arm64.zip"
  : >"${tmp_dir}/bin/updater-Kai-0.2.8-windows-amd64.zip"
  : >"${tmp_dir}/bin/updater-Kai-0.2.8-linux-amd64.tar.gz"
  : >"${tmp_dir}/bin/BUILD_TIME"
  : >"${tmp_dir}/bin/GIT_COMMIT"
  : >"${tmp_dir}/bin/SHA256SUMS"

  failures=0
  expect() {
    # expect <文件> <期望出现的行> <说明>
    if grep -qF -- "$2" "$1"; then
      printf '  ✓ %s\n' "$3"
    else
      printf '  ✗ %s（未找到: %s）\n' "$3" "$2"
      failures=$((failures + 1))
    fi
  }
  reject() {
    if grep -qF -- "$2" "$1"; then
      printf '  ✗ %s（不应出现: %s）\n' "$3" "$2"
      failures=$((failures + 1))
    else
      printf '  ✓ %s\n' "$3"
    fi
  }

  echo '── 正式版 ──'
  VERSION='v0.2.8' PRERELEASE='false' DL_BASE='https://example.com/dl/v0.2.8' \
    COMMIT_SHA='0123456789abcdef0123456789abcdef01234567' REPO_URL='https://example.com/repo' \
    BUILD_TIME='2026-01-02 03:04:05 UTC' \
    ARTIFACT_DIR="${tmp_dir}/bin" OUT_FILE="${tmp_dir}/stable.md" CHANGELOG='- 修复 A
- 优化 B' \
    render_body >/dev/null
  expect "${tmp_dir}/stable.md" '| 版本号 | v0.2.8 |' '版本号正确'
  expect "${tmp_dir}/stable.md" '✅ 正式发布版本 (Stable)' '正式版标记'
  expect "${tmp_dir}/stable.md" '| 构建时间 | 2026-01-02 03:04:05 UTC |' '构建时间取传入值'
  expect "${tmp_dir}/stable.md" \
    '[0123456](https://example.com/repo/commit/0123456789abcdef0123456789abcdef01234567)' \
    '显示短号、链接用完整 SHA'
  expect "${tmp_dir}/stable.md" '| macOS | ARM64 | [Kai-0.2.8-darwin-arm64.zip](https://example.com/dl/v0.2.8/Kai-0.2.8-darwin-arm64.zip) |' 'macOS arm64 下载行（版本段不影响平台解析）'
  expect "${tmp_dir}/stable.md" '| macOS | x86_64 | [Kai-0.2.8-darwin-amd64.zip]' 'macOS amd64 下载行'
  expect "${tmp_dir}/stable.md" '| Windows | x86_64 | [Kai-0.2.8-windows-amd64-install.exe]' 'Windows 安装包下载行'
  expect "${tmp_dir}/stable.md" 'Windows 安装程序（exe，双击安装）' 'Windows 安装包说明'
  expect "${tmp_dir}/stable.md" '| Linux | x86_64 | [Kai-0.2.8-linux-amd64.AppImage]' 'Linux AppImage 下载行'
  expect "${tmp_dir}/stable.md" 'AppImage 便携版（chmod +x 后直接运行）' 'AppImage 说明'
  expect "${tmp_dir}/stable.md" '| Linux | x86_64 | [Kai-0.2.8-linux-amd64.deb]' 'Linux deb 下载行'
  expect "${tmp_dir}/stable.md" 'Debian/Ubuntu 安装包（dpkg -i）' 'deb 说明'
  expect "${tmp_dir}/stable.md" '| Linux | x86_64 | [Kai-0.2.8-linux-amd64.rpm]' 'Linux rpm 下载行'
  expect "${tmp_dir}/stable.md" 'Fedora/RHEL 安装包（rpm -i）' 'rpm 说明'
  expect "${tmp_dir}/stable.md" '| Linux | x86_64 | [Kai-0.2.8-linux-amd64.pkg.tar.zst]' 'Linux Arch 包下载行（多段后缀）'
  expect "${tmp_dir}/stable.md" 'Arch Linux 安装包（pacman -U）' 'Arch 包说明'
  expect "${tmp_dir}/stable.md" '## 下载' '有下载小节标题'
  expect "${tmp_dir}/stable.md" '- 修复 A' '更新内容保留'
  expect "${tmp_dir}/stable.md" '## 校验' '有 SHA256SUMS 时写校验节'
  expect "${tmp_dir}/stable.md" 'sha256sum -c SHA256SUMS' 'Linux 校验命令'
  expect "${tmp_dir}/stable.md" 'shasum -a 256 -c SHA256SUMS' 'macOS 校验命令'
  # 反斜杠必须原样出现在正文里：dash 的 echo 会把它当转义
  expect "${tmp_dir}/stable.md" 'Get-FileHash .\Kai-0.2.8-windows-amd64-install.exe -Algorithm SHA256' 'Windows 校验命令（反斜杠未被吞）'
  expect "${tmp_dir}/stable.md" 'Get-FileHash .\Kai-0.2.8-windows-arm64-install.exe -Algorithm SHA256' 'Windows ARM64 校验命令'
  expect "${tmp_dir}/stable.md" 'certutil -hashfile' 'certutil 备选命令'
  reject "${tmp_dir}/stable.md" '## ⚠️ 预发布版本' '正式版不出现预发布警示'
  reject "${tmp_dir}/stable.md" 'updater-' 'updater 升级包不进下载表格与校验命令'
  reject "${tmp_dir}/stable.md" 'BUILD_TIME' '更新器专用文件不进下载表格'
  reject "${tmp_dir}/stable.md" 'SHA256SUMS |' '校验文件不进下载表格'

  echo '── 预发布 ──'
  # CHANGELOG 显式置空：上一条用例给 render_body 传过 CHANGELOG，
  # 变量赋值会残留在当前 shell，不清掉这一条就走不到「无更新内容」兜底。
  VERSION='v0.2.8-rc.1' PRERELEASE='true' DL_BASE='https://example.com/dl/v0.2.8-rc.1' \
    STABLE_URL='https://example.com/releases/latest' \
    ARTIFACT_DIR="${tmp_dir}/bin" OUT_FILE="${tmp_dir}/pre.md" CHANGELOG='' \
    render_body >/dev/null
  expect "${tmp_dir}/pre.md" '## ⚠️ 预发布版本' '预发布警示块'
  expect "${tmp_dir}/pre.md" '🚧 预发布版本 (Pre-release)' '预发布标记'
  expect "${tmp_dir}/pre.md" '[最新正式版](https://example.com/releases/latest)' '稳定版指引链接'
  expect "${tmp_dir}/pre.md" '## 下载' '预发布也有下载小节标题'
  expect "${tmp_dir}/pre.md" '暂无更新内容。' '无 changelog 时的兜底文案'

  echo '── 空产物目录 ──'
  mkdir -p "${tmp_dir}/empty"
  VERSION='v0.2.8' PRERELEASE='false' DL_BASE='https://example.com/dl/v0.2.8' \
    ARTIFACT_DIR="${tmp_dir}/empty" OUT_FILE="${tmp_dir}/empty.md" \
    render_body >/dev/null
  expect "${tmp_dir}/empty.md" '> 本次未产出可下载的附件。' '空目录提示而不是空表格'
  reject "${tmp_dir}/empty.md" '| 平台 | 架构 |' '空目录不画表头'

  echo '── 只有内部文件 ──'
  mkdir -p "${tmp_dir}/internal"
  : >"${tmp_dir}/internal/SHA256SUMS"
  : >"${tmp_dir}/internal/BUILD_TIME"
  : >"${tmp_dir}/internal/updater-Kai-0.2.8-darwin-arm64.zip"
  VERSION='v0.2.8' PRERELEASE='false' DL_BASE='https://example.com/dl/v0.2.8' \
    ARTIFACT_DIR="${tmp_dir}/internal" OUT_FILE="${tmp_dir}/internal.md" \
    render_body >/dev/null
  expect "${tmp_dir}/internal.md" '> 本次未产出可下载的附件。' '只剩内部文件时按空产物处理'

  if [ "${failures}" -ne 0 ]; then
    printf '\n自检失败: %d 项\n' "${failures}" >&2
    exit 1
  fi
  printf '\n自检通过\n'
  exit 0
fi

render_body
