#!/usr/bin/env node
'use strict';

// 从 CNB 或 GitHub Release 下载产物到 assets/，并写出发布清单 gitee-release.json。
//
// DOWNLOAD_SOURCE（默认 cnb）决定**同一次发布的全部附件只来自一个平台**，绝不混用：
// 打包产物里注入了 RepoSource，客户端据此选升级通道；一半 CNB、一半 GitHub 等于随机分配。
//   - cnb    ：CNB Release 上的附件（各平台自己编译，同版本字节数与 GitHub 不同），
//              附件名按版本在本地拼出，大小用 Range 探测，正文本地生成，下载链路不碰 GitHub API
//              （CNB API 匿名 401，拿不到资产清单，所以不能走 API）。
//   - github ：GitHub Release 元数据（清单 / 大小 / 正文 / browser_download_url），
//              下载依次走 daocloud → ghfast.top → gh-proxy.com → 直连 四级回退。
//
// 唯一的跳过规则：**单文件 > 100MB**（Gitee 单附件上限），这类文件在境内链路要传
// 几分钟，只是为了让服务端拒收，所以连下载都不做。
// 其它产物一律照常下载并上传 —— updater-*、BUILD_TIME、GIT_COMMIT、SHA256SUMS、
// 以及某次体积没超 100MB 的 AppImage 都不特殊处理。
//
// 环境变量：
//   DOWNLOAD_SOURCE        cnb（默认）/ github；其它值直接报错（shell 层也会先归一化）
//   CNB_REPO               CNB 仓库路径，默认 dtapp/kai
//   RELEASE_VERSION        可选，v1.2.3 / 1.2.3 / nightly；留空取最新 Release
//   PRERELEASE             可选，shell 用 resolve-prerelease.sh 算出的结果；
//                          cnb 源只作兜底，判定以版本号后缀为准
//   RELEASE_COMMIT         可选，本次发布的 commit SHA（上游 GitHub Actions 随 version 一起传）。
//                          用途是**核对打包产物**：产物里（预发布版的 GIT_COMMIT 附件、
//                          二进制里的 buildinfo.GitCommit）带着打包时的 commit，
//                          下载后比对二者是否一致；不参与 tag / Release 的指向决策。
//   KAI_GITHUB_TOKEN       可选，避开 api.github.com 匿名限流（与 .cnb.yml 同一个只读令牌）
//   OUT_DIR                默认 assets
//   MANIFEST               默认 gitee-release.json
//   MAX_FILE_BYTES         默认 100MB（Gitee 单附件上限）
//   MAX_TOTAL_BYTES        默认 800MB（仓库 1GB 留余量）
//
// 命令行：
//   --resolve-tag    只把最新发布的 tag 打到 stdout（日志全走 stderr），供 shell 在
//                    VERSION 为空时填充。cnb 源从 CNB 公开 Release 列表页解析，
//                    github 源取 GitHub Release；cnb 解析不到才回退 GitHub API。
//   --self-test      跑纯函数自检（不联网）。

const fs = require('fs');
const http = require('http');
const https = require('https');

function fail(msg) {
  console.error('错误：' + msg);
  process.exit(1);
}

function mb(n) { return (n / 1048576).toFixed(1) + ' MB'; }
function rate(bps) {
  if (bps >= 1048576) return mb(bps) + '/s';
  if (bps >= 1024) return (bps / 1024).toFixed(0) + ' KB/s';
  return bps.toFixed(0) + ' B/s';
}
function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }

const RAW_SOURCE = (process.env.DOWNLOAD_SOURCE || '').trim();
const DOWNLOAD_SOURCE = RAW_SOURCE || 'cnb';
if (DOWNLOAD_SOURCE !== 'cnb' && DOWNLOAD_SOURCE !== 'github') {
  fail('DOWNLOAD_SOURCE 只能是 cnb 或 github（当前值：' + (RAW_SOURCE || '<空>') + '）');
}

