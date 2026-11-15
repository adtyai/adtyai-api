// adtyai-api 的文档漂移检查：文档必须跟着代码走。
//
//   node scripts/adty/doc-refs.mjs            和 makefile 的 check-docs、CI 用的一样
//   node scripts/adty/doc-refs.mjs --verbose  额外打印通过项统计
//   node scripts/adty/doc-refs.mjs --strict   删除清单漏项也算失败（默认只 WARN）
//   node scripts/adty/doc-refs.mjs --since=<rev>  只检查 <rev>..HEAD 之间删掉的文件
//
// 三类检查：
//   一、文档里引用的仓库文件必须存在。文档里的路径常常只写文件名后面几段（表格里的
//       `lib/address.ts`）或者写成 `{a.ts,b.tsx}`，所以除了仓库根目录和 web/ 下的精确路径，
//       还按路径结尾在仓库文件里找；`/opt/...` 这种服务器上的绝对路径不查。
//       已删文件在文档里出现是正常的（upstream-touchpoints.md 记的就是删了什么），
//       路径在 docs/adty/deleted-paths.txt 里的不再报。
//   二、docs/openapi/api.json 的 paths 必须都能在 router/*.go 里找到注册。
//      只读静态分析：先按函数调用关系求出每个 Group 的前缀，再拼出路由；:id 和 {id}
//       统一成 {}，结尾斜杠不区分。上游保留但本 fork 关闭的路由写在豁免清单里。
//   三、docs/adty/deleted-paths.txt 必须覆盖最近删掉的文件。历史遗留漏项很多，
//      所以默认只警告，加 --strict 才失败（CI 用默认模式）。
//
// 有错时退出码 1。豁免清单是 scripts/adty/doc-refs-allow.txt，修完以后要清空。
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import path from 'node:path'

const scriptDir = import.meta.dirname
const repoRoot = path.resolve(scriptDir, '..', '..')
const allowListPath = path.join(scriptDir, 'doc-refs-allow.txt')
const deletedListPath = path.join(repoRoot, 'docs/adty/deleted-paths.txt')
const openapiPath = path.join(repoRoot, 'docs/openapi/api.json')
const routerDir = path.join(repoRoot, 'router')
const DEFAULT_DEPTH = 200
const USAGE =
  '用法：node scripts/adty/doc-refs.mjs [--verbose] [--strict] [--since=<rev>]（也可以用环境变量 DOC_REFS_SINCE）'

// 这些文档要跟着代码走。别的 README（en/fr/ja/...）暂时不查，先只看主文档。
const DOC_FILES = [
  'docs/adty/features.md',
  'docs/adty/deploy.md',
  'docs/adty/conventions.md',
  'docs/adty/upstream-touchpoints.md',
  'docs/authentication.md',
  'AGENTS.md',
  'web/AGENTS.md',
  'README.md',
]

const REF_EXTENSIONS = new Set(['go', 'ts', 'tsx', 'sh', 'mjs', 'json', 'yml', 'yaml', 'css'])
// 至少带一个 /，避免把 go.mod、package.json 这种当路径；扩展名在过滤时再判断。
const REF_PATTERN = /(?:\.{0,2}\/)?[A-Za-z0-9_@.-]+(?:\/[A-Za-z0-9_@.-]+)+\.[A-Za-z0-9]+/g

const problems = []
const warnings = []

function problem(file, line, message, details = []) {
  problems.push({ file, line, message, details })
}

function warning(file, line, message, details = []) {
  warnings.push({ file, line, message, details })
}

function readText(file) {
  return readFileSync(file, 'utf8')
}

function normalizeSlashes(value) {
  return value.replace(/\/{2,}/g, '/')
}

// 拼接 Group 前缀。gin 的 joinPaths 会保留结尾斜杠，这里统一去掉，比较时两边都一样。
function joinPrefix(base, relative) {
  const left = normalizeSlashes(base || '')
  const right = relative || ''
  let joined = left === '' ? right : right === '' ? left : path.posix.join(left, right)
  joined = normalizeSlashes(joined)
  if (joined !== '' && !joined.startsWith('/')) {
    joined = `/${joined}`
  }
  return joined
}

