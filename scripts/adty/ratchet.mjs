// adtyai-api 的 lint、copyright:check 棘轮（实施方案任务 0.3），check.sh 的这两步调用它。
// 上游原样就有 lint error 和不合要求的文件头（docs/adty/baseline.md），上游文件不改，
// 所以只和本目录下的两份清单比较，拦清单以外的新问题：
//   node scripts/adty/ratchet.mjs lint        web 的 lint error 按文件计数，和 baseline-lint.txt 比较
//   node scripts/adty/ratchet.mjs copyright   web 的 copyright:check 列出的文件，和 baseline-copyright.txt 比较
//   node scripts/adty/ratchet.mjs --update    按现在的代码重写两份清单（合并上游以后用）
// 清单里的路径相对 web/，按路径排序。两个工具都只读：lint 不带 --fix，copyright:check 带 --check。
import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const scriptDir = import.meta.dirname
const repoRoot = path.resolve(scriptDir, '..', '..')
const webDir = path.join(repoRoot, 'web')
const lintListPath = path.join(scriptDir, 'baseline-lint.txt')
const copyrightListPath = path.join(scriptDir, 'baseline-copyright.txt')
const updateCommand = 'node scripts/adty/ratchet.mjs --update'
const usage = '用法：node scripts/adty/ratchet.mjs lint | copyright | --update'

function fail(message) {
  console.error(`ratchet.mjs: ${message}`)
  process.exit(1)
}

function fromRoot(file) {
  return path.relative(repoRoot, file)
}

// 工具输出太长时只打印开头
function clip(text) {
  return text.length > 4000 ? `${text.slice(0, 4000)}\n……（后面省略）` : text
}

// 工具的结果没法拿来比较（没跑起来、崩了、输出格式变了）：打印它的输出，然后失败，不能悄悄通过
function toolFailure(run, reason) {
  for (const text of [run.stdout, run.stderr]) {
    if (text.trim() !== '') {
      console.error(clip(text.trimEnd()))
    }
  }
  fail(`${run.command} 的结果不对，没法比较：${reason}`)
}

// 在 web 里跑 package.json 的脚本。lint 和 copyright:check 没问题时退出码 0，有问题时 1
function runWebScript(args) {
  const command = `bun run ${args.join(' ')}`
  const result = spawnSync('bun', ['run', ...args], {
    cwd: webDir,
    encoding: 'utf8',
    maxBuffer: 256 * 1024 * 1024,
  })
  if (result.error) {
    fail(`${command} 没有跑起来：${result.error.message}`)
  }
  const run = { command, status: result.status, stdout: result.stdout, stderr: result.stderr }
  if (run.status !== 0 && run.status !== 1) {
    toolFailure(run, result.signal ? `被信号 ${result.signal} 终止` : `退出码 ${run.status}`)
  }
  return run
}

// lint 的 error 按文件计数，warning 不算
function lintErrors() {
  const run = runWebScript(['lint', '--format=json'])
  let report
  try {
    report = JSON.parse(run.stdout)
  } catch {
    toolFailure(run, '输出不是 JSON')
  }
  if (!Array.isArray(report?.diagnostics) || !(report.number_of_files > 0)) {
    toolFailure(run, '输出里没有 diagnostics，或者一个文件也没检查')
  }
  const errors = report.diagnostics.filter((d) => d.severity === 'error')
  if (errors.some((d) => typeof d.filename !== 'string' || d.filename === '')) {
    toolFailure(run, '有的 error 没有文件名')
  }
  // oxlint 有 error 时退出码 1，只有 warning 时 0；对不上说明结果不可信
  if (run.status !== (errors.length > 0 ? 1 : 0)) {
    toolFailure(run, `退出码 ${run.status}，error 却有 ${errors.length} 个`)
  }
  const counts = new Map()
  for (const d of errors) {
    counts.set(d.filename, (counts.get(d.filename) ?? 0) + 1)
  }
  const warnings = report.diagnostics.filter((d) => d.severity === 'warning').length
  return { counts, errors, warnings, checkedFiles: report.number_of_files }
}