const OUT_DIR = process.env.OUT_DIR || 'assets';
const MANIFEST = process.env.MANIFEST || 'gitee-release.json';
const GH_API = process.env.GH_API || 'https://api.github.com/repos/dtapps/kai';
const GH_TOKEN = (process.env.KAI_GITHUB_TOKEN || '').trim();
const CNB_REPO = (process.env.CNB_REPO || 'dtapp/kai').trim();
const CNB_BASE = 'https://cnb.cool/' + CNB_REPO;
const MAX_FILE = Number(process.env.MAX_FILE_BYTES || 100 * 1024 * 1024);
const MAX_TOTAL = Number(process.env.MAX_TOTAL_BYTES || 800 * 1024 * 1024);
const RELEASE_COMMIT = (process.env.RELEASE_COMMIT || '').trim();
const PRERELEASE_ENV = (process.env.PRERELEASE || '').trim().toLowerCase();

// github 源：境内直连 GitHub 资产只有 10~30 KB/s 且常被掐断，所以先走代理镜像，直连只做兜底。
const GH_MIRRORS = [
  { name: 'daocloud', wrap: (u) => 'https://files.m.daocloud.io/' + u.replace(/^https?:\/\//, '') },
  { name: 'ghfast', wrap: (u) => 'https://ghfast.top/' + u },
  { name: 'gh-proxy', wrap: (u) => 'https://gh-proxy.com/' + u },
  { name: '直连', wrap: (u) => u }
];
// cnb 源：CNB Release 附件直链可匿名下载，没有镜像这一层，绕开 GitHub 反代。
const CNB_MIRRORS = [{ name: 'CNB', wrap: (u) => u }];
const mirrors = DOWNLOAD_SOURCE === 'cnb' ? CNB_MIRRORS : GH_MIRRORS;

// ── 版本号 / 资产名 ─────────────────────────────────────────────────────────

// 只认版本号后缀，与 scripts/ci/resolve-prerelease.sh 同口径：
// -rc / -beta / -alpha / -nightly（大小写不敏感）算预发布，-hotfix / -core 之类不算。
function preByVersion(tag) {
  return /-(rc|beta|alpha|nightly)([.-]?[0-9A-Za-z.-]*)?$/i.test(String(tag || ''));
}

// cnb 源固定附件名：CNB API 匿名 401 拿不到资产清单，只能按版本在本地拼。
// 与 .github/workflows/release.yml、render-release-body.sh 的命名逐字一致。
function cnbAssetNames(ver, pre) {
  const names = [];
  for (const arch of ['amd64', 'arm64']) {
    names.push('Kai-' + ver + '-darwin-' + arch + '.zip');
    names.push('updater-Kai-' + ver + '-darwin-' + arch + '.zip');
    names.push('Kai-' + ver + '-windows-' + arch + '-install.exe');
    names.push('updater-Kai-' + ver + '-windows-' + arch + '.zip');
    for (const ext of ['AppImage', 'deb', 'rpm', 'pkg.tar.zst']) {
      names.push('Kai-' + ver + '-linux-' + arch + '.' + ext);
    }
    names.push('updater-Kai-' + ver + '-linux-' + arch + '.tar.gz');
  }
  names.push('SHA256SUMS');
  if (pre) names.push('BUILD_TIME', 'GIT_COMMIT');
  return names;
}

function cnbAssetUrl(tag, name) {
  return CNB_BASE + '/-/releases/download/' + encodeURIComponent(tag) + '/' + encodeURIComponent(name);
}

// ── HTTP ────────────────────────────────────────────────────────────────────

// 断点续传要带 Range 头，重定向到签名地址时也得一路带着，否则续传变成从头下
function request(url, redirects, withAuth, rangeFrom) {
  return new Promise((resolve, reject) => {
    const lib = url.indexOf('https:') === 0 ? https : http;
    const headers = { 'User-Agent': 'kai-release-sync' };
    if (GH_TOKEN && withAuth) headers.Authorization = 'token ' + GH_TOKEN;
    if (rangeFrom > 0) headers.Range = 'bytes=' + rangeFrom + '-';
    const req = lib.get(url, { headers }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        if (!redirects) return reject(new Error('重定向次数过多: ' + url));
        return resolve(request(res.headers.location, redirects - 1, withAuth, rangeFrom));
      }
      if (res.statusCode !== 200 && res.statusCode !== 206) {
        let body = '';
        res.on('data', (c) => { body += c; });
        res.on('end', () => {
          reject(new Error('HTTP ' + res.statusCode + ' ' + url + '\n' + body.slice(0, 300)));
        });
        return;
      }
      resolve(res);
    });
    req.on('error', reject);
    req.setTimeout(60000, () => req.destroy(new Error('请求超时: ' + url)));
  });
}