// 路由比对：:id、{id}、*path 都归一化成 {}，结尾斜杠和重复斜杠不区分。
function normalizeRoutePath(routePath) {
  let value = normalizeSlashes(routePath || '/')
  value = value.replace(/\/\*[^/]+/g, '/{}')
  value = value.replace(/\/:[^/]+/g, '/{}')
  value = value.replace(/\/\{[^/}]+\}/g, '/{}')
  value = normalizeSlashes(value)
  if (value.length > 1) {
    value = value.replace(/\/+$/, '')
  }
  return value === '' ? '/' : value
}

function readAllowList() {
  const docPaths = []
  const openapiPaths = []
  if (!existsSync(allowListPath)) {
    return { docPaths, openapiPaths }
  }
  for (const raw of readText(allowListPath).split(/\r?\n/)) {
    const line = raw.trim()
    if (line === '' || line.startsWith('#')) {
      continue
    }
    if (line.startsWith('openapi:')) {
      openapiPaths.push(line.slice('openapi:'.length).trim())
    } else {
      docPaths.push(line)
    }
  }
  return { docPaths, openapiPaths }
}

// 仓库里的文件（含还没 git add 的新文件），用来按路径结尾查找文档里缩写过的引用。
function listRepoFiles() {
  try {
    const output = execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard'], {
      cwd: repoRoot,
      encoding: 'utf8',
      maxBuffer: 64 * 1024 * 1024,
    })
    return output.split('\0').filter(Boolean)
  } catch {
    return []
  }
}

function addSuffixes(target, files) {
  for (const file of files) {
    const parts = file.split('/')
    for (let index = 0; index < parts.length; index += 1) {
      target.add(parts.slice(index).join('/'))
    }
  }
}

function readDeletedPathList() {
  const files = new Set()
  const dirs = []
  const suffixes = new Set()
  if (!existsSync(deletedListPath)) {
    return { files, dirs, suffixes }
  }
  for (const raw of readText(deletedListPath).split(/\r?\n/)) {
    const line = raw.trim()
    if (line === '' || line.startsWith('#')) {
      continue
    }
    if (line.endsWith('/')) {
      dirs.push(line)
    } else {
      files.add(line)
    }
  }
  addSuffixes(suffixes, files)
  return { files, dirs, suffixes }
}

// 文档里的路径是不是回指一个已删文件（这些文档就是在记删除）。
function isDeletedReference(reference, deleted) {
  return (
    deleted.files.has(reference) ||
    deleted.suffixes.has(reference) ||
    deleted.dirs.some((dir) => reference.startsWith(dir) || dir.startsWith(`${reference}/`))
  )
}

function isAllowedDocReference(reference, allow) {
  return allow.docPaths.some((entry) => {
    if (entry === reference || reference.endsWith(`/${entry}`) || entry.endsWith(`/${reference}`)) {
      return true
    }
    // 豁免清单只写文件名时，按文件名匹配（三个已知失效项就是这样写的）
    return !entry.includes('/') && path.posix.basename(reference) === entry
  })
}

// 文档引用是否指向一个真实文件：仓库根目录、web/ 下、按路径结尾都算。
function referenceExists(reference, docFile, fileSuffixes) {
  const candidates = [path.join(repoRoot, reference), path.join(repoRoot, 'web', reference)]
  if (reference.startsWith('./') || reference.startsWith('../')) {
    candidates.push(path.resolve(repoRoot, path.dirname(docFile), reference))
  }
  for (const candidate of candidates) {
    if (existsSync(candidate)) {
      return true
    }
  }
  return fileSuffixes.has(reference)
}

