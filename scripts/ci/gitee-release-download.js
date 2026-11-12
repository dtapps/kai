#!/usr/bin/env node
'use strict';

// 从 GitHub Release 下载产物到 assets/，并写出发布清单 gitee-release.json。
//
// 唯一的跳过规则：**单文件 > 100MB**（Gitee 单附件上限），这类文件在境内链路要传
// 几分钟，只是为了让服务端拒收，所以连下载都不做。
// 其它产物一律照常下载并上传 —— updater-*、BUILD_TIME、GIT_COMMIT、SHA256SUMS、
// 以及某次体积没超 100MB 的 AppImage 都不特殊处理。
//
// 环境变量：
//   RELEASE_VERSION        可选，v1.2.3 / 1.2.3 / nightly；留空取 GitHub 最新 Release
//   KAI_GITHUB_TOKEN       可选，避开 api.github.com 匿名限流（与 .cnb.yml 同一个只读令牌）
//   OUT_DIR                默认 assets
//   MANIFEST               默认 gitee-release.json
//   MAX_FILE_BYTES         默认 100MB（Gitee 单附件上限）
//   MAX_TOTAL_BYTES        默认 800MB（仓库 1GB 留余量）

const fs = require('fs');
const http = require('http');
const https = require('https');

const OUT_DIR = process.env.OUT_DIR || 'assets';
const MANIFEST = process.env.MANIFEST || 'gitee-release.json';
const GH_API = process.env.GH_API || 'https://api.github.com/repos/dtapps/kai';
const GH_TOKEN = (process.env.KAI_GITHUB_TOKEN || '').trim();
const MAX_FILE = Number(process.env.MAX_FILE_BYTES || 100 * 1024 * 1024);
const MAX_TOTAL = Number(process.env.MAX_TOTAL_BYTES || 800 * 1024 * 1024);

