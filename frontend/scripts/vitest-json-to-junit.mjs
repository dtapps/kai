// 零依赖：Vitest JSON reporter 输出 → JUnit XML（供 codecov/test-results-action 上传）
// 用法：node scripts/vitest-json-to-junit.mjs [输入json] [输出xml]
// 默认读 test-results.json、写 test-report.xml（均在 frontend/ 下）。
// Vitest 3.x 无内置 junit reporter，故走 json → junit 转换（与 certflow 方案一致）。

import { readFileSync, writeFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const input = resolve(here, '..', process.argv[2] || 'test-results.json')
const output = resolve(here, '..', process.argv[3] || 'test-report.xml')

function escapeXml(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

let data
try {
  data = JSON.parse(readFileSync(input, 'utf8'))
} catch (e) {
  // 没有测试结果产物时不报错，避免拖垮 CI（coverage 上传不受影响）。
  console.warn(`[vitest-json-to-junit] 未找到 ${input}，跳过 JUnit 生成：${e.message}`)
  process.exit(0)
}

const suites = Array.isArray(data.testResults) ? data.testResults : []
const testcases = []

for (const suite of suites) {
  const file = suite.name || 'unknown'
  const assertions = Array.isArray(suite.assertionResults) ? suite.assertionResults : []
  for (const a of assertions) {
    const name = a.fullName || a.title || 'unknown'
    const time = (Number(a.duration) || 0) / 1000
    let caseXml = `  <testcase name="${escapeXml(name)}" classname="${escapeXml(file)}" time="${time.toFixed(3)}">`
    if (a.status === 'failed') {
      const msg = (Array.isArray(a.failureMessages) ? a.failureMessages.join('\n') : String(a.failureMessages || ''))
      caseXml += `\n    <failure message="${escapeXml((msg || '').split('\n')[0])}">${escapeXml(msg)}</failure>\n  `
    } else if (a.status === 'skipped' || a.status === 'pending') {
      caseXml += '\n    <skipped/>\n  '
    }
    caseXml += '</testcase>'
    testcases.push(caseXml)
  }
}

const xml = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="vitest" tests="${testcases.length}">
${suites
  .map((s) => {
    const cases = (Array.isArray(s.assertionResults) ? s.assertionResults : [])
      .map((a) => {
        const name = a.fullName || a.title || 'unknown'
        const time = (Number(a.duration) || 0) / 1000
        let c = `    <testcase name="${escapeXml(name)}" classname="${escapeXml(s.name || 'unknown')}" time="${time.toFixed(3)}">`
        if (a.status === 'failed') {
          const msg = Array.isArray(a.failureMessages) ? a.failureMessages.join('\n') : String(a.failureMessages || '')
          c += `\n      <failure message="${escapeXml((msg || '').split('\n')[0])}">${escapeXml(msg)}</failure>\n    `
        } else if (a.status === 'skipped' || a.status === 'pending') {
          c += '\n      <skipped/>\n    '
        }
        return c + '</testcase>'
      })
      .join('\n')
    return `  <testsuite name="${escapeXml(s.name || 'unknown')}" tests="${(Array.isArray(s.assertionResults) ? s.assertionResults : []).length}">\n${cases}\n  </testsuite>`
  })
  .join('\n')}
</testsuites>
`

writeFileSync(output, xml, 'utf8')
console.log(`[vitest-json-to-junit] 已生成 ${output}（${testcases.length} 个用例）`)