function readText(url, withAuth) {
  return request(url, 5, withAuth !== false).then((res) => new Promise((resolve, reject) => {
    let body = '';
    res.setEncoding('utf8');
    res.on('data', (c) => { body += c; });
    res.on('end', () => resolve(body));
    res.on('error', reject);
  }));
}

// Range: bytes=1- 探测真实总长。各平台自己编译，同版本字节数不同，不能复用 GitHub 的 asset.size。
// 服务器忽略 Range 返回 200 时退回 Content-Length。
function probeSize(url) {
  return request(url, 5, false, 1).then((res) => {
    const cr = res.headers['content-range'];
    let size = 0;
    if (cr) {
      const m = /\/(\d+)\s*$/.exec(cr);
      if (m) size = Number(m[1]);
    }
    if (!size && res.headers['content-length']) {
      size = Number(res.headers['content-length']) + (res.statusCode === 206 ? 1 : 0);
    }
    res.resume();
    if (!Number.isFinite(size) || size <= 0) throw new Error('无法从 Content-Range 读出文件大小: ' + url);
    return size;
  });
}

// 落盘后验文件头，防反代 200 回 HTML 错误页（继续传会在下次续传里永远续的是 HTML）。
function verifyHeader(dest, name) {
  let fd;
  try {
    fd = fs.openSync(dest, 'r');
    const ext = name.slice(name.lastIndexOf('.') + 1).toLowerCase();
    if (ext !== 'deb' && ext !== 'fpk') {
      const buf = Buffer.alloc(16);
      const n = fs.readSync(fd, buf, 0, buf.length, 0);
      const head = buf.subarray(0, n).toString('utf8').trim().toLowerCase();
      if (/^<!doctype|^<html/.test(head)) {
        const err = new Error('文件头是 HTML 错误页');
        err.badHeader = true;
        throw err;
      }
      return;
    }
    const want = ext === 'deb' ? Buffer.from('!<arch>\n', 'latin1') : Buffer.from([0x1f, 0x8b]);
    const buf = Buffer.alloc(want.length);
    const n = fs.readSync(fd, buf, 0, buf.length, 0);
    if (n !== want.length || !buf.equals(want)) {
      const err = new Error('文件头校验失败（.' + ext + ' 应为 ' +
        (ext === 'deb' ? '!<arch>\\n' : '1f 8b') + '），可能被反代换成错误页');
      err.badHeader = true;
      throw err;
    }
  } finally {
    if (fd !== undefined) fs.closeSync(fd);
  }
}