const COPYRIGHT_LIST_HEADER = 'copyright: headers need to be updated in:'

// copyright:check 列出的文件。输出格式见 web/scripts/add-copyright.mjs 的 main()：汇总行写 stdout；
// 有问题时 stderr 先写一行 COPYRIGHT_LIST_HEADER，再每个文件一行「- 路径」，退出码 1
function copyrightFiles() {
  const run = runWebScript(['copyright:check'])
  const summary = /^copyright: checked (\d+),.*$/m.exec(run.stdout)
  if (!summary || Number(summary[1]) === 0) {
    toolFailure(run, '没有汇总行，或者一个文件也没检查')
  }
  const lines = run.stderr.split(/\r?\n/)
  const files = []
  const start = lines.indexOf(COPYRIGHT_LIST_HEADER)
  if (start !== -1) {
    for (const line of lines.slice(start + 1)) {
      if (!line.startsWith('- ')) {
        break
      }
      files.push(line.slice(2))
    }
  }
  if (run.status !== (files.length > 0 ? 1 : 0)) {
    toolFailure(run, `退出码 ${run.status}，列出的文件却有 ${files.length} 个`)
  }
  return { files: [...new Set(files)].sort(), summary: summary[0] }
}

// 清单一行一个文件，按路径严格递增（排过序、不重复）；空行跳过
function readList(listPath, parseLine) {
  if (!existsSync(listPath)) {
    fail(`找不到 ${fromRoot(listPath)}，先运行 ${updateCommand}`)
  }
  const entries = new Map()
  let previous = ''
  const lines = readFileSync(listPath, 'utf8').split(/\r?\n/)
  for (const [index, line] of lines.entries()) {
    if (line === '') {
      continue
    }
    const entry = parseLine(line)
    if (!entry || entry.file <= previous) {
      fail(
        `${fromRoot(listPath)} 第 ${index + 1} 行格式不对、没排序或者重复：${line}（运行 ${updateCommand} 重新生成）`
      )
    }
    entries.set(entry.file, entry.value)
    previous = entry.file
  }
  return entries
}

// baseline-lint.txt 一行是「路径 error 数」
function parseLintLine(line) {
  const match = /^(\S(?:.*\S)?) ([1-9]\d*)$/.exec(line)
  return match && { file: match[1], value: Number(match[2]) }
}

// baseline-copyright.txt 一行是一个路径
function parseCopyrightLine(line) {
  return line.trim() === line ? { file: line, value: true } : null
}

function writeList(listPath, lines) {
  writeFileSync(listPath, lines.map((line) => `${line}\n`).join(''))
}

function countLines(listPath) {
  if (!existsSync(listPath)) {
    return 0
  }
  return readFileSync(listPath, 'utf8')
    .split(/\r?\n/)
    .filter((line) => line !== '').length
}

function sortedKeys(map) {
  return [...map.keys()].sort()
}

function sum(values) {
  let total = 0
  for (const value of values) {
    total += value
  }
  return total
}

// error 在文件里的位置（字符偏移），用来按出现顺序排列
function errorOffset(d) {
  return d.labels?.[0]?.span?.offset ?? 0
}

