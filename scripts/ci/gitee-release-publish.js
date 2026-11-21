#!/usr/bin/env node
'use strict';

// 按 gitee-release-download.js 写出的清单，把 Release 建好并逐个上传附件。
//
// 为什么不用 release@gitee 插件：
//   1. 插件的 assertFiles 是静态清单，做不到「某个文件超 100MB 就跳过它」；
//   2. 插件 400 时响应体为空，看不到 Gitee 到底说了什么；
//   3. 插件把正文当 KEY=VALUE 参数（不能换行、不能有裸 =、整个值当相对路径 open）。
//
// 环境变量（**不能以 GITEE_ 开头**：Gitee 流水线自己占用了这个前缀，
// 全局参数用 GITEE_xxx 会被平台拒绝或覆盖，所以统一走 KAI_ 前缀）：
//   KAI_GITEE_TOKEN  必填，个人令牌（repo 读写）
//   MANIFEST         默认 gitee-release.json
//   KAI_GITEE_API    默认 https://gitee.com/api/v5/repos/dtapps/kai
//
// 命令行：
//   无参数              正常发布：读清单 → 建 Release → 上传附件
//                       （nightly 清理由 .workflow/release.yml 调 delete-release.sh --keep 完成）
//   --check-exists [tag] 只查该 tag 是否已在 Gitee 发布，stdout 输出 exists / absent，
//                       日志走 stderr；tag 省略时取 RELEASE_VERSION。
//                       workflow 在下载前调用它，让 GitHub / CNB 两条触发链路先到先得。

const fs = require('fs');

const MANIFEST = process.env.MANIFEST || 'gitee-release.json';
const API = process.env.KAI_GITEE_API || 'https://gitee.com/api/v5/repos/dtapps/kai';
const TOKEN = (process.env.KAI_GITEE_TOKEN || '').trim();

function fail(msg) {
  console.error('错误：' + msg);
  process.exit(1);
}
function mb(n) { return (n / 1048576).toFixed(1) + ' MB'; }

// 只打印路径，不打印带 access_token 的完整 URL。
// quiet=true 时日志走 stderr：--check-exists 用 $(...) 捕获 stdout 判断 exists/absent，
// stdout 必须只有那一个词，诊断信息不能混进去。
async function gt(method, path, options, quiet) {
  const url = API + path + (path.indexOf('?') === -1 ? '?' : '&') +
    'access_token=' + encodeURIComponent(TOKEN);
  const res = await fetch(url, Object.assign({ method }, options || {}));
  const text = await res.text();
  const line = '  ' + method + ' ' + path + ' → HTTP ' + res.status +
    (text ? ' ' + text.slice(0, 300) : '');
  if (quiet) console.error(line); else console.log(line);
  return { status: res.status, text };
}

// 查询 tag 对应的 Release。Gitee 的 GET /releases/tags/{tag} 在「没有这个 Release」时返回
// **200 + body `null`**（不是 404），所以只看状态码会误判成已存在；必须解析 body 取 id。
// 返回 null 表示不存在；查询失败直接 fail（调用方无法可靠的判断存在性时不能假装不存在）。
async function findExisting(tag, quiet) {
  const res = await gt('GET', '/releases/tags/' + encodeURIComponent(tag), undefined, quiet);
  if (res.status !== 200 && res.status !== 404) {
    fail('查询 Release 失败（HTTP ' + res.status + '）：' + res.text.slice(0, 300));
  }
  if (res.status === 200) {
    try {
      const obj = JSON.parse(res.text);
      return obj && obj.id ? obj : null;
    } catch (e) {
      return null;
    }
  }
  return null;
}

// --check-exists：workflow 下载前调用，判断本版本是否已在 Gitee 发布（另一条触发链路先到先得）。
// stdout 只输出一个词 exists / absent，日志全部走 stderr，供 shell 用 $(...) 捕获。
async function checkExists(tag) {
  if (!tag) {
    console.error('错误：--check-exists 需要版本号（RELEASE_VERSION）');
    process.exit(2);
  }
  const old = await findExisting(tag, true);
  console.error(old
    ? '已存在 Release：' + old.id + '（' + (old.name || tag) + '）'
    : '尚无 Release：' + tag);
  process.stdout.write(old ? 'exists' : 'absent');
}