// 境内直连 GitHub 资产只有 10~30 KB/s 且常被掐断，所以先走代理镜像，直连只做兜底。
const MIRRORS = [
  { name: 'daocloud', wrap: (u) => 'https://files.m.daocloud.io/' + u.replace(/^https?:\/\//, '') },
  { name: 'ghfast', wrap: (u) => 'https://ghfast.top/' + u },
  { name: 'gh-proxy', wrap: (u) => 'https://gh-proxy.com/' + u },
  { name: '直连', wrap: (u) => u }
];

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

function readText(url) {
  return request(url, 5, true).then((res) => new Promise((resolve, reject) => {
    let body = '';
    res.setEncoding('utf8');
    res.on('data', (c) => { body += c; });
    res.on('end', () => resolve(body));
    res.on('error', reject);
  }));
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

// 单个文件：最多切 3 个源、累计重试 8 次，每次从已落盘的字节续传
function downloadFile(url, dest, expectedSize) {
  let mirror = 0;
  let tries = 0;
  function step() {
    const from = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
    if (expectedSize > 0 && from === expectedSize) {
      console.log('  已完整（' + mb(from) + '）');
      return Promise.resolve();
    }
    if (expectedSize > 0 && from > expectedSize) fs.unlinkSync(dest); // 上次写了脏数据
    return downloadOnce(MIRRORS[mirror].wrap(url), dest, from).then((written) => {
      if (expectedSize > 0 && written !== expectedSize) {
        throw new Error('大小不符：' + written + ' != ' + expectedSize);
      }
      console.log('  完成 ' + mb(written) + '（' + MIRRORS[mirror].name + '）');
    }).catch((err) => {
      tries++;
      const now = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
      console.log('  中断：' + (err && err.message ? err.message : err) +
        '，已下 ' + mb(now) + '，第 ' + tries + ' 次');
      if (tries >= 8) throw new Error('下载失败（重试 ' + tries + ' 次）：' + url);
      if ((err && err.slow) || tries % 2 === 0) {
        if (mirror < MIRRORS.length - 1) {
          mirror++;
          console.log('  切换下载源 → ' + MIRRORS[mirror].name);
        }
      }
      return sleep(1500).then(step);
    });
  }
  return step();
}

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

// GitHub markdown → Gitee 正文 HTML。
// 保留：预发布提示 / 发布信息 / 下载 / 校验 / 更新内容（标题）；
// 「更新内容」的明细换成一句指引 —— 完整清单在 GitHub，超限附件也只在 GitHub 提供。
// 正文走 OpenAPI 表单 body，不再有 KEY=VALUE / PATH_MAX 限制，所以不需要分段。
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

async function main() {
  // 容器启动时工作目录可能已被删除（日志里的 shell-init getcwd 报错），先兜一下
  try { process.cwd(); } catch (e) { process.chdir('/'); }
  if (!fs.existsSync(OUT_DIR)) fs.mkdirSync(OUT_DIR, { recursive: true });

  const raw = (process.env.RELEASE_VERSION || '').trim();
  const rolling = /^nightly$/i.test(raw);
  let tag = raw ? (rolling ? 'nightly' : 'v' + raw.replace(/^v/i, '')) : '';

  // 指定了版本就按 tag 精确取（补发历史版本）；没指定就取最新。
  const relUrl = tag
    ? GH_API + '/releases/tags/' + encodeURIComponent(tag)
    : GH_API + '/releases/latest';
  console.log('版本号: ' + (tag || '未指定，取 GitHub 最新 Release'));

  const rel = JSON.parse(await readText(relUrl));
  if (!tag) {
    tag = String(rel.tag_name || '').trim();
    if (!tag) fail('GitHub 最新 Release 没有 tag_name，无法判定版本');
    console.log('自动判定版本：' + tag);
  }
  const ver = tag.replace(/^v/i, '');

  // ── 附件过滤：唯一规则是单文件体积，超 100MB 只跳过它自己 ──
  const all = rel.assets || [];
  if (!all.length) fail('GitHub Release ' + tag + ' 没有附件');

  const skipped = [];
  const assets = all.filter((a) => {
    if ((a.size || 0) > MAX_FILE) {
      skipped.push(a.name + '（' + mb(a.size || 0) + ' 超过 Gitee 单附件 100MB，跳过该文件）');
      return false;
    }
    return true;
  });
  if (!assets.length) fail('过滤后没有可上传 Gitee 的附件');

  const totalBytes = assets.reduce((s, a) => s + (a.size || 0), 0);
  console.log('共 ' + all.length + ' 个附件，需要下载并上传 ' + assets.length + ' 个（' + mb(totalBytes) + '）');
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
    return downloadFile(asset.browser_download_url, OUT_DIR + '/' + asset.name, asset.size || 0).then(worker);
  }
  const pool = [];
  for (let i = 0; i < Math.min(CONCURRENCY, queue.length); i++) pool.push(worker());
  await Promise.all(pool);

  // ── 正文与预发布判定 ──
  const body = String(rel.body || '')
    .replace(/\r\n/g, '\n')
    .split('https://github.com/dtapps/kai').join('https://gitee.com/dtapps/kai')
    .trim();
  const bodyHtml = renderBody(body || ('Kai ' + tag));
  const preByTag = /[-.](rc|beta|alpha|nightly|pre|preview)\d*(\.|\b)/i.test(ver) || rolling;
  const preByBody = /(预发布|测试版|抢先版|pre-?release|release candidate)/i.test(body);
  const pre = rel.prerelease === true || preByTag || preByBody;
  console.log('预发布：' + pre + '（GitHub prerelease=' + rel.prerelease +
    '，tag 后缀=' + preByTag + '，正文关键词=' + preByBody + '）');

  // ── 写清单，交给发布脚本 ──
  const manifest = {
    tag,
    ver,
    prerelease: pre,
    releaseName: 'Kai ' + tag + (pre ? ' (预发布)' : ''),
    targetCommitish: String(rel.target_commitish || 'master'),
    bodyHtml,
    outDir: OUT_DIR,
    totalBytes,
    files: assets.map((a) => ({ name: a.name, size: a.size || 0, path: OUT_DIR + '/' + a.name })),
    skipped
  };
  fs.writeFileSync(MANIFEST, JSON.stringify(manifest, null, 2) + '\n', 'utf8');

  console.log('---- 下载完成 ----');
  console.log('清单：' + MANIFEST + '（' + manifest.files.length + ' 个文件，' + mb(totalBytes) + '）');
  manifest.files.forEach((f) => console.log('  - ' + f.name + ' ' + mb(f.size)));
}

main().catch((err) => fail(err && err.message ? err.message : String(err)));