function checkLint() {
  const allowed = readList(lintListPath, parseLintLine)
  const { counts, errors, warnings, checkedFiles } = lintErrors()
  console.log(
    `lint：oxlint 检查了 ${checkedFiles} 个文件，${counts.size} 个文件有 error，共 ${errors.length} 个，` +
      `另有 ${warnings} 个 warning（不算）；清单记了 ${allowed.size} 个文件、${sum(allowed.values())} 个 error`
  )
  const over = sortedKeys(counts).filter((file) => counts.get(file) > (allowed.get(file) ?? 0))
  const fewer = sortedKeys(allowed).filter((file) => (counts.get(file) ?? 0) < allowed.get(file))
  if (over.length > 0) {
    console.error('lint：下面的文件有清单以外的 error（清单只记个数，所以列出文件里全部的 error）：')
    for (const file of over) {
      const limit = allowed.has(file) ? `清单记的是 ${allowed.get(file)} 个` : '清单里没有这个文件'
      console.error(`  ${file}：${counts.get(file)} 个 error，${limit}`)
      const fileErrors = errors.filter((d) => d.filename === file).sort((a, b) => errorOffset(a) - errorOffset(b))
      for (const d of fileErrors) {
        const span = d.labels?.[0]?.span
        const at = span ? `:${span.line}:${span.column}` : ''
        console.error(`    ${file}${at}  ${d.code}  ${d.message}`)
      }
    }
  }
  if (fewer.length > 0) {
    console.log(
      `lint：清单里有 ${fewer.length} 个文件的 error 比记的少或者文件已删除，可以运行 ${updateCommand} 更新清单`
    )
  }
  if (over.length > 0) {
    console.error('lint：失败，改掉上面新增的 error（完整输出：在 web 里跑 bun run lint）')
    process.exitCode = 1
  } else {
    console.log('lint：通过，没有清单以外的 error')
  }
}

function checkCopyright() {
  const allowed = readList(copyrightListPath, parseCopyrightLine)
  const { files, summary } = copyrightFiles()
  console.log(summary)
  console.log(`copyright：列出了 ${files.length} 个文件；清单记了 ${allowed.size} 个`)
  const flagged = new Set(files)
  const extra = files.filter((file) => !allowed.has(file))
  const fixed = sortedKeys(allowed).filter((file) => !flagged.has(file))
  if (extra.length > 0) {
    console.error('copyright：下面的文件不在清单里，文件头不合要求：')
    for (const file of extra) {
      console.error(`  ${file}`)
    }
  }
  if (fixed.length > 0) {
    console.log(
      `copyright：清单里有 ${fixed.length} 个文件已经合要求或者已删除，可以运行 ${updateCommand} 更新清单`
    )
  }
  if (extra.length > 0) {
    console.error(
      'copyright：失败。文件开头要有 QuantumNous 的 AGPL 文件头和 support@quantumnous.com；' +
        '换行和空行不影响检查。要统一成标准格式，在 web 里跑 bun run copyright' +
        '（它会把清单里的上游文件一起改掉）'
    )
    process.exitCode = 1
  } else {
    console.log('copyright：通过，没有清单以外的文件')
  }
}

function update() {
  // 两个工具都跑完、结果都可信，才写清单
  const { counts, errors } = lintErrors()
  const { files } = copyrightFiles()
  const oldLint = countLines(lintListPath)
  const oldCopyright = countLines(copyrightListPath)
  writeList(lintListPath, sortedKeys(counts).map((file) => `${file} ${counts.get(file)}`))
  writeList(copyrightListPath, files)
  console.log(
    `已重写 ${fromRoot(lintListPath)}：${counts.size} 个文件、${errors.length} 个 error（原来 ${oldLint} 行）`
  )
  console.log(`已重写 ${fromRoot(copyrightListPath)}：${files.length} 个文件（原来 ${oldCopyright} 行）`)
  console.log(
    '在测试副本里跑的，先用 adty-pull.sh 把两份清单拉回 Windows 仓库，再用 git diff scripts/adty/ 看变化：' +
      '新增的只应该是上游文件，我们自己的文件有问题要先改掉'
  )
}

const commands = { lint: checkLint, copyright: checkCopyright, '--update': update }
const args = process.argv.slice(2)
if (args.length !== 1 || !Object.hasOwn(commands, args[0])) {
  console.error(usage)
  process.exit(2)
}
commands[args[0]]()