function downloadOnce(url, dest, from) {
  return request(url, 5, false, from).then((res) => new Promise((resolve, reject) => {
    // 只有服务端确认 206 才续写，否则截断重来
    const append = from > 0 && res.statusCode === 206;
    const out = fs.createWriteStream(dest, { flags: append ? 'a' : 'w' });
    let written = append ? from : 0;
    let lastAt = Date.now();
    let lastBytes = written;
    let done = false;
    let lowStreak = 0;

    function abort(err) {
      if (done) return;
      done = true;
      out.destroy();
      res.destroy();
      reject(err);
    }

    res.on('data', (chunk) => {
      written += chunk.length;
      const now = Date.now();
      if (now - lastAt >= 5000) {
        const bps = (written - lastBytes) / ((now - lastAt) / 1000);
        console.log('    ' + mb(written) + '  ' + rate(bps));
        // 15 秒持续低于 30 KB/s 就换源：慢而不断的连接比断掉更耗时间
        lowStreak = bps < 30 * 1024 ? lowStreak + 1 : 0;
        if (lowStreak >= 3) {
          const err = new Error('速度过低（' + rate(bps) + '）');
          err.slow = true;
          abort(err);
          return;
        }
        lastAt = now;
        lastBytes = written;
      }
    });
    res.on('error', abort);
    res.on('aborted', () => abort(new Error('连接被中断')));
    out.on('error', abort);
    out.on('finish', () => {
      if (done) return;
      done = true;
      resolve(written);
    });
    res.pipe(out);
  }));
}

// 单个文件：最多切 4 个源（github）/ 累计重试 8 次，每次从已落盘的字节续传
function downloadFile(url, dest, expectedSize) {
  let mirror = 0;
  let tries = 0;
  function step() {
    const from = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
    if (expectedSize > 0 && from === expectedSize) {
      // 上次落盘的文件也要验头：同尺寸的 HTML 错误页不能因为「已完整」被放过
      return Promise.resolve().then(() => verifyHeader(dest, dest.split('/').pop())).then(() => {
        console.log('  已完整（' + mb(from) + '）');
      });
    }
    if (expectedSize > 0 && from > expectedSize) fs.unlinkSync(dest); // 上次写了脏数据
    return downloadOnce(mirrors[mirror].wrap(url), dest, from).then((written) => {
      if (expectedSize > 0 && written !== expectedSize) {
        throw new Error('大小不符：' + written + ' != ' + expectedSize);
      }
      verifyHeader(dest, dest.split('/').pop());
      console.log('  完成 ' + mb(written) + '（' + mirrors[mirror].name + '）');
    }).catch((err) => {
      tries++;
      // 文件头不对/脏数据必须整文件重下，否则续传永远在 HTML 后面追加
      if (err && (err.badHeader || /大小不符/.test(err.message || ''))) {
        try { fs.unlinkSync(dest); } catch (e) { /* 文件本来就不存在 */ }
      }
      const now = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
      console.log('  中断：' + (err && err.message ? err.message : err) +
        '，已下 ' + mb(now) + '，第 ' + tries + ' 次');
      if (tries >= 8) throw new Error('下载失败（重试 ' + tries + ' 次）：' + url);
      if ((err && err.slow) || tries % 2 === 0) {
        if (mirror < mirrors.length - 1) {
          mirror++;
          console.log('  切换下载源 → ' + mirrors[mirror].name);
        }
      }
      return sleep(1500).then(step);
    });
  }
  return step();
}

// ── 正文 ────────────────────────────────────────────────────────────────────

// 把某一整节（"## 标题" 到下一个 "## "）的内容换成 replacement，标题保留
function replaceSection(text, title, replacement) {
  const out = [];
  let inside = false;
  for (const line of text.split('\n')) {
    if (/^##\s+/.test(line)) {
      inside = line.trim() === '## ' + title;
      out.push(line);
      if (inside) { out.push(''); out.push(replacement); }
      continue;
    }
    if (!inside) out.push(line);
  }
  return out.join('\n');
}

// github markdown → Gitee 正文 HTML（github 源专用，marked 由流水线按需安装）。
// 保留：预发布提示 / 发布信息 / 下载 / 校验 / 更新内容（标题）；
// 「更新内容」的明细换成一句指引 —— 完整清单在 GitHub，超限附件也只在 GitHub 提供。
function renderBody(md) {
  const { marked } = require('marked');
  let html = marked.parse(replaceSection(md, '更新内容',
    '> 更新内容与超过 100MB 的附件（如 AppImage）请前往 GitHub Release 查看。'));
  // 只留链接文字：Gitee 页面自带附件列表，正文里的 GitHub 地址点了也是跳走
  html = html.replace(
    /<a\b[^>]*\bhref="([^"]*)"[^>]*>([\s\S]*?)<\/a>/g,
    (m, url, text) => text.replace(/<[^>]+>/g, '').trim()
  ).replace(/<a\b[^>]*>\s*<\/a>/g, '');
  console.log('正文：' + Buffer.byteLength(html) + ' 字节 HTML（更新内容已改为指引）');
  return html;
}

