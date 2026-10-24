// 零依赖：Vitest JSON reporter 输出 → JUnit XML（供 codecov/test-results-action 上传）
// 用法：node scripts/vitest-json-to-junit.mjs [输入json] [输出xml]
// 默认读 test-results.json、写 test-report.xml（均在 frontend/ 下）。
// Vitest 3.x 无内置 junit reporter，故走 json → junit 转换（与 certflow 方案一致）。

import { readFileSync, statSync, writeFileSync } from 'node:fs';
import { resolve, dirname, relative, isAbsolute, join, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const input = resolve(here, '..', process.argv[2] || 'test-results.json');
const output = resolve(here, '..', process.argv[3] || 'test-report.xml');

// 本脚本固定在 <repo>/frontend/scripts/ 下，往上两级即仓库根。
// 必须用脚本自身位置推根，不能用 process.cwd()：CI 里该步骤常在 frontend/ 下执行，
// cwd 不是仓库根，会算出 ../src/xxx 这种错路径。
const repoRoot = resolve(here, '..', '..');

// Codecov Test Analytics 只用 classname 属性定位用例归属文件（文件分组、跨 run 关联、
// 失败跳转都靠它）。Vitest 的 testResults[].name 是**绝对路径**，直接写进 classname 会带上
// runner 前缀（GitHub Actions 裸 runner /home/runner/work/<repo>/<repo>、容器 /__w/<repo>/<repo>、
// GitLab /builds/<group>/<repo>、CNB /workspace/<repo>……），同名用例在不同 run 变成不同记录，
// flaky 关联与跳转全失效。
// 因此在生成端归一化成「相对仓库根 + POSIX 分隔」，且**结果与绝对前缀无关**。

// 用 statSync().isFile() 而非 existsSync()：后者会命中 frontend/src/test 这类同名目录。
function isFileInRepo(relPath) {
  try {
    return statSync(join(repoRoot, relPath)).isFile();
  } catch {
    return false;
  }
}

function toRepoRelative(p) {
  if (!p) return '';
  const abs = isAbsolute(p) ? p : resolve(process.cwd(), p);

  // 第 1 层（常规，覆盖 99% 场景）：JSON 与脚本在同一份 checkout 树内，直接相减。
  // 与绝对前缀具体是什么无关：/__w、/home/runner/work、/workspace、/builds 都能吃掉。
  const rel = relative(repoRoot, abs);
  if (rel !== '' && !rel.startsWith('..') && !isAbsolute(rel)) {
    return rel.split(sep).join('/'); // Windows 反斜杠统一成 /
  }

  // 第 2 层（兜底）：路径不在这棵树里。例如测试在容器内跑（/__w/...）、转换在宿主上跑；
  // 或本机复现别的 CI 产出的 JSON（/home/runner/work/...、/builds/...）。
  // 改为从尾部取最长的「在仓库里真实存在」的后缀，结果同样与前缀无关。
  const segs = abs.split(/[\\/]+/).filter(Boolean);
  for (let i = segs.length; i >= 2; i--) {
    // i >= 2：避免误命中仓库根的同名单文件（package.json 之类）
    const candidate = segs.slice(segs.length - i).join('/');
    if (isFileInRepo(candidate)) return candidate;
  }
  return p; // 认不出来就原样返回，绝不凭空编造路径
}

function escapeXml(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function renderCase(a, indent) {
  const name = a.fullName || a.title || 'unknown';
  const time = (Number(a.duration) || 0) / 1000;
  let c = `${indent}<testcase name="${escapeXml(name)}" classname="${escapeXml(a.__classname)}" time="${time.toFixed(3)}">`;
  if (a.status === 'failed') {
    const msg = Array.isArray(a.failureMessages)
      ? a.failureMessages.join('\n')
      : String(a.failureMessages || '');
    c += `\n${indent}  <failure message="${escapeXml((msg || '').split('\n')[0])}">${escapeXml(msg)}</failure>\n${indent}`;
  } else if (a.status === 'skipped' || a.status === 'pending') {
    c += `\n${indent}  <skipped/>\n${indent}`;
  }
  return c + '</testcase>';
}

let data;
try {
  data = JSON.parse(readFileSync(input, 'utf8'));
} catch (e) {
  // 没有测试结果产物时不报错，避免拖垮 CI（coverage 上传不受影响）。
  console.warn(`[vitest-json-to-junit] 未找到 ${input}，跳过 JUnit 生成：${e.message}`);
  process.exit(0);
}

const suites = (Array.isArray(data.testResults) ? data.testResults : []).map((s) => {
  const classname = toRepoRelative(s.name) || 'unknown';
  const assertions = (Array.isArray(s.assertionResults) ? s.assertionResults : []).map((a) => ({
    ...a,
    __classname: classname,
  }));
  return { classname, assertions };
});

const total = suites.reduce((n, s) => n + s.assertions.length, 0);

const xml = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="vitest" tests="${total}">
${suites
  .map(
    (s) =>
      `  <testsuite name="${escapeXml(s.classname)}" tests="${s.assertions.length}">\n${s.assertions
        .map((a) => renderCase(a, '    '))
        .join('\n')}\n  </testsuite>`,
  )
  .join('\n')}
</testsuites>
`;

writeFileSync(output, xml, 'utf8');
console.log(`[vitest-json-to-junit] 已生成 ${output}（${total} 个用例）`);