async function main() {
  if (!TOKEN) {
    fail('未配置 KAI_GITEE_TOKEN（Gitee 个人令牌，repo 读写）');
  }
  if (!fs.existsSync(MANIFEST)) {
    fail('找不到清单 ' + MANIFEST + '：请先跑 gitee-release-download.js');
  }
  const m = JSON.parse(fs.readFileSync(MANIFEST, 'utf8'));
  if (!m.files || !m.files.length) fail('清单里没有文件');

  console.log('发布 ' + m.tag + ' → ' + m.releaseName);
  console.log('待上传 ' + m.files.length + ' 个（' + mb(m.totalBytes || 0) + '）');

  // ── 同名先删，保证幂等重跑 ──
  // 正常流程在 workflow 的 --check-exists 已经跳过「已发布」的情况；这里再删一次是兜底
  // （手动补发、或检查之后又有同名 Release），保证创建不会被 400 卡住。
  const old = await findExisting(m.tag);
  if (old) {
    console.log('删除同名旧 Release：' + old.id + '（' + (old.name || m.tag) + '）');
    const del = await gt('DELETE', '/releases/' + old.id);
    // 删不掉就别往下走：同名 Release 还在，创建会报 400，错误现场会被挪远
    if (del.status !== 204 && del.status !== 200) {
      fail('删除旧 Release 失败（HTTP ' + del.status + '）：' + del.text.slice(0, 300));
    }
  } else {
    console.log('无同名 Release，直接创建');
  }

  // ── 建 Release ──
  // target_commitish 用清单里的值：有 commit 就绑到它（顺手做的一次绑定），
  // 没有就用 GitHub Release 自带的值（通常是分支名）。
  const FALLBACK_BRANCH = 'master';
  const targetCommitish = m.targetCommitish || FALLBACK_BRANCH;
  const form = new URLSearchParams();
  form.set('access_token', TOKEN);
  form.set('tag_name', m.tag);
  form.set('name', m.releaseName);
  form.set('body', m.bodyHtml);
  form.set('target_commitish', targetCommitish);
  form.set('prerelease', m.prerelease ? 'true' : 'false');
  if (m.commit) {
    console.log('本次发布的 commit: ' + m.commit +
      (m.commitMatched === true ? '（产物 GIT_COMMIT 校验一致）' :
        m.commitMatched === false ? '（产物 GIT_COMMIT 校验不一致）' : '（无产物可比对）'));
  }
  let created = await gt('POST', '/releases', { body: form });
  // 绑定失败（多半是镜像仓库还没同步到这个提交）→ 回退到默认分支重试一次，
  // 不让整条发布变红：附件与正文才是这次发布的价值所在。
  if (created.status !== 200 && created.status !== 201 && targetCommitish !== FALLBACK_BRANCH) {
    console.log('⚠ 用 commit ' + targetCommitish + ' 创建 Release 失败（HTTP ' + created.status +
      '），回退到 ' + FALLBACK_BRANCH + ' 重试');
    form.set('target_commitish', FALLBACK_BRANCH);
    created = await gt('POST', '/releases', { body: form });
  }
  if (created.status !== 200 && created.status !== 201) {
    fail('创建 Release 失败（HTTP ' + created.status + '）：' + created.text.slice(0, 300));
  }
  // 同理：返回体可能是 null 或非 JSON，别让 JSON.parse 直接抛栈
  let release = null;
  try { release = JSON.parse(created.text); } catch (e) { release = null; }
  if (!release || !release.id) {
    fail('创建 Release 的响应里没有 id（HTTP ' + created.status + '）：' + created.text.slice(0, 300));
  }
  console.log('Release 已创建：id=' + release.id + ' ' + (release.html_url || ''));

  // ── 逐个上传：单个失败只记录，不中断其余文件 ──
  // Gitee 仓库附件总量 1GB（含历史发行版）。配额不足时返回
  //   {"message":"验证失败：文件大小已超出仓库附件配额：1 GB"}
  // 这属于「仓库容量不够，用户去 GitHub 下载即可」，不算发布失败，也不让流水线变红。
  // 其它 4xx / 网络异常仍算失败，避免把真实故障也吞掉。
  const QUOTA_RE = /附件配额|超出.*配额|quota/i;
  const ok = [];
  const quotaSkipped = [];
  const failed = [];
  // 配额是「仓库累计」，只会越用越满：某个 N 字节的文件被拒后，>=N 的必然也被拒。
  // 不再重复 POST 几十 MB 换取同一个 400，但比它小的文件仍要试（SHA256SUMS 往往能过）。
  let quotaMinSize = Infinity;
  for (const f of m.files) {
    if ((f.size || 0) >= quotaMinSize) {
      console.log('  - 已知仓库附件配额不足，跳过 ' + f.name + '（不再重复上传，不算失败）');
      quotaSkipped.push(f.name);
      continue;
    }
    console.log('上传 ' + f.name + '（' + mb(f.size || 0) + '）');
    try {
      const buf = await fs.promises.readFile(f.path);
      const fd = new FormData();
      fd.set('access_token', TOKEN);
      fd.set('file', new Blob([buf]), f.name);
      const res = await fetch(API + '/releases/' + release.id + '/attach_files', {
        method: 'POST',
        body: fd
      });
      const text = await res.text();
      if (res.status === 200 || res.status === 201) {
        console.log('  ✓ HTTP ' + res.status + ' ' + f.name);
        ok.push(f.name);
      } else if (QUOTA_RE.test(text)) {
        console.log('  - 仓库附件配额不足，跳过 ' + f.name + '（不算失败）');
        quotaMinSize = Math.min(quotaMinSize, f.size || 0);
        quotaSkipped.push(f.name);
      } else {
        console.log('  ✗ HTTP ' + res.status + ' ' + f.name + ' ' + text.slice(0, 300));
        failed.push(f.name + '（HTTP ' + res.status + ' ' + text.slice(0, 120) + '）');
      }
    } catch (err) {
      const msg = err && err.message ? err.message : String(err);
      console.log('  ✗ 异常 ' + f.name + '：' + msg);
      failed.push(f.name + '（' + msg + '）');
    }
  }

  console.log('---- 汇总 ----');
  console.log('成功 ' + ok.length + ' 个' + (ok.length ? '：' + ok.join(', ') : ''));
  if (quotaSkipped.length) {
    console.log('⚠ 仓库附件配额不足，跳过 ' + quotaSkipped.length + ' 个（不算失败）：');
    quotaSkipped.forEach((x) => console.log('  - ' + x));
    console.log('  这些附件请到 GitHub Release 下载；清理 Gitee 旧 Release 的附件后可重跑补齐。');
  }
  if (failed.length) {
    console.log('失败 ' + failed.length + ' 个：');
    failed.forEach((x) => console.log('  - ' + x));
    // 回滚删除刚建的 Release：GitHub 与 CNB 两条链路先到先得，若这边建了 Release
    // 却没传齐附件就退出，另一条链路会在下载前 --check-exists 看到「已存在」而跳过，
    // 最终给用户留下一个附件不全的 Release。删掉它，让另一条链路有机会完整重跑。
    // 注意配额不足（quotaSkipped）不算失败、不回滚——那种情况 Release 本身是有效的。
    console.log('上传失败，回滚删除刚建的 Release ' + release.id + '，让另一条触发链路能重跑');
    const rollback = await gt('DELETE', '/releases/' + release.id);
    if (rollback.status !== 200 && rollback.status !== 204) {
      console.log('⚠ 回滚删除失败（HTTP ' + rollback.status + '）：请手动删除 Release ' + release.id);
    }
    fail('有 ' + failed.length + ' 个附件上传失败，见上面的 HTTP 状态与响应体');
  }
  console.log('发布完成：' + (release.html_url || m.tag));
}

if (process.argv[2] === '--check-exists') {
  // 版本号从参数或 RELEASE_VERSION 取。token 缺失时 checkExists 里的查询会拿到
  // 401/403 → fail（非 0），workflow 侧的 $(... || true) 会得到空串并按「不存在」继续，
  // 随后 main 里的 token 校验会给出明确错误——不会静默跳过发布。
  checkExists(process.argv[3] || process.env.RELEASE_VERSION || '')
    .catch((err) => fail(err && err.message ? err.message : String(err)));
} else {
  main().catch((err) => fail(err && err.message ? err.message : String(err)));
}