// cnb 源正文：CNB API 匿名 401 拿不到 Release body，直接在本地生成，全程不碰 GitHub。
function renderCnbBody(tag, ver, pre, assets) {
  const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const out = [];
  if (pre) {
    out.push('<h2>⚠️ 预发布版本</h2>');
    out.push('<p>这是预发布版本，可能包含未充分测试的功能，不建议用于生产环境。</p>');
  }
  out.push('<h2>发布信息</h2>');
  out.push('<table><tr><th>项目</th><th>内容</th></tr>');
  out.push('<tr><td>版本号</td><td>' + esc(tag) + '</td></tr>');
  out.push('<tr><td>发布类型</td><td>' + (pre ? '预发布版本' : '正式版') + '</td></tr>');
  out.push('<tr><td>来源</td><td><a href="' + esc(CNB_BASE + '/-/releases/tag/' + encodeURIComponent(tag)) +
    '">CNB Release</a>（安装包与更新通道一一对应，请勿混用其它平台的产物）</td></tr>');
  out.push('</table>');
  out.push('<h2>下载</h2>');
  // 与 render-release-body.sh 的 is_artifact 同口径：内部文件不进下载表
  const shown = assets.filter((a) => !/^(updater-|SHA256SUMS$|BUILD_TIME$|GIT_COMMIT$)/.test(a.name));
  if (!shown.length) {
    out.push('<p>本次未产出可下载的附件。</p>');
  } else {
    out.push('<table><tr><th>文件</th><th>大小</th></tr>');
    for (const a of shown) {
      out.push('<tr><td><a href="' + esc(cnbAssetUrl(tag, a.name)) + '">' + esc(a.name) +
        '</a></td><td>' + mb(a.size) + '</td></tr>');
    }
    out.push('</table>');
  }
  out.push('<h2>校验</h2>');
  out.push('<p>附件含 <code>SHA256SUMS</code>（sha256sum 标准格式）。下载到同一目录后，' +
    'Linux 执行 <code>sha256sum -c SHA256SUMS</code>，macOS 执行 <code>shasum -a 256 -c SHA256SUMS</code>。</p>');
  const html = out.join('\n');
  console.log('正文：本地生成 ' + Buffer.byteLength(html) + ' 字节 HTML（cnb 源，未访问 GitHub API）');
  return html;
}

// 可选：Gitee release 插件把正文当 KEY=VALUE 参数、整值当相对路径 open，
// 换行 / 裸 = / 单段超 255 字节都会崩。压成单行、裸 = 转 &#61;、每 200 字节插 <br/>。
function pluginSafeBody(html) {
  const escaped = String(html).replace(/=/g, '&#61;');
  const out = [];
  let seg = '', bytes = 0;
  for (const ch of escaped.replace(/\r?\n/g, '')) {
    const n = Buffer.byteLength(ch);
    if (bytes + n > 200 && seg) {
      out.push(seg);
      seg = '';
      bytes = 0;
    }
    seg += ch;
    bytes += n;
  }
  if (seg) out.push(seg);
  return out.join('<br/>');
}

// ── 入口 ────────────────────────────────────────────────────────────────────