function checkDocReferences(deleted, allow) {
  const fileSuffixes = new Set()
  addSuffixes(fileSuffixes, listRepoFiles())
  let referenceCount = 0
  let absoluteCount = 0
  let urlCount = 0
  let deletedCount = 0
  let allowedCount = 0

  for (const docFile of DOC_FILES) {
    const absolute = path.join(repoRoot, docFile)
    if (!existsSync(absolute)) {
      problem(docFile, 1, '文档不存在')
      continue
    }
    const lines = readText(absolute).split(/\r?\n/)
    for (const [index, line] of lines.entries()) {
      for (const match of line.matchAll(REF_PATTERN)) {
        const raw = match[0]
        const extension = raw.slice(raw.lastIndexOf('.') + 1)
        if (!REF_EXTENSIONS.has(extension)) {
          continue
        }
        // 服务器上的绝对路径（/opt/adtyai/...）不是仓库文件
        if (raw.startsWith('/')) {
          absoluteCount += 1
          continue
        }
        // 链接里的 URL 不是仓库文件
        const before = line.slice(0, match.index)
        if (/(?:[a-z][a-z0-9+.-]*:\/\/|www\.)\S*$/i.test(before)) {
          urlCount += 1
          continue
        }
        referenceCount += 1
        const reference = raw.replace(/^\.\//, '')
        if (referenceExists(reference, docFile, fileSuffixes)) {
          continue
        }
        if (isDeletedReference(reference, deleted)) {
          deletedCount += 1
          continue
        }
        if (isAllowedDocReference(reference, allow)) {
          allowedCount += 1
          continue
        }
        problem(docFile, index + 1, `引用的文件不存在：${raw}`)
      }
    }
  }

  if (verbose) {
    console.log(
      `文档路径：${DOC_FILES.length} 个文档、${referenceCount} 处引用；` +
        `跳过服务器绝对路径 ${absoluteCount} 处、URL ${urlCount} 处、已删文件 ${deletedCount} 处；` +
        `豁免清单命中 ${allowedCount} 处`
    )
  }
}

// ---------- 检查二：openapi 路由必须真的注册 ----------

function readRouterFunctions() {
  const functions = new Map()
  if (!existsSync(routerDir)) {
    return functions
  }
  for (const fileName of readdirSync(routerDir)) {
    if (!fileName.endsWith('.go') || fileName.endsWith('_test.go')) {
      continue
    }
    const absolute = path.join(routerDir, fileName)
    const lines = readText(absolute).split(/\r?\n/)
    for (let index = 0; index < lines.length; index += 1) {
      const signature = /^func\s+(\w+)\s*\(/.exec(lines[index])
      if (!signature) {
        continue
      }
      let bodyStart = index
      while (bodyStart < lines.length && !/\{\s*$/.test(lines[bodyStart])) {
        bodyStart += 1
      }
      if (bodyStart >= lines.length) {
        continue
      }
      let bodyEnd = bodyStart + 1
      while (bodyEnd < lines.length && lines[bodyEnd] !== '}') {
        bodyEnd += 1
      }
      if (bodyEnd >= lines.length) {
        continue
      }
      const parameter = /^func\s+\w+\s*\(\s*(\w+)\s+\*gin\.(Engine|RouterGroup)/.exec(lines[index])
      functions.set(signature[1], {
        name: signature[1],
        fileName,
        fileText: lines.join('\n'),
        param: parameter ? parameter[1] : null,
        paramType: parameter ? parameter[2] : null,
        lines: lines.slice(bodyStart, bodyEnd + 1),
      })
      index = bodyEnd
    }
  }
  return functions
}

// 一个函数里的 Group 变量 -> 前缀。表达式都是 `x := y.Group("/path")` 的形式。
function resolveGroupVars(fn, entryPrefix) {
  const vars = new Map()
  if (fn.param !== null) {
    vars.set(fn.param, entryPrefix)
  }
  let changed = true
  while (changed) {
    changed = false
    for (const line of fn.lines) {
      if (/^\s*\/\//.test(line)) {
        continue
      }
      const match = /^\s*(\w+)\s*:=\s*(\w+)\.Group\(\s*"([^"]*)"/.exec(line)
      if (!match || !vars.has(match[2])) {
        continue
      }
      const value = joinPrefix(vars.get(match[2]), match[3])
      if (vars.get(match[1]) !== value) {
        vars.set(match[1], value)
        changed = true
      }
    }
  }
  return vars
}

// SetRouter 调用各 SetXxxRouter(router)，这些函数的前缀从调用点传进去。
function collectRoutePrefixes(functions) {
  const prefixes = new Map()
  const add = (name, prefix) => {
    if (!prefixes.has(name)) {
      prefixes.set(name, new Set())
    }
    const set = prefixes.get(name)
    if (set.has(prefix)) {
      return false
    }
    set.add(prefix)
    return true
  }
  for (const fn of functions.values()) {
    if (fn.paramType === 'Engine' && fn.param !== null) {
      add(fn.name, '')
    }
  }
  let changed = true
  let rounds = 0
  while (changed && rounds < 10) {
    changed = false
    rounds += 1
    for (const fn of functions.values()) {
      const entries = prefixes.get(fn.name)
      if (!entries || fn.param === null) {
        continue
      }
      for (const entry of [...entries]) {
        const vars = resolveGroupVars(fn, entry)
        for (const line of fn.lines) {
          if (/^\s*\/\//.test(line)) {
            continue
          }
          for (const call of line.matchAll(/(\w+)\s*\(\s*(\w+)/g)) {
            const callee = call[1]
            const argument = call[2]
            if (!functions.has(callee) || !vars.has(argument)) {
              continue
            }
            if (add(callee, vars.get(argument))) {
              changed = true
            }
          }
        }
      }
    }
  }
  return prefixes
}

// channel-router.go 的权限路由表在包级变量里，按 var 名字把表里的 path 找出来。
function extractPackageSlice(fileText, name) {
  const lines = fileText.split(/\r?\n/)
  const head = new RegExp(`^var\\s+${name}\\s*=`)
  for (let index = 0; index < lines.length; index += 1) {
    if (!head.test(lines[index])) {
      continue
    }
    let end = index + 1
    while (end < lines.length && lines[end] !== '}') {
      end += 1
    }
    return lines.slice(index, end + 1).join('\n')
  }
  return ''
}

function addTablePaths(addRoute, text, groupPrefix) {
  for (const match of text.matchAll(/\{\s*http\.Method\w+\s*,\s*"([^"]*)"/g)) {
    addRoute(joinPrefix(groupPrefix, match[1]))
  }
  for (const match of text.matchAll(/\bpath:\s*"([^"]*)"/g)) {
    addRoute(joinPrefix(groupPrefix, match[1]))
  }
}

function collectRegisteredRoutes() {
  const functions = readRouterFunctions()
  const prefixes = collectRoutePrefixes(functions)
  const routes = new Set()

  for (const fn of functions.values()) {
    const entries = prefixes.get(fn.name)
    if (!entries) {
      continue
    }
    for (const entry of entries) {
      const vars = resolveGroupVars(fn, entry)
      const body = fn.lines.join('\n')
      const addRoute = (routePath) => {
        routes.add(normalizeRoutePath(routePath))
      }

      for (const line of fn.lines) {
        if (/^\s*\/\//.test(line)) {
          continue
        }
        // apiRouter.GET("/xxx")、selfRoute.POST("/yyy") 这类
        for (const match of line.matchAll(/(\w+)\.(GET|POST|PUT|DELETE|PATCH|Any)\(\s*"([^"]*)"/g)) {
          if (vars.has(match[1])) {
            addRoute(joinPrefix(vars.get(match[1]), match[3]))
          }
        }
        // handlePermissionRoute(group, method, "/xxx", ...)
        const direct = /handlePermissionRoute\(\s*(\w+)(?:\.Group\(\s*"([^"]*)"[^)]*\))?\s*,\s*(?:http\.Method\w+|[\w.]+)\s*,\s*"([^"]*)"/g
        for (const match of line.matchAll(direct)) {
          if (!vars.has(match[1])) {
            continue
          }
          const groupPrefix = match[2] === undefined ? vars.get(match[1]) : joinPrefix(vars.get(match[1]), match[2])
          addRoute(joinPrefix(groupPrefix, match[3]))
        }
      }

      // for _, route := range xxx { handlePermissionRoute(group, route.method, route.path, ...) }
      const loop = /handlePermissionRoute\(\s*(\w+)\s*,\s*route\.method\s*,\s*route\.path/.exec(body)
      if (loop && vars.has(loop[1])) {
        const groupPrefix = vars.get(loop[1])
        addTablePaths(addRoute, body, groupPrefix)
        for (const match of body.matchAll(/range\s+(\w+)/g)) {
          const table = extractPackageSlice(fn.fileText, match[1])
          if (table !== '') {
            addTablePaths(addRoute, table, groupPrefix)
          }
        }
      }
    }
  }
  return routes
}

function findJsonKeyLine(text, key) {
  const needle = `${JSON.stringify(key)}:`
  const lines = text.split(/\r?\n/)
  for (let index = 0; index < lines.length; index += 1) {
    if (lines[index].includes(needle)) {
      return index + 1
    }
  }
  return 1
}

function checkOpenapiRoutes(routes, allow) {
  if (!existsSync(openapiPath)) {
    problem('docs/openapi/api.json', 1, '文件不存在')
    return
  }
  const raw = readText(openapiPath)
  let spec
  try {
    spec = JSON.parse(raw)
  } catch (error) {
    problem('docs/openapi/api.json', 1, `不是合法 JSON：${error.message}`)
    return
  }
  const paths = spec?.paths ?? {}
  const allowed = new Set(allow.openapiPaths)
  const allowedNormalized = new Set(allow.openapiPaths.map((entry) => normalizeRoutePath(entry)))
  const names = Object.keys(paths)
  let registered = 0
  for (const name of names) {
    const normalized = normalizeRoutePath(name)
    if (routes.has(normalized)) {
      registered += 1
      continue
    }
    if (allowed.has(name) || allowedNormalized.has(normalized)) {
      registered += 1
      continue
    }
    problem(
      'docs/openapi/api.json',
      findJsonKeyLine(raw, name),
      `openapi 路径没有注册：${name}（router/*.go 里找不到）`
    )
  }
  if (verbose) {
    console.log(`openapi：${names.length} 条路径，${registered} 条能在 router/*.go 里找到注册，路由集合 ${routes.size} 条`)
  }
}

// ---------- 检查三：删除清单必须覆盖已删文件 ----------

function checkDeletedPaths() {
  if (!existsSync(deletedListPath)) {
    problem('docs/adty/deleted-paths.txt', 1, '文件不存在')
    return
  }
  const args = ['log', '--diff-filter=D', '--name-only', '--pretty=format:']
  if (since) {
    args.push(`${since}..HEAD`)
  } else {
    args.push(`--max-count=${DEFAULT_DEPTH}`, 'HEAD')
  }
  let output
  try {
    output = execFileSync('git', args, { cwd: repoRoot, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 })
  } catch (error) {
    problem('docs/adty/deleted-paths.txt', 1, `git log 跑不起来：${error.message}`)
    return
  }
  const deletedFiles = [...new Set(output.split(/\r?\n/).map((line) => line.trim()).filter(Boolean))]
  const listed = new Set()
  const listedDirs = []
  for (const line of readText(deletedListPath).split(/\r?\n/)) {
    const entry = line.trim()
    if (entry === '' || entry.startsWith('#')) {
      continue
    }
    if (entry.endsWith('/')) {
      listedDirs.push(entry)
    } else {
      listed.add(entry)
    }
  }
  const missing = deletedFiles.filter(
    (file) => !listed.has(file) && !listedDirs.some((dir) => file.startsWith(dir))
  )
  const range = since ? `${since}..HEAD` : `最近 ${DEFAULT_DEPTH} 个提交`
  if (verbose) {
    console.log(`删除清单：${range} 删了 ${deletedFiles.length} 个文件，清单覆盖 ${deletedFiles.length - missing.length} 个`)
  }
  if (missing.length === 0) {
    return
  }
  const message = `删除清单漏了 ${missing.length} 个已删文件（${range}）`
  const details = missing.slice(0, 50).map((file) => `  ${file}`)
  if (missing.length > 50) {
    details.push(`  ……还有 ${missing.length - 50} 个`)
  }
  if (strict) {
    problem('docs/adty/deleted-paths.txt', 1, message, details)
  } else {
    warning('docs/adty/deleted-paths.txt', 1, message, details)
  }
}

// ---------- 入口 ----------

let verbose = false
let strict = false
let since = process.env.DOC_REFS_SINCE ?? ''
const args = process.argv.slice(2)
for (let index = 0; index < args.length; index += 1) {
  const arg = args[index]
  if (arg === '--verbose') {
    verbose = true
  } else if (arg === '--strict') {
    strict = true
  } else if (arg.startsWith('--since=')) {
    since = arg.slice('--since='.length)
  } else if (arg === '--since') {
    since = args[index + 1] ?? ''
    index += 1
  } else {
    console.error(USAGE)
    process.exit(2)
  }
}
if (since === '' && process.env.DOC_REFS_SINCE) {
  since = process.env.DOC_REFS_SINCE
}

const allow = readAllowList()
const deleted = readDeletedPathList()
checkDocReferences(deleted, allow)
const routes = collectRegisteredRoutes()
checkOpenapiRoutes(routes, allow)
checkDeletedPaths()

for (const item of warnings) {
  console.error(`WARN ${item.file}:${item.line} - ${item.message}`)
  for (const detail of item.details) {
    console.error(detail)
  }
}
for (const item of problems) {
  console.error(`${item.file}:${item.line} - ${item.message}`)
  for (const detail of item.details) {
    console.error(detail)
  }
}
if (problems.length > 0) {
  console.error(`doc-refs: ${problems.length} problem(s)`)
  process.exit(1)
}
console.log(`doc-refs: 0 problem(s)`)