// VERSION 为空时由 shell 调用：只输出 tag，日志走 stderr，便于命令替换直接取值。
async function resolveTag() {
  // cnb 源从 CNB 公开 Release 列表页解析，全程不碰 GitHub API（CNB API 匿名 401，
  // 但列表 HTML 是公开的）。列表按发布时间倒序，第一条 releases/tag/... 即最新 tag。
  if (DOWNLOAD_SOURCE === 'cnb') {
    try {
      const html = await readText(CNB_BASE + '/-/releases', false);
      const m = /releases\/tag\/([^"'<>\s]+)/.exec(html);
      if (m) {
        const tag = decodeURIComponent(m[1]);
        process.stderr.write('从 CNB Release 列表解析到最新 tag: ' + tag + '\n');
        process.stdout.write(tag + '\n');
        return;
      }
      process.stderr.write('CNB Release 列表里没找到 tag，回退 GitHub API\n');
    } catch (err) {
      process.stderr.write('CNB Release 列表读取失败（' +
        (err && err.message ? err.message : String(err)).split('\n')[0] + '），回退 GitHub API\n');
    }
  }
  const rel = JSON.parse(await readText(GH_API + '/releases/latest'));
  const tag = String(rel.tag_name || '').trim();
  if (!tag) fail('最新 Release 没有 tag_name，无法判定版本');
  process.stderr.write('自动判定版本：' + tag + '\n');
  process.stdout.write(tag + '\n');
}

async function main() {
  // 容器启动时工作目录可能已被删除（日志里的 shell-init getcwd 报错），先兜一下
  try { process.cwd(); } catch (e) { process.chdir('/'); }

  if (process.argv[2] === '--resolve-tag') return resolveTag();
  if (process.argv[2] === '--self-test') return selfTest();

  if (!fs.existsSync(OUT_DIR)) fs.mkdirSync(OUT_DIR, { recursive: true });

  const raw = (process.env.RELEASE_VERSION || '').trim();
  const rolling = /^nightly$/i.test(raw);
  let tag = raw ? (rolling ? 'nightly' : 'v' + raw.replace(/^v/i, '')) : '';

  console.log('下载源：' + DOWNLOAD_SOURCE +
    (DOWNLOAD_SOURCE === 'cnb' ? '（' + CNB_BASE + '，全程不碰 GitHub API）' : '（GitHub Release）'));
  console.log('版本号: ' + (tag || '未指定，取最新 Release'));

  let ver = '';
  let assets = [];
  let skipped = [];
  let totalBytes = 0;
  let bodyHtml = '';
  let pre = false;
  let targetCommitish = RELEASE_COMMIT || 'master';

  if (DOWNLOAD_SOURCE === 'cnb') {
    if (!tag) fail('cnb 源需要明确的版本号：shell 应先用 --resolve-tag 取出最新 tag');
    ver = tag.replace(/^v/i, '');
    pre = preByVersion(tag) || PRERELEASE_ENV === 'true';
    const preByTag = preByVersion(tag);
    console.log('预发布：' + pre + '（tag 后缀=' + preByTag + '，shell PRERELEASE=' + PRERELEASE_ENV + '）');

    // 固定文件名 + Range 探测真实大小（不能用 GitHub 的 asset.size）。
    // 探测顺带当存在性检查：404 的说明该平台没有这个附件（如正式版没有 GIT_COMMIT），跳过。
    for (const name of cnbAssetNames(ver, pre)) {
      const url = cnbAssetUrl(tag, name);
      let size;
      try {
        size = await probeSize(url);
      } catch (err) {
        console.log('  跳过 ' + name + '（探测失败，按未发布处理）：' +
          (err && err.message ? err.message : String(err)).split('\n')[0]);
        continue;
      }
      if (size > MAX_FILE) {
        skipped.push(name + '（' + mb(size) + ' 超过 Gitee 单附件 100MB，跳过该文件）');
        continue;
      }
      assets.push({ name, size, url });
    }
    if (!assets.length) fail('cnb 源没有探测到可上传 Gitee 的附件（tag=' + tag + '）');
    totalBytes = assets.reduce((s, a) => s + a.size, 0);
    bodyHtml = renderCnbBody(tag, ver, pre, assets);
  } else {
    // 指定了版本就按 tag 精确取（补发历史版本）；没指定就取最新。
    const relUrl = tag
      ? GH_API + '/releases/tags/' + encodeURIComponent(tag)
      : GH_API + '/releases/latest';
    const rel = JSON.parse(await readText(relUrl));
    if (!tag) {
      tag = String(rel.tag_name || '').trim();
      if (!tag) fail('GitHub 最新 Release 没有 tag_name，无法判定版本');
      console.log('自动判定版本：' + tag);
    }
    ver = tag.replace(/^v/i, '');

    // ── 附件过滤：唯一规则是单文件体积，超 100MB 只跳过它自己 ──
    const all = rel.assets || [];
    if (!all.length) fail('GitHub Release ' + tag + ' 没有附件');
    assets = all.filter((a) => {
      if ((a.size || 0) > MAX_FILE) {
        skipped.push(a.name + '（' + mb(a.size || 0) + ' 超过 Gitee 单附件 100MB，跳过该文件）');
        return false;
      }
      return true;
    }).map((a) => ({ name: a.name, size: a.size || 0, url: a.browser_download_url }));
    if (!assets.length) fail('过滤后没有可上传 Gitee 的附件');
    totalBytes = assets.reduce((s, a) => s + a.size, 0);

    const body = String(rel.body || '')
      .replace(/\r\n/g, '\n')
      .split('https://github.com/dtapps/kai').join('https://gitee.com/dtapps/kai')
      .trim();
    bodyHtml = renderBody(body || ('Kai ' + tag));
    const preByTag = preByVersion(tag) || rolling;
    const preByBody = /(预发布|测试版|抢先版|pre-?release|release candidate)/i.test(body);
    pre = rel.prerelease === true || preByTag || preByBody;
    console.log('预发布：' + pre + '（GitHub prerelease=' + rel.prerelease +
      '，tag 后缀=' + preByTag + '，正文关键词=' + preByBody + '）');
    targetCommitish = RELEASE_COMMIT || String(rel.target_commitish || 'master');
  }

  console.log('共探测到 ' + (assets.length + skipped.length) + ' 个附件，需要下载并上传 ' +
    assets.length + ' 个（' + mb(totalBytes) + '）');
  skipped.forEach((s) => console.log('  跳过 ' + s));
  if (totalBytes > MAX_TOTAL) {
    fail('待上传 ' + mb(totalBytes) + ' 超过安全阈值 ' + mb(MAX_TOTAL) +
      '（Gitee 仓库附件总量 1GB，含历史发行版）。请先清理旧 Release 的附件');
  }

  // ── 下载（3 路并发）──
  const CONCURRENCY = 3;
  let seq = 0;
  const total = assets.length;
  const queue = assets.slice();
  function worker() {
    if (!queue.length) return Promise.resolve();
    const asset = queue.shift();
    seq++;
    console.log('[' + seq + '/' + total + '] ' + asset.name + '（' + mb(asset.size || 0) + '）');
    return downloadFile(asset.url, OUT_DIR + '/' + asset.name, asset.size || 0).then(worker);
  }
  const pool = [];
  for (let i = 0; i < Math.min(CONCURRENCY, queue.length); i++) pool.push(worker());
  await Promise.all(pool);

  // ── 校验打包产物 ──
  // 预发布版本的产物里带 GIT_COMMIT 文件（打包时写进去的短 sha），它必须与本次传入的
  // commit 对得上，否则搬过去的就不是这次提交构建的产物。
  // 正式版不带这个附件（GIT_COMMIT 仅预发布附带），跳过校验。
  let commitMatched = null; // null=没附件可校验，true/false=校验结果
  const gcPath = OUT_DIR + '/GIT_COMMIT';
  if (fs.existsSync(gcPath)) {
    const short = fs.readFileSync(gcPath, 'utf8').replace(/\s+/g, '');
    if (!RELEASE_COMMIT) {
      console.log('⚠ 产物带 GIT_COMMIT（' + short + '），但本次未传入 commit，无法比对');
      commitMatched = false;
    } else if (RELEASE_COMMIT.indexOf(short) === 0) {
      console.log('✓ 产物 GIT_COMMIT（' + short + '）与本次 commit 一致');
      commitMatched = true;
    } else {
      console.log('⚠ 产物 GIT_COMMIT（' + short + '）与本次 commit（' + RELEASE_COMMIT +
        '）不一致：搬过去的产物不是这次提交构建的');
      commitMatched = false;
    }
  } else {
    console.log('产物不带 GIT_COMMIT 附件（仅预发布附带），跳过 commit 校验');
  }

  // ── 写清单，交给发布脚本 ──
  // target_commitish 优先用上游传来的 commit（顺手做的一次绑定，语义与 GitHub 一致）：
  // 绑不上也无妨，publish.js 会在创建失败时回退到默认分支重试。
  const manifest = {
    tag,
    ver,
    prerelease: pre,
    downloadSource: DOWNLOAD_SOURCE,
    releaseName: 'Kai ' + tag + (pre ? ' (预发布)' : ''),
    targetCommitish,
    commit: RELEASE_COMMIT,
    commitMatched,
    bodyHtml,
    bodyHtmlPlugin: pluginSafeBody(bodyHtml),
    outDir: OUT_DIR,
    totalBytes,
    files: assets.map((a) => ({ name: a.name, size: a.size || 0, path: OUT_DIR + '/' + a.name })),
    skipped
  };
  fs.writeFileSync(MANIFEST, JSON.stringify(manifest, null, 2) + '\n', 'utf8');

  console.log('---- 下载完成 ----');
  console.log('清单：' + MANIFEST + '（' + manifest.files.length + ' 个文件，' + mb(totalBytes) +
    '，来源 ' + DOWNLOAD_SOURCE + '）');
  manifest.files.forEach((f) => console.log('  - ' + f.name + ' ' + mb(f.size)));
}

function selfTest() {
  const checks = [];
  const check = (name, ok) => {
    checks.push(!!ok);
    console.log('  ' + (ok ? '✓' : '✗') + ' ' + name);
  };
  check('rc 预发布', preByVersion('v1.2.3-rc.1'));
  check('nightly 预发布', preByVersion('v1.2.3-nightly.20261116'));
  check('正式版', !preByVersion('v1.2.3'));
  check('hotfix 不是预发布', !preByVersion('v1.2.3-hotfix'));
  const names = cnbAssetNames('1.2.3', true);
  check('cnb 附件数 21', names.length === 21);
  check('cnb 含 deb', names.indexOf('Kai-1.2.3-linux-amd64.deb') !== -1);
  check('cnb 含 GIT_COMMIT', names.indexOf('GIT_COMMIT') !== -1);
  check('cnb 正式版不含 GIT_COMMIT', cnbAssetNames('1.2.3', false).indexOf('GIT_COMMIT') === -1);
  const flat = pluginSafeBody('a=b\nc=d');
  check('插件正文单行', flat.indexOf('\n') === -1);
  check('插件正文裸 = 转义', flat.indexOf('&#61;') !== -1 && flat.indexOf('=') === -1);
  const long = pluginSafeBody('x'.repeat(500));
  check('插件正文每 200 字节断行', long.split('<br/>').length === 3);
  const tmp = require('os').tmpdir() + '/kai-hdr-' + process.pid;
  fs.writeFileSync(tmp, Buffer.from('!<arch>\n'));
  let hdrOk = true;
  try { verifyHeader(tmp, 'x.deb'); } catch (e) { hdrOk = false; }
  check('deb 文件头', hdrOk);
  fs.writeFileSync(tmp, '<html>oops');
  let hdrBad = false;
  try { verifyHeader(tmp, 'x.zip'); } catch (e) { hdrBad = true; }
  check('HTML 错误页被拒', hdrBad);
  fs.unlinkSync(tmp);
  if (checks.indexOf(false) !== -1) fail('自检失败');
  console.log('自检通过');
}

if (require.main === module) {
  main().catch((err) => fail(err && err.message ? err.message : String(err)));
}

module.exports = { preByVersion, cnbAssetNames, pluginSafeBody, verifyHeader, probeSize, downloadFile };
